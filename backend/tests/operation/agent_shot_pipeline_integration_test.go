package operation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
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
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

const shotBusinessToken = "isolated-shot-business-service-token"

func shotTool(t *testing.T, b *agentwire.Boundary, router http.Handler, g agent.Grant, name, callID string, input any) agent.CallReceipt {
	t.Helper()
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	body, err := json.Marshal(agent.CallInput{Grant: g, Call: agent.CallIdentity{TaskID: "shot-integration", CallID: callID, CheckpointID: "saved", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)}, Input: raw, MaxMicros: 10_000_000})
	require.NoError(t, err)
	r := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/tools/"+name+"/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+shotBusinessToken)
	r.Header.Set("X-Agent-Audience", "agent-business")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		_, cause := b.AtomicTool(c, name, agent.CallInput{Grant: g, Call: agent.CallIdentity{TaskID: "shot-integration", CallID: callID, CheckpointID: "saved", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)}, Input: raw, MaxMicros: 10_000_000})
		t.Logf("tool %s: %v", name, cause)
	}
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var receipt agent.CallReceipt
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	return receipt
}

func shotApprove(t *testing.T, b *agentwire.Boundary, actor identity.Principal, g agent.Grant, receipt agent.CallReceipt) agent.Grant {
	t.Helper()
	var proposal agent.Proposal
	require.NoError(t, json.Unmarshal(receipt.Result, &proposal))
	d, err := b.Store.Decide(t.Context(), actor, g.Scope.ProjectID, g.RootRunID, agent.DecisionInput{RequestID: uuid.New(), Action: "approve", ProposalID: proposal.ProposalID, CandidateHash: proposal.CandidateHash, ExpectedAuthorizationRevision: proposal.AuthorizationRevision})
	require.NoError(t, err)
	g, err = b.Store.Quiesce(t.Context(), g.RootRunID, agent.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: d.DecisionID, AuthorizationRevision: d.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: agent.ContinuationID(g.RootRunID, d.DecisionID), LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	return g
}

// shotLostPut loses only the acknowledgement of a real conditional object
// write. A new worker must recover that original object, not render another one.
type shotLostPut struct {
	toolapp.WorkObjectsFactory
	lost   bool
	writes int
}
type shotLostObjects struct {
	toolapp.DepthObjects
	owner *shotLostPut
}

func (f *shotLostPut) Open(ctx context.Context, owner mediaapp.StorageWorkOwner, inputs []mediadomain.MediaAsset) (toolapp.DepthObjects, error) {
	o, err := f.WorkObjectsFactory.Open(ctx, owner, inputs)
	if err != nil {
		return nil, err
	}
	return &shotLostObjects{o, f}, nil
}
func (o *shotLostObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, n int64, mime, sha string) error {
	err := o.DepthObjects.PutIfAbsent(ctx, key, r, n, mime, sha)
	if err == nil {
		o.owner.writes++
		if !o.owner.lost {
			o.owner.lost = true
			return io.ErrUnexpectedEOF
		}
	}
	return err
}

func TestAgentShotOriginalQuoteRecoveryVideoAndProjectDelivery(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_SHOT_PIPELINE_DB_DSN")
	if dsn == "" {
		t.Skip("dedicated isolated shot-pipeline PostgreSQL database and loopback COS fixture required")
	}
	t.Setenv("GLOWLOOM_TEST_OPERATION_STORE_DB_DSN", dsn)
	t.Setenv("TEST_STORAGE_AK", "fixture-access")
	t.Setenv("TEST_STORAGE_SK", "fixture-secret")
	cfg, err := json.Marshal(storagefixture.Configuration(t, "http://127.0.0.1:19000", "glowloom-shot-1250000000"))
	require.NoError(t, err)
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", string(cfg))
	database := businessEmptyFixtureDB(t, dsn)
	for _, name := range []string{"schema.sql", "data.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "db", name))
		require.NoError(t, err)
		require.NoError(t, database.Exec(string(schema)).Error)
	}
	objects := businessStorageRegistry(t)
	require.NoError(t, objects.Ping(t.Context()))
	f := seedManagedFixture(t, database)
	// Real readiness and commercial ledgers, using explicitly synthetic catalog
	// evidence and credentials; no live supplier or paid account is contacted.
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_query=false,supports_cancel=false,moderation='provider' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
	seedFixtureProcurement(t, database, f.identity.PriceRuleVersionID, f.identity.ModelProfileVersionID)
	seedGatewayAvailability(t, database, f, json.RawMessage(`[]`))
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],supports_query=true WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_validation SET mode='text2video' WHERE release_id=?`, f.release).Error)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='video.generate',mode='text2video' WHERE id IN (SELECT operation_id FROM catalog.model_validation WHERE release_id=?)`, f.release).Error)
	fundOperationProject(t, database, f.identity.ProjectID)
	require.NoError(t, database.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, f.identity.ProjectID).Error)
	require.NoError(t, database.Exec(`UPDATE billing.budget SET limit_micros=20000000 WHERE project_id=?`, f.identity.ProjectID).Error)
	var actor identity.Principal
	require.NoError(t, database.Raw(`SELECT u.id,u.org_id,u.role,p.owner_scope_id AS scope_id FROM operation.operation o JOIN identity."user" u ON u.id=o.confirmed_by JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?`, f.identity.OperationID).Scan(&actor).Error)
	require.NotEqual(t, uuid.Nil, actor.ID)
	actor = identityfixture.Session(t, database, actor)
	var model string
	require.NoError(t, database.Raw(`SELECT m.model_key FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, f.identity.ModelProfileVersionID).Scan(&model).Error)
	businessStorageEnvironment(t, database)
	b := agentwire.NewBoundary(database, nil).WithCanvasRuntime(canvasfixture.CommandRuntime(t))
	a := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: agent.Hash([]byte("original shot")), Scope: agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: f.identity.ProjectID}, BudgetMicros: 10_000_000}
	require.NoError(t, b.Store.AdmitRun(t.Context(), a))
	g, err := b.Store.RegisterGrant(t.Context(), agent.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	b.BusinessHandler(shotBusinessToken).Register(router)
	created := shotTool(t, b, router, g, "canvas_create", "canvas", map[string]any{"name": "Original shot", "kind": "short_drama"})
	var doc canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &doc))
	text := "One blue shot"
	node := canvasdomain.Node{ID: uuid.New(), Title: "Shot", NodeType: canvasdomain.NodeTypeText, NodeAction: canvasdomain.NodeActionResource, Config: canvasdomain.NodeConfig{Text: &text}}
	changed := shotTool(t, b, router, g, "canvas_commands", "source", agentwire.CanvasCommandsInput{CanvasID: doc.ID, ExpectedRevision: doc.Revision, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &node}}})
	var saved struct {
		Revision canvas.Revision `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(changed.Result, &saved))
	source := domain.CanvasSource{CanvasID: doc.ID, NodeID: node.ID, Revision: int64(saved.Revision)}
	quoted := shotTool(t, b, router, g, "operation_quote", "quote", map[string]any{"model_key": model, "capability": "video.generate", "mode": "text2video", "prompt": text, "params": map[string]any{}, "output_count": 1, "source": source})
	var quote struct {
		OperationID uuid.UUID `json:"operation_id"`
	}
	require.NoError(t, json.Unmarshal(quoted.Result, &quote))
	original, err := operationStore(database).FindOperation(t.Context(), actor, g.Scope.ProjectID, quote.OperationID)
	require.NoError(t, err)
	require.Equal(t, &source, original.Source, "owning query must preserve immutable canvas provenance")
	proposal := shotTool(t, b, router, g, "operation_propose", "generation-proposal", map[string]any{"operation_id": quote.OperationID, "expected_canvas_revision": source.Revision})
	g = shotApprove(t, b, actor, g, proposal)
	shotTool(t, b, router, g, "operation_confirm", "confirm", map[string]any{"operation_id": quote.OperationID})
	file := filepath.Join(t.TempDir(), "source.mp4")
	out, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=25", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", file).CombinedOutput()
	require.NoError(t, err, string(out))
	video, err := os.ReadFile(file)
	require.NoError(t, err)
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer fixture-only-not-real", r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			posts.Add(1)
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, text, payload["prompt"])
			_, _ = io.WriteString(w, `{"id":"original-shot","status":"queued"}`)
		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(video)
		default:
			_, _ = io.WriteString(w, `{"id":"original-shot","status":"completed","unsigned_urls":["https://openrouter.ai/api/v1/videos/original-shot/content?index=0"],"usage":{"billable_requests":1,"output_count":1}}`)
		}
	}))
	defer server.Close()
	local, err := url.Parse(server.URL)
	require.NoError(t, err)
	originalTransport := http.DefaultTransport
	http.DefaultTransport = editorWireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "openrouter.ai" {
			return originalTransport.RoundTrip(r)
		}
		forward := r.Clone(r.Context())
		target := *r.URL
		target.Scheme = local.Scheme
		target.Host = local.Host
		forward.URL = &target
		return server.Client().Transport.RoundTrip(forward)
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	s := operationStore(database)
	loaded, err := s.LoadWorkflowOperation(t.Context(), quote.OperationID)
	require.NoError(t, err)
	requestKey := loaded.ProviderRequestKey
	require.NotEmpty(t, requestKey)
	_, err = s.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: quote.OperationID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey})
	require.NoError(t, err)
	require.NoError(t, s.BeginProviderCall(t.Context(), op.BeginProviderCallInput{OperationID: quote.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true}))
	var pinned int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=?`, g.Scope.ProjectID, quote.OperationID).Scan(&pinned).Error)
	require.EqualValues(t, 1, pinned, "formal Agent quote must atomically pin output storage before dispatch")
	results := op.NewRoutedManagedResults(staging.NewRoutedFactory(objects, pgmedia.NewStorageRoutes(database)), s)
	worker := managed.NewActivities(s, fixtureUnsealer{}, providerhttp.NewClient(&http.Client{}), results, managed.NewVideos())
	progress, err := worker.Step(t.Context(), op.ManagedStep{OperationID: quote.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "accepted", progress.State)
	// Discard the caller's reply and reconstruct the worker from persisted facts.
	worker = managed.NewActivities(s, fixtureUnsealer{}, providerhttp.NewClient(&http.Client{}), results, managed.NewVideos())
	replay, err := worker.Step(t.Context(), op.ManagedStep{OperationID: quote.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, progress, replay)
	require.EqualValues(t, 1, posts.Load())
	taskID := "original-shot"
	require.NoError(t, s.CompleteProviderCall(t.Context(), op.CompleteProviderCallInput{OperationID: quote.OperationID, Action: "submit", Attempt: 1, ProviderTaskID: &taskID, Outcome: "ok", State: "accepted"}))
	_, err = s.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: quote.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted, ProviderTaskID: &taskID})
	require.NoError(t, err)
	require.NoError(t, s.BeginProviderCall(t.Context(), op.BeginProviderCallInput{OperationID: quote.OperationID, Action: "query", Attempt: 1, ProviderTaskID: &taskID}))
	progress, err = worker.Step(t.Context(), op.ManagedStep{OperationID: quote.OperationID, Action: "query"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	require.NoError(t, s.CompleteProviderCall(t.Context(), op.CompleteProviderCallInput{OperationID: quote.OperationID, Action: "query", Attempt: 1, ProviderTaskID: &taskID, Outcome: "ok", State: "succeeded", Managed: true}))
	_, err = s.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: quote.OperationID, From: []domain.Status{domain.StatusSubmitted}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = s.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: quote.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	ingest := managed.NewRoutedIngestActivities(database, objects, results, app.NewMediaWorkerScopeAccess, s)
	require.NoError(t, ingest.Ingest(t.Context(), quote.OperationID))
	require.NoError(t, ingest.Ingest(t.Context(), quote.OperationID))
	cost, err := s.LoadProviderCost(t.Context(), quote.OperationID)
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(database, s, billingpg.NewStore(database))
	finish := operationflow.SettlementInput{OperationID: quote.OperationID.String(), From: domain.StatusIngesting, To: domain.StatusCompleted, ActualCostMicros: cost.ActualCostMicros}
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
	detail, err := op.NewPublicQuery(s).Task(t.Context(), actor, g.Scope.ProjectID, quote.OperationID)
	require.NoError(t, err)
	require.Len(t, detail.Outputs, 1)
	input := agentwire.ShotDeliveryInput{CanvasID: doc.ID, NodeID: node.ID, OperationID: quote.OperationID, OutputID: detail.Outputs[0].ID, ExpectedRevision: source.Revision, DurationMS: 500, AspectRatio: "1:1", FPS: 25, ProjectFormat: tool.OutputFCPXML}
	g = shotApprove(t, b, actor, g, shotTool(t, b, router, g, "shot_delivery_propose", "adoption-proposal", input))
	delivery := shotTool(t, b, router, g, "shot_delivery_create", "delivery", input)
	var delivered struct {
		DeliveryID      uuid.UUID `json:"delivery_id"`
		VideoExportID   uuid.UUID `json:"video_export_id"`
		ProjectExportID uuid.UUID `json:"project_export_id"`
	}
	require.NoError(t, json.Unmarshal(delivery.Result, &delivered))
	exports := mediawire.ProvideRoutedMediaExportStore(database, mediawire.NewMediaWorkerScopeAccess)
	ff, err := toolff.NewRenderer(mediaflow.FFProber{})
	require.NoError(t, err)
	renderer := project.NewRenderer(ff)
	for _, id := range []uuid.UUID{delivered.VideoExportID, delivered.ProjectExportID} {
		fault := &shotLostPut{WorkObjectsFactory: mediawire.ToolObjects(database, objects)}
		w := toolapp.NewRoutedWorker(exports, fault, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		work := toolapp.WorkID{JobID: id, Attempt: 1, ExecutionID: uuid.New()}
		_, err = w.Execute(t.Context(), work)
		require.Error(t, err, "must observe real object-write acknowledgement loss")
		w = toolapp.NewRoutedWorker(exports, fault, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		job, err := w.Execute(t.Context(), work)
		require.NoError(t, err)
		require.Equal(t, tool.ReviewRequired, job.Status)
		reads := shotTool(t, b, router, g, "shot_delivery_get", "pending-"+id.String(), map[string]any{"delivery_id": delivered.DeliveryID})
		var live struct {
			Status agent.DeliveryStatus `json:"status"`
		}
		require.NoError(t, json.Unmarshal(reads.Result, &live))
		require.NotEqual(t, agent.DeliveryCompleted, live.Status)
		_, err = exports.Review(t.Context(), actor, id, uuid.New(), toolapp.ReviewInput{ProjectID: g.Scope.ProjectID, Revision: job.Revision, SHA256: *job.SHA256, LocalReviewConfirmed: true})
		require.NoError(t, err)
		readObjects := mediaobjects.NewRoutedReadObjects(objects)
		query := toolapp.NewRoutedExportQuery(exports, readObjects, readObjects, 5*time.Minute)
		r, size, err := query.Download(t.Context(), actor, g.Scope.ProjectID, id)
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		require.EqualValues(t, len(data), size)
		if id == delivered.ProjectExportID {
			frozen, err := exports.Frozen(t.Context(), actor, g.Scope.ProjectID, id)
			require.NoError(t, err)
			require.Equal(t, tool.OutputFCPXML, frozen.OutputKind)
			require.Len(t, frozen.Inputs, 1)
			require.Equal(t, *detail.Outputs[0].MediaAssetID, frozen.Inputs[0].AssetID)
			require.EqualValues(t, 500, frozen.Timeline.Clips[0].DurationMS)
			path := filepath.Join(t.TempDir(), "delivered-project.zip")
			require.NoError(t, os.WriteFile(path, data, 0600))
			file, err := os.Open(path)
			require.NoError(t, err)
			require.NoError(t, project.Verify(t.Context(), file, size, frozen))
			require.NoError(t, file.Close())
		} else {
			path := filepath.Join(t.TempDir(), "delivered.mp4")
			require.NoError(t, os.WriteFile(path, data, 0600))
			out, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
			require.NoError(t, err, string(out))
		}
	}
	live := shotTool(t, b, router, g, "shot_delivery_get", "complete", map[string]any{"delivery_id": delivered.DeliveryID})
	var completed struct {
		Status agent.DeliveryStatus `json:"status"`
	}
	require.NoError(t, json.Unmarshal(live.Result, &completed))
	require.Equal(t, agent.DeliveryCompleted, completed.Status)
	produced, err := exports.Get(t.Context(), actor, g.Scope.ProjectID, delivered.VideoExportID)
	require.NoError(t, err)
	require.NotNil(t, produced.AssetID)
	require.NoError(t, database.Exec(`UPDATE media.media_asset SET is_delete=true WHERE id=?`, *produced.AssetID).Error)
	missing := shotTool(t, b, router, g, "shot_delivery_get", "output-deleted", map[string]any{"delivery_id": delivered.DeliveryID})
	require.NoError(t, json.Unmarshal(missing.Result, &completed))
	require.Equal(t, agent.DeliveryIncomplete, completed.Status, "succeeded rows cannot stand in for unavailable actual artifacts")
	shotTool(t, b, router, g, "shot_delivery_create", "delivery-retry", input)
	var count int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='settle'`, quote.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, database.Raw(`SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?`, doc.ID).Scan(&count).Error)
	require.EqualValues(t, 2, count)
	require.EqualValues(t, 1, posts.Load())
}
