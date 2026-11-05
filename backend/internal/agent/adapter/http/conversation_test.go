package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
