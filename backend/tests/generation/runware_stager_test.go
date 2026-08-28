package generation_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	runwareadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/runware"
)

type stagerRoundTripFunc func(*http.Request) (*http.Response, error)

func (function stagerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type stagedObjectCall struct {
	objectKey, mediaType, sha256 string
	contents                     []byte
}

type recordingStagingStore struct {
	mu    sync.Mutex
	err   error
	calls []stagedObjectCall
}

func (store *recordingStagingStore) EnsurePrivateObject(
	_ context.Context,
	objectKey string,
	contents []byte,
	mediaType, sha256 string,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls = append(store.calls, stagedObjectCall{
		objectKey: objectKey, contents: append([]byte(nil), contents...), mediaType: mediaType, sha256: sha256,
	})
	return store.err
}

func (store *recordingStagingStore) snapshot() []stagedObjectCall {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]stagedObjectCall(nil), store.calls...)
}

func TestRunwareImageStagerDownloadsValidatesAndStoresOnePrivatePNG(t *testing.T) {
	contents := testPNG(t, 4, 3)
	store := &recordingStagingStore{}
	var requested []string
	transport := stagerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.String())
		if request.URL.Path == "/source/image.png" {
			return stagerResponse(http.StatusTemporaryRedirect, "", "", map[string]string{
				"Location": "https://im.runware.ai/final/image.png",
			}), nil
		}
		return stagerResponse(http.StatusOK, "image/png", string(contents), nil), nil
	})
	stager, err := runwareadapter.NewImageStager(runwareadapter.ImageStagerConfig{
		ObjectStore: store, Transport: transport,
		ResolveIP: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("203.0.113.10")}, nil
		},
		DownloadTimeout: time.Second, MaxImageBytes: 1 << 20, MaxPixels: 100,
	})
	if err != nil {
		t.Fatalf("construct Runware image stager: %v", err)
	}
	workspaceID, projectID, providerJobID, imageID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	request := runwareadapter.StageImageRequest{
		WorkspaceID: workspaceID, ProjectID: projectID, ProviderJobID: providerJobID,
		ImageUUID: imageID, ImageURL: "https://im.runware.ai/source/image.png",
		Width: 4, Height: 3, OutputFormat: "PNG",
	}

	first, err := stager.StageImage(context.Background(), request)
	if err != nil {
		t.Fatalf("stage valid Runware image: %v", err)
	}
	second, err := stager.StageImage(context.Background(), request)
	if err != nil || second != first {
		t.Fatalf("replay valid Runware image staging: first=%#v second=%#v err=%v", first, second, err)
	}
	expectedKey := "staging/" + workspaceID + "/" + providerJobID + "/" + imageID + ".png"
	if first.OutputKey != imageID || first.StagingObjectKey != expectedKey ||
		first.SHA256 != sha256Hex(contents) || first.Bytes != int64(len(contents)) ||
		first.MediaType != "image/png" || first.Width != 4 || first.Height != 3 {
		t.Fatalf("staged Runware output drifted: %#v", first)
	}
	calls := store.snapshot()
	if len(calls) != 2 {
		t.Fatalf("private staging calls = %d, want 2 idempotent ensures", len(calls))
	}
	for _, call := range calls {
		if call.objectKey != expectedKey || call.mediaType != "image/png" ||
			call.sha256 != sha256Hex(contents) || !bytes.Equal(call.contents, contents) {
			t.Fatalf("private staging object drifted: %#v", call)
		}
	}
	if len(requested) != 4 || requested[0] != request.ImageURL ||
		requested[1] != "https://im.runware.ai/final/image.png" ||
		requested[2] != request.ImageURL || requested[3] != "https://im.runware.ai/final/image.png" {
		t.Fatalf("Runware redirect requests drifted: %#v", requested)
	}
}

