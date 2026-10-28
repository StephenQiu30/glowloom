package subject_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	subjecthttp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/http"
)

type sourceActorFake struct {
	personal identity.Principal
	err      error
	last     identity.Principal
	write    bool
}

func (s *sourceActorFake) PersonalSourceActor(_ context.Context, actor identity.Principal, write bool) (identity.Principal, error) {
	s.last = actor
	s.write = write
	return s.personal, s.err
}

type sourceLibraryFake struct {
	actor       identity.Principal
	calls       int
	input       media.SubjectCatalogueQuery
	detail      *media.SubjectSourceItem
	detailCalls int
	disappear   bool
}

func (s *sourceLibraryFake) List(_ context.Context, actor identity.Principal, q media.SubjectCatalogueQuery) (media.SubjectSourcePage, error) {
	s.calls++
	s.actor = actor
	s.input = q
	return media.SubjectSourcePage{CurrentActorID: actor.ID, CurrentScopeID: actor.ScopeID, Items: []media.SubjectSourceItem{}, Page: q.Page, PageSize: q.PageSize, Capabilities: media.SubjectSourceCapabilities{UpscaledFilter: true}}, nil
}
func (s *sourceLibraryFake) Detail(_ context.Context, actor identity.Principal, id uuid.UUID) (media.SubjectSourceItem, error) {
	s.calls++
	s.actor = actor
	s.detailCalls++
	if s.disappear && s.detailCalls > 1 {
		return media.SubjectSourceItem{}, media.ErrNotFound
	}
	if s.detail != nil {
		return *s.detail, nil
	}
	return media.SubjectSourceItem{ID: id, Source: "personal_asset", Kind: "image", Title: "参考.png", CreatedAt: time.Now(), UpdatedAt: time.Now(), Media: media.SubjectSourceMedia{LibraryAssetSummary: media.LibraryAssetSummary{ID: id, Kind: "image", Revision: 1, ByteSize: 20, MIMEType: "image/png", FileName: "参考.png"}, Origin: "upload"}}, nil
}

type sourcePreviewFake struct {
	actor  identity.Principal
	scope  mediadomain.LibraryScope
	id     uuid.UUID
	media  media.LibraryAssetSummary
	onRead func()
}

func (s *sourcePreviewFake) Preview(_ context.Context, actor identity.Principal, scope mediadomain.LibraryScope, id uuid.UUID) (media.LibraryMediaPreview, error) {
	s.actor, s.scope, s.id = actor, scope, id
	if s.onRead != nil {
		s.onRead()
	}
	return media.LibraryMediaPreview{Asset: s.media, URL: "https://example.test/private-source", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func TestSubjectSourcePreviewResolvesOwnerAndRechecksAfterSigning(t *testing.T) {
	target := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	personal := target
	personal.OrgID = uuid.New()
	personal.ScopeID = uuid.New()
	actor := &sourceActorFake{personal: personal}
	repo := &sourceLibraryFake{}
	item, _ := repo.Detail(t.Context(), target, uuid.New())
	repo.detail = &item
	repo.detailCalls = 0
	files := &sourcePreviewFake{media: item.Media.LibraryAssetSummary}
	service := media.NewSubjectLibrary(actor, repo, files)
	if _, err := service.Preview(t.Context(), target, item.ID); err != nil || files.actor != personal || files.scope.Kind != mediadomain.LibraryPersonal || files.id != item.Media.ID {
		t.Fatal("personal source owner resolution", err)
	}
	project := uuid.New()
	item.ID = uuid.New()
	item.Source = "generated_output"
	item.ProjectID = &project
	item.Media.Origin = "generated"
	if _, err := service.Preview(t.Context(), target, item.ID); err != nil || files.actor != target || files.scope.ProjectID == nil || *files.scope.ProjectID != project || files.id != item.Media.ID || files.id == item.ID {
		t.Fatal("generated output preview trusted source id as asset/owner", err)
	}
	repo.detailCalls = 0
	files.onRead = func() { repo.disappear = true }
	if _, err := service.Preview(t.Context(), target, item.ID); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("source deleted during signing leaked lease", err)
	}
}

func TestSubjectSourcesKeepTargetAuthoritySeparateFromPersonalOwner(t *testing.T) {
	target := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	personal := target
	personal.ScopeID = uuid.New()
	personal.OrgID = uuid.New()
	actor := &sourceActorFake{personal: personal}
	repo := &sourceLibraryFake{}
	service := media.NewSubjectLibrary(actor, repo, nil)
	page, err := service.List(t.Context(), target, media.SubjectCatalogueQuery{Kind: "image", Page: 2, PageSize: 12, FavoriteOnly: true, Quality: "upscaled"})
	if err != nil || page.CurrentScopeID != target.ScopeID || page.CurrentActorID != target.ID || repo.actor != target || actor.last != target || !repo.input.FavoriteOnly || repo.input.Page != 2 || repo.input.Quality != "upscaled" {
		t.Fatalf("source authority/query: %#v, %v", page, err)
	}
	if !page.Capabilities.UpscaledFilter {
		t.Fatal("real operation-based upscale filter unavailable")
	}
	actor.err = identity.ErrForbidden
	if _, err := service.List(t.Context(), target, repo.input); !errors.Is(err, identity.ErrForbidden) || repo.calls != 1 {
		t.Fatal("revoked target reached source owner")
	}
	actor.err = nil
	actor.personal.ID = uuid.New()
	if _, err := service.SourceActor(t.Context(), target, true); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("server resolver changed actor identity")
	}
}

type sourceUploaderFake struct {
	calls int
	actor identity.Principal
}

func (s *sourceUploaderFake) UploadPersonalFor(c *gin.Context, a identity.Principal) {
	s.calls++
	s.actor = a
	c.Status(201)
}

func TestSubjectSourceHTTPRejectsOwnerSelectionAndChecksUploadBeforeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	personal := target
	personal.OrgID = uuid.New()
	personal.ScopeID = uuid.New()
	actor := &sourceActorFake{personal: personal}
	repo := &sourceLibraryFake{}
	upload := &sourceUploaderFake{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", target) })
	subjecthttp.NewSourceHandler(media.NewSubjectLibrary(actor, repo, nil), upload).Register(router.Group("/api"))
	for _, query := range []string{"kind=image&scope=personal", "kind=image&actor_id=other", "kind=image&kind=video", "kind=text", "kind=image&page_size=121", "kind=image&quality=8k", "kind=audio&quality=upscaled"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), "GET", "/api/subjects/sources?"+query, nil))
		if recorder.Code != 422 {
			t.Fatalf("unbounded %s: %d", query, recorder.Code)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid query reached library")
	}
	filtered := httptest.NewRecorder()
	router.ServeHTTP(filtered, httptest.NewRequestWithContext(t.Context(), "GET", "/api/subjects/sources?kind=image&quality=upscaled&favorite_only=true", nil))
	if filtered.Code != 200 || repo.input.Quality != "upscaled" || !repo.input.FavoriteOnly {
		t.Fatal("real source filters lost", filtered.Code)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), "POST", "/api/subjects/sources/uploads", nil))
	if recorder.Code != 201 || upload.actor != personal || !actor.write {
		t.Fatal("upload did not resolve authorized personal owner")
	}
	actor.err = identity.ErrForbidden
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), "POST", "/api/subjects/sources/uploads", nil))
	if recorder.Code != 403 || upload.calls != 1 {
		t.Fatal("revoked target allowed multipart body handling")
	}
}
