// Package agentboundary_test exercises the production Go/Node HTTP boundary.
// Authentication and provider output are controlled fixtures; handlers, the
// independent compiled Node process, Pi Harness and PostgreSQL are real.
package agentboundary_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// fixtureInputs uses the production draft codec while pinning a deterministic
// fixture model; catalog/provider availability is outside this transport test.
type fixtureInputs struct{ scope domain.Scope }

func (f fixtureInputs) Prepare(ctx context.Context, actor identity.Principal, project, session uuid.UUID, raw json.RawMessage, quote *app.QuoteRef, modelKey string) (app.AuthorizedConversationInput, app.ConversationModel, error) {
	if err := ctx.Err(); err != nil {
		return app.AuthorizedConversationInput{}, app.ConversationModel{}, err
	}
	if actor.ID != f.scope.ActorID || actor.OrgID != f.scope.OrgID || actor.ScopeID != f.scope.ScopeID || project != f.scope.ProjectID || session == uuid.Nil || quote != nil || modelKey != "boundary-fixture" {
		return app.AuthorizedConversationInput{}, app.ConversationModel{}, domain.ErrInvalid
	}
	draft, err := prompt.DecodeDraft(raw)
	if err != nil {
		return app.AuthorizedConversationInput{}, app.ConversationModel{}, err
	}
	input, err := prompt.NewConversationInput(draft)
	return app.AuthorizedConversationInput{ConversationInput: input}, app.ConversationModel{ReleaseID: uuid.NewSHA1(project, []byte("boundary-release")), VersionID: uuid.NewSHA1(project, []byte("boundary-version")), ContextWindow: 8192}, err
}
func boundaryDraft(text string) map[string]any {
	return map[string]any{"schemaVersion": 1, "document": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}}, "references": []any{}, "modelVersion": nil, "mode": nil, "params": map[string]any{}, "labelState": map[string]any{"highWater": map[string]any{"image": 0, "video": 0, "audio": 0, "subject": 0}, "allocations": map[string]any{}}}
}

// fixtureAccess deliberately replaces login/project membership only. Store
// privacy, grants, fencing, accounting and receipt persistence remain real.
type fixtureAccess struct {
	scope   domain.Scope
	other   uuid.UUID
	revoked atomic.Bool
}

func (a *fixtureAccess) AuthorizeRequest(_ context.Context, p identity.Principal, project uuid.UUID, _ bool) error {
	if a.revoked.Load() || project != a.scope.ProjectID || p.OrgID != a.scope.OrgID || p.ScopeID != a.scope.ScopeID || (p.ID != a.scope.ActorID && p.ID != a.other) {
		return identity.ErrForbidden
	}
	return nil
}
func (a *fixtureAccess) AuthorizeWorker(ctx context.Context, p identity.Principal, project uuid.UUID) error {
	return a.AuthorizeRequest(ctx, p, project, true)
}
func (a *fixtureAccess) AuthorizeProject(ctx context.Context, p identity.Principal, project uuid.UUID, write bool) error {
	return a.AuthorizeRequest(ctx, p, project, write)
}

type capturedCall struct {
	path  string
	input domain.CallInput
}
type capture struct {
	mu    sync.Mutex
	calls []capturedCall
}

func (c *capture) add(path string, body []byte) {
	var in domain.CallInput
	if json.Unmarshal(body, &in) == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls = append(c.calls, capturedCall{path: path, input: in})
	}
}
func (c *capture) all() []capturedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedCall(nil), c.calls...)
}

