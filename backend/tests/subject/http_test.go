package subject_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	subjecthttp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func TestSubjectHTTPRejectsUnknownAndAmbiguousQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	query := &queryFake{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	subjecthttp.NewHandler(nil, query).Register(router.Group("/api"))
	for _, path := range []string{"/api/subjects?page_size=121", "/api/subjects?q=a&q=b", "/api/subjects?project_id=" + uuid.NewString(), "/api/subjects?order=name", "/api/subjects?from=bad"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		if response.Code != 422 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
	if query.calls != 0 {
		t.Fatal("invalid query reached owner")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/subjects?q=%E4%B8%BB%E4%BD%93&time_field=created&order=created_asc&page=2&page_size=12", nil))
	if response.Code != 200 || query.last.Query != "主体" || query.last.Page != 2 || query.last.PageSize != 12 || query.last.Order != "created_asc" {
		t.Fatalf("valid bounded query: %d %#v", response.Code, query.last)
	}
	var page application.Page
	if json.Unmarshal(response.Body.Bytes(), &page) != nil {
		t.Fatal("invalid response")
	}
}

type queryFake struct {
	calls int
	last  application.ListInput
}

func (q *queryFake) List(_ context.Context, a identity.Principal, in application.ListInput) (application.Page, error) {
	q.calls++
	q.last = in
	return application.Page{CurrentActorID: a.ID, CurrentScopeID: a.ScopeID, Items: []application.Detail{}}, nil
}
func (q *queryFake) Get(context.Context, identity.Principal, uuid.UUID) (application.Detail, error) {
	return application.Detail{}, application.ErrNotFound
}
func (q *queryFake) ReadVersion(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (domain.Version, error) {
	return domain.Version{}, application.ErrNotFound
}
func (q *queryFake) BoundMedia(context.Context, identity.Principal, uuid.UUID) (application.BoundMedia, error) {
	return application.BoundMedia{}, application.ErrNotFound
}

func TestSubjectHTTPClosesClientMediaFactsAndReturnsSafeSettings(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	media := fact("image")
	repo.media.facts[media.AssetID] = media
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	subjecthttp.NewHandler(application.NewService(repo, time.Now), &queryFake{}).Register(router.Group("/api"))
	call := func(body string, key bool) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/subjects", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if key {
			request.Header.Set("Idempotency-Key", uuid.NewString())
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	payload := `{"expected_revision":0,"name":"主体","primary":{"asset_id":"` + media.AssetID.String() + `"},"description":"","references":[]}`
	if got := call(payload, false); got.Code != 422 {
		t.Fatalf("missing permanent key: %d", got.Code)
	}
	injected := strings.Replace(payload, `"primary":{`, `"primary":{"sha256":"client-forged",`, 1)
	if got := call(injected, true); got.Code != 422 || len(repo.heads) != 0 {
		t.Fatalf("client media facts admitted: %d", got.Code)
	}
	response := call(payload, true)
	if response.Code != 200 {
		t.Fatalf("valid save: %d %s", response.Code, response.Body.String())
	}
	for _, private := range []string{"sha256", "personal_org_id", "personal_actor_id", "object_key", "source_project_id"} {
		if bytes.Contains(response.Body.Bytes(), []byte(private)) {
			t.Fatalf("leaked %s", private)
		}
	}
	var result application.Detail
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Primary.BindingID == uuid.Nil || result.Primary.AssetID != media.AssetID {
		t.Fatal("missing stable safe binding")
	}
	request := httptest.NewRequestWithContext(t.Context(), "PUT", "/api/subjects/"+result.ID.String()+"/favorite", strings.NewReader(`{"expected_revision":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, request)
	if missing.Code != 422 {
		t.Fatalf("missing favorite was interpreted as false: %d", missing.Code)
	}
}
