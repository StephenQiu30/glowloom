package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ControlTransfer records permanent, actor-scoped cancellation and recovery
// commands. Only an ended execution can be retried or reconciled; an expired
// lease alone is never proof that an old physical writer has stopped.
func (s *TransferStore) ControlTransfer(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, revision int64, action string) (domain.TransferJob, error) {
	var result domain.TransferJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if id == uuid.Nil || key == uuid.Nil || revision < 1 || action != "cancel" && action != "retry" && action != "reconcile" {
		return result, domain.ErrInvalidLibrary
	}
	hash, err := transferHash(struct {
		ID       uuid.UUID `json:"id"`
		Revision int64     `json:"revision"`
		Action   string    `json:"action"`
	}{id, revision, action})
	if err != nil {
		return result, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, key); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, false); err != nil {
			return err
		}
		cached, err := transferReplay(tx, actor, key, hash, action)
		if err != nil {
			return err
		}
		if cached != nil {
			if cached.ID != id {
				return application.ErrLibraryKeyConflict
			}
			result = *cached
			return nil
		}
		if _, _, err := ensureTransferLibraries(tx, actor, source, target); err != nil {
			return err
		}
		row, err = s.readJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return domain.ErrTransferConflict
		}
		emit := ""
		switch action {
		case "cancel":
			if row.Status == "cancelled" {
				break
			}
			if row.Status == "succeeded" || row.Status == "partial_failed" || row.Status == "failed" {
				return domain.ErrTransferConflict
			}
			var started bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_object WHERE job_id=? AND write_started)`, id).Scan(&started).Error; err != nil {
				return err
			}
			if row.Status == "queued" && row.ProcessEnded && !row.ExecutionUnconfirmed && !started {
				if err := tx.Exec(`UPDATE media.transfer_item SET status='cancelled',failure_code='cancelled' WHERE job_id=? AND status='queued'`, id).Error; err != nil {
					return err
				}
				if err := libraryChanged(tx, `UPDATE media.transfer_job SET status='cancelled',stage='completed',cancellation_requested=true,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, s.clock().UTC(), id, revision); err != nil {
					return err
				}
			} else {
				if err := libraryChanged(tx, `UPDATE media.transfer_job SET status='cancel_requested',stage='cleanup',cancellation_requested=true,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, s.clock().UTC(), id, revision); err != nil {
					return err
				}
				emit = "cancel"
			}
		case "retry", "reconcile":
			if action == "retry" && row.CancellationRequested {
				return domain.ErrTransferConflict
			}
			if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Attempt >= 2147483647 {
				return domain.ErrTransferConflict
			}
			if action == "retry" && (row.Status != "failed" && row.Status != "partial_failed" || row.NeedsReconciliation) || action == "reconcile" && (row.Status != "needs_reconciliation" || !row.NeedsReconciliation) {
				return domain.ErrTransferConflict
			}
			if action == "retry" {
				var unsafe bool
				if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_object o JOIN media.transfer_item i USING(job_id,item_index) WHERE o.job_id=? AND i.status<>'succeeded' AND o.write_started AND o.status<>'removed')`, id).Scan(&unsafe).Error; err != nil {
					return err
				}
				if unsafe {
					return domain.ErrTransferConflict
				}
				if err := tx.Exec(`UPDATE media.transfer_object o SET status='pending',write_started=false FROM media.transfer_item i WHERE o.job_id=i.job_id AND o.item_index=i.item_index AND o.job_id=? AND i.status='failed'`, id).Error; err != nil {
					return err
				}
				if err := tx.Exec(`UPDATE media.transfer_item SET status='queued',failure_code=NULL WHERE job_id=? AND status='failed'`, id).Error; err != nil {
					return err
				}
				emit = "start"
			} else {
				emit = "reconcile"
			}
			if err := libraryChanged(tx, `UPDATE media.transfer_job SET status='queued',stage='frozen',attempt=attempt+1,revision=revision+1,needs_reconciliation=false,execution_id=NULL,worker_fence=NULL,lease_until=NULL,updated_at=? WHERE id=? AND revision=?`, s.clock().UTC(), id, revision); err != nil {
				return err
			}
		}
		row, err = s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		if err != nil {
			return err
		}
		return recordTransferCommand(tx, actor, key, hash, action, result, emit)
	})
	return result, err
}

var _ application.TransferRepository = (*TransferStore)(nil)