func TestNodeProductionHTTPBoundary(t *testing.T) {
	// This suite requires an already provisioned, isolated full-schema database.
	// It never runs schema.sql, truncates tables or changes existing services.
	dsn := requiredDSN(t, "GLOWLOOM_TEST_AGENT_BOUNDARY_DB_DSN")
	nodeDSN := requiredDSN(t, "GLOWLOOM_AGENT_TEST_DB_DSN")
	adminURL, _ := url.Parse(dsn)
	nodeURL, _ := url.Parse(nodeDSN)
	if adminURL.Hostname() != nodeURL.Hostname() || adminURL.Port() != nodeURL.Port() || adminURL.Path != nodeURL.Path || nodeURL.User.Username() != "glowloom_agent" {
		t.Fatal("boundary DSNs must reference the same isolated database; Node must use glowloom_agent")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated boundary PostgreSQL:", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.PingContext(t.Context()); err != nil {
		t.Fatal("boundary PostgreSQL unavailable:", err)
	}
	for _, table := range []string{"agent.service_sessions", "agent.runtime_records", "agent.runtime_events", "agent_business.run_authorization", "agent_business.call_receipt"} {
		var exists bool
		if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&exists).Error; err != nil || !exists {
			t.Fatalf("full current schema required: %s exists=%v error=%v", table, exists, err)
		}
	}

	node := os.Getenv("GLOWLOOM_TEST_AGENT_NODE_BINARY")
	if node == "" {
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatal("Node 24.22.2 is required:", err)
		}
	}
	version, err := exec.CommandContext(t.Context(), node, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "v24.22.2" {
		t.Fatalf("production baseline requires Node v24.22.2, got %q: %v", strings.TrimSpace(string(version)), err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(root, "agent")
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "pnpm", "--dir", agentDir, "build")
	build.Env = append(os.Environ(), "PATH="+filepath.Dir(node)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile current Node production entry: %v\n%s", err, output)
	}

	gin.SetMode(gin.TestMode)
	scope := domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}
	access := &fixtureAccess{scope: scope, other: uuid.New()}
	store := pg.NewStore(db, func(*gorm.DB) app.ControlAccess { return access }, time.Now)
	businessToken, runtimeToken := uuid.NewString()+uuid.NewString(), uuid.NewString()+uuid.NewString()
	var models, tools atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseModel := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseModel)
	const snapshotResult = `{"admitted_runs":1,"source":"agent_business.run_authorization"}`
	capabilities := app.Capabilities{
		Snapshot: func(ctx context.Context, got domain.Scope) (any, error) {
			if got != scope {
				return nil, domain.ErrNotFound
			}
			var count int64
			err := db.WithContext(ctx).Raw(`SELECT count(*) FROM agent_business.run_authorization WHERE scope->>'actor_id'=? AND scope->>'project_id'=?`, scope.ActorID, scope.ProjectID).Scan(&count).Error
			return map[string]any{"project_id": scope.ProjectID, "admitted_runs": count}, err
		},
		Model: func(ctx context.Context, got domain.Scope, _ uuid.UUID, raw json.RawMessage, _ int64) (domain.CallResult, error) {
			if got != scope {
				return domain.CallResult{}, domain.ErrNotFound
			}
			var input struct {
				ModelKey string `json:"model_key"`
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if json.Unmarshal(raw, &input) != nil || input.ModelKey != "boundary-fixture" {
				return domain.CallResult{}, domain.ErrInvalid
			}
			n := models.Add(1)
			if n == 1 {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return domain.CallResult{}, ctx.Err()
				}
				return completed(`{"output":"","tool_calls":[{"id":"boundary-snapshot-1","name":"project_snapshot","arguments":{}}],"usage":{"input_tokens":5,"output_tokens":2},"stop_reason":"tool_use","model_key":"boundary-fixture"}`, 1), nil
			}
			if n != 2 || tools.Load() != 1 {
				return domain.CallResult{}, fmt.Errorf("expected exactly one tool before final: model=%d tools=%d", n, tools.Load())
			}
			found := false
			for _, message := range input.Messages {
				if message.Role == "tool" {
					var content string
					if json.Unmarshal(message.Content, &content) == nil && domain.Hash([]byte(content)) == domain.Hash([]byte(snapshotResult)) {
						found = true
					}
				}
			}
			if !found {
				return domain.CallResult{}, fmt.Errorf("successful formal tool result missing from model transcript")
			}
			return completed(`{"output":"Boundary fixture completed after the committed project snapshot.","usage":{"input_tokens":7,"output_tokens":3},"stop_reason":"stop","model_key":"boundary-fixture"}`, 1), nil
		},
		Tools: []app.Tool{{Descriptor: app.ToolDescriptor{Name: "project_snapshot", Roles: []string{"producer"}, Description: "Read this isolated project's admitted run from PostgreSQL", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: "read", ContractVersion: "v1", Replay: "safe"}, Execute: func(ctx context.Context, got domain.Scope, _ uuid.UUID, raw json.RawMessage) (domain.CallResult, error) {
			if got != scope || domain.Hash(raw) != domain.Hash([]byte(`{}`)) {
				return domain.CallResult{}, domain.ErrInvalid
			}
			var count int64
			if err := db.WithContext(ctx).Raw(`SELECT count(*) FROM agent_business.run_authorization WHERE scope->>'actor_id'=? AND scope->>'project_id'=?`, scope.ActorID, scope.ProjectID).Scan(&count).Error; err != nil {
				return domain.CallResult{}, err
			}
			if count != 1 {
				return domain.CallResult{}, fmt.Errorf("expected one admitted run, got %d", count)
			}
			tools.Add(1)
			// Formal read tools are free; only the model fixture consumes budget.
			return completed(snapshotResult, 0), nil
		}}},
	}
	businessRouter := gin.New()
	agenthttp.NewBusinessHandler(store, capabilities, businessToken).Register(businessRouter)
	captured := &capture{}
	businessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/execute") || strings.HasSuffix(r.URL.Path, "/models/stream") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if r.Header.Get("Authorization") == "Bearer "+businessToken && r.Header.Get("X-Agent-Audience") == "agent-business" {
				captured.add(r.URL.Path, body)
			}
		}
		businessRouter.ServeHTTP(w, r)
	}))
	t.Cleanup(businessServer.Close)
	var sid uuid.UUID
	// Registered before launchNode so the process is reaped before its rows are
	// removed. This also cleans partial executions after an assertion failure.
	t.Cleanup(func() {
		if sid != uuid.Nil {
			cleanupSession(t, db, sid)
		}
	})

	nodeOrigin := launchNode(t, node, agentDir, nodeDSN, businessServer.URL, runtimeToken, businessToken)
	runtime, err := runtimeclient.New(nodeOrigin, runtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	publicRouter := gin.New()
	api := publicRouter.Group("/api", func(c *gin.Context) {
		principal := scope.Principal()
		if c.GetHeader("X-Fixture-Actor") == "other" {
			principal.ID = access.other
		}
		c.Set("principal", principal)
	})
	agenthttp.NewPublicHandler(store, access, runtime).WithConversationInputs(fixtureInputs{scope: scope}).Register(api)
	publicServer := httptest.NewServer(publicRouter)
	t.Cleanup(publicServer.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	base := publicServer.URL + "/api/projects/" + scope.ProjectID.String() + "/agent/sessions"

	t.Run("directional service identities", func(t *testing.T) {
		for _, tc := range []struct{ origin, path, token, audience string }{
			{nodeOrigin, "/internal/agent/sessions?actor_id=" + scope.ActorID.String() + "&project_id=" + scope.ProjectID.String(), "", ""},
			{nodeOrigin, "/internal/agent/sessions?actor_id=" + scope.ActorID.String() + "&project_id=" + scope.ProjectID.String(), businessToken, "agent"},
			{nodeOrigin, "/internal/agent/sessions?actor_id=" + scope.ActorID.String() + "&project_id=" + scope.ProjectID.String(), runtimeToken, "agent-business"},
			{businessServer.URL, "/internal/agent-business/tools", "", ""},
			{businessServer.URL, "/internal/agent-business/tools", runtimeToken, "agent-business"},
			{businessServer.URL, "/internal/agent-business/tools", businessToken, "agent"},
		} {
			requestJSON(t, client, "GET", tc.origin+tc.path, nil, map[string]string{"Authorization": "Bearer " + tc.token, "X-Agent-Audience": tc.audience}, 401)
		}
	})

	create := map[string]any{"request_id": uuid.NewString(), "title": "Cross-process boundary fixture"}
	var session runtimeclient.SessionView
	decode(t, requestJSON(t, client, "POST", base, create, nil, 201), &session)
	sid, err = uuid.Parse(session.ID)
	if err != nil || session.ActorID != scope.ActorID.String() {
		t.Fatal("untrusted session identity:", session.ID, err)
	}
	var again runtimeclient.SessionView
	decode(t, requestJSON(t, client, "POST", base, create, nil, 201), &again)
	if again.ID != session.ID {
		t.Fatal("duplicate session created another identity")
	}
	changed := cloneMap(create)
	changed["title"] = "Changed same request"
	requestJSON(t, client, "POST", base, changed, nil, 409)
	spoof := cloneMap(create)
	spoof["actor_id"] = access.other
	requestJSON(t, client, "POST", base, spoof, nil, 422)
	otherHeader := map[string]string{"X-Fixture-Actor": "other"}
	requestJSON(t, client, "GET", base+"/"+sid.String(), nil, otherHeader, 404)
	requestJSON(t, client, "GET", strings.Replace(base, "/agent/sessions", "/agent-sessions", 1), nil, nil, 404)
	requestJSON(t, client, "GET", base+"/"+sid.String()+"/stream", nil, nil, 404)

	start := map[string]any{"request_id": uuid.NewString(), "expected_execution_revision": 0, "input": boundaryDraft("Read a formal project snapshot, then report its receipt."), "manifest": map[string]any{"model_key": "boundary-fixture", "max_tokens": 256, "max_micros": 10}, "budget_micros": 100}
	var accepted runtimeclient.AcceptedRun
	decode(t, requestJSON(t, client, "POST", base+"/"+sid.String()+"/runs", start, nil, 202), &accepted)
	rid, err := uuid.Parse(accepted.RootRunID)
	if err != nil || rid != domain.RootRunID(sid, uuid.MustParse(start["request_id"].(string))) || accepted.SubmissionID != domain.SubmissionID(rid).String() {
		t.Fatal("cross-language stable run/submission identity mismatch", err)
	}
	runURL := base + "/" + sid.String() + "/runs/" + rid.String()
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		body := requestJSON(t, client, "GET", runURL, nil, nil, 200)
		t.Fatalf("production Harness never reached Go model capability; snapshot=%s", body)
	}
	var repeated runtimeclient.AcceptedRun
	decode(t, requestJSON(t, client, "POST", base+"/"+sid.String()+"/runs", start, nil, 202), &repeated)
	if repeated != accepted {
		t.Fatal("duplicate accepted run changed identity/revision")
	}
	changed = cloneMap(start)
	changed["input"] = boundaryDraft("Changed input under same request")
	requestJSON(t, client, "POST", base+"/"+sid.String()+"/runs", changed, nil, 409)
	requestJSON(t, client, "GET", runURL, nil, otherHeader, 404)
	requestJSON(t, client, "GET", runURL+"/events", nil, otherHeader, 404)
	requestJSON(t, client, "GET", strings.Replace(runURL, scope.ProjectID.String(), uuid.NewString(), 1), nil, nil, 404)

	before := readSnapshot(t, client, runURL)
	if before.SnapshotCursor == "0:0" || before.State.Status != "running" {
		t.Fatalf("expected committed running snapshot before releasing model: %+v", before.State)
	}
	// A real browser HTTP subscriber disconnects while the model request is in
	// flight. Only this subscription is canceled; no Stop command is submitted.
	firstCursor := readSSEFrame(t, client, runURL+"/events", "0:0")
	authority, err := store.CurrentAuthority(t.Context(), rid)
	if err != nil || authority.Terminal || authority.Barrier || authority.Current != nil {
		t.Fatal("browser disconnect changed business authority", authority, err)
	}
	releaseModel()
	final := waitCompleted(t, client, runURL)
	if final.State.ModelSteps == nil || *final.State.ModelSteps != 2 || final.State.ToolSteps == nil || *final.State.ToolSteps != 1 || models.Load() != 2 || tools.Load() != 1 {
		t.Fatalf("expected actual official Harness model/tool/final: state=%+v model=%d tool=%d", final.State, models.Load(), tools.Load())
	}
	if !bytes.Contains(requestJSON(t, client, "GET", runURL, nil, nil, 200), []byte("Boundary fixture completed")) {
		t.Fatal("public run snapshot omitted the committed final answer")
	}
	nextCursor := readSSEFrame(t, client, runURL+"/events", firstCursor)
	if cursorCompare(t, firstCursor, nextCursor) >= 0 {
		t.Fatal("SSE Last-Event-ID did not resume strictly after committed cursor")
	}
	postSnapshot := readSSEFrame(t, client, runURL+"/events", before.SnapshotCursor)
	if cursorCompare(t, before.SnapshotCursor, postSnapshot) >= 0 {
		t.Fatal("snapshot cursor lost subsequent committed model/tool events")
	}
	requestJSON(t, client, "GET", runURL+"/events", nil, map[string]string{"Last-Event-ID": "bad-cursor"}, 400)

	var ledger struct {
		Total, Completed, Models, Tools, Invalid, CostMicros int64
	}
	var recordCount, eventCount int64
	if err := db.Raw(`SELECT count(*) total,
		count(*) FILTER (WHERE status='completed') completed,
		count(*) FILTER (WHERE kind='model' AND name='chat' AND status='completed' AND cost_micros=1) models,
		count(*) FILTER (WHERE kind='tool' AND name='project_snapshot' AND status='completed' AND cost_micros=0) tools,
		count(*) FILTER (WHERE status<>'completed') invalid,
		coalesce(sum(cost_micros),0) cost_micros
		FROM agent_business.call_receipt WHERE root_run_id=?`, rid).Scan(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM agent.runtime_records WHERE session_id=?`, sid.String()).Scan(&recordCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM agent.runtime_events WHERE session_id=? AND payload->>'root_run_id'=?`, sid.String(), rid.String()).Scan(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	authority, err = store.CurrentAuthority(t.Context(), rid)
	if err != nil || ledger.Total != 3 || ledger.Completed != 3 || ledger.Models != 2 || ledger.Tools != 1 || ledger.Invalid != 0 || ledger.CostMicros != 2 || recordCount == 0 || eventCount == 0 || authority.ReservedMicros != ledger.CostMicros {
		var receipts []struct {
			Kind, Name, Status string
			CostMicros         int64
			Result             json.RawMessage
		}
		readErr := db.Raw(`SELECT kind,name,status,cost_micros,result FROM agent_business.call_receipt WHERE root_run_id=? ORDER BY kind,call_id`, rid).Scan(&receipts).Error
		data, _ := json.Marshal(receipts)
		t.Fatalf("real PG persistence/accounting: ledger=%+v runtime_records=%d events=%d reserved=%d error=%v original_receipts=%s read_error=%v", ledger, recordCount, eventCount, authority.ReservedMicros, err, data, readErr)
	}

	// Read/replay the original Go receipts over authenticated HTTP. Replaying an
	// intent returns its original receipt and never invokes a model/tool again.
	serviceHeaders := map[string]string{"Authorization": "Bearer " + businessToken, "X-Agent-Audience": "agent-business"}
	originalCalls := captured.all()
	if len(originalCalls) != 3 {
		t.Fatalf("expected two model and one tool HTTP request, got %d", len(originalCalls))
	}
	for _, original := range originalCalls {
		in := original.input
		query := url.Values{"submission_id": {in.Grant.SubmissionID.String()}, "task_id": {in.Call.TaskID}, "checkpoint_id": {in.Call.CheckpointID}, "attempt": {fmt.Sprint(in.Call.Attempt)}, "contract_version": {in.Call.ContractVersion}}
		path := businessServer.URL + "/internal/agent-business/runs/" + rid.String() + "/calls/" + url.PathEscape(in.Call.CallID) + "?" + query.Encode()
		var saved, replay domain.CallReceipt
		decode(t, requestJSON(t, client, "GET", path, nil, serviceHeaders, 200), &saved)
		decode(t, requestJSON(t, client, "POST", businessServer.URL+original.path, in, serviceHeaders, 200), &replay)
		wantCost := int64(0)
		if saved.Kind == "model" {
			wantCost = 1
		}
		if saved.ReceiptID != replay.ReceiptID || saved.Call != in.Call || saved.Status != "completed" || replay.Status != "completed" || saved.CostMicros != wantCost || replay.CostMicros != wantCost || domain.Hash(saved.Result) != domain.Hash(replay.Result) {
			t.Fatal("original Go receipt changed on HTTP replay")
		}
		in.Input = json.RawMessage(`{"changed":true}`)
		in.Call.InputHash = domain.Hash(in.Input)
		requestJSON(t, client, "POST", businessServer.URL+original.path, in, serviceHeaders, 409)
		in.Call.CallID = "new-with-forged-grant"
		in.Grant.GrantID = uuid.New()
		requestJSON(t, client, "POST", businessServer.URL+original.path, in, serviceHeaders, 403)
	}
	if models.Load() != 2 || tools.Load() != 1 {
		t.Fatal("repeated Go receipt executed a second side effect")
	}
	access.revoked.Store(true)
	requestJSON(t, client, "GET", runURL, nil, nil, 403)
	requestJSON(t, client, "GET", runURL+"/events", nil, nil, 403)
	access.revoked.Store(false)
	t.Logf("compiled production Node + Go HTTP + real PG passed: model=%d tool=%d completed_receipts=%d model_cost=%d tool_cost=0 reserved=%d records=%d events=%d; principal/access and provider outputs are controlled fixtures", models.Load(), tools.Load(), ledger.Completed, ledger.CostMicros, authority.ReservedMicros, recordCount, eventCount)
}

func requiredDSN(t *testing.T, key string) string {
	t.Helper()
	dsn := os.Getenv(key)
	u, err := url.Parse(dsn)
	if err != nil || dsn == "" || u.User == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasSuffix(u.Path, "_test") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatalf("%s must explicitly configure an isolated loopback *_test PostgreSQL database", key)
	}
	return dsn
}

