package operation_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	catalogpg "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

const harnessServiceToken = "isolated-shot-harness-go-to-node-token"

// The subprocess imports compiled production modules, uses a real restricted PG
// pool and the real BusinessClient. Only supplier HTTP responses are controlled.
const harnessNodeBootstrap = `
import {Pool} from 'pg';
import {SessionService} from './dist/src/sessions/service.js';
import {BusinessClient} from './dist/src/business/client.js';
import {buildApp} from './dist/src/http/app.js';
const pool=new Pool({connectionString:process.env.D1_RUNTIME_DSN,max:4,statement_timeout:10000});
if((await pool.query('SELECT current_user AS role')).rows[0].role!=='glowloom_agent')throw Error('restricted Agent role required');
const service=new SessionService(pool,new BusinessClient(process.env.D1_BUSINESS_ORIGIN,process.env.D1_BUSINESS_TOKEN),{leaseMs:30000,deliveryPollMs:1000,deliveryTimeoutMs:180000,onRecoveryEvent:event=>console.error(JSON.stringify(event))});
const app=await buildApp({service,serviceToken:process.env.D1_SERVICE_TOKEN});
await app.listen(Number(process.env.D1_HTTP_PORT??0),'127.0.0.1');
console.log(app.getHttpServer().address().port);
process.stdin.once('data',()=>service.startRecovery());
process.on('SIGTERM',()=>void app.close().finally(()=>pool.end()).finally(()=>process.exit(0)));
`

type harnessNode struct {
	origin string
	start  func()
	stop   func()
}

func startHarnessNode(t *testing.T, dsn, business string, ports ...string) *harnessNode {
	t.Helper()
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "glowloom_agent", parsed.User.Username())
	require.Contains(t, []string{"127.0.0.1", "localhost"}, parsed.Hostname())
	require.True(t, strings.HasSuffix(parsed.Path, "_test"), "dedicated test runtime database required")
	root, err := filepath.Abs("../../../agent")
	require.NoError(t, err)
	log, err := os.CreateTemp(t.TempDir(), "node-*.log")
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "node", "--input-type=module", "-e", harnessNodeBootstrap)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "D1_RUNTIME_DSN="+dsn, "D1_BUSINESS_ORIGIN="+business, "D1_BUSINESS_TOKEN="+shotBusinessToken, "D1_SERVICE_TOKEN="+harnessServiceToken)
	if len(ports) > 0 {
		cmd.Env = append(cmd.Env, "D1_HTTP_PORT="+ports[0])
	}
	cmd.Stderr = log
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	stop := sync.OnceFunc(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case waitErr := <-finished:
			if waitErr != nil {
				t.Logf("compiled Agent exit: %v", waitErr)
			}
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			waitErr := <-finished
			if waitErr != nil {
				t.Logf("compiled Agent killed: %v", waitErr)
			}
		}
		_ = input.Close()
		require.NoError(t, log.Close())
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			if len(data) > 12000 {
				data = data[len(data)-12000:]
			}
			t.Logf("compiled Agent log: %s", data)
		}
	})
	t.Cleanup(stop)
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			lines <- scanner.Text()
		} else {
			lines <- ""
		}
	}()
	var line string
	select {
	case line = <-lines:
	case <-time.After(60 * time.Second):
		stop()
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("compiled Agent did not start: %s", data)
	}
	port, err := strconv.Atoi(line)
	if err != nil {
		stop()
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("compiled Agent startup failed: %s", data)
	}
	n := &harnessNode{origin: "http://127.0.0.1:" + strconv.Itoa(port), start: sync.OnceFunc(func() { _, err := io.WriteString(input, "ready\n"); require.NoError(t, err) }), stop: stop}
	t.Logf("compiled Agent pid=%d origin=%s log=%s", cmd.Process.Pid, n.origin, log.Name())
	return n
}

type harnessRun struct {
	RootRunID    uuid.UUID `json:"root_run_id"`
	SubmissionID uuid.UUID `json:"submission_id"`
	State        struct {
		Status            string    `json:"status"`
		SegmentID         uuid.UUID `json:"segment_id"`
		ExecutionRevision int64     `json:"execution_revision"`
		Proposal          struct {
			ProposalID            uuid.UUID `json:"proposal_id"`
			CandidateHash         string    `json:"candidate_hash"`
			AuthorizationRevision int64     `json:"authorization_revision"`
		} `json:"proposal"`
		Failure        json.RawMessage `json:"failure"`
		Reconciliation json.RawMessage `json:"reconciliation"`
		DeliveryWait   json.RawMessage `json:"delivery_wait"`
		Production     json.RawMessage `json:"production"`
	} `json:"state"`
}

