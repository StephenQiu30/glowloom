package script_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	objectsadapter "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	pg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func scriptDocument(t *testing.T, db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal, project uuid.UUID, name string, data []byte) uuid.UUID {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	upload := mediaapp.NewUploadService(mediapg.NewStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: name}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := mediapg.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := objects.Remove(ctx, asset.ObjectKey); err != nil {
			t.Error(err)
		}
	})
	return result.Asset.ID
}

func scriptImportStore(db *gorm.DB, objects *objectstorage.Client) *pg.ImportStore {
	return pg.NewImportStore(db, func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, func(tx *gorm.DB) app.SourceAssetReader {
		return mediaapp.NewDocumentSources(mediapg.NewDocumentSourceStore(tx), objects)
	})
}

func TestScriptFileImportPGAdmissionFreezeReplayAndProjectFence(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	one := scriptDocument(t, db, storage, actor, pid, "第一章.txt", []byte("甲😀\r\n乙"))
	two := scriptDocument(t, db, storage, actor, pid, "第二章.txt", []byte{0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65})
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{two, one}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, input)
	if err != nil || accepted.Status != "queued" || len(accepted.Files) != 2 || accepted.Files[0].AssetID != two {
		t.Fatal("ordered freeze", accepted, err)
	}
	replay, err := app.NewImportService(pg.NewImportStore(db, func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, nil), time.Now).Create(t.Context(), actor, input)
	if err != nil || replay.ID != accepted.ID || replay.Revision != 1 {
		t.Fatal("disabled runtime must preserve durable replay", replay, err)
	}
	page, err := service.List(t.Context(), actor, pid, 0, 100)
	if err != nil || len(page.Items) != 1 || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID {
		t.Fatal("refresh recovery", page, err)
	}
	if sources, versions, _ := scriptCounts(t, owner, pid); sources != 0 || versions != 0 {
		t.Fatal("acceptance published", sources, versions)
	}
	_, write := sourceCommand()
	write.ProjectID = pid
	if _, err := app.NewSourceService(scriptStore(db), &sourceObjects{data: make(map[string][]byte)}, time.Now).Write(t.Context(), actor, write); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("editor bypassed active import", err)
	}
	if blocked, err := scriptStore(db).HasInflightWork(t.Context(), actor, pid); err != nil || !blocked {
		t.Fatal("project lifecycle ignored import", blocked, err)
	}
	input.Key = uuid.New()
	if _, err := service.Create(t.Context(), actor, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("parallel import", err)
	}
	if err := db.Exec(`UPDATE script.import_file SET sha256=? WHERE job_id=?`, "0123456789012345678901234567890123456789012345678901234567890123", accepted.ID).Error; err == nil {
		t.Fatal("runtime rewrote originals")
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(t.Context(), actor, pid, accepted.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked source owner", err)
	}
}

func importDelivery(t *testing.T, owner *gorm.DB, actor identityapp.Principal, key uuid.UUID) app.ImportDelivery {
	t.Helper()
	var row struct{ Payload string }
	if err := owner.Raw(`SELECT o.payload::text FROM infra.outbox o JOIN script.import_command c ON c.event_id=o.id WHERE c.actor_id=? AND c.request_id=?`, actor.ID, key).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data app.ImportDelivery `json:"data"`
	}
	if err := json.Unmarshal([]byte(row.Payload), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func scriptImportWorker(db *gorm.DB, storage *objectstorage.Client, extractor app.SourceExtractor) *app.ImportWorker {
	objects := objectsadapter.NewStorage(storage)
	return scriptImportWorkerWithObjects(db, storage, extractor, objects)
}
func scriptImportWorkerWithObjects(db *gorm.DB, storage *objectstorage.Client, extractor app.SourceExtractor, objects app.PrivateObjects) *app.ImportWorker {
	store := scriptImportStore(db, storage)
	sources := app.NewSourceService(scriptStore(db), objects, time.Now)
	return app.NewImportWorker(store, mediaapp.NewDocumentSources(mediapg.NewDocumentSourceStore(db), storage), extractor, func(a app.ImportAuthority) *app.SourceService {
		return app.NewSourceServiceSharingIO(pg.NewImportSourceStore(db, func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, a), objects, time.Now, sources)
	}, app.NewSourceRecovery(pg.NewImportRecoveryStore(db, func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }), sources, time.Now), time.Now)
}

