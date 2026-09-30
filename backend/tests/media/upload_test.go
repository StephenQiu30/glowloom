package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func uploadPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

type uploadRepoFake struct {
	mu           sync.Mutex
	authorizeErr error
	commitErr    error
	exists       bool
	existsErr    error
	asset        domain.MediaAsset
	rends        []domain.Rendition
	commits      int
}

func (r *uploadRepoFake) AuthorizeUpload(context.Context, identityapp.Principal, uuid.UUID) (string, error) {
	return "16:9", r.authorizeErr
}
func (*uploadRepoFake) FindUpload(context.Context, identityapp.Principal, mediaapp.UploadRequest) (mediaapp.UploadResult, bool, error) {
	return mediaapp.UploadResult{}, false, nil
}
func (r *uploadRepoFake) CommitUpload(_ context.Context, _ identityapp.Principal, _ mediaapp.UploadRequest, a domain.MediaAsset, rends []domain.Rendition) (mediaapp.UploadResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commits++
	r.asset = a
	r.rends = rends
	return mediaapp.UploadResult{Asset: mediaapp.UploadSummary(a)}, r.commitErr
}
func (r *uploadRepoFake) UploadAssetExists(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return r.exists, r.existsErr
}

type uploadObjectsFake struct {
	mu      sync.Mutex
	items   map[string]mediaapp.ObjectInfo
	statErr error
	removed []string
}

func (s *uploadObjectsFake) PutIfAbsent(_ context.Context, key string, reader io.Reader, size int64, mime, sha string) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if int64(len(data)) != size || hex.EncodeToString(hash[:]) != sha {
		return mediaapp.ErrObjectMismatch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]mediaapp.ObjectInfo{}
	}
	if _, found := s.items[key]; found {
		return mediaapp.ErrObjectAlreadyExists
	}
	s.items[key] = mediaapp.ObjectInfo{Size: size, ContentType: mime, SHA256: sha}
	return nil
}
func (s *uploadObjectsFake) Stat(_ context.Context, key string) (mediaapp.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[key], s.statErr
}
func (s *uploadObjectsFake) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	s.removed = append(s.removed, key)
	return nil
}

func uploadInput(t *testing.T) mediaapp.UploadInput {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(uploadPNG(t)), "参考.png")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return mediaapp.UploadInput{Actor: identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer},
		Request: mediaapp.UploadRequest{ProjectID: uuid.New(), Key: uuid.New(), FileName: "参考.png", RequestID: uuid.New()}, File: file, LocalReviewConfirmed: true}
}

func TestUploadPublishesOnlyExplicitReviewedVerifiedMedia(t *testing.T) {
	for _, tc := range []struct {
		name    string
		review  bool
		statErr error
		wantErr error
	}{
		{"review required", false, nil, mediaapp.ErrInvalidUpload},
		{"object verification failure", true, mediaapp.ErrUnavailable, mediaapp.ErrUnavailable},
		{"reviewed ready", true, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, objects := &uploadRepoFake{}, &uploadObjectsFake{statErr: tc.statErr}
			service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			in := uploadInput(t)
			in.LocalReviewConfirmed = tc.review
			result, err := service.Upload(t.Context(), in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Upload error=%v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if repo.commits != 0 || len(objects.items) != 0 || result.Asset.ID != uuid.Nil {
					t.Fatalf("failed upload published: commits=%d objects=%d result=%+v", repo.commits, len(objects.items), result)
				}
				return
			}
			if !repo.asset.CanReference() || repo.asset.Origin != domain.OriginUpload || repo.asset.SourceOperationID != nil || len(repo.rends) != 2 || len(objects.items) != 3 || !bytes.Contains(repo.asset.ModerationDetail, []byte("local_workspace_owner_review")) {
				t.Fatalf("reviewed upload=%+v, renditions=%d objects=%d", repo.asset, len(repo.rends), len(objects.items))
			}
		})
	}
}

func TestUploadCleanupPreservesUnknownCommittedObjects(t *testing.T) {
	for _, tc := range []struct {
		name        string
		exists      bool
		existsErr   error
		wantObjects int
	}{
		{"known rollback removes owned objects", false, nil, 0},
		{"commit completed before cancellation", true, nil, 3},
		{"commit cannot be determined", false, mediaapp.ErrUnavailable, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &uploadRepoFake{commitErr: context.Canceled, exists: tc.exists, existsErr: tc.existsErr}
			objects := &uploadObjectsFake{}
			service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			_, err := service.Upload(t.Context(), uploadInput(t))
			if !errors.Is(err, context.Canceled) || len(objects.items) != tc.wantObjects {
				t.Fatalf("unknown commit cleanup: err=%v objects=%d want=%d", err, len(objects.items), tc.wantObjects)
			}
		})
	}
}

