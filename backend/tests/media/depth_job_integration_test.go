package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolhttp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

type depthHTTPFixture struct {
	db              *gorm.DB
	store           *toolpg.DepthStore
	objects         *objectstorage.Client
	actor           identityapp.Principal
	project, source uuid.UUID
	input           toolapp.DepthCreateInput
	router          *gin.Engine
}

func depthStore(db *gorm.DB, enabled bool) *toolpg.DepthStore {
	var source toolpg.TranscriptionSourceFactory
	if enabled {
		source = func(tx *gorm.DB) toolapp.TranscriptionSourceReader {
			return canvasapp.NewTranscriptionSourceReader(canvaspg.NewStore(tx, func(t *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(t), nil) }))
		}
	}
	return toolpg.NewDepthStore(db, source, func(tx *gorm.DB) toolapp.DepthDerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
}

func depthHTTP(t *testing.T) *depthHTTPFixture {
	t.Helper()
	db := mediaStoreDB(t).WithContext(t.Context())
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	input := depthTestVideo(t, "30", "2")
	upload := mediaapp.NewUploadService(mediapg.NewStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "source.mp4"}, File: input, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual private video upload", err)
	}
	node := transcriptionNode(t, db, actor, project, result.Asset.ID, "video")
	f := &depthHTTPFixture{db: db, store: depthStore(db, true), objects: objects, actor: actor, project: project, source: result.Asset.ID, input: toolapp.DepthCreateInput{CanvasID: node.CanvasID, NodeID: node.NodeID, Revision: node.Revision}, router: gin.New()}
	f.router.Use(httpapi.Middleware("http://localhost:3000"))
	g := f.router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", f.actor); c.Next() })
	toolhttp.NewDepthHandler(f.store, toolapp.NewDepthQuery(f.store, objects, objects)).Register(g)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var keys []string
		if err := db.WithContext(ctx).Raw(`SELECT object_key FROM media.media_asset WHERE project_id=? UNION SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=? UNION SELECT o.object_key FROM mediatool.depth_object o JOIN mediatool.depth_job j ON j.id=o.job_id WHERE j.project_id=?`, project, project, project).Scan(&keys).Error; err != nil {
			t.Error(err)
			return
		}
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("cleanup exact task depth object", err)
			}
		}
	})
	return f
}

