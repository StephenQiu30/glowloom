package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	record "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	ce "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/event"
	wa "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	wd "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type recordedCopyDelivery struct {
	calls    int
	fail     bool
	delivery wa.ProjectCopyDelivery
}

func (s *recordedCopyDelivery) Deliver(_ context.Context, d wa.ProjectCopyDelivery) error {
	s.calls++
	s.delivery = d
	if s.fail {
		return errors.New("synthetic workflow delivery unavailable")
	}
	return nil
}
func TestProjectCopyRealOutboxDeliveryProofAndRetry(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, source := lifecycleActorProject(ctx, t, db)
	store := projectCopyStore(db)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		ID           uuid.UUID
		Payload      []byte
		PartitionKey string
	}
	if err := db.Raw(`SELECT id,payload,partition_key FROM infra.outbox WHERE topic=? AND payload->'data'->>'copy_job_id'=? AND payload->'data'->>'action'='requested'`, ce.ProjectCopyTopic, job.ID.String()).Scan(&event).Error; err != nil || event.ID == uuid.Nil {
		t.Fatal("actual copy outbox", err)
	}
	starter := &recordedCopyDelivery{fail: true}
	handler := ce.NewProjectCopyHandler(inbox.NewStore(db), store, starter)
	message := record.Record{Topic: ce.ProjectCopyTopic, Key: []byte(event.PartitionKey), Value: event.Payload}
	// An injected actor under a real event UUID must not poison the durable inbox.
	var forged map[string]any
	if json.Unmarshal(event.Payload, &forged) != nil {
		t.Fatal("event JSON")
	}
	forged["actor"] = map[string]any{"kind": "user", "id": uuid.NewString()}
	fake, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	injected := message
	injected.Value = fake
	if err := handler.Handle(ctx, injected); err != nil {
		t.Fatal(err)
	}
	if starter.calls != 0 {
		t.Fatal("injected actor delivered")
	}
	if err := handler.Handle(ctx, message); err == nil || starter.calls != 1 {
		t.Fatal("delivery failure acknowledged", err)
	}
	starter.fail = false
	if err := handler.Handle(ctx, message); err != nil || starter.calls != 2 {
		t.Fatal("failed external delivery not retried", err)
	}
	if err := handler.Handle(ctx, message); err != nil || starter.calls != 2 {
		t.Fatal("durable duplicate delivery ran twice", err)
	}
	if starter.delivery.OrgID != actor.OrgID || starter.delivery.JobID != job.ID || starter.delivery.EventID != event.ID || starter.delivery.ActorID != actor.ID {
		t.Fatal("committed identity lost")
	}
}
func TestProjectCopyActualAuditOutboxFailureRollsBackEveryOwner(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, source := lifecycleActorProject(ctx, t, db)
	function := "copy_outbox_test_" + uuid.New().String()[:8]
	trigger := function + "_trigger"
	create := `CREATE FUNCTION workspace.` + function + `()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.topic='lanverse.audit.recorded.v1' AND NEW.payload->>'org_id'='` + actor.OrgID.String() + `' THEN RAISE EXCEPTION 'synthetic exact copy audit failure'; END IF; RETURN NEW; END $$`
	if err := db.Exec(create).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER ` + trigger + ` BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION workspace.` + function + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		database := db.WithContext(cleanup)
		if err := database.Exec(`DROP TRIGGER IF EXISTS ` + trigger + ` ON infra.outbox`).Error; err != nil {
			t.Error(err)
		}
		if err := database.Exec(`DROP FUNCTION IF EXISTS workspace.` + function + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	if _, err := projectCopyStore(db).Create(ctx, actor, copyInput(source), time.Now()); err == nil {
		t.Fatal("copy admitted without required audit")
	}
	var projects, jobs, snapshots int
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE org_id=? AND id<>?`, actor.OrgID, source).Scan(&projects).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_job WHERE org_id=?`, actor.OrgID).Scan(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT (SELECT count(*) FROM media.project_copy_snapshot WHERE org_id=?)+(SELECT count(*) FROM canvas.project_copy_snapshot WHERE org_id=?)`, actor.OrgID, actor.OrgID).Scan(&snapshots).Error; err != nil {
		t.Fatal(err)
	}
	if projects != 0 || jobs != 0 || snapshots != 0 {
		t.Fatal("cross-owner partial admission persisted", projects, jobs, snapshots)
	}
}
func TestProjectCopyInterruptedActivityRetainsUnacknowledgedWorker(t *testing.T) {
	job := projectCopyJob()
	worker := uuid.New()
	if err := job.Start(worker, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := job.Interrupt(worker); err != nil || job.WorkerID != worker || !job.ExecutionUnconfirmed || !job.NeedsReconciliation {
		t.Fatal("timeout invented cessation", err)
	}
	if err := job.RequestCancel(); err != nil {
		t.Fatal(err)
	}
	if err := job.RequestReconciliation(); !errors.Is(err, wd.ErrProjectCopyStateConflict) {
		t.Fatal("live unknown worker reclaimed", err)
	}
	if err := job.Fail(worker, "object_write_unknown", false, true); err != nil || job.ExecutionUnconfirmed || job.WorkerID != uuid.Nil || !job.CancellationRequested {
		t.Fatal("actual cessation did not preserve uncertainty", err)
	}
	if err := job.RequestReconciliation(); err != nil {
		t.Fatal(err)
	}
	if err := job.ResumeReconciliation(uuid.New()); err != nil || job.Status != "cancel_requested" {
		t.Fatal("stopped recovery did not retain cleanup", err)
	}
}
