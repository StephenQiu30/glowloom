package script_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptFileImportPGAdmissionOutboxFailureRollsBackAllFrozenFacts(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "受理失败.txt", []byte("没有隐式受理"))
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	// A zero-row audit prevents committing even an otherwise successful admission.
	// partition_key is text. Use one exact permanent event route in a task-owned trigger.
	name := "import_outbox_" + uuid.New().String()[:8]
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT ON infra.outbox FOR EACH ROW WHEN (NEW.partition_key='` + pid.String() + `') EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + name + ` ON infra.outbox`).Error; err != nil {
			t.Error(err)
		}
		if err := owner.Exec(`DROP FUNCTION script.` + name + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	if _, err := service.Create(t.Context(), actor, in); err == nil {
		t.Fatal("zero-row outbox accepted")
	}
	for _, table := range []string{"script.import_job", "script.import_file", "script.import_command", "script.request"} {
		var count int64
		if err := owner.Table(table).Where("project_id=?", pid).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("frozen facts escaped admission rollback", table, count, err)
		}
	}
}

func TestScriptFileImportPGAllFailedRetryRejectsChangedDraftWithoutIO(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "失败.txt", []byte("冻结原件"))
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := scriptImportWorker(db, storage, failedImportExtractor{}).Execute(t.Context(), importDelivery(t, owner, actor, in.Key)); err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(t.Context(), actor, pid, accepted.ID)
	if err != nil || current.Status != "failed" || !current.Retryable || current.LatestVersionID != nil {
		t.Fatal("all-failed invented a draft", current, err)
	}
	if s, v, _ := scriptCounts(t, owner, pid); s != 0 || v != 0 {
		t.Fatal("empty failure published", s, v)
	}
	_, edit := sourceCommand()
	edit.ProjectID = pid
	if _, err := app.NewSourceService(scriptStore(db), scriptobjects.NewStorage(storage), time.Now).Write(t.Context(), actor, edit); err != nil {
		t.Fatal(err)
	}
	retry := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: current.Revision, Action: "retry"}
	if _, err := service.Control(t.Context(), actor, retry); !errors.Is(err, app.ErrConflict) {
		t.Fatal("retry ignored changed complete head", err)
	}
	after, err := service.Get(t.Context(), actor, pid, accepted.ID)
	if err != nil || after.Attempt != 1 || after.Revision != current.Revision {
		t.Fatal("rejected retry changed job", after, err)
	}
	var attempts int64
	if err := owner.Raw(`SELECT count(*) FROM script.import_attempt WHERE job_id=?`, accepted.ID).Scan(&attempts).Error; err != nil || attempts != 1 {
		t.Fatal("rejected retry persisted attempt", attempts, err)
	}
}

func TestScriptFileImportPGUnknownPublicationObjectsStayFencedUntilExactReconcile(t *testing.T) {
	for _, fault := range []string{"absent_unknown_put", "foreign_digest"} {
		t.Run(fault, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			actor, pid := scriptActorProject(t, owner)
			storage := scriptStorage(t)
			objects := scriptobjects.NewStorage(storage)
			data := []byte("冻结的精确原件😀")
			file := scriptDocument(t, db, storage, actor, pid, "未知.txt", data)
			admin := scriptAdmin(t, owner, actor.OrgID)
			service := app.NewImportService(scriptImportStore(db, storage), time.Now)
			input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
			accepted, err := service.Create(t.Context(), actor, input)
			if err != nil {
				t.Fatal(err)
			}
			var uncertain app.PrivateObjects = neverScriptPut{PrivateObjects: objects}
			if fault == "foreign_digest" {
				uncertain = &lostPrivatePut{PrivateObjects: objects}
			}
			worker := scriptImportWorkerWithObjects(db, storage, extract.NewExtractor(), uncertain)
			if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, input.Key)); !errors.Is(err, app.ErrNeedsReconciliation) {
				t.Fatal(err)
			}
			plan := pendingScriptPlan(t, owner, pid)
			cleanScriptPlan(t, objects, plan)
			fact := plan.Objects[0]
			if fact.SHA256 != domain.ContentSHA(data) {
				t.Fatal("expected first owned original fact")
			}
			if fault == "foreign_digest" {
				if err := objects.Remove(t.Context(), fact.Key); err != nil {
					t.Fatal(err)
				}
				bad := []byte("不是冻结字节")
				if err := objects.PutIfAbsent(t.Context(), fact.Key, bytes.NewReader(bad), int64(len(bad)), fact.MIME, domain.ContentSHA(bad)); err != nil {
					t.Fatal(err)
				}
			}
			if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			before, err := service.Get(t.Context(), admin, pid, accepted.ID)
			if err != nil {
				t.Fatal(err)
			}
			cancel := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: before.Revision, Action: "cancel"}
			if _, err := service.Control(t.Context(), admin, cancel); err != nil {
				t.Fatal(err)
			}
			if err := worker.Control(t.Context(), importDelivery(t, owner, admin, cancel.Key)); err != nil {
				t.Fatal(err)
			}
			pending, err := service.Get(t.Context(), admin, pid, accepted.ID)
			if err != nil || pending.Status != "cancel_requested" || !pending.NeedsReconciliation || pending.Retryable || pending.ActiveIO {
				t.Fatal("unknown object cleanup invented success", pending, err)
			}
			if fault == "foreign_digest" {
				r, err := objects.Get(t.Context(), fact.Key)
				if err != nil {
					t.Fatal("removed foreign digest", err)
				}
				actual, err := io.ReadAll(r)
				err = errors.Join(err, r.Close())
				if err != nil || string(actual) != "不是冻结字节" {
					t.Fatal("foreign object modified", err)
				}
				if err := objects.Remove(t.Context(), fact.Key); err != nil {
					t.Fatal(err)
				}
			}
			// This isolated fixture supplies the previously unknown response bytes. The
			// worker never reconstructs/re-Puts missing input during cancellation.
			if err := objects.PutIfAbsent(t.Context(), fact.Key, bytes.NewReader(data), fact.ByteSize, fact.MIME, fact.SHA256); err != nil {
				t.Fatal(err)
			}
			reconcile := cancel
			reconcile.Key = uuid.New()
			reconcile.RequestID = uuid.New()
			reconcile.Action = "reconcile"
			reconcile.ExpectedRevision = pending.Revision
			if _, err := service.Control(t.Context(), admin, reconcile); err != nil {
				t.Fatal(err)
			}
			if err := worker.Control(t.Context(), importDelivery(t, owner, admin, reconcile.Key)); err != nil {
				t.Fatal(err)
			}
			after, err := service.Get(t.Context(), admin, pid, accepted.ID)
			if err != nil || after.Status != "cancelled" || after.NeedsReconciliation || after.ActiveIO {
				t.Fatal("exact reconciliation did not recover", after, err)
			}
			if s, v, _ := scriptCounts(t, owner, pid); s != 0 || v != 0 {
				t.Fatal("control published old content", s, v)
			}
		})
	}
}