func completed(result string, costMicros int64) domain.CallResult {
	return domain.CallResult{Status: "completed", Result: json.RawMessage(result), CostMicros: costMicros}
}
func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func decode(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode HTTP response: %v: %s", err, raw)
	}
}
func requestJSON(t *testing.T, client *http.Client, method, endpoint string, body any, headers map[string]string, want int) []byte {
	t.Helper()
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, input)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s HTTP request: %v", method, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, req.URL.Path, res.StatusCode, want, raw)
	}
	return raw
}
func readSnapshot(t *testing.T, client *http.Client, endpoint string) runtimeclient.RunSnapshot {
	t.Helper()
	var snapshot runtimeclient.RunSnapshot
	decode(t, requestJSON(t, client, "GET", endpoint, nil, nil, 200), &snapshot)
	return snapshot
}
func waitCompleted(t *testing.T, client *http.Client, endpoint string) runtimeclient.RunSnapshot {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		snapshot := readSnapshot(t, client, endpoint)
		if snapshot.State.Status == "completed" {
			return snapshot
		}
		if snapshot.State.Status != "accepted" && snapshot.State.Status != "running" {
			raw, _ := json.Marshal(snapshot)
			t.Fatalf("production Harness ended without completed result: %s", raw)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out awaiting completed production Harness: %+v", snapshot.State)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
func readSSEFrame(t *testing.T, client *http.Client, endpoint, after string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", after)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal("open public SSE:", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("public SSE status=%d content-type=%s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var cursor, event, data string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			cursor = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && cursor != "":
			if event == "" || !json.Valid([]byte(data)) {
				t.Fatal("invalid committed SSE frame")
			}
			return cursor
		}
	}
	t.Fatalf("public SSE ended without committed event: %v", scanner.Err())
	return ""
}
func cursorCompare(t *testing.T, a, b string) int {
	t.Helper()
	var as, ao, bs, bo int64
	if n, _ := fmt.Sscanf(a, "%d:%d", &as, &ao); n != 2 {
		t.Fatal("invalid cursor:", a)
	}
	if n, _ := fmt.Sscanf(b, "%d:%d", &bs, &bo); n != 2 {
		t.Fatal("invalid cursor:", b)
	}
	if as < bs || (as == bs && ao < bo) {
		return -1
	}
	if as == bs && ao == bo {
		return 0
	}
	return 1
}