func TestRunwareImageStagerRejectsPrivateResolutionAndRedirectBeforeDownload(t *testing.T) {
	store := &recordingStagingStore{}
	downloads := 0
	transport := stagerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		downloads++
		return stagerResponse(http.StatusTemporaryRedirect, "", "", map[string]string{
			"Location": "https://127.0.0.1/private.png",
		}), nil
	})
	privateResolver := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.7")}, nil
	}
	stager, err := runwareadapter.NewImageStager(runwareadapter.ImageStagerConfig{
		ObjectStore: store, Transport: transport, ResolveIP: privateResolver,
		DownloadTimeout: time.Second, MaxImageBytes: 1 << 20, MaxPixels: 100,
	})
	if err != nil {
		t.Fatalf("construct private-resolution stager: %v", err)
	}
	request := validStageImageRequest()
	if _, err = stager.StageImage(context.Background(), request); err == nil || !strings.Contains(err.Error(), "output host") {
		t.Fatalf("private DNS result was accepted: %v", err)
	}
	if downloads != 0 || len(store.snapshot()) != 0 {
		t.Fatalf("private DNS result reached download/store: downloads=%d stores=%d", downloads, len(store.snapshot()))
	}

	publicResolver := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	stager, err = runwareadapter.NewImageStager(runwareadapter.ImageStagerConfig{
		ObjectStore: store, Transport: transport, ResolveIP: publicResolver,
		DownloadTimeout: time.Second, MaxImageBytes: 1 << 20, MaxPixels: 100,
	})
	if err != nil {
		t.Fatalf("construct redirect stager: %v", err)
	}
	if _, err = stager.StageImage(context.Background(), request); err == nil || !strings.Contains(err.Error(), "output URL") {
		t.Fatalf("private redirect was accepted: %v", err)
	}
	if downloads != 1 || len(store.snapshot()) != 0 {
		t.Fatalf("private redirect reached another download/store: downloads=%d stores=%d", downloads, len(store.snapshot()))
	}
}

func TestRunwareImageStagerRejectsInvalidMediaBoundsAndStorageFailure(t *testing.T) {
	validPNG := testPNG(t, 4, 3)
	cases := []struct {
		name, mediaType string
		contents        []byte
		width, height   int
		maxBytes        int64
		storeError      error
	}{
		{name: "media type", mediaType: "image/jpeg", contents: validPNG, width: 4, height: 3, maxBytes: 1 << 20},
		{name: "response bytes", mediaType: "image/png", contents: validPNG, width: 4, height: 3, maxBytes: 8},
		{name: "dimensions", mediaType: "image/png", contents: validPNG, width: 5, height: 3, maxBytes: 1 << 20},
		{name: "storage", mediaType: "image/png", contents: validPNG, width: 4, height: 3, maxBytes: 1 << 20, storeError: errors.New("injected object store outage")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := &recordingStagingStore{err: testCase.storeError}
			transport := stagerRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return stagerResponse(http.StatusOK, testCase.mediaType, string(testCase.contents), nil), nil
			})
			stager, err := runwareadapter.NewImageStager(runwareadapter.ImageStagerConfig{
				ObjectStore: store, Transport: transport,
				ResolveIP: func(context.Context, string) ([]net.IP, error) {
					return []net.IP{net.ParseIP("203.0.113.10")}, nil
				},
				DownloadTimeout: time.Second, MaxImageBytes: testCase.maxBytes, MaxPixels: 100,
			})
			if err != nil {
				t.Fatalf("construct invalid-output stager: %v", err)
			}
			request := validStageImageRequest()
			request.Width, request.Height = testCase.width, testCase.height
			if _, err = stager.StageImage(context.Background(), request); err == nil {
				t.Fatal("invalid Runware output was accepted")
			}
			if testCase.storeError == nil && len(store.snapshot()) != 0 {
				t.Fatalf("invalid Runware output reached private storage: %#v", store.snapshot())
			}
		})
	}
}

func validStageImageRequest() runwareadapter.StageImageRequest {
	return runwareadapter.StageImageRequest{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), ProviderJobID: uuid.NewString(),
		ImageUUID: uuid.NewString(), ImageURL: "https://im.runware.ai/source/image.png",
		Width: 4, Height: 3, OutputFormat: "PNG",
	}
}

func stagerResponse(status int, mediaType, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	if mediaType != "" {
		header.Set("Content-Type", mediaType)
	}
	for name, value := range headers {
		header.Set(name, value)
	}
	return &http.Response{
		StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)),
	}
}