func harnessRequest(t *testing.T, method, address string, body any, out any) {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, address, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.GreaterOrEqual(t, response.StatusCode, 200, string(raw))
	require.Less(t, response.StatusCode, 300, string(raw))
	if out != nil {
		require.NoError(t, json.Unmarshal(raw, out), string(raw))
	}
}
func waitHarnessRun(t *testing.T, address, status string) harnessRun {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var run harnessRun
	for time.Now().Before(deadline) {
		harnessRequest(t, http.MethodGet, address, nil, &run)
		if run.State.Status == status {
			return run
		}
		if run.State.Status == "failed" || run.State.Status == "needs_reconciliation" || run.State.Status == "stopped" {
			t.Fatalf("wanted %s, run=%+v", status, run)
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("waiting for %s: last run=%+v", status, run)
	return run
}
func approveHarnessRun(t *testing.T, address string, run harnessRun) {
	t.Helper()
	require.NotEqual(t, uuid.Nil, run.State.Proposal.ProposalID)
	harnessRequest(t, http.MethodPost, address+"/commands", agent.DecisionInput{RequestID: uuid.New(), Action: "approve", ProposalID: run.State.Proposal.ProposalID, CandidateHash: run.State.Proposal.CandidateHash, ExpectedAuthorizationRevision: run.State.Proposal.AuthorizationRevision}, nil)
}

// harnessSupplier emits only provider protocol responses; every business tool,
// authorization, operation, wallet entry and output still comes from real Go.
type harnessWriteTracker struct {
	toolapp.WorkObjectsFactory
	attempts map[string]int
}
type harnessTrackedObjects struct {
	toolapp.DepthObjects
	owner *harnessWriteTracker
}

func (f *harnessWriteTracker) Open(ctx context.Context, owner mediaapp.StorageWorkOwner, inputs []mediadomain.MediaAsset) (toolapp.DepthObjects, error) {
	objects, err := f.WorkObjectsFactory.Open(ctx, owner, inputs)
	if err != nil {
		return nil, err
	}
	return &harnessTrackedObjects{DepthObjects: objects, owner: f}, nil
}
func (o *harnessTrackedObjects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, n int64, mime, sha string) error {
	o.owner.attempts[key]++
	return o.DepthObjects.PutIfAbsent(ctx, key, reader, n, mime, sha)
}

type harnessSupplier struct {
	t          *testing.T
	source     domain.CanvasSource
	videoModel string
	video      []byte
	chats      atomic.Int32
	videos     atomic.Int32
}
type harnessChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func harnessToolResult(messages []harnessChatMessage, name string, out any) error {
	names := map[string]string{}
	var raw string
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			names[call.ID] = call.Function.Name
		}
		if message.Role != "tool" || names[message.ToolCallID] != name {
			continue
		}
		if err := json.Unmarshal(message.Content, &raw); err != nil {
			return err
		}
	}
	if raw == "" {
		return fmt.Errorf("missing original tool result %s", name)
	}
	return json.Unmarshal([]byte(raw), out)
}
func (s *harnessSupplier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer fixture-only-not-real" {
		http.Error(w, "fixture credential required", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/chat/completions") {
		s.chat(w, r)
		return
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
		var request struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Prompt != "One blue shot" {
			s.t.Errorf("original provider prompt changed: %+v error=%v", request, err)
			http.Error(w, "invalid original prompt", http.StatusBadRequest)
			return
		}
		s.videos.Add(1)
		_, _ = io.WriteString(w, `{"id":"harness-original-shot","status":"queued"}`)
	case strings.HasSuffix(r.URL.Path, "/content"):
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(s.video)
	default:
		_, _ = io.WriteString(w, `{"id":"harness-original-shot","status":"completed","unsigned_urls":["https://openrouter.ai/api/v1/videos/harness-original-shot/content?index=0"],"usage":{"billable_requests":1,"output_count":1}}`)
	}
}
func (s *harnessSupplier) chat(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Messages []harnessChatMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	step := s.chats.Add(1)
	name := ""
	var args any
	quote := struct {
		OperationID uuid.UUID `json:"operation_id"`
	}{}
	result := func(tool string, out any) bool {
		if err := harnessToolResult(request.Messages, tool, out); err != nil {
			s.t.Errorf("provider step %d: %v", step, err)
			http.Error(w, err.Error(), 400)
			return false
		}
		return true
	}
	switch step {
	case 1:
		name = "operation_quote"
		args = map[string]any{"model_key": s.videoModel, "capability": "video.generate", "mode": "text2video", "prompt": "One blue shot", "params": map[string]any{}, "output_count": 1, "source": s.source}
	case 2, 3, 5, 6, 7:
		if !result("operation_quote", &quote) {
			return
		}
		switch step {
		case 2:
			name = "operation_propose"
			args = map[string]any{"operation_id": quote.OperationID, "expected_canvas_revision": s.source.Revision}
		case 3:
			name = "operation_confirm"
			args = map[string]any{"operation_id": quote.OperationID}
		case 5:
			name = "operation_task"
			args = map[string]any{"operation_id": quote.OperationID}
		case 6, 7:
			var task op.TaskDetail
			if !result("operation_task", &task) {
				return
			}
			if len(task.Outputs) != 1 {
				s.t.Errorf("original output missing: %+v", task)
				http.Error(w, "original output missing", 400)
				return
			}
			name = "shot_delivery_propose"
			if step == 7 {
				name = "shot_delivery_create"
			}
			args = agentwire.ShotDeliveryInput{CanvasID: s.source.CanvasID, NodeID: s.source.NodeID, OperationID: quote.OperationID, OutputID: task.Outputs[0].ID, ExpectedRevision: s.source.Revision, DurationMS: 500, AspectRatio: "1:1", FPS: 25, ProjectFormat: tool.OutputFCPXML}
		}
	case 4, 8:
	default:
		s.t.Errorf("unexpected model send %d", step)
		http.Error(w, "unexpected new model send", 400)
		return
	}
	message := map[string]any{"content": "Waiting for original business work and review."}
	finish := "stop"
	if name != "" {
		encoded, err := json.Marshal(args)
		if err != nil {
			s.t.Error(err)
			http.Error(w, "invalid tool fixture", 500)
			return
		}
		message = map[string]any{"content": nil, "tool_calls": []any{map[string]any{"id": fmt.Sprintf("d1-step-%d", step), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}
		finish = "tool_calls"
	}
	s.t.Logf("controlled model step=%d tool=%s", step, name)
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": message}}, "usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3}})
}

