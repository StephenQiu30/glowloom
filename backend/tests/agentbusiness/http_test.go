package agentbusiness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const businessToken = "business-only-test-token-00000000000000"
const runtimeToken = "runtime-only-test-token-000000000000000"

type identityStore struct {
	app.BusinessStore
	calls atomic.Int32
}

func (s *identityStore) RegisterGrant(context.Context, domain.GrantRequest) (domain.Grant, error) {
	s.calls.Add(1)
	return domain.Grant{}, domain.ErrGrant
}
func TestInternalServiceRequiresIndependentTokenAndAudience(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &identityStore{}
	router := gin.New()
	agenthttp.NewBusinessHandler(store, app.Capabilities{}, businessToken).Register(router)
	for _, tc := range []struct {
		token, audience string
		want            int
	}{{"", "", 401}, {runtimeToken, "agent-business", 401}, {businessToken, "agent", 401}, {businessToken, "agent-business", 403}} {
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/grants", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Agent-Audience", tc.audience)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != tc.want {
			t.Fatalf("status %d expected %d body %s", out.Code, tc.want, out.Body.String())
		}
	}
	if store.calls.Load() != 1 {
		t.Fatal("unauthenticated request reached business authority")
	}
}

type projectAccess struct{}

func (projectAccess) AuthorizeProject(context.Context, identity.Principal, uuid.UUID, bool) error {
	return nil
}

type runtimeCapture struct {
	actor uuid.UUID
	calls int
}

func (r *runtimeCapture) Request(_ context.Context, _ string, _ map[string]string, _ url.Values, body any, _ string) (*http.Response, error) {
	r.calls++
	encoded, _ := json.Marshal(body)
	var trusted struct {
		ActorID uuid.UUID `json:"actor_id"`
	}
	_ = json.Unmarshal(encoded, &trusted)
	r.actor = trusted.ActorID
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
}
func TestPublicSessionUsesAuthenticatedActorAndRejectsSpoofedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	runtime := &runtimeCapture{}
	router := gin.New()
	protected := router.Group("/api", func(c *gin.Context) { c.Set("principal", actor) })
	agenthttp.NewPublicHandler(nil, projectAccess{}, runtime).Register(protected)
	path := "/api/projects/" + uuid.NewString() + "/agent/sessions"
	for i, body := range []string{`{"request_id":"` + uuid.NewString() + `","title":"Private","actor_id":"` + uuid.NewString() + `"}`, `{"request_id":"` + uuid.NewString() + `","title":"Private"}`} {
		req := httptest.NewRequestWithContext(t.Context(), "POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if i == 0 && out.Code != 422 {
			t.Fatalf("accepted spoof %d", out.Code)
		}
		if i == 1 && (out.Code != 200 || runtime.actor != actor.ID) {
			t.Fatalf("trusted actor missing: %d %v", out.Code, runtime.actor)
		}
	}
	if runtime.calls != 1 {
		t.Fatal("spoof forwarded")
	}
	old := httptest.NewRecorder()
	router.ServeHTTP(old, httptest.NewRequestWithContext(t.Context(), "GET", strings.Replace(path, "agent/sessions", "agent-sessions", 1), nil))
	if old.Code != 404 {
		t.Fatal("old compatibility route retained")
	}
}
func TestRuntimeClientKeepsDirectionTokenAndCursor(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+runtimeToken || r.Header.Get("X-Agent-Audience") != "agent" || r.Header.Get("Last-Event-ID") != "42:7" {
			t.Error("missing trusted headers")
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := runtimeclient.New(server.URL, runtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Request(t.Context(), "subscribeAgentEvents", map[string]string{"rid": uuid.NewString()}, nil, nil, "42:7")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if calls.Load() != 1 {
		t.Fatal("request not made")
	}
}
func TestControlDeliveryLostConfirmationRetriesSameDecision(t *testing.T) {
	s, a, _ := fixture(t)
	d := decide(t, s, a, "pause", 1)
	var calls atomic.Int32
	receipt := domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: d.DecisionID, AuthorizationRevision: d.AuthorizationRevision, Status: "applied", ExecutionRevision: 2}
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var input runtimeclient.ControlRequest
		if json.Unmarshal(raw, &input) != nil || input.DecisionId != d.DecisionID.String() {
			t.Error("decision identity changed")
		}
		if calls.Add(1) == 1 {
			hijack, ok := w.(http.Hijacker)
			if !ok {
				t.Error("no hijacker")
				return
			}
			conn, _, err := hijack.Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(receipt)
		select {
		case <-done:
		default:
			close(done)
		}
	}))
	defer server.Close()
	client, err := runtimeclient.New(server.URL, runtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); client.DeliverControls(ctx, s, nil) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("control not retried")
	}
	cancel()
	<-stopped
	if calls.Load() != 2 {
		t.Fatalf("delivery count %d", calls.Load())
	}
	// The stable receipt remains acceptable if the client's final acknowledgement was lost.
	if err := s.Acknowledge(t.Context(), a.RootRunID, receipt); err != nil {
		t.Fatal(err)
	}
	_ = bytes.NewBuffer(nil)
}
