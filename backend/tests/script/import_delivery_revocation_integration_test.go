package script_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func assertRevokedImportDelivery(t *testing.T, db *gorm.DB, store app.ImportWorkerStore, worker *app.ImportWorker, delivery app.ImportDelivery) {
	t.Helper()
	if allowed, err := store.VerifyImportDelivery(t.Context(), delivery); err != nil || allowed {
		t.Fatal("revoked permanent controller retained delivery authority", allowed, err)
	}
	evidence := func() []byte {
		t.Helper()
		var body []byte
		if err := db.Raw(`SELECT jsonb_build_object('state',to_jsonb(s),'events',(SELECT count(*) FROM infra.outbox WHERE partition_key=?),'sources',(SELECT count(*) FROM script.script_source WHERE project_id=?),'versions',(SELECT count(*) FROM script.script_version WHERE project_id=?)) FROM script.import_state s WHERE job_id=?`, delivery.ProjectID.String(), delivery.ProjectID, delivery.ProjectID, delivery.JobID).Row().Scan(&body); err != nil {
			t.Fatal("read exact import control evidence", err)
		}
		return body
	}
	before := evidence()
	if err := worker.Control(t.Context(), delivery); err != nil {
		t.Fatal("rejected delivery was not safely ignored", err)
	}
	if !bytes.Equal(before, evidence()) {
		t.Fatal("rejected delivery changed state, audit or published content")
	}
}

// All metadata is synthetic and rolled back. No original object is created or
// read: admission freezes SQL facts, and rejected delivery stops before I/O.
func TestScriptImportDeliveryRevocationInRollbackTransaction(t *testing.T) {
	_, owner := scriptTestDB(t)
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("roll back synthetic import delivery", err)
		}
	}()
	actor, project := scriptActorProject(t, tx)
	controller := scriptAdmin(t, tx, actor.OrgID)
	asset := uuid.New()
	key := "projects/" + project.String() + "/document/2026/10/" + asset.String() + ".txt"
	if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,codec,byte_size,sha256,moderation_status,aigc_marked) VALUES(?,?,'document','upload','ready',?,'fixture.txt','text/plain','txt',3,repeat('a',64),'passed',true)`, asset, project, key).Error; err != nil {
		t.Fatal(err)
	}
	store := scriptImportStore(tx, nil)
	service := app.NewImportService(store, time.Now)
	job, err := service.Create(t.Context(), actor, app.ImportCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{asset}, RightsConfirmed: true})
	if err != nil {
		t.Fatal("admit synthetic SQL-only document", err)
	}
	command := app.ImportControl{ProjectID: project, JobID: job.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: job.Revision, Action: "cancel"}
	if _, err := service.Control(t.Context(), controller, command); err != nil {
		t.Fatal(err)
	}
	delivery := importDelivery(t, tx, controller, command.Key)
	if allowed, err := store.VerifyImportDelivery(t.Context(), delivery); err != nil || !allowed {
		t.Fatal("current permanent controller lost delivery authority", allowed, err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
		t.Fatal(err)
	}
	worker := app.NewImportWorker(store, nil, nil, nil, app.NewSourceRecovery(scriptStore(tx), nil, time.Now), time.Now)
	assertRevokedImportDelivery(t, tx, store, worker, delivery)
}