func (f *depthHTTPFixture) request(t *testing.T, method, url string, key uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	if key != uuid.Nil {
		req.Header.Set("Idempotency-Key", key.String())
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}
func (f *depthHTTPFixture) create(t *testing.T) tooldomain.DepthJob {
	t.Helper()
	rec := f.request(t, "POST", "/api/projects/"+f.project.String()+"/media-depths", uuid.New(), f.input)
	var j tooldomain.DepthJob
	if rec.Code != 202 || json.Unmarshal(rec.Body.Bytes(), &j) != nil {
		t.Fatalf("depth admission %d %s", rec.Code, rec.Body.String())
	}
	return j
}
func (f *depthHTTPFixture) worker(t *testing.T, p toolapp.DepthProcessor, o toolapp.DepthObjects) *toolapp.DepthWorker {
	t.Helper()
	check, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	return toolapp.NewDepthWorker(f.store, o, p, mediaflow.FFProber{}, mediaflow.FFRenderer{}, check)
}

func TestDepthJobActualNativeHTTPPrivateReviewCanvas(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(2 * time.Minute)
	a := toolflow.NewDepthActivities(f.worker(t, native, toolflow.NewObjects(f.objects)), f.store)
	env.RegisterActivityWithOptions(a.ProcessDepth, activity.RegisterOptions{Name: toolflow.DepthActivity})
	env.RegisterActivityWithOptions(a.InterruptDepth, activity.RegisterOptions{Name: toolflow.DepthFailureActivity})
	env.ExecuteWorkflow(toolflow.DepthWorkflow, toolapp.DepthWorkID{JobID: j.ID, Attempt: j.Attempt})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal("real native worker under SDK", err)
	}
	var rendered tooldomain.DepthJob
	if err := env.GetWorkflowResult(&rendered); err != nil || rendered.Status != tooldomain.DepthReviewRequired || rendered.AssetID == nil || rendered.SHA256 == nil {
		t.Fatalf("actual result %+v %v", rendered, err)
	}
	base := "/api/media-depths/" + j.ID.String()
	scope := "?project_id=" + f.project.String()
	if rec := f.request(t, "GET", base+"/download"+scope, uuid.Nil, nil); rec.Code != 409 {
		t.Fatal("pending output downloaded", rec.Code)
	}
	if _, err := mediaapp.NewAssetQuery(mediapg.NewStore(f.db), nil).Reference(t.Context(), f.actor, f.project, *rendered.AssetID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("pending output referenceable", err)
	}
	rec := f.request(t, "GET", base+"/preview"+scope, uuid.Nil, nil)
	var preview toolapp.DepthPreview
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &preview) != nil || preview.SHA256 != *rendered.SHA256 || preview.Revision != rendered.Revision {
		t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
	}
	review := toolapp.ReviewInput{ProjectID: f.project, Revision: rendered.Revision, SHA256: *rendered.SHA256, LocalReviewConfirmed: true}
	bad := review
	bad.SHA256 = strings.Repeat("0", 64)
	if r := f.request(t, "POST", base+"/review", uuid.New(), bad); r.Code != 409 {
		t.Fatal("wrong digest review", r.Code)
	}
	key := uuid.New()
	rec = f.request(t, "POST", base+"/review", key, review)
	if rec.Code != 200 {
		t.Fatalf("review %d %s", rec.Code, rec.Body.String())
	}
	replay := f.request(t, "POST", base+"/review", key, review)
	if replay.Code != 200 || replay.Body.String() != rec.Body.String() {
		t.Fatal("review receipt was not permanent")
	}
	rec = f.request(t, "GET", base+"/download"+scope, uuid.Nil, nil)
	h := sha256.Sum256(rec.Body.Bytes())
	if rec.Code != 200 || hex.EncodeToString(h[:]) != *rendered.SHA256 {
		t.Fatal("reviewed private bytes differ", rec.Code)
	}
	var artifact struct{ Artifact []byte }
	if err := f.db.Raw(`SELECT artifact FROM mediatool.depth_result_intent WHERE job_id=? AND attempt=1`, j.ID).Scan(&artifact).Error; err != nil {
		t.Fatal(err)
	}
	var frozen toolapp.DepthArtifact
	if json.Unmarshal(artifact.Artifact, &frozen) != nil || !frozen.Receipt.Native.ProcessGroupJoined || frozen.Receipt.Output.FrameCount != 60 || len(frozen.Renditions) != 2 {
		t.Fatal("actual permanent native receipt incomplete")
	}
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }
	service := canvasapp.NewService(canvaspg.NewStore(f.db, mediaFactory))
	doc, err := service.Get(t.Context(), f.actor, f.input.CanvasID)
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), NodeType: "video", NodeAction: "resource", RefType: "media_asset", RefID: rendered.AssetID, Title: "Actual depth"}
	saved, err := service.Execute(t.Context(), f.actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: doc.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{node}}}})
	if err != nil {
		t.Fatal("actual ready result canvas reference", err)
	}
	refreshed, err := service.Get(t.Context(), f.actor, doc.ID)
	if err != nil || refreshed.Revision != saved.Revision || len(refreshed.Nodes) != 2 {
		t.Fatal("depth result failed durable canvas refresh", err)
	}
	if r := f.request(t, "POST", base+"/cancel", uuid.New(), toolapp.ControlInput{ProjectID: f.project, Revision: rendered.Revision + 1}); r.Code != 409 {
		t.Fatal("reviewed output cancellable", r.Code)
	}
	t.Logf("actual DepthJob %s native60frames + private three-object receipt + pending review + SHA review + download + formal canvas refresh", j.ID)
}

type countedDepth struct {
	toolapp.DepthProcessor
	calls atomic.Int32
}

func (p *countedDepth) Process(ctx context.Context, in *mediaapp.Downloaded, sha string, report func(toolapp.DepthPhase) error) (*toolapp.DepthOutput, error) {
	p.calls.Add(1)
	return p.DepthProcessor.Process(ctx, in, sha, report)
}

type unknownDepthObjects struct {
	toolapp.DepthObjects
	unknown  atomic.Bool
	started  atomic.Bool
	lastOnly bool
}

func (o *unknownDepthObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, sha string) error {
	err := o.DepthObjects.PutIfAbsent(ctx, key, r, size, mime, sha)
	if err == nil && (!o.lastOnly || strings.HasSuffix(key, "/proxy_720p.mp4")) {
		o.started.Store(true)
		return io.ErrUnexpectedEOF
	}
	return err
}

