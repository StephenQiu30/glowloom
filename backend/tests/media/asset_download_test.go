package media_test

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type originalReadStore struct {
	asset  domain.MediaAsset
	reads  int
	change func(*domain.MediaAsset)
}

func (r *originalReadStore) FindAsset(_ context.Context, _ identityapp.Principal, _, _ uuid.UUID) (domain.MediaAsset, error) {
	r.reads++
	if r.reads == 2 && r.change != nil {
		r.change(&r.asset)
	}
	return r.asset, nil
}
func (r *originalReadStore) ListReadyAssets(context.Context, identityapp.Principal, uuid.UUID, string, uuid.UUID, int) ([]domain.MediaAsset, error) {
	return nil, nil
}
func TestAssetOriginalRequiresCurrentEligibleIdentityAndSavedLocation(t *testing.T) {
	data := []byte("actual original")
	a := routedReadImage()
	a.ByteSize = int64(len(data))
	a.ObjectLocation = readLocation("tos-main", a.ObjectKey)
	objects := &routedReadObjects{data: map[domain.ObjectRef][]byte{a.ObjectLocation.Ref: data}}
	repo := &originalReadStore{asset: a}
	file, err := mediaapp.NewAssetDownloads(repo, objects).Open(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if err != nil || string(got) != string(data) || repo.reads != 2 {
		t.Fatal("original must be reauthorized after open", err)
	}
	for _, change := range []func(*domain.MediaAsset){func(a *domain.MediaAsset) { a.Revision++ }, func(a *domain.MediaAsset) { a.IsDelete = true }, func(a *domain.MediaAsset) { a.ProjectID = uuid.New() }, func(a *domain.MediaAsset) { a.ObjectLocation = readLocation("cos-main", a.ObjectKey) }} {
		repo = &originalReadStore{asset: a, change: change}
		if file, err := mediaapp.NewAssetDownloads(repo, objects).Open(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); file != nil || !errors.Is(err, mediaapp.ErrOriginalChanged) {
			t.Fatal("changed fact streamed", err)
		}
	}
	for _, change := range []func(*domain.MediaAsset){func(a *domain.MediaAsset) { a.IsDelete = true }, func(a *domain.MediaAsset) { a.Status = domain.StatusProcessing }, func(a *domain.MediaAsset) { a.ModerationStatus = domain.ModerationRejected }, func(a *domain.MediaAsset) { a.Kind = domain.KindDocument }, func(a *domain.MediaAsset) { a.Kind = domain.KindModel }, func(a *domain.MediaAsset) { a.ProjectID = uuid.New() }} {
		bad := a
		change(&bad)
		repo = &originalReadStore{asset: bad}
		before := objects.calls
		if file, err := mediaapp.NewAssetDownloads(repo, objects).Open(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); file != nil || !errors.Is(err, mediaapp.ErrNotFound) || objects.calls != before {
			t.Fatal("ineligible original opened", err)
		}
	}
	a.ObjectLocation = nil
	repo = &originalReadStore{asset: a}
	before := objects.calls
	if _, err := mediaapp.NewAssetDownloads(repo, objects).Open(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, mediaapp.ErrUnavailable) || objects.calls != before {
		t.Fatal("legacy bucket guessed", err)
	}
}

type originalLegacyObjects struct {
	calls  int
	closed bool
}

func (s *originalLegacyObjects) Get(context.Context, string) (io.ReadCloser, error) {
	s.calls++
	return &originalLegacyBody{Reader: strings.NewReader("legacy original"), closed: &s.closed}, nil
}

type originalLegacyBody struct {
	io.Reader
	closed *bool
}

func (b *originalLegacyBody) Close() error { *b.closed = true; return nil }
func TestAssetOriginalLegacyCompatibilityAndAttachmentHeaders(t *testing.T) {
	a := routedReadImage()
	a.ByteSize = int64(len("legacy original"))
	a.FileName = "../../不可信\r\n文件.png"
	legacy := &originalLegacyObjects{}
	query := mediaapp.NewAssetDownloads(&originalReadStore{asset: a}, &routedReadObjects{}, legacy)
	router := gin.New()
	mediahttp.NewAssetDownloadHandler(query).Register(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/projects/"+a.ProjectID.String()+"/media/"+a.ID.String()+"/original", nil))
	if response.Code != 200 || response.Body.String() != "legacy original" || legacy.calls != 1 || !legacy.closed || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("original attachment contract lost", response.Code, response.Body.String())
	}
	disposition, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || strings.ContainsAny(params["filename"], "/\\\r\n") {
		t.Fatal("unsafe attachment filename", disposition, params, err)
	}
	a.ObjectLocation = readLocation("tos-main", a.ObjectKey)
	if _, err := mediaapp.NewAssetDownloads(&originalReadStore{asset: a}, nil, legacy).Open(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, mediaapp.ErrUnavailable) || legacy.calls != 1 {
		t.Fatal("persisted route fell back to legacy", err)
	}
}
