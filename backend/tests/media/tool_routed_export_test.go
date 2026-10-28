package media_test

import (
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	objects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	flow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func TestRoutedTimelineExportRealPostgresNativeTOSAndFFmpeg(t *testing.T) {
	owner, runtime := routedGeneratedDatabase(t)
	actor, project := mediaStoreProject(t, owner)
	registry, writes, _ := controlledGeneratedTOS(t)
	routes := pgmedia.NewStorageRoutes(owner)
	if e := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: "tos-frozen", Provider: "tos", Endpoint: "https://tos-fixture.example.invalid", SigningEndpoint: "https://tos-fixture.example.invalid", Region: "cn-beijing", Bucket: "controlled-fixture"}}); e != nil {
		t.Fatal(e)
	}
	op := seedIngestOperation(t, owner, actor, project, "image")
	if e := owner.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'operation',?,?,'tos-frozen','controlled-fixture','controlled-v1')`, uuid.New(), project, op).Error; e != nil {
		t.Fatal(e)
	}
	rr := pgmedia.NewStorageRoutes(runtime)
	repo := pgmedia.NewScopedIngestRepository(runtime, app.NewMediaWorkerScopeAccess)
	ingest := media.NewRoutedIngestService(repo, newRoutedIngestFixture(), flow.FFProber{}, flow.FFRenderer{}, objects.NewGeneratedObjects(registry, rr), rr)
	output, e := ingest.Ingest(t.Context(), media.IngestInput{OperationID: op.String(), SeqNo: 1, URL: "synthetic image"})
	if e != nil {
		t.Fatal(e)
	}
	if e = repo.RecordModeration(t.Context(), op, uuid.MustParse(output.OutputID), domain.ModerationPassed, ""); e != nil {
		t.Fatal(e)
	}
	if e = repo.CompleteGeneratedWork(t.Context(), op, 1); e != nil {
		t.Fatal(e)
	}
	// The activation row is a local fixture only; native SDK connections stay on TLS wire.
	if e = owner.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES('controlled-tool-v1',repeat('a',64),'tos-frozen','{}'::jsonb,86400) ON CONFLICT DO NOTHING`).Error; e != nil {
		t.Fatal(e)
	}
	if e = owner.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'controlled-tool-v1','tos-frozen') ON CONFLICT(singleton) DO UPDATE SET revision=excluded.revision,profile_id=excluded.profile_id`).Error; e != nil {
		t.Fatal(e)
	}
	timeline := exportTimeline(t)
	assetID := uuid.MustParse(output.MediaAssetID)
	timeline.Clips[0].AssetID = &assetID
	freeze := &fixedTimeline{timeline: timeline}
	makeStore := func(worker bool) *pgtool.Store {
		return pgtool.NewStore(runtime, func(*gorm.DB) tool.TimelineReader { return freeze }, func(tx *gorm.DB) tool.DerivedMedia {
			if worker {
				return media.NewDerivedService(mediaWorkerStore(tx))
			}
			return media.NewDerivedService(mediaTestStore(tx))
		}).WithStorageAdmission(func(tx *gorm.DB) tool.ToolStorageLedger { return pgmedia.NewStorageRoutes(tx) })
	}
	store := makeStore(false)
	workerStore := makeStore(true)
	job, e := store.Create(t.Context(), actor, project, uuid.New(), tool.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if e != nil {
		t.Fatal("admit", e)
	}
	renderer, e := toolff.NewRenderer(flow.FFProber{})
	if e != nil {
		t.Fatal(e)
	}
	worker := tool.NewRoutedWorker(workerStore, toolflow.NewRoutedObjects(registry, rr, nil), renderer, flow.FFProber{}, flow.FFUploadRenderer{})
	work := tool.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	result, e := worker.Execute(t.Context(), work)
	if e != nil {
		t.Fatal("render/publish", e)
	}
	if result.Status != tooldomain.ReviewRequired || result.AssetID == nil || *writes != 6 {
		t.Fatalf("wrong produced metadata %+v writes %d", result, *writes)
	}
	if _, e = worker.Execute(t.Context(), work); e != nil || *writes != 6 {
		t.Fatal("replay created another object", e, *writes)
	}
	reader := objects.NewRoutedReadObjects(registry)
	query := tool.NewRoutedExportQuery(store, reader, reader, nil, nil)
	preview, e := query.Preview(t.Context(), actor, project, job.ID)
	if e != nil || !strings.Contains(preview.URL, "controlled-fixture") {
		t.Fatal("preview exact location", e)
	}
	if _, _, e = query.Download(t.Context(), actor, project, job.ID); e == nil {
		t.Fatal("unreviewed output downloaded")
	}
	reviewed, e := store.Review(t.Context(), actor, job.ID, uuid.New(), tool.ReviewInput{ProjectID: project, Revision: result.Revision, SHA256: *result.SHA256, LocalReviewConfirmed: true})
	if e != nil || reviewed.Status != tooldomain.Succeeded {
		t.Fatal("review", e)
	}
	body, size, e := query.Download(t.Context(), actor, project, job.ID)
	if e != nil {
		t.Fatal("download", e)
	}
	b, e := io.ReadAll(body)
	closeErr := body.Close()
	if e != nil || closeErr != nil || int64(len(b)) != size {
		t.Fatal("download bytes", e, closeErr)
	}
}
