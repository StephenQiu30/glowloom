package script_test

import (
	"testing"
	"time"

	currenttask "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extract"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func TestScriptFileImportTaskPGActualPrivateOriginalToVersion(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "真实任务.txt", []byte("数据库任务原文😀"))
	store := scriptImportStore(db, storage)
	service := app.NewImportService(store, time.Now)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	var outbox struct {
		ID      uuid.UUID
		Payload string
	}
	if err := owner.Raw(`SELECT o.id,o.payload::text FROM infra.outbox o JOIN script.import_command c ON c.event_id=o.id WHERE c.actor_id=? AND c.request_id=?`, actor.ID, in.Key).Scan(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	taskfixture.RunWork(t, db, currenttask.Import, map[string]any{"job_id": accepted.ID}, currenttask.ImportHandler(scriptImportWorker(t, db, storage, extract.NewExtractor()), store))
	current, err := service.Get(ctx, actor, pid, accepted.ID)
	if err != nil || current.Status != "succeeded" || current.LatestVersionID == nil || current.ActiveIO || current.NeedsReconciliation {
		t.Fatal("actual workflow durable state", current, err)
	}
	replay, err := service.Create(ctx, actor, in)
	if err != nil || replay.ID != accepted.ID {
		t.Fatal("duplicate command", err)
	}
	var count int
	if err := owner.Raw("SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=? AND completed_at IS NOT NULL AND epoch=1", currenttask.Import, outbox.ID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("immutable task receipt", count, err)
	}
}
