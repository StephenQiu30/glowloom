package media_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type linkUploadFixture struct {
	uploads, authorizations int
	replay                  bool
	fileName                string
	body                    string
	failure                 error
}

func (f *linkUploadFixture) AuthorizePersonal(context.Context, identity.Principal) error {
	f.authorizations++
	return f.failure
}
func (f *linkUploadFixture) BeginUpload(context.Context, identity.Principal, domain.LibraryScope, uuid.UUID) (mediaapp.UploadTarget, error) {
	return mediaapp.UploadTarget{SessionID: uuid.New(), Policy: mediaapp.UploadPolicy{MaxVideoBytes: 500 << 20}}, nil
}
func (f *linkUploadFixture) LookupPersonalVideoLink(context.Context, identity.Principal, uuid.UUID, string) (mediaapp.PersonalUploadResult, bool, error) {
	return mediaapp.PersonalUploadResult{Asset: mediaapp.LibraryAssetSummary{ID: uuid.New(), Kind: "video"}}, f.replay, nil
}
func (f *linkUploadFixture) UploadPersonal(_ context.Context, in mediaapp.UploadInput) (mediaapp.PersonalUploadResult, error) {
	f.uploads++
	f.fileName = in.Request.FileName
	body, _ := io.ReadAll(in.File.File)
	f.body = string(body)
	if !in.LocalReviewConfirmed || in.Request.ProjectID != uuid.Nil {
		return mediaapp.PersonalUploadResult{}, errors.New("invalid ownership")
	}
	return mediaapp.PersonalUploadResult{Asset: mediaapp.LibraryAssetSummary{ID: uuid.New(), Kind: "video"}}, nil
}

type linkResolverFixture struct {
	calls    int
	fileName string
}

func (f *linkResolverFixture) Resolve(_ context.Context, _ string, _ int64) (*mediaapp.Downloaded, error) {
	f.calls++
	file, err := os.CreateTemp("", "glowloom-link-fixture-*")
	if err != nil {
		return nil, err
	}
	f.fileName = file.Name()
	_, err = io.WriteString(file, "real downloaded video fixture")
	if err != nil {
		return nil, err
	}
	_, err = file.Seek(0, io.SeekStart)
	return &mediaapp.Downloaded{File: file, Size: 29, MIMEType: "video/mp4", SHA256: strings.Repeat("a", 64)}, err
}
func TestVideoLinkImportsOnlyRealResolvedBytesAndReplaysWithoutEgress(t *testing.T) {
	upload := &linkUploadFixture{}
	resolver := &linkResolverFixture{}
	service := mediaapp.NewVideoLinkService(upload, resolver)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	key, request := uuid.New(), uuid.New()
	result, err := service.Import(t.Context(), actor, key, request, mediaapp.VideoLinkRequest{URL: "https://v.douyin.com/abc/", LocalReviewConfirmed: true})
	if err != nil || result.Asset.Kind != "video" || upload.uploads != 1 || upload.body != "real downloaded video fixture" {
		t.Fatal(result, err, upload)
	}
	if _, err := os.Stat(resolver.fileName); !os.IsNotExist(err) {
		t.Fatal("resolved temporary file remains", err)
	}
	upload.replay = true
	if _, err := service.Import(t.Context(), actor, key, request, mediaapp.VideoLinkRequest{URL: "https://v.douyin.com/abc/", LocalReviewConfirmed: true}); err != nil || resolver.calls != 1 {
		t.Fatal("permanent replay fetched network", resolver.calls, err)
	}
	if _, err := service.Import(t.Context(), actor, key, request, mediaapp.VideoLinkRequest{URL: "https://example.com/video", LocalReviewConfirmed: true}); err == nil {
		t.Fatal("non-platform source accepted")
	}
	if _, err := service.Import(t.Context(), actor, key, request, mediaapp.VideoLinkRequest{URL: "https://v.douyin.com/abc/"}); err == nil {
		t.Fatal("unreviewed link accepted")
	}
}
