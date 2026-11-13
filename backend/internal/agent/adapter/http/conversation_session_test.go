package http

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type conversationSessionAuthority struct {
	want identity.Principal
	seen bool
}

func (a *conversationSessionAuthority) check(got identity.Principal) error {
	a.seen = true
	if got != a.want {
		return identity.ErrForbidden
	}
	return nil
}

func (a *conversationSessionAuthority) ReadConversationInput(_ context.Context, actor identity.Principal, _ uuid.UUID, raw json.RawMessage) (json.RawMessage, error) {
	return raw, a.check(actor)
}

func (a *conversationSessionAuthority) UnresolvedConversationRuns(_ context.Context, actor identity.Principal, _, _ uuid.UUID) ([]uuid.UUID, error) {
	return []uuid.UUID{}, a.check(actor)
}

func TestConversationReadsKeepAuthenticatedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, operation := range []string{"getAgentRun", "getCurrentAgentSubmission"} {
		t.Run(operation, func(t *testing.T) {
			actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 7, PlatformEpoch: 3}
			scope := domain.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: uuid.New()}
			authority := &conversationSessionAuthority{want: actor}
			runtime := historyRuntime(func(context.Context, string, map[string]string, url.Values, any, string) (*stdhttp.Response, error) {
				return &stdhttp.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"input":{"plain_text":"hello"}}`))}, nil
			})
			handler := NewPublicHandler(nil, nil, runtime).WithConversationHistory(authority, authority)
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
			c.Params = gin.Params{{Key: "sid", Value: uuid.NewString()}}
			c.Set("principal", actor)
			handler.proxy(c, operation, scope, nil)
			require.True(t, authority.seen)
			require.Equal(t, 200, response.Code, response.Body.String())
		})
	}
}