func TestScriptFileImportPGRevokedControllerCanBeRecoveredByNewExplicitController(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "控制者撤权.txt", []byte("未执行"))
	first := scriptAdmin(t, owner, actor.OrgID)
	second := scriptAdmin(t, owner, actor.OrgID)
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	command := app.ImportControl{ProjectID: pid, JobID: accepted.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: accepted.Revision, Action: "cancel"}
	response, err := service.Control(t.Context(), first, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	worker := scriptImportWorker(db, storage, extract.NewExtractor())
	if err := worker.Control(t.Context(), importDelivery(t, owner, first, command.Key)); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked controller authorized", err)
	}
	current, err := service.Get(t.Context(), second, pid, accepted.ID)
	if err != nil || current.Revision != response.Revision || !current.CancellationRequested || !current.CanControl {
		t.Fatal("public recovery path lost", current, err)
	}
	// CancellationRequested alone exposes the explicit reconcile gate even when
	// no uncertainty exists. A different controller never replays the old key.
	command.Key = uuid.New()
	command.RequestID = uuid.New()
	command.Action = "reconcile"
	command.ExpectedRevision = current.Revision
	if _, err := service.Control(t.Context(), second, command); err != nil {
		t.Fatal("existing public reconcile unreachable", err)
	}
	if err := worker.Control(t.Context(), importDelivery(t, owner, second, command.Key)); err != nil {
		t.Fatal(err)
	}
	after, err := service.Get(t.Context(), second, pid, accepted.ID)
	if err != nil || after.Status != "cancelled" || after.ActiveIO || after.NeedsReconciliation {
		t.Fatal(after, err)
	}
}

func TestScriptFileImportPGInternalPublicationCannotUsePublicSourceRecovery(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "所属控制.txt", []byte("只能经导入控制"))
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	if _, err := service.Create(t.Context(), actor, input); err != nil {
		t.Fatal(err)
	}
	objects := scriptobjects.NewStorage(storage)
	worker := scriptImportWorkerWithObjects(db, storage, extract.NewExtractor(), &lostPrivatePut{PrivateObjects: objects})
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, input.Key)); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal(err)
	}
	plan := pendingScriptPlan(t, owner, pid)
	cleanScriptPlan(t, objects, plan)
	id := uuid.NewSHA1(plan.Command.Key, []byte("source-write/"+actor.ID.String()+"/"+pid.String()))
	normal := app.NewSourceRecovery(scriptStore(db), app.NewSourceService(scriptStore(db), objects, time.Now), time.Now)
	page, err := normal.List(t.Context(), actor, pid, 0, 100)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("public Source8 lists import publication", len(page.Items), err)
	}
	if _, err := normal.Get(t.Context(), actor, pid, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("public Source8 read import publication", err)
	}
	command := app.SourceControlCommand{ProjectID: pid, IntentID: id, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: 1}
	if _, err := normal.Control(t.Context(), actor, command); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("public Source8 accepted import control", err)
	}
	var controls int64
	if err := owner.Raw(`SELECT count(*) FROM script.source_control WHERE project_id=?`, pid).Scan(&controls).Error; err != nil || controls != 0 {
		t.Fatal("public control wrote", controls, err)
	}
}
