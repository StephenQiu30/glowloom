package operation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	pgai "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	aiapp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	aidomain "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	op "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
	project "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/adapter/editproject"
	toolff "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/adapter/ffmpeg"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/domain"
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
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	database := conn.DB
	objects := businessStorageRegistry(t)
	require.NoError(t, objects.Ping(t.Context()))
	f := seedManagedFixture(t, database)
	var actor identity.Principal
	require.NoError(t, database.Raw(`SELECT u.id,u.org_id,u.role,p.owner_scope_id AS scope_id FROM operation.operation o JOIN identity."user" u ON u.id=o.confirmed_by JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?`, f.identity.OperationID).Scan(&actor).Error)
	require.NotEqual(t, uuid.Nil, actor.ID)
	actor = identityfixture.Session(t, database, actor)
	businessStorageEnvironment(t, database)
	publish := pgai.New(database, actor, func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error { return nil })
	price := aidomain.Price{Request: 100000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	m := aidomain.Model{ID: uuid.New(), Name: "pipeline-" + uuid.NewString(), Protocol: "media", Capability: "video.generate", Modes: []string{"text2video"}, Price: price, MaxInput: 8192, MaxOutput: 1, PurchaseCap: 100000, OutputPolicy: "provider_success_v1"}
	require.NoError(t, publish.PublishModel(t.Context(), m))
	require.NoError(t, publish.Grant(t.Context(), actor.ScopeID, "pipeline-funding", 100))
	config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"video"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 60}
	configRaw, _ := json.Marshal(config)
	route := aidomain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "media", BaseURL: config.BaseURL, UpstreamModel: "pipeline", Purchase: price, DriverConfig: configRaw}
	require.NoError(t, publish.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 10, 10000))
	gateway := &aigateway.Runtime{DB: database, Objects: objects, Keys: pipelineKeys{}, Provider: rebuiltProvider(func(context.Context, aiapp.Dispatch, func(json.RawMessage) error) (aidomain.Receipt, error) {
		return aidomain.Receipt{}, aidomain.ErrUnknown
	})}
	model := m.Name
	b := agentwire.NewBoundary(database, nil).WithCanvasRuntime(canvasfixture.CommandRuntime(t)).WithGateway(gateway)
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
	original, err := (aigateway.Creation{Runtime: gateway}).Task(t.Context(), actor, g.Scope.ProjectID, quote.OperationID)
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
	gateway.ResultDownloads = pipelineDownloads{t: t, body: video}
	gateway.Provider = rebuiltProvider(func(_ context.Context, d aiapp.Dispatch, _ func(json.RawMessage) error) (aidomain.Receipt, error) {
		posts.Add(1)
		raw, _ := json.Marshal(op.GenerationResult{Outcome: op.ProviderOutcomeCompleted, State: op.ProviderTaskSucceeded, TaskID: "original-shot", URLs: []string{"https://openrouter.ai/api/v1/videos/original-shot/content?index=0"}})
		cost, e := d.Route.Purchase.Cost(aidomain.Usage{})
		return aidomain.Receipt{Body: raw, PurchaseMicros: cost, UpstreamID: "original-shot"}, e
	})
	for range 2 {
		_, err = (aigateway.Creation{Runtime: gateway}).Execute(t.Context(), taskruntime.Job{BusinessID: quote.OperationID.String()})
		require.NoError(t, err)
	}
	for range 2 {
		require.NoError(t, gateway.Deliver(t.Context(), quote.OperationID))
	}
	require.EqualValues(t, 1, posts.Load())
	detail, err := (aigateway.Creation{Runtime: gateway}).Task(t.Context(), actor, g.Scope.ProjectID, quote.OperationID)
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
	require.NoError(t, database.Raw(`SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'`, quote.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, database.Raw(`SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?`, doc.ID).Scan(&count).Error)
	require.EqualValues(t, 2, count)
	require.EqualValues(t, 1, posts.Load())
}

// The gateway checks frozen URL/credential authority before this read-only fixture.
type pipelineKeys struct{}

func (pipelineKeys) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"fixture-only-not-real"}`), nil
}

type pipelineDownloads struct {
	t    *testing.T
	body []byte
}

func (d pipelineDownloads) Download(_ context.Context, _ string) (*mediaapp.Downloaded, error) {
	f, err := os.CreateTemp(d.t.TempDir(), "gateway-original-*")
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(d.body); err != nil {
		return nil, err
	}
	return &mediaapp.Downloaded{File: f, Size: int64(len(d.body)), MIMEType: "video/mp4", SHA256: aidomain.Digest(d.body)}, nil
}
func (d pipelineDownloads) DownloadAuthenticated(ctx context.Context, url, header string) (*mediaapp.Downloaded, error) {
	require.Equal(d.t, "Bearer fixture-only-not-real", header)
	return d.Download(ctx, url)
}
