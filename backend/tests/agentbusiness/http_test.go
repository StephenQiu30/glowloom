package agentbusiness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const businessToken = "business-only-test-token-00000000000000"
const runtimeToken = "runtime-only-test-token-000000000000000"

func TestHTTPOriginalCallLookupChecksInputHashAndRecoversPendingReceipt(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 50)
	first, err := s.ExecuteCall(t.Context(), "model", "model", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{}, io.ErrUnexpectedEOF
	})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	agenthttp.NewBusinessHandler(s, app.Capabilities{}, businessToken).Register(router)
	for _, tc := range []struct {
		hash   string
		status int
	}{{in.Call.InputHash, 200}, {domain.Hash([]byte("different")), 409}} {
		query := url.Values{"submission_id": {g.SubmissionID.String()}, "task_id": {in.Call.TaskID}, "checkpoint_id": {in.Call.CheckpointID}, "attempt": {"1"}, "contract_version": {"v1"}, "input_hash": {tc.hash}}
		r := httptest.NewRequestWithContext(t.Context(), "GET", "/internal/agent-business/runs/"+a.RootRunID.String()+"/calls/"+in.Call.CallID+"?"+query.Encode(), nil)
		r.Header.Set("Authorization", "Bearer "+businessToken)
		r.Header.Set("X-Agent-Audience", "agent-business")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("original lookup status=%d want=%d: %s", w.Code, tc.status, w.Body.String())
		}
		if w.Code == 200 {
			var got domain.CallReceipt
			if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.ReceiptID != first.ReceiptID || got.Status != "unknown" {
				t.Fatal("original pending identity lost")
			}
		}
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	recoveries := 0
	recovering := gin.New()
	agenthttp.NewBusinessHandler(s, app.Capabilities{ModelRecover: func(ctx context.Context, id uuid.UUID) (domain.CallResult, error) {
		recoveries++
		out := domain.CallResult{Status: "completed", Result: json.RawMessage(`{"original":"recovered"}`), CostMicros: 25}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, e := agentpg.CompleteCall(tx, id, out); return e })
		return out, err
	}}, businessToken).Register(recovering)
	for range 2 {
		query := url.Values{"submission_id": {g.SubmissionID.String()}, "task_id": {in.Call.TaskID}, "checkpoint_id": {in.Call.CheckpointID}, "attempt": {"1"}, "contract_version": {"v1"}, "input_hash": {in.Call.InputHash}}
		r := httptest.NewRequestWithContext(t.Context(), "GET", "/internal/agent-business/runs/"+a.RootRunID.String()+"/calls/"+in.Call.CallID+"?"+query.Encode(), nil)
		r.Header.Set("Authorization", "Bearer "+businessToken)
		r.Header.Set("X-Agent-Audience", "agent-business")
		w := httptest.NewRecorder()
		recovering.ServeHTTP(w, r)
		var got domain.CallReceipt
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.ReceiptID != first.ReceiptID || got.Status != "completed" || got.CostMicros != 25 {
			t.Fatalf("original HTTP recovery failed: %s", w.Body.String())
		}
	}
	if recoveries != 1 {
		t.Fatalf("reconciled original again: %d", recoveries)
	}
}

// This transport retry test must not drain unrelated tests' durable queues.
// Claim selection is covered by authority tests; acknowledgement remains real PG.
type isolatedControlQueue struct {
	app.BusinessStore
	decision     domain.Decision
	acknowledged bool
}

func (q *isolatedControlQueue) ClaimDecision(context.Context, time.Time) (domain.Decision, bool, error) {
	return q.decision, !q.acknowledged, nil
}
func (q *isolatedControlQueue) Acknowledge(ctx context.Context, id uuid.UUID, r domain.AppliedReceipt) error {
	err := q.BusinessStore.Acknowledge(ctx, id, r)
	if err == nil {
		q.acknowledged = true
	}
	return err
}

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
	queue := &isolatedControlQueue{BusinessStore: s, decision: d}
	go func() { defer close(stopped); client.DeliverControls(ctx, queue, nil) }()
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
