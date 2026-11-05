package media_test

import (
	"testing"
	"time"

	currenttask "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestMediaPurgeActualDatabasePrivateObjectsAndUnknownRecovery(t *testing.T) {
	for _, unknownOutcome := range []bool{false, true} {
		t.Run(map[bool]string{false: "actual all-object removal", true: "actual unknown removal then exact reconciliation"}[unknownOutcome], func(t *testing.T) {
			db := libraryRuntimeDB(t)
			actor, _, in, keys, objects := purgeBinaryFixture(t, db)
			repo := mediaPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
			job, err := repo.CreatePurge(t.Context(), actor, in)
			if err != nil {
				t.Fatal(err)
			}
			private := &purgeUnknownRemove{purgeFixtureObjects: objects, once: unknownOutcome}
			run := currenttask.PurgeHandler(mediaapp.NewPurgeWorker(repo.Worker, private), repo.Worker)
			ctx := t.Context()
			taskfixture.RunWork(t, db, currenttask.Purge, map[string]any{"job_id": job.ID}, run)
			current, err := repo.GetPurge(ctx, actor, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if unknownOutcome {
				if current.Status != "needs_reconciliation" || private.removeCount != 1 {
					t.Fatal("unknown actual deletion fabricated a terminal result", current)
				}
				key := uuid.New()
				if _, err := repo.ControlPurge(ctx, actor, job.ID, key, current.Revision, "reconcile"); err != nil {
					t.Fatal(err)
				}
				taskfixture.RunWork(t, db, currenttask.Purge, map[string]any{"job_id": job.ID, "attempt": 2}, run)
				current, err = repo.GetPurge(ctx, actor, job.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if current.Status != "succeeded" || private.removeCount != len(keys) {
				t.Fatal("actual private-object workflow did not settle exactly", current, private.removeCount)
			}
			for _, key := range keys {
				exists, err := glbTestObjects(t).Exists(ctx, key)
				if err != nil || exists {
					t.Fatal("actual key still exists after successful workflow", err)
				}
			}
			var maxEpoch int
			if err := db.Raw("SELECT max(epoch) FROM infra.task_work WHERE kind=? AND reference->>'job_id'=?", currenttask.Purge, job.ID.String()).Scan(&maxEpoch).Error; err != nil || maxEpoch != 1 {
				t.Fatal("unexpected destructive retry", maxEpoch, err)
			}
		})
	}
}