func recoverableSourceExitFault(t *testing.T, owner *gorm.DB, actor uuid.UUID) func() {
	t.Helper()
	name := "import_exit_" + fmt.Sprint(uuid.New().ID())
	// UUID.ID is decimal only and this helper constructs trusted test identifiers.
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE UPDATE ON script.command_state FOR EACH ROW WHEN (NEW.actor_id='` + actor.String() + `'::uuid AND NEW.io_state='ended') EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if err := owner.Exec(`DROP TRIGGER ` + name + ` ON script.command_state`).Error; err != nil {
				t.Error(err)
			}
			if err := owner.Exec(`DROP FUNCTION script.` + name + `()`).Error; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	return release
}

func TestScriptFileImportPGPublicationExitProofFailureStillJoinsAndCleansExactObjects(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	admin := scriptAdmin(t, owner, actor.OrgID)
	file := scriptDocument(t, db, storage, actor, pid, "实际退出.txt", []byte("已有冻结字节"))
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	release := recoverableSourceExitFault(t, owner, actor.ID)
	objects := objectsadapter.NewStorage(storage)
	worker := scriptImportWorkerWithObjects(db, storage, extract.NewExtractor(), &lostPrivatePut{PrivateObjects: objects})
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, in.Key)); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("unknown source call", err)
	}
	plan := pendingScriptPlan(t, owner, pid)
	cleanScriptPlan(t, objects, plan)
	var ioState string
	if err := owner.Raw(`SELECT io_state FROM script.command_state WHERE actor_id=? AND request_id=?`, actor.ID, plan.Command.Key).Scan(&ioState).Error; err != nil || ioState != "running" {
		t.Fatal("RETURNNULL not exercised", ioState, err)
	}
	release()
	current, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || current.ActiveIO || !current.NeedsReconciliation {
		t.Fatal("outer exit fact", current, err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	control := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: current.Revision, Action: "cancel"}
	if _, err := service.Control(t.Context(), admin, control); err != nil {
		t.Fatal(err)
	}
	if err := worker.Control(t.Context(), importDelivery(t, owner, admin, control.Key)); err != nil {
		t.Fatal(err)
	}
	after, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || after.Status != "cancelled" || after.NeedsReconciliation || after.ActiveIO {
		t.Fatal("actual source exit proof was lost between services", after, err)
	}
	for _, fact := range plan.Objects {
		r, err := objects.Get(t.Context(), fact.Key)
		if err == nil {
			_, err = io.ReadAll(r)
			err = errors.Join(err, r.Close())
		}
		if !errors.Is(err, app.ErrObjectMissing) {
			t.Fatal("own unpublished object remains", fmt.Sprint(fact.SHA256), err)
		}
	}
}

