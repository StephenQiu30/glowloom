package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type trialAuth struct{ err error }

func (a trialAuth) AuthorizeDevelopmentTrial(context.Context, identityapp.Principal) error {
	return a.err
}

type trialClient struct {
	state                 application.TrialState
	owner                 string
	started               int
	input                 application.TrialInput
	signal                string
	exists                bool
	signalID              string
	cancelOriginalMissing bool
}

func (c *trialClient) Start(_ context.Context, input application.TrialInput) (application.TrialState, error) {
	c.started++
	c.input = input
	if c.exists && c.state.Fingerprint != input.Request.Fingerprint() {
		return c.state, trialtask.ErrConflict
	}
	return c.state, nil
}
func (c *trialClient) State(_ context.Context, id string) (application.TrialState, error) {
	if id != c.owner {
		return c.state, trialtask.ErrMissing
	}
	return c.state, nil
}
func (c *trialClient) Turn(_ context.Context, id string, _ application.TrialMessage) error {
	c.signalID = id
	c.signal = "turn"
	return nil
}
func (c *trialClient) Cancel(_ context.Context, id string) error {
	c.signalID = id
	c.signal = "cancel"
	return nil
}
func (c *trialClient) Recover(_ context.Context, id string) error {
	c.signalID = id
	c.signal = "recover"
	return nil
}
func trialRouter(actor identityapp.Principal, auth trialAuth, c *trialClient) *gin.Engine {
	r := gin.New()
	r.Use(func(ctx *gin.Context) { ctx.Set("principal", actor) })
	operationhttp.NewTrialHandler(c, auth, nil).Register(r.Group("/api"))
	return r
}
func TestTrialHTTPRejectsCreatorRevokedAdminAndRuntimeInjection(t *testing.T) {
	for _, tc := range []struct {
		name, role, body string
		authErr          error
		want             int
	}{
		{"creator", "creator", `{}`, nil, 403}, {"revoked admin", "platform_admin", `{}`, errors.New("revoked"), 403},
		{"runtime injection", "platform_admin", `{"thread_id":"foreign","binary":"/bin/sh"}`, nil, 422},
		{"multiple objects", "platform_admin", `{} {}`, nil, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &trialClient{}
			r := trialRouter(identityapp.Principal{PlatformRole: tc.role}, trialAuth{tc.authErr}, c)
			res := httptest.NewRecorder()
			r.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials", strings.NewReader(tc.body)))
			require.Equal(t, tc.want, res.Code)
			require.Zero(t, c.started)
		})
	}
}
func TestTrialHTTPStartsWithoutProjectPriceOrBudgetAndRedactsThread(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	request := application.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "chat", Model: "fixture", Prompt: "hello"}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	c := &trialClient{owner: application.TrialID(actor, request.RequestID), state: application.TrialState{State: "succeeded", Fingerprint: request.Fingerprint(), Turns: []application.TrialTurn{{TrialProgress: application.TrialProgress{ThreadID: "private-thread", State: "succeeded", Text: "answer"}}}}}
	r := trialRouter(actor, trialAuth{}, c)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials", strings.NewReader(string(body))))
	require.Equal(t, 202, res.Code)
	require.Equal(t, actor, c.input.Actor)
	require.Equal(t, request, c.input.Request)
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/ai-trials/"+request.RequestID.String(), nil))
	require.Equal(t, 200, res.Code)
	require.NotContains(t, res.Body.String(), "private-thread")
	require.Contains(t, res.Body.String(), "answer")
	c.exists = true
	request.Prompt = "changed"
	body, err = json.Marshal(request)
	require.NoError(t, err)
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials", strings.NewReader(string(body))))
	require.Equal(t, 409, res.Code)
	other := actor
	other.ID = uuid.New()
	res = httptest.NewRecorder()
	trialRouter(other, trialAuth{}, c).ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/ai-trials/"+request.RequestID.String(), nil))
	require.Equal(t, 404, res.Code)
}
func TestTrialHTTPRecoveryUsesOnlyOriginalStoredVideoIdentity(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	id := uuid.New()
	configID, keyID := uuid.New(), uuid.New()
	c := &trialClient{owner: application.TrialID(actor, id), state: application.TrialState{State: "unknown", Source: "cloud", Kind: "video", Model: "fixture", ProviderID: uuid.New(), Fingerprint: "original", Turns: []application.TrialTurn{{Prompt: "original", TrialProgress: application.TrialProgress{TaskID: "original-task", ConfigVersionID: configID, CredentialID: keyID}}}}}
	res := httptest.NewRecorder()
	trialRouter(actor, trialAuth{}, c).ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials/"+id.String()+"/recover", nil))
	require.Equal(t, 202, res.Code)
	require.Equal(t, "recover", c.signal)
	require.Equal(t, c.owner, c.signalID)
}

func TestTrialHTTPCancelStopsActiveReadOnlyRecoveryWhenOriginalIsClosed(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	id := uuid.New()
	c := &trialClient{owner: application.TrialID(actor, id), cancelOriginalMissing: true, state: application.TrialState{State: "unknown"}}
	response := httptest.NewRecorder()
	trialRouter(actor, trialAuth{}, c).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials/"+id.String()+"/cancel", nil))
	require.Equal(t, 204, response.Code)
	require.Equal(t, c.owner, c.signalID)
}