func TestDepthJobActualReconcileAllObjectsThenCancelPendingWithoutInference(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	p := &countedDepth{DepthProcessor: native}
	o := &unknownDepthObjects{DepthObjects: toolflow.NewObjects(f.objects), lastOnly: true}
	o.unknown.Store(true)
	worker := f.worker(t, p, o)
	unknown, err := worker.Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err == nil || !unknown.NeedsReconciliation || unknown.ExecutionUnconfirmed {
		t.Fatalf("last object unknown fence %+v %v", unknown, err)
	}
	key := uuid.New()
	command, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, key, unknown.Revision, "reconcile")
	if err != nil || !command.ReconciliationRequested {
		t.Fatal("durable reconcile intent missing", err)
	}
	if _, err = f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), command.Revision, "reconcile"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("duplicate new reconciliation intent", err)
	}
	replay, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, key, unknown.Revision, "reconcile")
	if err != nil || replay.Revision != command.Revision {
		t.Fatal("original reconcile command not replayable", err)
	}
	o.unknown.Store(false)
	recovered, err := toolapp.NewDepthWorker(f.store, o, nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
	if err != nil || recovered.Status != tooldomain.DepthReviewRequired || recovered.ReconciliationRequested || recovered.NeedsReconciliation || p.calls.Load() != 1 {
		t.Fatalf("exact receipt recovery %+v %v", recovered, err)
	}
	if _, err = f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), recovered.Revision, "cancel"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || cancelled.Status != tooldomain.DepthCancelled || p.calls.Load() != 1 {
		t.Fatalf("pending physical cleanup %+v %v", cancelled, err)
	}
	asset, err := mediapg.NewStore(f.db).FindAsset(t.Context(), f.actor, f.project, *recovered.AssetID)
	if err != nil || asset.Status != "rejected" || asset.ModerationStatus != "rejected" {
		t.Fatal("pending asset not permanently rejected", err)
	}
	if _, err := toolflow.NewObjects(f.objects).Stat(t.Context(), asset.ObjectKey); !errors.Is(err, toolapp.ErrObjectMissing) {
		t.Fatal("pending original not physically removed", err)
	}
	next, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), cancelled.Revision, "retry")
	if err != nil || next.Attempt != 2 || next.Status != tooldomain.DepthQueued || next.SourceAssetID != j.SourceAssetID || next.SourceSHA256 != j.SourceSHA256 || next.Source != j.Source {
		t.Fatal("explicit retry did not keep source/fence", err)
	}
	if err := f.store.EndProcess(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("old physical attempt wrote new state", err)
	}
}
func (o *unknownDepthObjects) Stat(ctx context.Context, key string) (mediaapp.ObjectInfo, error) {
	if o.unknown.Load() && o.started.Load() {
		return mediaapp.ObjectInfo{}, io.ErrUnexpectedEOF
	}
	return o.DepthObjects.Stat(ctx, key)
}

func TestDepthJobActualUnknownPutCancelReconcileWithoutInference(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	p := &countedDepth{DepthProcessor: native}
	o := &unknownDepthObjects{DepthObjects: toolflow.NewObjects(f.objects)}
	o.unknown.Store(true)
	worker := f.worker(t, p, o)
	result, err := worker.Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err == nil || result.Status != tooldomain.DepthFailed || !result.NeedsReconciliation || result.ExecutionUnconfirmed || result.Retryable || p.calls.Load() != 1 {
		t.Fatalf("unknown Put escaped fence %+v %v calls%d", result, err, p.calls.Load())
	}
	if _, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), result.Revision, "retry"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("unknown Put retried", err)
	}
	cancel, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), result.Revision, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if cancel.Status == tooldomain.DepthCancelled {
		t.Fatal("unknown object cancellation faked")
	}
	reconcile, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), cancel.Revision, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	o.unknown.Store(false)
	result, err = worker.Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
	if err != nil || result.Status != tooldomain.DepthCancelled || result.NeedsReconciliation || p.calls.Load() != 1 {
		t.Fatalf("exact cleanup reconciliation %+v %v calls%d cmdrev%d", result, err, p.calls.Load(), reconcile.Revision)
	}
	var keys []string
	if err := f.db.Raw(`SELECT object_key FROM mediatool.depth_object WHERE job_id=? AND attempt=1`, j.ID).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if _, err := toolflow.NewObjects(f.objects).Stat(t.Context(), key); !errors.Is(err, toolapp.ErrObjectMissing) {
			t.Fatal("unpublished result retained", err)
		}
	}
	source, err := mediapg.NewStore(f.db).FindAsset(t.Context(), f.actor, f.project, f.source)
	if err != nil || !source.CanReference() {
		t.Fatal("source media changed", err)
	}
	if _, err := f.objects.Stat(t.Context(), source.ObjectKey); err != nil {
		t.Fatal("source object removed", err)
	}
}