type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}
func (l *lockedLog) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
func launchNode(t *testing.T, binary, dir, dsn, businessOrigin, runtimeToken, businessToken string) string {
	t.Helper()
	var listen net.ListenConfig
	listener, err := listen.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	// Cleanup owns graceful termination and joining even after the test context ends.
	processContext, cancelProcess := context.WithCancel(context.WithoutCancel(t.Context()))
	cmd := exec.CommandContext(processContext, binary, "--enable-source-maps", filepath.Join(dir, "dist", "src", "server.js"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GLOWLOOM_AGENT_HTTP_ADDR="+addr, "GLOWLOOM_AGENT_DB_DSN="+dsn, "GLOWLOOM_AGENT_BUSINESS_ORIGIN="+businessOrigin, "GLOWLOOM_AGENT_SERVICE_TOKEN="+runtimeToken, "GLOWLOOM_AGENT_BUSINESS_TOKEN="+businessToken, "GLOWLOOM_AGENT_POOL_MAX=4", "GLOWLOOM_AGENT_LEASE_MS=10000", "GLOWLOOM_LOG_LEVEL=error")
	logs := &lockedLog{}
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		cancelProcess()
		t.Fatal("start compiled production Node:", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		defer cancelProcess()
		checkExit := func(err error) {
			if err == nil {
				return
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() && (status.Signal() == syscall.SIGTERM || status.Signal() == syscall.SIGKILL) {
					return // These signals are explicitly sent by this cleanup.
				}
			}
			t.Error("production Node child exit:", err)
		}
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			checkExit(err)
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			select {
			case err := <-done:
				checkExit(err)
			case <-time.After(2 * time.Second):
				t.Error("Node child did not exit after SIGKILL")
			}
		}
	})
	origin := "http://" + addr
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			done <- err
			safe := logs.text()
			for _, secret := range []string{dsn, runtimeToken, businessToken} {
				safe = strings.ReplaceAll(safe, secret, "[redacted]")
			}
			t.Fatalf("production Node exited before readiness: %v\n%s", err, safe)
		default:
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin+"/readyz", nil)
		if err != nil {
			t.Fatal("build production Node readiness request:", err)
		}
		res, err := client.Do(request)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				return origin
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("compiled production Node did not become ready")
	return ""
}
func cleanupSession(t *testing.T, db *gorm.DB, sid uuid.UUID) {
	t.Helper()
	// The test does not call schema DDL and removes only its random session.
	for _, query := range []string{
		`DELETE FROM agent.public_messages WHERE session_id=?`,
		`DELETE FROM agent.service_control_inbox WHERE root_run_id IN (SELECT id FROM agent.service_runs WHERE session_id=?)`,
		`DELETE FROM agent.service_runs WHERE session_id=?`,
		`DELETE FROM agent.service_sessions WHERE id=?`,
		`DELETE FROM agent.runtime_document_versions WHERE session_id=?`,
		`DELETE FROM agent.runtime_events WHERE session_id=?`,
		`DELETE FROM agent.runtime_records WHERE session_id=?`,
		`DELETE FROM agent.runtime_sessions WHERE session_id=?`,
		`DELETE FROM agent_business.quiesced_receipt WHERE root_run_id IN (SELECT root_run_id FROM agent_business.run_authorization WHERE session_id=?)`,
		`DELETE FROM agent_business.call_receipt WHERE root_run_id IN (SELECT root_run_id FROM agent_business.run_authorization WHERE session_id=?)`,
		`DELETE FROM agent_business.proposal WHERE root_run_id IN (SELECT root_run_id FROM agent_business.run_authorization WHERE session_id=?)`,
		`DELETE FROM agent_business.control_decision WHERE root_run_id IN (SELECT root_run_id FROM agent_business.run_authorization WHERE session_id=?)`,
		`DELETE FROM agent_business.execution_grant WHERE root_run_id IN (SELECT root_run_id FROM agent_business.run_authorization WHERE session_id=?)`,
		`DELETE FROM agent_business.run_authorization WHERE session_id=?`,
	} {
		if err := db.Exec(query, sid.String()).Error; err != nil {
			t.Errorf("remove only this boundary session: %v", err)
		}
	}
}
