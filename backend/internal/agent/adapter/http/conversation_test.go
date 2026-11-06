package http

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type conversationStore struct {
	app.BusinessStore
	authority domain.Authority
}

func (s *conversationStore) ReadAuthority(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (domain.Authority, error) {
	return s.authority, nil
}

func TestConversationControlStatusDoesNotRequireAgentAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &conversationStore{authority: domain.Authority{Admission: domain.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), Scope: domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", s.authority.Scope.Principal()) })
	NewPublicHandler(s, nil, nil).Register(router.Group("/api"))
	path := "/api/projects/" + s.authority.Scope.ProjectID.String() + "/agent/sessions/" + s.authority.SessionID.String() + "/runs/" + s.authority.RootRunID.String() + "/commands/current"
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
	if out.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private control response must not be cached")
	}
	if out.Code != 200 {
		t.Fatalf("Go control facts must remain readable without Agent: status %d body %s", out.Code, out.Body.String())
	}
}

type conversationSourceFixture struct {
	called bool
	actor  uuid.UUID
}

func (f *conversationSourceFixture) CheckConversationSources(_ context.Context, a identity.Principal, _ uuid.UUID, _ prompt.Draft) error {
	f.called = true
	if a.ID != f.actor {
		return identity.ErrForbidden
	}
	return prompt.ErrReferenceUnavailable
}
func (s *conversationStore) ValidateGrant(context.Context, domain.Grant) (domain.Grant, error) {
	return domain.Grant{Scope: s.authority.Scope}, nil
}
func TestConversationModelSourceCheckUsesCurrentGrantAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &conversationStore{authority: domain.Authority{Admission: domain.Admission{Scope: domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}}}}
	source := &conversationSourceFixture{actor: s.authority.Scope.ActorID}
	router := gin.New()
	token := strings.Repeat("a", 48)
	NewBusinessHandler(s, app.Capabilities{}, token).WithInputSources(source).Register(router)
	raw := `{"grant":{},"draft":{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"test"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}}`
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/input-sources/validate", strings.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Agent-Audience", "agent-business")
	out := httptest.NewRecorder()
	router.ServeHTTP(out, request)
	if out.Code != 422 || !source.called || !strings.Contains(out.Body.String(), "conversation_reference_unavailable") {
		t.Fatalf("revoked source allowed at model boundary: %d %s", out.Code, out.Body.String())
	}
}

type historyProjectAccess func(context.Context, identity.Principal, uuid.UUID, bool) error

func (f historyProjectAccess) AuthorizeProject(ctx context.Context, principal identity.Principal, project uuid.UUID, write bool) error {
	return f(ctx, principal, project, write)
}

type historyRuntime func(context.Context, string, map[string]string, url.Values, any, string) (*stdhttp.Response, error)

func (f historyRuntime) Request(ctx context.Context, op string, params map[string]string, query url.Values, body any, cursor string) (*stdhttp.Response, error) {
	return f(ctx, op, params, query, body, cursor)
}

func TestConversationHistoryUpdateRequiresProjectWriteAndVerifiesReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}
	sid := uuid.NewString()
	writable := false
	mismatch := false
	calls := 0
	access := historyProjectAccess(func(_ context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
		if actor.ID != scope.ActorID || project != scope.ProjectID || !write {
			t.Fatalf("incorrect project write authorization: %v %v %v", actor.ID, project, write)
		}
		if !writable {
			return identity.ErrForbidden
		}
		return nil
	})
	runtime := historyRuntime(func(_ context.Context, op string, params map[string]string, _ url.Values, body any, _ string) (*stdhttp.Response, error) {
		calls++
		input := body.(runtimeclient.UpdateSessionRequest)
		if op != "updateAgentSession" || params["sid"] != sid ||
			input.Action != "rename" || input.Title == nil || *input.Title != "新标题" ||
			input.ActorID != scope.ActorID.String() || input.ProjectID != scope.ProjectID.String() ||
			input.OrgID != scope.OrgID.String() || input.ScopeID != scope.ScopeID.String() {
			t.Fatalf("incorrect trusted mutation: %s %v %+v", op, params, input)
		}
		id := sid
		if mismatch {
			id = uuid.NewString()
		}
		reply := `{"id":"` + id + `","actor_id":"` + input.ActorID + `","org_id":"` + input.OrgID + `","scope_id":"` + input.ScopeID + `","project_id":"` + input.ProjectID + `","title":"新标题","pinned":true}`
		return &stdhttp.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply))}, nil
	})
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", scope.Principal()) })
	NewPublicHandler(nil, access, runtime).Register(router.Group("/api"))
	path := "/api/projects/" + scope.ProjectID.String() + "/agent/sessions/" + strings.ToUpper(sid)
	send := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), "PATCH", path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		return out
	}
	rename := `{"action":"rename","title":" 新标题 "}`
	if out := send(rename); out.Code != 403 || calls != 0 {
		t.Fatalf("read-only actor can mutate history: %d %s", out.Code, out.Body)
	}
	writable = true
	if out := send(`{"action":"delete","title":"同时改名"}`); out.Code != 422 || calls != 0 {
		t.Fatalf("mixed mutation accepted: %d %s", out.Code, out.Body)
	}
	if out := send(rename); out.Code != 200 || calls != 1 || !strings.Contains(out.Body.String(), "新标题") {
		t.Fatalf("rename receipt not returned: %d %s", out.Code, out.Body)
	}
	mismatch = true
	if out := send(rename); out.Code != 404 || calls != 2 {
		t.Fatalf("unexpected session receipt exposed: %d %s", out.Code, out.Body)
	}
}
