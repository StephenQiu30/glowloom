package media_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaTransferRevokedActorCannotReadReplayRecoverOrPublishUnknownObjects(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	owner := libraryOwnerDB(t)
	objects := &transferUnknownWrite{ProjectCopyObjects: copyobjects.NewProjectCopyObjects(f.objects), failAt: 1}
	unknown, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
	if err != nil || !unknown.NeedsReconciliation {
		t.Fatal(unknown, err)
	}
	key := uuid.New()
	requested, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.WithContext(context.Background()).Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, f.actor.ID).Error; err != nil {
			t.Error(err)
		}
	})
	checks := []func() error{
		func() error { _, err := f.repo.GetTransfer(t.Context(), f.actor, unknown.ID); return err },
		func() error { _, err := f.repo.CreateTransfer(t.Context(), f.actor, f.input); return err },
		func() error {
			_, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "reconcile")
			return err
		},
		func() error {
			_, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(requested))
			return err
		},
	}
	for index, check := range checks {
		if err := check(); !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatal("revocation granted cached or physical authority", index, err)
		}
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, *requested.Items[0].TargetAssetID).Scan(&count).Error; err != nil || count != 0 || objects.writes != 1 {
		t.Fatal("revoked actor published or created more private objects", count, objects.writes, err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(requested))
	if err != nil || result.Status != "succeeded" {
		t.Fatal("authorized explicit original recovery failed", result, err)
	}
}

func TestMediaTransferExpiredLeaseNeverStealsPhysicalWorkAndEndedFenceIsExact(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	now := time.Now()
	repo := pgmedia.NewTransferStore(f.db, libraryTestAccess, transferTestGuards, func() time.Time { return now })
	lease, err := repo.ClaimTransfer(t.Context(), transferExecution(f.job))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := repo.ClaimTransfer(t.Context(), transferExecution(f.job)); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("expiry was mistaken for physical cessation", err)
	}
	if err := repo.BeginTransferWrite(t.Context(), lease, 0, ""); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("expired physical owner retained write authority", err)
	}
	forged := lease
	forged.Fence = uuid.New()
	if err := repo.EndTransferPhysical(t.Context(), forged); err == nil {
		t.Fatal("foreign fence declared physical work ended")
	}
	owner := libraryOwnerDB(t)
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.WithContext(context.Background()).Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, f.actor.ID).Error
	})
	if err := repo.EndTransferPhysical(t.Context(), lease); err != nil {
		t.Fatal("exact stopped writer could not record cessation after revocation", err)
	}
	if _, err := repo.FinishTransfer(t.Context(), lease); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("cessation proof was mistaken for publication permission", err)
	}
	var facts struct{ ProcessEnded, ExecutionUnconfirmed bool }
	if err := f.db.Raw(`SELECT process_ended,execution_unconfirmed FROM media.transfer_job WHERE id=?`, f.job.ID).Scan(&facts).Error; err != nil || !facts.ProcessEnded || facts.ExecutionUnconfirmed {
		t.Fatal("cessation facts were not preserved", facts, err)
	}
}

func TestMediaTransferControlZeroRowsRollsBackEveryPermanentReceiptAndState(t *testing.T) {
	for _, action := range []string{"cancel", "reconcile"} {
		for _, table := range []string{"transfer_command", "transfer_job"} {
			t.Run(action+"/"+table, func(t *testing.T) {
				f := newTransferRecoveryFixture(t, 1)
				before := f.job
				if action == "reconcile" {
					objects := &transferUnknownWrite{ProjectCopyObjects: copyobjects.NewProjectCopyObjects(f.objects), failAt: 1}
					var err error
					before, err = mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
					if err != nil || !before.NeedsReconciliation {
						t.Fatal(before, err)
					}
				}
				owner := libraryOwnerDB(t)
				name := "transfer_fault_" + uuid.NewString()[:8]
				condition, event := "NEW.job_id", "INSERT"
				if table == "transfer_job" {
					condition, event = "NEW.id", "UPDATE"
				}
				ddl := `CREATE FUNCTION media.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF ` + condition + `='` + f.job.ID.String() + `'::uuid THEN RETURN NULL; END IF; RETURN NEW; END; $$; CREATE TRIGGER ` + name + ` BEFORE ` + event + ` ON media.` + table + ` FOR EACH ROW EXECUTE FUNCTION media.` + name + `() `
				if err := owner.Exec(ddl).Error; err != nil {
					t.Fatal(err)
				}
				remove := func() error {
					return owner.WithContext(context.Background()).Exec(`DROP TRIGGER IF EXISTS ` + name + ` ON media.` + table + `; DROP FUNCTION IF EXISTS media.` + name + `()`).Error
				}
				t.Cleanup(func() {
					if err := remove(); err != nil {
						t.Error(err)
					}
				})
				key := uuid.New()
				if _, err := f.repo.ControlTransfer(t.Context(), f.actor, before.ID, key, before.Revision, action); err == nil {
					t.Fatal("zero-row owning write fabricated successful command")
				}
				after, err := f.repo.GetTransfer(t.Context(), f.actor, before.ID)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatal("zero-row command leaked state", after, err)
				}
				var leaked int64
				if err := f.db.Raw(`SELECT count(*) FROM media.transfer_command WHERE actor_id=? AND idem_key=?`, f.actor.ID, key).Scan(&leaked).Error; err != nil || leaked != 0 {
					t.Fatal("zero-row command leaked permanent receipt", leaked, err)
				}
				if err := remove(); err != nil {
					t.Fatal(err)
				}
				if _, err := f.repo.ControlTransfer(t.Context(), f.actor, before.ID, key, before.Revision, action); err != nil {
					t.Fatal("rolled-back original key could not be accepted", err)
				}
			})
		}
	}
}