func TestDepthJobPGCommandsNilRuntimeCASAndImmutableACL(t *testing.T) {
	f := depthHTTP(t)
	key := uuid.New()
	j, err := f.store.Create(t.Context(), f.actor, f.project, key, f.input)
	if err != nil {
		t.Fatal(err)
	}
	disabled := depthStore(f.db, false)
	same, err := disabled.Create(t.Context(), f.actor, f.project, key, f.input)
	if err != nil || same.ID != j.ID {
		t.Fatal("disabled runtime destroyed original receipt", err)
	}
	if _, err := disabled.Create(t.Context(), f.actor, f.project, uuid.New(), f.input); !errors.Is(err, toolapp.ErrUnavailable) {
		t.Fatal("disabled new create", err)
	}
	other, foreign := mediaStoreProject(t, f.db)
	if _, err := disabled.Get(t.Context(), other, foreign, j.ID); !errors.Is(err, toolapp.ErrNotFound) {
		t.Fatal("cross project leak", err)
	}
	if _, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), j.Revision+1, "cancel"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("stale control accepted", err)
	}
	cancel, err := disabled.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), j.Revision, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := toolapp.NewDepthWorker(disabled, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || stopped.Status != tooldomain.DepthCancelled {
		t.Fatalf("disabled queued cancel did not complete %+v %v (cmdrev%d)", stopped, err, cancel.Revision)
	}
	if _, err := disabled.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), stopped.Revision, "retry"); !errors.Is(err, toolapp.ErrUnavailable) {
		t.Fatal("disabled retry accepted", err)
	}
	var rights struct{ Frozen, Source, Result, Commands, State, Objects, Delete bool }
	if err := f.db.Raw(`SELECT has_column_privilege('lanverse_app','mediatool.depth_job','frozen','UPDATE') frozen,has_column_privilege('lanverse_app','mediatool.depth_job','source_sha256','UPDATE') source,has_table_privilege('lanverse_app','mediatool.depth_result_intent','UPDATE') result,has_table_privilege('lanverse_app','mediatool.depth_command','UPDATE') commands,has_column_privilege('lanverse_app','mediatool.depth_job','status','UPDATE') state,has_column_privilege('lanverse_app','mediatool.depth_object','status','UPDATE') objects,has_table_privilege('lanverse_app','mediatool.depth_result_intent','DELETE') delete`).Scan(&rights).Error; err != nil {
		t.Fatal(err)
	}
	if rights.Frozen || rights.Source || rights.Result || rights.Commands || rights.Delete || !rights.State || !rights.Objects {
		t.Fatalf("invalid immutable ACL %+v", rights)
	}
	pageResponse := f.request(t, "GET", "/api/projects/"+f.project.String()+"/media-depths", uuid.Nil, nil)
	var page toolapp.DepthPage
	if pageResponse.Code != 200 || json.Unmarshal(pageResponse.Body.Bytes(), &page) != nil || page.CurrentActorID != f.actor.ID || page.CurrentOrgID != f.actor.OrgID || len(page.Items) != 1 {
		t.Fatal("refresh scope/current job missing", pageResponse.Code)
	}
	unknown := map[string]any{"canvas_id": f.input.CanvasID, "node_id": f.input.NodeID, "revision": f.input.Revision, "input_path": "/tmp/untrusted.mp4"}
	if rec := f.request(t, "POST", "/api/projects/"+f.project.String()+"/media-depths", uuid.New(), unknown); rec.Code != 422 {
		t.Fatal("native path accepted", rec.Code)
	}
}

func TestDepthJobPGNonOwnerTransactionAndWriteDenials(t *testing.T) {
	f := depthHTTP(t)
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL ROLE lanverse_app").Error; err != nil {
			return err
		}
		s := depthStore(tx, true)
		j, err := s.Create(t.Context(), f.actor, f.project, uuid.New(), f.input)
		if err != nil {
			return err
		}
		if _, err = s.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), j.Revision, "cancel"); err != nil {
			return err
		}
		stop, err := toolapp.NewDepthWorker(s, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
		if err != nil || stop.Status != tooldomain.DepthCancelled {
			return errors.Join(err, errors.New("nonowner queued cancel did not stop"))
		}
		for _, q := range []string{"UPDATE mediatool.depth_job SET frozen='{}' WHERE false", "UPDATE mediatool.depth_result_intent SET output_sha256=repeat('0',64) WHERE false", "UPDATE mediatool.depth_command SET response='{}' WHERE false", "DELETE FROM mediatool.depth_result_intent WHERE false"} {
			err := tx.Transaction(func(child *gorm.DB) error { return child.Exec(q).Error })
			if err == nil || !strings.Contains(err.Error(), "permission denied") {
				return errors.New("nonowner immutable write permitted")
			}
		}
		return nil
	}); err != nil {
		t.Fatal("actual nonowner command/ownership/immutable proof", err)
	}
}