func seedHarnessModel(t *testing.T, database *gorm.DB, video bool) (managedFixture, string) {
	t.Helper()
	f := seedManagedFixture(t, database)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_query=false,supports_cancel=false,moderation='provider' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
	seedFixtureProcurement(t, database, f.identity.PriceRuleVersionID, f.identity.ModelProfileVersionID)
	seedGatewayAvailability(t, database, f, json.RawMessage(`[{"field":"max_tokens","label":"Max tokens","type":"integer","component":"input","min":1,"max":1000}]`))
	if video {
		require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
		require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],supports_query=true,param_schema='[]' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
		require.NoError(t, database.Exec(`UPDATE catalog.model_validation SET mode='text2video' WHERE release_id=?`, f.release).Error)
		require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='video.generate',mode='text2video' WHERE id IN(SELECT operation_id FROM catalog.model_validation WHERE release_id=?)`, f.release).Error)
	}
	var model string
	require.NoError(t, database.Raw(`SELECT model_key FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, f.identity.ModelProfileVersionID).Scan(&model).Error)
	return f, model
}

func TestAgentShotHarnessPublicRunOriginalTasksAndReviewedDelivery(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_SHOT_PIPELINE_DB_DSN")
	runtimeDSN := os.Getenv("GLOWLOOM_AGENT_TEST_DB_DSN")
	if dsn == "" || runtimeDSN == "" {
		t.Skip("isolated Go shot PG, restricted Agent runtime PG and COS19000 required")
	}
	t.Setenv("GLOWLOOM_TEST_OPERATION_STORE_DB_DSN", dsn)
	t.Setenv("TEST_STORAGE_AK", "fixture-access")
	t.Setenv("TEST_STORAGE_SK", "fixture-secret")
	database := operationStoreDB(t).WithContext(t.Context())
	var savedRoute struct{ Revision, Bucket, Endpoint string }
	require.NoError(t, database.Raw(`SELECT r.revision,p.bucket,p.endpoint FROM media.storage_route r JOIN media.storage_profile p ON p.id=r.profile_id`).Scan(&savedRoute).Error)
	bucket := "glowloom-shot-1250000000"
	revision := uint64(time.Now().UnixNano())
	if savedRoute.Revision != "" {
		require.Equal(t, "http://127.0.0.1:19000", savedRoute.Endpoint, "do not replace an unrelated storage identity")
		bucket = savedRoute.Bucket
		old, err := strconv.ParseUint(savedRoute.Revision, 10, 64)
		require.NoError(t, err)
		revision = max(revision, old+1)
	}
	storageConfig := storagefixture.Configuration(t, "http://127.0.0.1:19000", bucket)
	storageConfig.Revision = strconv.FormatUint(revision, 10)
	cfg, err := json.Marshal(storageConfig)
	require.NoError(t, err)
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", string(cfg))
	objects := businessStorageRegistry(t)
	require.NoError(t, objects.Ping(t.Context()))
	textFixture, textModel := seedHarnessModel(t, database, false)
	f, videoModel := seedHarnessModel(t, database, true)
	fundOperationProject(t, database, f.identity.ProjectID)
	require.NoError(t, database.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, f.identity.ProjectID).Error)
	require.NoError(t, database.Exec(`UPDATE billing.budget SET limit_micros=20000000 WHERE project_id=?`, f.identity.ProjectID).Error)
	var actor identity.Principal
	require.NoError(t, database.Raw(`SELECT u.id,u.org_id,u.role,p.owner_scope_id AS scope_id FROM operation.operation o JOIN identity."user" u ON u.id=o.confirmed_by JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?`, f.identity.OperationID).Scan(&actor).Error)
	actor = identityfixture.Session(t, database, actor)
	businessStorageEnvironment(t, database)
	canvasRuntime := canvasfixture.CommandRuntime(t)
	b := agentwire.NewBoundary(database, nil).WithCanvasRuntime(canvasRuntime)
	prep := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: agent.Hash([]byte("harness source fixture")), Scope: agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: f.identity.ProjectID}, BudgetMicros: 10000000}
	require.NoError(t, b.Store.AdmitRun(t.Context(), prep))
	grant, err := b.Store.RegisterGrant(t.Context(), agent.GrantRequest{RootRunID: prep.RootRunID, SubmissionID: prep.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	bootstrap := gin.New()
	b.BusinessHandler(shotBusinessToken).Register(bootstrap)
	created := shotTool(t, b, bootstrap, grant, "canvas_create", "harness-canvas", map[string]any{"name": "Compiled Harness source", "kind": "short_drama"})
	var doc canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &doc))
	text := "One blue shot"
	sourceNode := canvasdomain.Node{ID: uuid.New(), Title: "Shot", NodeType: canvasdomain.NodeTypeText, NodeAction: canvasdomain.NodeActionResource, Config: canvasdomain.NodeConfig{Text: &text}}
	saved := shotTool(t, b, bootstrap, grant, "canvas_commands", "harness-source", agentwire.CanvasCommandsInput{CanvasID: doc.ID, ExpectedRevision: doc.Revision, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &sourceNode}}})
	var changed struct {
		Revision canvas.Revision `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(saved.Result, &changed))
	source := domain.CanvasSource{CanvasID: doc.ID, NodeID: sourceNode.ID, Revision: int64(changed.Revision)}
	file := filepath.Join(t.TempDir(), "supplier.mp4")
	out, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=25", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", file).CombinedOutput()
	require.NoError(t, err, string(out))
	video, err := os.ReadFile(file)
	require.NoError(t, err)
	supplier := &harnessSupplier{t: t, source: source, videoModel: videoModel, video: video}
	remote := httptest.NewTLSServer(supplier)
	defer remote.Close()
	target, err := url.Parse(remote.URL)
	require.NoError(t, err)
	originalTransport := http.DefaultTransport
	route := editorWireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "openrouter.ai" {
			if r.URL.Hostname() != "127.0.0.1" && r.URL.Hostname() != "localhost" {
				return nil, fmt.Errorf("fixture blocks external host %s", r.URL.Hostname())
			}
			return originalTransport.RoundTrip(r)
		}
		forward := r.Clone(r.Context())
		address := *r.URL
		address.Scheme = target.Scheme
		address.Host = target.Host
		forward.URL = &address
		return remote.Client().Transport.RoundTrip(forward)
	})
	operations := operationpg.NewScopedStore(database, operationWorkerAccess, operationWorkerAccess, operationPlatformAccess).WithExecutionAvailability(func(tx *gorm.DB) op.ExecutionAvailability { return catalogpg.NewStore(tx) })
	bills := billingpg.NewScopedStore(database, func(tx *gorm.DB) billing.ProjectAccess { return operationWorkerAccess(tx).(billing.ProjectAccess) })
	gateway := modelgateway.New(database, operations, bills, fixtureUnsealer{}, &http.Client{Transport: route})
	router := gin.New()
	server := httptest.NewUnstartedServer(router)
	businessOrigin := "http://" + server.Listener.Addr().String()
	node := startHarnessNode(t, runtimeDSN, businessOrigin)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	protected := router.Group("/api", func(c *gin.Context) { c.Set("principal", actor) })
	require.NoError(t, app.RegisterAgentBoundary(ctx, router, protected, database, app.AgentBoundaryOptions{Origin: node.origin, ServiceToken: harnessServiceToken, BusinessToken: shotBusinessToken, Model: gateway.Model, ModelRecover: gateway.Recover, CanvasRuntime: canvasRuntime}, nil))
	// RuntimeClient must clone the concrete default Transport before this remap.
	http.DefaultTransport = route
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	server.Start()
	defer server.Close()
	defer node.stop()
	node.start()
	prefix := server.URL + "/api/projects/" + f.identity.ProjectID.String() + "/agent/sessions"
	var session struct {
		ID                uuid.UUID `json:"id"`
		ExecutionRevision int64     `json:"execution_revision"`
	}
	harnessRequest(t, http.MethodPost, prefix, map[string]any{"request_id": uuid.New(), "title": "Actual compiled Harness shot", "canvas_id": doc.ID}, &session)
	readRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, node.origin+"/internal/agent/sessions/"+session.ID.String()+"?actor_id="+actor.ID.String()+"&project_id="+f.identity.ProjectID.String(), nil)
	require.NoError(t, err)
	readRequest.Header.Set("Authorization", "Bearer "+harnessServiceToken)
	readRequest.Header.Set("X-Agent-Audience", "agent")
	readResponse, err := (&http.Client{Timeout: 20 * time.Second}).Do(readRequest)
	require.NoError(t, err)
	readBytes, err := io.ReadAll(readResponse.Body)
	require.NoError(t, err)
	require.NoError(t, readResponse.Body.Close())
	t.Logf("real Node private session read: status=%d body=%s", readResponse.StatusCode, readBytes)
	var realSession runtimeclient.SessionView
	require.NoError(t, json.Unmarshal(readBytes, &realSession))
	requestID := uuid.New()
	var accepted struct {
		RootRunID    uuid.UUID `json:"root_run_id"`
		SubmissionID uuid.UUID `json:"submission_id"`
	}
	input := promptdomain.Draft{SchemaVersion: 1, Document: promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph", Content: []promptdomain.Inline{{Type: "text", Text: text}}}}}, References: []promptdomain.Reference{}, Params: map[string]any{}, LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]promptdomain.LabelAllocation{}}}
	require.NoError(t, input.Validate())
	// This is the same owning freezer used by PublicHandler.Start, checked here
	// only to diagnose fixture readiness before entering the cross-service run.
	rawDraft, err := json.Marshal(input)
	require.NoError(t, err)
	_, _, inputErr := b.ConversationInputs(nil).Prepare(t.Context(), actor, f.identity.ProjectID, session.ID, rawDraft, nil, textModel)
	if inputErr != nil {
		models, err := catalogapp.NewListModelsQuery(app.NewCatalogStore(database)).Execute(t.Context(), actor, catalogapp.ListModelsInput{ProjectID: f.identity.ProjectID, ModelKey: textModel, Capability: "text.generate", Limit: 2})
		t.Logf("model fixture catalog: %+v error=%v", models, err)
	}
	require.NoError(t, inputErr, "real public conversation fixture must be ready")
	runBody := map[string]any{"request_id": requestID, "expected_execution_revision": session.ExecutionRevision, "input": input, "manifest": map[string]any{"model_key": textModel, "params": map[string]any{}, "max_tokens": 1000, "max_micros": 1000000}, "budget_micros": 10000000, "goal_contract": agent.GoalContract{Kind: agent.GoalProduction, RequiredDeliveries: 1, Source: &source, Delivery: &agent.GoalDeliverySpec{DurationMS: 500, AspectRatio: "1:1", FPS: 25, ProjectFormat: tool.OutputFCPXML}}}
	// Fence only this test's exact durable root after any failure, even if the
	// forward to Node failed after Go admission. Keep its original ledger intact.
	expectedRoot := agent.RootRunID(session.ID, requestID)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		current, err := b.Store.CurrentAuthority(cleanupCtx, expectedRoot)
		if errors.Is(err, agent.ErrNotFound) {
			return
		}
		if err != nil {
			t.Error("read failed fixture root for Stop", err)
			return
		}
		if current.Terminal {
			return
		}
		_, err = b.Store.Decide(cleanupCtx, actor, f.identity.ProjectID, expectedRoot, agent.DecisionInput{RequestID: uuid.NewSHA1(expectedRoot, []byte("failed-harness-fixture-stop")), Action: "stop", ExpectedAuthorizationRevision: current.AuthorizationRevision})
		if err != nil {
			t.Error("fence failed fixture root", err)
		} else {
			t.Logf("formally stopped failed fixture root %s; original funds retained", expectedRoot)
		}
	})
	harnessRequest(t, http.MethodPost, prefix+"/"+session.ID.String()+"/runs", runBody, &accepted)
	require.Equal(t, agent.RootRunID(session.ID, requestID), accepted.RootRunID)
	address := prefix + "/" + session.ID.String() + "/runs/" + accepted.RootRunID.String()
	approveHarnessRun(t, address, waitHarnessRun(t, address, "waiting_approval"))
	waiting := waitHarnessRun(t, address, "waiting_delivery")
	require.Equal(t, accepted.RootRunID, waiting.RootRunID)
	require.EqualValues(t, 4, supplier.chats.Load())
	var original struct {
		ID     uuid.UUID
		Status domain.Status
	}
	require.NoError(t, database.Raw(`SELECT o.id,o.status FROM agent_business.operation_commitment link JOIN agent_business.call_receipt c ON c.receipt_id=link.receipt_id JOIN operation.operation o ON o.id=link.operation_id WHERE c.root_run_id=? AND c.kind=?`, accepted.RootRunID, agent.CallKindTool).Scan(&original).Error)
	require.NotEqual(t, uuid.Nil, original.ID)
	require.Equal(t, domain.StatusConfirmed, original.Status)
	originalOperation, err := operationStore(database).FindOperation(t.Context(), actor, f.identity.ProjectID, original.ID)
	require.NoError(t, err)
	require.Equal(t, &f.identity.ModelProfileVersionID, originalOperation.ModelProfileVersionID)
	authority, err := b.Store.CurrentAuthority(t.Context(), accepted.RootRunID)
	require.NoError(t, err)
	var selection struct {
		ModelKey  string    `json:"model_key"`
		ReleaseID uuid.UUID `json:"release_id"`
	}
	require.NoError(t, json.Unmarshal(authority.ModelSelection, &selection))
	require.Equal(t, textModel, selection.ModelKey)
	require.Equal(t, textFixture.release, selection.ReleaseID)
	require.NotEqual(t, textFixture.identity.ModelProfileVersionID, f.identity.ModelProfileVersionID)
	before, err := b.Store.CostSnapshot(t.Context(), accepted.RootRunID)
	require.NoError(t, err)
	// Restart the independent Node while the original task is pending. The frozen
	// production wait is persisted in PG; no BusinessClient result is fabricated.
	node.stop()
	finishHarnessOperation(t, database, objects, original.ID)
	require.EqualValues(t, 1, supplier.videos.Load())
	recoveredCost, err := b.Store.CostSnapshot(t.Context(), accepted.RootRunID)
	require.NoError(t, err)
	require.Equal(t, before.PendingCommitment+before.SettledCharge, recoveredCost.PendingCommitment+recoveredCost.SettledCharge, "worker replay charged the root twice")
	t.Logf("original operation %s finished while Node stopped; chats=%d", original.ID, supplier.chats.Load())
	nodeAddress, err := url.Parse(node.origin)
	require.NoError(t, err)
	restarted := startHarnessNode(t, runtimeDSN, businessOrigin, nodeAddress.Port())
	defer restarted.stop()
	restarted.start()
	resumed := waitHarnessRun(t, address, "waiting_approval")
	require.Equal(t, accepted.RootRunID, resumed.RootRunID)
	require.Equal(t, accepted.SubmissionID, resumed.SubmissionID)
	require.EqualValues(t, 6, supplier.chats.Load(), "original production continuation must read, then propose adoption")
	approveHarnessRun(t, address, resumed)
	waitHarnessRun(t, address, "waiting_delivery")
	require.EqualValues(t, 8, supplier.chats.Load())
	var delivery struct{ Result []byte }
	require.NoError(t, database.Raw(`SELECT result FROM agent_business.call_receipt WHERE root_run_id=? AND name='shot_delivery_create'`, accepted.RootRunID).Scan(&delivery).Error)
	var delivered struct {
		DeliveryID      uuid.UUID `json:"delivery_id"`
		VideoExportID   uuid.UUID `json:"video_export_id"`
		ProjectExportID uuid.UUID `json:"project_export_id"`
	}
	require.NoError(t, json.Unmarshal(delivery.Result, &delivered))
	require.NotEqual(t, uuid.Nil, delivered.DeliveryID)
	detail, err := op.NewPublicQuery(operationStore(database)).Task(t.Context(), actor, f.identity.ProjectID, original.ID)
	require.NoError(t, err)
	require.Len(t, detail.Outputs, 1)
	artifacts := finishHarnessExports(t, database, objects, actor, f.identity.ProjectID, doc.ID, *detail.Outputs[0].MediaAssetID, delivered.VideoExportID, delivered.ProjectExportID, func() {
		var pending harnessRun
		harnessRequest(t, http.MethodGet, address, nil, &pending)
		require.Equal(t, "waiting_delivery", pending.State.Status, "queued output or review_required cannot complete the original goal")
	})
	complete := waitHarnessRun(t, address, "completed")
	require.Equal(t, accepted.RootRunID, complete.RootRunID)
	require.Equal(t, accepted.SubmissionID, complete.SubmissionID)
	var facts struct {
		Goal agent.GoalCompletion `json:"goal_completion"`
	}
	require.NoError(t, json.Unmarshal(complete.State.Production, &facts))
	require.Equal(t, agent.DeliveryCompleted, facts.Goal.Status)
	require.Equal(t, 1, facts.Goal.RequiredDeliveries)
	require.Equal(t, 1, facts.Goal.CompletedDeliveries)
	require.Empty(t, facts.Goal.Missing)
	settled, err := b.Store.CostSnapshot(t.Context(), accepted.RootRunID)
	require.NoError(t, err)
	require.Zero(t, settled.PendingCommitment)
	require.EqualValues(t, 9*100000, settled.SettledCharge)
	boundEvidence := 0
	for _, evidence := range settled.Evidence {
		if evidence.OperationID != nil {
			boundEvidence++
			require.NotNil(t, evidence.SettlementID)
		}
	}
	require.Equal(t, 9, boundEvidence, "eight model settlements and one media settlement retain their original funds IDs")
	var counts struct {
		Settles, Operations, Exports, Decisions, Quiesced int64
		Charge                                            int64
	}
	require.NoError(t, database.Raw(`SELECT
 (SELECT count(*) FROM billing.ledger_entry l JOIN agent_business.operation_commitment link ON link.operation_id=l.operation_id JOIN agent_business.call_receipt c ON c.receipt_id=link.receipt_id WHERE c.root_run_id=? AND l.entry_type='settle') AS settles,
 (SELECT sum(l.amount_micros) FROM billing.ledger_entry l JOIN agent_business.operation_commitment link ON link.operation_id=l.operation_id JOIN agent_business.call_receipt c ON c.receipt_id=link.receipt_id WHERE c.root_run_id=? AND l.entry_type='settle') AS charge,
 (SELECT count(*) FROM agent_business.operation_commitment link JOIN agent_business.call_receipt c ON c.receipt_id=link.receipt_id WHERE c.root_run_id=? AND c.kind=?) AS operations,
 (SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?) AS exports,
 (SELECT count(*) FROM agent_business.control_decision WHERE root_run_id=? AND delivery_state='applied') AS decisions,
 (SELECT count(*) FROM agent_business.quiesced_receipt WHERE root_run_id=?) AS quiesced`, accepted.RootRunID, accepted.RootRunID, accepted.RootRunID, agent.CallKindTool, doc.ID, accepted.RootRunID, accepted.RootRunID).Scan(&counts).Error)
	require.EqualValues(t, 9, counts.Settles)
	require.Equal(t, settled.SettledCharge, counts.Charge)
	require.EqualValues(t, 1, counts.Operations)
	require.EqualValues(t, 2, counts.Exports)
	require.EqualValues(t, 2, counts.Decisions)
	require.EqualValues(t, 2, counts.Quiesced)
	var replay struct {
		RootRunID uuid.UUID `json:"root_run_id"`
	}
	harnessRequest(t, http.MethodPost, prefix+"/"+session.ID.String()+"/runs", runBody, &replay)
	require.Equal(t, accepted.RootRunID, replay.RootRunID)
	afterReplay, err := b.Store.CostSnapshot(t.Context(), accepted.RootRunID)
	require.NoError(t, err)
	require.Equal(t, settled.SettledCharge, afterReplay.SettledCharge)
	require.EqualValues(t, 8, supplier.chats.Load())
	require.EqualValues(t, 1, supplier.videos.Load())
	if directory := os.Getenv("GLOWLOOM_AGENT_SHOT_ARTIFACT_DIR"); directory != "" {
		require.NoError(t, os.MkdirAll(directory, 0700))
		for name, data := range artifacts {
			path := filepath.Join(directory, name)
			require.NoError(t, os.WriteFile(path, data, 0600))
			t.Logf("local controlled-provider full-chain artifact: %s bytes=%d", path, len(data))
		}
	}
	t.Logf("root=%s original operation=%s delivery=%s video=%s project=%s models=%d video_posts=%d settled_micros=%d", accepted.RootRunID, original.ID, delivered.DeliveryID, delivered.VideoExportID, delivered.ProjectExportID, supplier.chats.Load(), supplier.videos.Load(), settled.SettledCharge)
}

func finishHarnessOperation(t *testing.T, database *gorm.DB, objects *objectstorage.Registry, id uuid.UUID) {
	t.Helper()
	store := operationStore(database)
	loaded, err := store.LoadWorkflowOperation(t.Context(), id)
	require.NoError(t, err)
	key := loaded.ProviderRequestKey
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &key})
	require.NoError(t, err)
	require.NoError(t, store.BeginProviderCall(t.Context(), op.BeginProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, DispatchRequired: true}))
	results := op.NewRoutedManagedResults(staging.NewRoutedFactory(objects, pgmedia.NewStorageRoutes(database)), store)
	worker := managed.NewActivities(store, fixtureUnsealer{}, providerhttp.NewClient(&http.Client{}), results, managed.NewVideos())
	progress, err := worker.Step(t.Context(), op.ManagedStep{OperationID: id, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "accepted", progress.State)
	worker = managed.NewActivities(store, fixtureUnsealer{}, providerhttp.NewClient(&http.Client{}), results, managed.NewVideos())
	replay, err := worker.Step(t.Context(), op.ManagedStep{OperationID: id, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, progress, replay)
	taskID := "harness-original-shot"
	require.NoError(t, store.CompleteProviderCall(t.Context(), op.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, ProviderTaskID: &taskID, Outcome: "ok", State: "accepted"}))
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted, ProviderTaskID: &taskID})
	require.NoError(t, err)
	require.NoError(t, store.BeginProviderCall(t.Context(), op.BeginProviderCallInput{OperationID: id, Action: "query", Attempt: 1, ProviderTaskID: &taskID}))
	progress, err = worker.Step(t.Context(), op.ManagedStep{OperationID: id, Action: "query"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	require.NoError(t, store.CompleteProviderCall(t.Context(), op.CompleteProviderCallInput{OperationID: id, Action: "query", Attempt: 1, ProviderTaskID: &taskID, Outcome: "ok", State: "succeeded", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSubmitted}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	ingest := managed.NewRoutedIngestActivities(database, objects, results, app.NewMediaWorkerScopeAccess, store)
	require.NoError(t, ingest.Ingest(t.Context(), id))
	require.NoError(t, ingest.Ingest(t.Context(), id))
	cost, err := store.LoadProviderCost(t.Context(), id)
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(database, store, billingpg.NewStore(database))
	finish := operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusIngesting, To: domain.StatusCompleted, ActualCostMicros: cost.ActualCostMicros}
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
}

func finishHarnessExports(t *testing.T, database *gorm.DB, objects *objectstorage.Registry, actor identity.Principal, pid, canvasID, inputAsset, videoID, projectID uuid.UUID, pending func()) map[string][]byte {
	t.Helper()
	artifacts := map[string][]byte{}
	exports := mediawire.ProvideRoutedMediaExportStore(database, mediawire.NewMediaWorkerScopeAccess)
	ff, err := toolff.NewRenderer(mediaflow.FFProber{})
	require.NoError(t, err)
	renderer := project.NewRenderer(ff)
	for _, id := range []uuid.UUID{videoID, projectID} {
		fault := &shotLostPut{WorkObjectsFactory: mediawire.ToolObjects(database, objects)}
		tracker := &harnessWriteTracker{WorkObjectsFactory: fault, attempts: map[string]int{}}
		worker := toolapp.NewRoutedWorker(exports, tracker, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		work := toolapp.WorkID{JobID: id, Attempt: 1, ExecutionID: uuid.New()}
		_, err = worker.Execute(t.Context(), work)
		require.Error(t, err, "observe original conditional object write acknowledgement loss")
		worker = toolapp.NewRoutedWorker(exports, tracker, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		job, err := worker.Execute(t.Context(), work)
		require.NoError(t, err)
		require.Equal(t, tool.ReviewRequired, job.Status)
		pending()
		reads := mediaobjects.NewRoutedReadObjects(objects)
		query := toolapp.NewRoutedExportQuery(exports, reads, reads, 5*time.Minute)
		// A reviewer verifies actual output bytes before affirming the original job.
		preview, err := query.Preview(t.Context(), actor, pid, id)
		require.NoError(t, err)
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, preview.URL, nil)
		require.NoError(t, err)
		response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		size := int64(len(data))
		require.Equal(t, *job.SHA256, fmt.Sprintf("%x", sha256.Sum256(data)))
		path := filepath.Join(t.TempDir(), "reviewed.mp4")
		if id == projectID {
			path = filepath.Join(t.TempDir(), "reviewed-project.zip")
		}
		require.NoError(t, os.WriteFile(path, data, 0600))
		if id == projectID {
			frozen, err := exports.Frozen(t.Context(), actor, pid, id)
			require.NoError(t, err)
			require.Equal(t, tool.OutputFCPXML, frozen.OutputKind)
			require.Len(t, frozen.Inputs, 1)
			require.Equal(t, inputAsset, frozen.Inputs[0].AssetID)
			require.EqualValues(t, 500, frozen.Timeline.Clips[0].DurationMS)
			file, err := os.Open(path)
			require.NoError(t, err)
			require.NoError(t, project.Verify(t.Context(), file, size, frozen))
			require.NoError(t, file.Close())
		} else {
			output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
			require.NoError(t, err, string(output))
		}
		name := "glowloom-agent-contract-browser.mp4"
		if id == projectID {
			name = "glowloom-agent-contract-browser.zip"
		}
		artifacts[name] = data
		reviewKey := uuid.New()
		reviewInput := toolapp.ReviewInput{ProjectID: pid, Revision: job.Revision, SHA256: *job.SHA256, LocalReviewConfirmed: true}
		reviewed, err := exports.Review(t.Context(), actor, id, reviewKey, reviewInput)
		require.NoError(t, err)
		replayed, err := exports.Review(t.Context(), actor, id, reviewKey, reviewInput)
		require.NoError(t, err)
		require.Equal(t, reviewed.Revision, replayed.Revision)
		changed := reviewInput
		changed.SHA256 = strings.Repeat("b", 64)
		_, err = exports.Review(t.Context(), actor, id, reviewKey, changed)
		require.True(t, errors.Is(err, toolapp.ErrConflict), "same review key cannot change the inspected hash: %v", err)
		attachment, attachmentSize, err := query.Download(t.Context(), actor, pid, id)
		require.NoError(t, err)
		downloaded, err := io.ReadAll(attachment)
		require.NoError(t, err)
		require.NoError(t, attachment.Close())
		require.Equal(t, size, attachmentSize)
		require.Equal(t, data, downloaded)
		require.NotEmpty(t, tracker.attempts)
		for key, count := range tracker.attempts {
			require.Equal(t, 1, count, "original output identity %s must be written once across worker recovery", key)
		}
		require.Equal(t, len(tracker.attempts), fault.writes, "main output and legitimate preview derivatives each have one original write")
	}
	var count int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?`, canvasID).Scan(&count).Error)
	require.EqualValues(t, 2, count)
	return artifacts
}