func TestScriptFileImportPGWorkerActualOriginalRichVersionAndImmutableAttempt(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	first := scriptDocument(t, db, storage, actor, pid, "第一章.txt", []byte("甲😀\r\n乙 "))
	second := scriptDocument(t, db, storage, actor, pid, "第二章.txt", []byte{0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65})
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{first, second}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	worker := scriptImportWorker(db, storage, extract.NewExtractor())
	delivery := importDelivery(t, owner, actor, input.Key)
	if err := worker.Execute(t.Context(), delivery); err != nil {
		t.Fatal("actual file worker", err)
	}
	current, err := service.Get(t.Context(), actor, pid, accepted.ID)
	if err != nil || current.Status != "succeeded" || current.ActiveIO || current.NeedsReconciliation || current.LatestScriptRevision != 1 || current.LatestVersionID == nil {
		t.Fatal("published complete batch", current, err)
	}
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil || len(base.Sources) != 2 || base.Sources[0].Origin != "file" || *base.Sources[0].MediaAssetID != first || *base.Sources[1].MediaAssetID != second || base.Sources[1].Provenance.Encoding != "utf-16le" {
		t.Fatal("formal file provenance", base, err)
	}
	sourceService := app.NewSourceService(scriptStore(db), objectsadapter.NewStorage(storage), time.Now)
	text, err := sourceService.VersionText(t.Context(), actor, pid, *current.LatestVersionID)
	if err != nil || text != "甲😀\n乙 \n\n中文" {
		t.Fatalf("canonical file text %q %v", text, err)
	}
	if err := worker.Execute(t.Context(), delivery); err != nil {
		t.Fatal("retired duplicate delivery", err)
	}
	if sources, versions, _ := scriptCounts(t, owner, pid); sources != 2 || versions != 1 {
		t.Fatal("duplicated file version", sources, versions)
	}
	if err := db.Exec(`UPDATE script.import_file_result SET result='{}' WHERE job_id=?`, accepted.ID).Error; err == nil {
		t.Fatal("mutable extraction history")
	}
	for _, record := range base.Sources {
		for _, fact := range []string{record.Original.Key, record.Rich.Key} {
			key := fact
			t.Cleanup(func() {
				if err := storage.Remove(context.Background(), key); err != nil {
					t.Error(err)
				}
			})
		}
	}
	for _, fact := range []string{base.Version.Text.Key, base.Version.Rich.Key} {
		key := fact
		t.Cleanup(func() {
			if err := storage.Remove(context.Background(), key); err != nil {
				t.Error(err)
			}
		})
	}
}

type failImportFileOnce struct {
	original app.SourceExtractor
	calls    int
	failed   bool
}

func (e *failImportFileOnce) Extract(ctx context.Context, file *mediaapp.Downloaded) (app.ExtractedDocument, error) {
	e.calls++
	if !e.failed {
		e.failed = true
		return app.ExtractedDocument{}, app.ErrExtractionFailed
	}
	return e.original.Extract(ctx, file)
}