func TestUploadReadEnforcesContentLimitsAndCleansTemporaryFiles(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	for _, name := range []string{"../reference.png", "a\\reference.png", "reference\x00.png", strings.Repeat("界", 86)} {
		if _, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(uploadPNG(t)), name); !errors.Is(err, mediaapp.ErrInvalidUpload) {
			t.Fatalf("unsafe filename error=%v", err)
		}
	}
	if _, err := mediaapp.ReadUpload(t.Context(), strings.NewReader("MZ executable"), "reference.mp4"); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatalf("false extension error=%v", err)
	}
	pngBytes := uploadPNG(t)
	reader := io.MultiReader(bytes.NewReader(pngBytes), io.LimitReader(zeroUploadReader{}, mediaapp.MaxUploadImageBytes))
	if _, err := mediaapp.ReadUpload(t.Context(), reader, "reference.png"); !errors.Is(err, mediaapp.ErrUploadTooLarge) {
		t.Fatalf("image oversize error=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mediaapp.ReadUpload(ctx, bytes.NewReader(pngBytes), "reference.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled copy error=%v", err)
	}
	entries, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed uploads left temp files=%d err=%v", len(entries), err)
	}
}

type zeroUploadReader struct{}

func (zeroUploadReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type uploadLongVideoProbe struct{}

func (uploadLongVideoProbe) Probe(context.Context, *mediaapp.Downloaded) (mediaapp.ProbeResult, error) {
	width, height, duration := int32(10), int32(10), int32(60001)
	codec := "h264"
	return mediaapp.ProbeResult{Kind: domain.KindVideo, Extension: "mp4", Width: &width, Height: &height, DurationMS: &duration, Codec: &codec}, nil
}
func TestUploadRejectsVideoPastSixtySecondsBeforeRendering(t *testing.T) {
	repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
	in := uploadInput(t)
	in.File.MIMEType = "video/mp4"
	service := mediaapp.NewUploadService(repo, uploadLongVideoProbe{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUnsupportedUpload) || repo.commits != 0 || len(objects.items) != 0 {
		t.Fatalf("long video accepted: %v", err)
	}
}

type uploadDimensionProbe struct {
	kind          domain.Kind
	width, height int32
}

func (p uploadDimensionProbe) Probe(context.Context, *mediaapp.Downloaded) (mediaapp.ProbeResult, error) {
	codec, extension := "png", "png"
	duration := int32(1000)
	if p.kind == domain.KindVideo {
		codec, extension = "h264", "mp4"
	}
	return mediaapp.ProbeResult{Kind: p.kind, Extension: extension, Width: &p.width, Height: &p.height, Codec: &codec, DurationMS: &duration}, nil
}

type uploadRenderSpy struct{ calls int }

func (r *uploadRenderSpy) Render(context.Context, *mediaapp.Downloaded, mediaapp.ProbeResult, string) ([]mediaapp.RenditionFile, error) {
	r.calls++
	return nil, mediaapp.ErrUnavailable
}

func TestUploadDimensionsAreBoundedBeforeRendering(t *testing.T) {
	for _, kind := range []domain.Kind{domain.KindImage, domain.KindVideo} {
		for _, tc := range []struct {
			name          string
			width, height int32
			reject        bool
		}{
			{"maximum edge", 8192, 1, false},
			{"maximum pixels", 8000, 5000, false},
			{"wide", 8193, 1, true},
			{"tall", 1, 8193, true},
			{"too many pixels", 8192, 4883, true},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				repo, renderer := &uploadRepoFake{}, &uploadRenderSpy{}
				in := uploadInput(t)
				if kind == domain.KindVideo {
					in.File.MIMEType = "video/mp4"
				}
				service := mediaapp.NewUploadService(repo, uploadDimensionProbe{kind, tc.width, tc.height}, renderer, &uploadObjectsFake{}, time.Now)
				_, err := service.Upload(t.Context(), in)
				if tc.reject {
					if !errors.Is(err, mediaapp.ErrUnsupportedUpload) || renderer.calls != 0 {
						t.Fatalf("oversized decoded dimensions reached renderer: err=%v calls=%d", err, renderer.calls)
					}
				} else if !errors.Is(err, mediaapp.ErrUnavailable) || renderer.calls != 1 {
					t.Fatalf("valid boundary dimensions rejected: err=%v calls=%d", err, renderer.calls)
				}
			})
		}
	}
}