type uncertainDepthProcessor struct{ path string }

func (p *uncertainDepthProcessor) Process(_ context.Context, in *mediaapp.Downloaded, _ string, _ func(toolapp.DepthPhase) error) (*toolapp.DepthOutput, error) {
	p.path = in.File.Name()
	return nil, toolapp.ErrDepthCessationUncertain
}

func TestDepthJobPGUnknownNativeFenceAndNoInferredStop(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	p := &uncertainDepthProcessor{}
	id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
	job, err := f.worker(t, p, toolflow.NewObjects(f.objects)).Execute(t.Context(), id)
	if !errors.Is(err, toolapp.ErrDepthCessationUncertain) || !job.ExecutionUnconfirmed || !job.NeedsReconciliation || job.Retryable || job.Status != tooldomain.DepthFailed {
		t.Fatalf("unknown native ceased prematurely %+v %v", job, err)
	}
	// This is a protocol fault injection, not actual inference. Its exact staged
	// input must survive uncertainty; the test owns its explicit later removal.
	if _, err := os.Stat(p.path); err != nil {
		t.Fatal("uncertain native input prematurely removed", err)
	}
	defer func() {
		if err := os.Remove(p.path); err != nil {
			t.Error(err)
		}
	}()
	for _, action := range []string{"retry", "reconcile"} {
		if _, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), job.Revision, action); !errors.Is(err, toolapp.ErrConflict) {
			t.Fatal("unknown execution admitted "+action, err)
		}
	}
	if _, err := f.store.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}); !errors.Is(err, toolapp.ErrWorkerBusy) {
		t.Fatal("unknown native owner replaced", err)
	}
	active, err := f.store.HasInflightWork(t.Context(), f.actor, f.project)
	if err != nil || !active {
		t.Fatal("unknown native omitted from project guard", err)
	}
	if err := f.store.EndProcess(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("foreign worker declared cessation", err)
	}
}

type cancellingDepth struct {
	toolapp.DepthProcessor
	f        *depthHTTPFixture
	job      uuid.UUID
	t        *testing.T
	children []int
}

func (p *cancellingDepth) Process(ctx context.Context, in *mediaapp.Downloaded, sha string, report func(toolapp.DepthPhase) error) (*toolapp.DepthOutput, error) {
	return p.DepthProcessor.Process(ctx, in, sha, func(phase toolapp.DepthPhase) error {
		if err := report(phase); err != nil {
			return err
		}
		if phase == toolapp.DepthInferring {
			current, err := p.f.store.Get(ctx, p.f.actor, p.f.project, p.job)
			if err != nil {
				return err
			}
			if _, err = p.f.store.Control(ctx, p.f.actor, p.f.project, p.job, uuid.New(), current.Revision, "cancel"); err != nil {
				return err
			}
			for _, child := range depthOwnedChildren(p.t, os.Getpid()) {
				if strings.Contains(strings.ToLower(child.program), "python") {
					p.children = append(p.children, child.pid)
				}
			}
			return toolapp.ErrCancelled
		}
		return nil
	})
}

func TestDepthJobActualCancellationWaitsNativeCessation(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	p := &cancellingDepth{DepthProcessor: native, f: f, job: j.ID, t: t}
	job, err := f.worker(t, p, toolflow.NewObjects(f.objects)).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || job.Status != tooldomain.DepthCancelled || job.ExecutionUnconfirmed || len(p.children) == 0 {
		t.Fatalf("actual native cancellation %+v %v ownedchildren%d", job, err, len(p.children))
	}
	depthAssertStopped(t, p.children)
	var outputs int
	if err := f.db.Raw(`SELECT count(*) FROM mediatool.depth_result_intent WHERE job_id=?`, j.ID).Scan(&outputs).Error; err != nil || outputs != 0 {
		t.Fatal("cancelled native output published", outputs, err)
	}
}
