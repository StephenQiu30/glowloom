package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type personalHistoryReader struct {
	actor identityapp.Principal
	input operationapp.PersonalHistoryInput
	page  operationapp.PersonalHistoryPage
	err   error
	calls int
}

func (s *personalHistoryReader) ListPersonalHistory(_ context.Context, a identityapp.Principal, i operationapp.PersonalHistoryInput) (operationapp.PersonalHistoryPage, error) {
	s.actor, s.input = a, i
	s.calls++
	return s.page, s.err
}
func historyActor() identityapp.Principal {
	return identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
}
func historyTime(t time.Time) *time.Time { return &t }
func TestPersonalHistoryValidatesIdentityAndFiltersBeforeStorage(t *testing.T) {
	actor := historyActor()
	for _, tc := range []struct {
		name  string
		input operationapp.PersonalHistoryInput
	}{
		{"negative limit", operationapp.PersonalHistoryInput{Limit: -1}}, {"oversized page", operationapp.PersonalHistoryInput{Limit: 201}}, {"unknown direction", operationapp.PersonalHistoryInput{Order: "random"}},
		{"unknown kind", operationapp.PersonalHistoryInput{Kind: "model"}},
		{"control in search", operationapp.PersonalHistoryInput{Search: "cat\x00"}}, {"long search", operationapp.PersonalHistoryInput{Search: strings.Repeat("猫", 201)}}, {"invalid utf8", operationapp.PersonalHistoryInput{Search: string([]byte{0xff})}},
		{"invalid cursor", operationapp.PersonalHistoryInput{After: &operationapp.TaskCursor{ID: uuid.New()}}}, {"empty interval", operationapp.PersonalHistoryInput{CreatedFrom: historyTime(time.Unix(100, 0)), CreatedBefore: historyTime(time.Unix(100, 0))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &personalHistoryReader{}
			_, err := operationapp.NewPersonalHistoryQuery(s).List(t.Context(), actor, tc.input)
			if !errors.Is(err, operationapp.ErrInvalidPublicQuery) || s.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, s.calls)
			}
		})
	}
	actor.ScopeID = uuid.Nil
	s := &personalHistoryReader{}
	if _, err := operationapp.NewPersonalHistoryQuery(s).List(t.Context(), actor, operationapp.PersonalHistoryInput{}); !errors.Is(err, identityapp.ErrForbidden) || s.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, s.calls)
	}
}
func TestPersonalHistoryNormalizesInputsAndKeepsErrorChain(t *testing.T) {
	s := &personalHistoryReader{}
	page, err := operationapp.NewPersonalHistoryQuery(s).List(t.Context(), historyActor(), operationapp.PersonalHistoryInput{Search: "  晨光  ", Capability: "image.generate"})
	if err != nil || page.Items == nil || s.input.Search != "晨光" || s.input.Order != "desc" || s.input.Limit != 40 {
		t.Fatalf("page=%+v input=%+v err=%v", page, s.input, err)
	}
	sentinel := errors.New("database unavailable")
	s.err = sentinel
	if _, err := operationapp.NewPersonalHistoryQuery(s).List(t.Context(), historyActor(), operationapp.PersonalHistoryInput{}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
func historyRouter(actor identityapp.Principal, s *personalHistoryReader) *gin.Engine {
	r := gin.New()
	r.Use(httpapi.Middleware("http://localhost:3000"))
	r.Use(func(c *gin.Context) { c.Set("principal", actor) })
	operationhttp.NewPersonalHistoryHandler(operationapp.NewPersonalHistoryQuery(s)).Register(r.Group("/api"))
	return r
}
func historyRequest(ctx context.Context, r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequestWithContext(ctx, "GET", path, nil))
	return w
}
func TestPersonalHistoryCursorBindsIdentityScopeAndAllFilters(t *testing.T) {
	actor := historyActor()
	last := operationapp.TaskSummary{ID: uuid.New(), ProjectID: uuid.New(), CreateTime: time.Unix(100, 0).UTC()}
	s := &personalHistoryReader{page: operationapp.PersonalHistoryPage{Items: []operationapp.PersonalHistoryItem{{TaskSummary: last}}, Next: &operationapp.TaskCursor{ID: last.ID, CreateTime: last.CreateTime}}}
	router := historyRouter(actor, s)
	base := "/api/operations/history?kind=image&capability=image.generate&search=cat&favorite_only=true&resolutions=2K,4K&ratios=16:9&generation_modes=all_reference&process_types=super_resolution&order=asc&limit=1&created_from=1970-01-01T00:00:00Z&created_before=2030-01-01T00:00:00Z"
	first := historyRequest(t.Context(), router, base)
	var result operationhttp.PersonalHistoryPageResponse
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &result) != nil || result.NextCursor == nil {
		t.Fatalf("first=%d %s", first.Code, first.Body)
	}
	cursor := "&cursor=" + url.QueryEscape(*result.NextCursor)
	second := historyRequest(t.Context(), router, base+cursor)
	if second.Code != 200 || s.input.After == nil || s.input.After.ID != last.ID {
		t.Fatalf("second=%d %s", second.Code, second.Body)
	}
	for _, changed := range []string{strings.Replace(base, "favorite_only=true", "favorite_only=false", 1), strings.Replace(base, "resolutions=2K,4K", "resolutions=1K", 1), strings.Replace(base, "ratios=16:9", "ratios=9:16", 1), strings.Replace(base, "generation_modes=all_reference", "generation_modes=tts", 1), strings.Replace(base, "process_types=super_resolution", "process_types=frame_interpolation", 1), strings.Replace(base, "kind=image", "kind=video", 1), strings.Replace(base, "image.generate", "video.generate", 1), strings.Replace(base, "search=cat", "search=dog", 1), strings.Replace(base, "order=asc", "order=desc", 1), strings.Replace(base, "limit=1", "limit=2", 1), strings.Replace(base, "1970-01-01", "1970-01-02", 1), strings.Replace(base, "2030-01-01", "2030-01-02", 1)} {
		before := s.calls
		res := historyRequest(t.Context(), router, changed+cursor)
		if res.Code != 422 || s.calls != before {
			t.Fatalf("changed filter accepted %s: %d", changed, res.Code)
		}
	}
	for _, change := range []func(*identityapp.Principal){func(a *identityapp.Principal) { a.ID = uuid.New() }, func(a *identityapp.Principal) { a.OrgID = uuid.New() }, func(a *identityapp.Principal) { a.ScopeID = uuid.New() }} {
		other := actor
		change(&other)
		before := s.calls
		res := historyRequest(t.Context(), historyRouter(other, s), base+cursor)
		if res.Code != 422 || s.calls != before {
			t.Fatalf("foreign cursor accepted %d", res.Code)
		}
	}
	if first.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("missing private response policy")
	}
}
func TestPersonalHistoryHTTPRejectsUnsupportedAndDuplicateConditions(t *testing.T) {
	s := &personalHistoryReader{}
	router := historyRouter(historyActor(), s)
	for _, q := range []string{"actor_id=" + uuid.NewString(), "search=cat&search=dog", "created_from=yesterday", "created_from=2026-10-28T00:00:00Z&created_before=2026-10-27T00:00:00Z", "limit=0", "favorite_only=1", "resolutions=2K&resolutions=4K", "resolutions=2K,2K", "process_types=unknown", "kind=model", "order=sideways", "cursor=garbage"} {
		res := historyRequest(t.Context(), router, "/api/operations/history?"+q)
		if res.Code != 422 || s.calls != 0 {
			t.Fatalf("query=%s response=%d %s calls=%d", q, res.Code, res.Body, s.calls)
		}
	}
}