func TestScriptFileImportPGPartialRetryKeepsLineageAndOriginalFrozenOrder(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	first := scriptDocument(t, db, storage, actor, pid, "先失败.txt", []byte("第一份😀"))
	second := scriptDocument(t, db, storage, actor, pid, "成功.docx", docxFile(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>第二份中文</w:t></w:r></w:p>`))
	store := scriptImportStore(db, storage)
	service := app.NewImportService(store, time.Now)
	extractor := &failImportFileOnce{original: extract.NewExtractor()}
	worker := scriptImportWorker(db, storage, extractor)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{first, second}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, in.Key)); err != nil {
		t.Fatal(err)
	}
	partial, err := service.Get(t.Context(), actor, pid, accepted.ID)
	if err != nil || partial.Status != "partial" || !partial.Retryable || partial.Files[0].Status != "failed" || partial.Files[1].SourceID == nil {
		t.Fatal("transparent partial", partial, err)
	}
	adminView, err := service.Get(t.Context(), scriptAdmin(t, owner, actor.OrgID), pid, accepted.ID)
	if err != nil || !adminView.CanControl || adminView.Retryable {
		t.Fatal("administrator received creator-only retry gate", adminView, err)
	}
	secondID := *partial.Files[1].SourceID
	firstVersion := *partial.LatestVersionID
	retry := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: partial.Revision, Action: "retry"}
	if _, err := service.Control(t.Context(), actor, retry); err != nil {
		t.Fatal(err)
	}
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, retry.Key)); err != nil {
		t.Fatal("retry only failed file", err)
	}
	current, err := service.Get(t.Context(), actor, pid, accepted.ID)
	if err != nil || current.Status != "succeeded" || current.Attempt != 2 || extractor.calls != 3 || *current.Files[1].SourceID != secondID {
		t.Fatal("reimported success or changed lineage", current, extractor.calls, err)
	}
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil || len(base.Sources) != 2 || *base.Sources[0].MediaAssetID != first || *base.Sources[1].MediaAssetID != second {
		t.Fatal("retry lost original file order", base, err)
	}
	history, err := scriptStore(db).LoadBase(t.Context(), actor, pid, &firstVersion)
	if err != nil || len(history.Sources) != 1 || history.Sources[0].ID != secondID {
		t.Fatal("partial history overwritten", history, err)
	}
	var outcomes, publications int64
	if err := owner.Raw(`SELECT count(*) FROM script.import_file_result WHERE job_id=?`, accepted.ID).Scan(&outcomes).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Raw(`SELECT count(*) FROM script.import_publication WHERE job_id=?`, accepted.ID).Scan(&publications).Error; err != nil || outcomes != 3 || publications != 2 {
		t.Fatal("attempt history dropped", outcomes, publications, err)
	}
}

func TestScriptFileImportPGQueuedAdminCancelRevokedCreatorAndPermanentDelivery(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	admin := scriptAdmin(t, owner, actor.OrgID)
	file := scriptDocument(t, db, storage, actor, pid, "未执行.txt", []byte("原件保留"))
	store := scriptImportStore(db, storage)
	service := app.NewImportService(store, time.Now)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	cancel := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: accepted.Revision, Action: "cancel"}
	response, err := service.Control(t.Context(), admin, cancel)
	if err != nil || response.Status != "cancel_requested" {
		t.Fatal("admin intent", response, err)
	}
	delivery := importDelivery(t, owner, admin, cancel.Key)
	spoof := delivery
	spoof.ActorID = actor.ID
	worker := scriptImportWorker(db, storage, extract.NewExtractor())
	if err := worker.Control(t.Context(), spoof); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatal("borrowed command actor", err)
	}
	if err := worker.Control(t.Context(), delivery); err != nil {
		t.Fatal("revoked creator queued cleanup", err)
	}
	current, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || current.Status != "cancelled" || current.NeedsReconciliation || current.ActiveIO {
		t.Fatal("false cancelled", current, err)
	}
	replay, err := service.Control(t.Context(), admin, cancel)
	if err != nil || replay.Revision != response.Revision || replay.Status != response.Status {
		t.Fatal("rewritten202", replay, err)
	}
	if err := store.FailImportWorkflow(t.Context(), app.ImportWork{JobID: accepted.ID, Attempt: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || after.Status != "cancelled" || after.Revision != current.Revision {
		t.Fatal("late workflow regressed cancelled", after, err)
	}
	if sources, versions, _ := scriptCounts(t, owner, pid); sources != 0 || versions != 0 {
		t.Fatal("admin published", sources, versions)
	}
	if blocked, err := scriptStore(db).HasInflightWork(t.Context(), admin, pid); err != nil || blocked {
		t.Fatal("cancelled fence", blocked, err)
	}
}

type pausedImportExtractor struct {
	started chan struct{}
	once    sync.Once
	stopped chan struct{}
}

func (e *pausedImportExtractor) Extract(ctx context.Context, _ *mediaapp.Downloaded) (app.ExtractedDocument, error) {
	e.once.Do(func() { close(e.started) })
	<-ctx.Done()
	close(e.stopped)
	return app.ExtractedDocument{}, ctx.Err()
}

func TestScriptFileImportPGActiveCancellationJoinsActualExtractionAndLostOwnerStaysFenced(t *testing.T) {
	t.Run("joined", func(t *testing.T) {
		db, owner := scriptTestDB(t)
		actor, pid := scriptActorProject(t, owner)
		storage := scriptStorage(t)
		file := scriptDocument(t, db, storage, actor, pid, "取消.txt", []byte("尚未发表"))
		service := app.NewImportService(scriptImportStore(db, storage), time.Now)
		in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
		accepted, err := service.Create(t.Context(), actor, in)
		if err != nil {
			t.Fatal(err)
		}
		extractor := &pausedImportExtractor{started: make(chan struct{}), stopped: make(chan struct{})}
		worker := scriptImportWorker(db, storage, extractor)
		delivery := importDelivery(t, owner, actor, in.Key)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		done := make(chan error, 1)
		joined := make(chan struct{})
		go func() { defer close(joined); done <- worker.Execute(ctx, delivery) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-joined:
			case <-time.After(5 * time.Second):
				t.Error("import owning test routine did not exit")
			}
		})
		select {
		case <-extractor.started:
		case <-time.After(5 * time.Second):
			t.Fatal("extractor not active")
		}
		active, err := service.Get(t.Context(), actor, pid, accepted.ID)
		if err != nil || !active.ActiveIO {
			t.Fatal(active, err)
		}
		control := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: active.Revision, Action: "cancel"}
		if _, err := service.Control(t.Context(), actor, control); err != nil {
			t.Fatal(err)
		}
		if err := worker.Control(t.Context(), importDelivery(t, owner, actor, control.Key)); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatal("executor exit", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("lost owning goroutine")
		}
		select {
		case <-extractor.stopped:
		default:
			t.Fatal("cancel before actual exit")
		}
		current, err := service.Get(t.Context(), actor, pid, accepted.ID)
		if err != nil || current.Status != "cancelled" || current.ActiveIO || current.NeedsReconciliation {
			t.Fatal("joined cancellation", current, err)
		}
	})
	t.Run("lost_owner", func(t *testing.T) {
		db, owner := scriptTestDB(t)
		actor, pid := scriptActorProject(t, owner)
		storage := scriptStorage(t)
		file := scriptDocument(t, db, storage, actor, pid, "失联.txt", []byte("停止未知"))
		store := scriptImportStore(db, storage)
		service := app.NewImportService(store, time.Now)
		in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
		accepted, err := service.Create(t.Context(), actor, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ClaimImport(t.Context(), importDelivery(t, owner, actor, in.Key), uuid.New(), time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := store.FailImportWorkflow(t.Context(), app.ImportWork{JobID: accepted.ID, Attempt: 1}, time.Now()); err != nil {
			t.Fatal(err)
		}
		current, err := service.Get(t.Context(), actor, pid, accepted.ID)
		if err != nil {
			t.Fatal(err)
		}
		control := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: current.Revision, Action: "cancel"}
		if _, err := service.Control(t.Context(), actor, control); err != nil {
			t.Fatal(err)
		}
		if err := scriptImportWorker(db, storage, extract.NewExtractor()).Control(t.Context(), importDelivery(t, owner, actor, control.Key)); err != nil {
			t.Fatal(err)
		}
		after, err := service.Get(t.Context(), actor, pid, accepted.ID)
		if err != nil || after.Status == "cancelled" || !after.ActiveIO || !after.NeedsReconciliation || after.Retryable {
			t.Fatal("missing local registry invented cessation", after, err)
		}
	})
}

type failedImportExtractor struct{}

func (failedImportExtractor) Extract(context.Context, *mediaapp.Downloaded) (app.ExtractedDocument, error) {
	return app.ExtractedDocument{}, app.ErrExtractionFailed
}

func TestScriptFileImportPGOuterExitProofFailureRecoversOnlyJoinedPermanentController(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	admin := scriptAdmin(t, owner, actor.OrgID)
	file := scriptDocument(t, db, storage, actor, pid, "未发布.txt", []byte("真实原件"))
	store := scriptImportStore(db, storage)
	service := app.NewImportService(store, time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	name := "import_outer_exit_" + fmt.Sprint(uuid.New().ID())
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE UPDATE ON script.import_state FOR EACH ROW WHEN (NEW.job_id='` + accepted.ID.String() + `'::uuid AND NEW.io_state='ended') EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if err := owner.Exec(`DROP TRIGGER ` + name + ` ON script.import_state`).Error; err != nil {
				t.Error(err)
			}
			if err := owner.Exec(`DROP FUNCTION script.` + name + `()`).Error; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	worker := scriptImportWorker(db, storage, failedImportExtractor{})
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, input.Key)); err == nil {
		t.Fatal("zero-row exit fault was not exercised")
	}
	before, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || !before.ActiveIO {
		t.Fatal(before, err)
	}
	release()
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	control := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: before.Revision, Action: "cancel"}
	if _, err := service.Control(t.Context(), admin, control); err != nil {
		t.Fatal(err)
	}
	delivery := importDelivery(t, owner, admin, control.Key)
	forged := delivery
	forged.EventID = uuid.New()
	if err := worker.Control(t.Context(), forged); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatal("fabricated controller", err)
	}
	if err := worker.Control(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(t.Context(), admin, pid, accepted.ID)
	if err != nil || current.Status != "cancelled" || current.ActiveIO || current.NeedsReconciliation {
		t.Fatal("joined exit cannot be recorded", current, err)
	}
}
