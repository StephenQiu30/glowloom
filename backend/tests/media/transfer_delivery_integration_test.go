package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func transferCommittedDelivery(t *testing.T, f transferRecoveryFixture) mediaapp.TransferDelivery {
	t.Helper()
	var payload []byte
	if err := f.db.Raw(`SELECT o.payload FROM media.transfer_command c JOIN infra.outbox o ON o.id=c.event_id WHERE c.actor_id=? AND c.idem_key=?`, f.actor.ID, f.input.Key).Row().Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Data mediaapp.TransferDelivery `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event.Data
}

func TestMediaTransferDeliveryRequiresExactPermanentOutboxAndCurrentActor(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	delivery := transferCommittedDelivery(t, f)
	if ok, err := f.repo.VerifyTransferDelivery(t.Context(), delivery); err != nil || !ok {
		t.Fatal("committed permanent delivery was not proven", ok, err)
	}
	for _, change := range []func(*mediaapp.TransferDelivery){
		func(d *mediaapp.TransferDelivery) { d.EventID = uuid.New() },
		func(d *mediaapp.TransferDelivery) { d.RequestID = uuid.New() },
		func(d *mediaapp.TransferDelivery) { d.ActorID = uuid.New() },
		func(d *mediaapp.TransferDelivery) { d.OrgID = uuid.New() },
		func(d *mediaapp.TransferDelivery) { d.Attempt++ },
		func(d *mediaapp.TransferDelivery) { d.Action = "cancel" },
		func(d *mediaapp.TransferDelivery) { d.ExecutionID = uuid.NewString() },
	} {
		forged := delivery
		change(&forged)
		if ok, err := f.repo.VerifyTransferDelivery(t.Context(), forged); ok || err == nil {
			t.Fatal("foreign command reached physical dispatch", ok, err)
		}
	}
	owner := libraryOwnerDB(t)
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.WithContext(context.Background()).Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, f.actor.ID).Error
	})
	if ok, err := f.repo.VerifyTransferDelivery(t.Context(), delivery); ok || !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("committed event resurrected revoked actor", ok, err)
	}
}

func TestMediaTransferInterruptRetainsUnknownFenceUntilActualCessation(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	work := transferExecution(f.job)
	lease, err := f.repo.ClaimTransfer(t.Context(), work)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InterruptTransfer(t.Context(), work); err != nil {
		t.Fatal(err)
	}
	unknown, err := f.repo.GetTransfer(t.Context(), f.actor, f.job.ID)
	if err != nil || !unknown.NeedsReconciliation || !unknown.ExecutionUnconfirmed || unknown.Status != "needs_reconciliation" {
		t.Fatal("orchestrator timeout released unconfirmed physical owner", unknown, err)
	}
	if _, err := f.repo.ClaimTransfer(t.Context(), transferExecution(f.job)); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("unknown physical owner was stolen", err)
	}
	if _, err := f.repo.ControlTransfer(t.Context(), f.actor, f.job.ID, uuid.New(), unknown.Revision, "reconcile"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("unknown physical owner was considered ended", err)
	}
	if err := f.repo.EndTransferPhysical(t.Context(), lease); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InterruptTransfer(t.Context(), work); err != nil {
		t.Fatal(err)
	}
	ended, err := f.repo.GetTransfer(t.Context(), f.actor, f.job.ID)
	if err != nil || ended.ExecutionUnconfirmed || !ended.NeedsReconciliation {
		t.Fatal("late interruption erased actual cessation", ended, err)
	}
	queued, err := f.repo.ControlTransfer(t.Context(), f.actor, f.job.ID, uuid.New(), ended.Revision, "reconcile")
	if err != nil || queued.Attempt != 2 {
		t.Fatal("actual ceased owner could not enter explicit recovery", queued, err)
	}
	if ok, err := f.repo.VerifyTransferDelivery(t.Context(), transferCommittedDelivery(t, f)); err != nil || ok {
		t.Fatal("obsolete proven attempt was dispatched again", ok, err)
	}
	if err := f.repo.EndTransferPhysical(t.Context(), lease); err == nil {
		t.Fatal("old fence changed a newer accepted attempt")
	}
}
