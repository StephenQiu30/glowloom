package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ControlTransfer records permanent, actor-scoped cancellation and recovery
// commands. Only an ended execution can be retried or reconciled; an expired
// lease alone is never proof that an old physical writer has stopped.
func (s *TransferStore) ControlTransfer(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, revision int64, action application.TransferAction) (domain.TransferJob, error) {
	var result domain.TransferJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if id == uuid.Nil || key == uuid.Nil || revision < 1 || action != application.TransferActionCancel && action != application.TransferActionRetry && action != application.TransferActionReconcile {
		return result, domain.ErrInvalidLibrary
	}
	hash, err := transferHash(struct {
		ID       uuid.UUID                  `json:"id"`
		Revision int64                      `json:"revision"`
		Action   application.TransferAction `json:"action"`
	}{id, revision, action})
	if err != nil {
		return result, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, key); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false, true)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, true); err != nil {
			return err
		}
		cached, err := s.transferReplay(tx, actor, key, hash, action)
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
		if _, _, err := s.ensureTransferLibraries(tx, actor, source, target); err != nil {
			return err
		}
		row, err = s.readJob(tx, actor, id, true, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return domain.ErrTransferConflict
		}
		var emit application.TransferAction
		emitEvent := false
		switch action {
		case application.TransferActionCancel:
			if row.Status == domain.TransferStatusCancelled {
				break
			}
			if row.Status == domain.TransferStatusSucceeded || row.Status == domain.TransferStatusPartialFailed || row.Status == domain.TransferStatusFailed {
				return domain.ErrTransferConflict
			}
			physical, err := storageExists(tx.Model(&transferObjectRow{}).Where("job_id = ?", id))
			if err != nil {
				return err
			}
			if row.Status == domain.TransferStatusQueued && row.ProcessEnded && !row.ExecutionUnconfirmed && !physical {
				if err := tx.Model(&transferItemRow{}).Where("job_id = ? AND status = ?", id, domain.TransferItemStatusQueued).Updates(map[string]any{"status": domain.TransferItemStatusCancelled, "failure_code": domain.TransferFailureCancelled}).Error; err != nil {
					return err
				}
				if err := updateOne(tx, "media.transfer_job", map[string]any{"id": id, "revision": revision}, map[string]any{"status": domain.TransferStatusCancelled, "stage": domain.TransferStageCompleted, "cancellation_requested": true, "revision": revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
					return err
				}
			} else {
				if err := updateOne(tx, "media.transfer_job", map[string]any{"id": id, "revision": revision}, map[string]any{"status": domain.TransferStatusCancelRequested, "stage": domain.TransferStageCleanup, "cancellation_requested": true, "revision": revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
					return err
				}
				emit = application.TransferActionCancel
				emitEvent = true
			}
		case application.TransferActionRetry, application.TransferActionReconcile:
			if action == application.TransferActionRetry && row.CancellationRequested {
				return domain.ErrTransferConflict
			}
			if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Attempt >= 2147483647 {
				return domain.ErrTransferConflict
			}
			if action == application.TransferActionRetry && (row.Status != domain.TransferStatusFailed && row.Status != domain.TransferStatusPartialFailed || row.NeedsReconciliation) || action == application.TransferActionReconcile && (row.Status != domain.TransferStatusNeedsReconciliation || !row.NeedsReconciliation) {
				return domain.ErrTransferConflict
			}
			if action == application.TransferActionRetry {
				unsafe, err := storageExists(tx.Table("media.transfer_object AS o").Joins("JOIN media.transfer_item AS i ON i.job_id = o.job_id AND i.item_index = o.item_index").Where("o.job_id = ? AND i.status <> ? AND o.write_started = ? AND o.status <> ?", id, domain.TransferItemStatusSucceeded, true, application.ProjectCopyObjectRemoved))
				if err != nil {
					return err
				}
				if unsafe {
					return domain.ErrTransferConflict
				}
				if err := retryTransferItems(ctx, tx, row); err != nil {
					return err
				}
				emit = application.TransferActionStart
				emitEvent = true
			} else {
				emit = application.TransferActionReconcile
				emitEvent = true
			}
			if err := updateOne(tx, "media.transfer_job", map[string]any{"id": id, "revision": revision}, map[string]any{"status": domain.TransferStatusQueued, "stage": domain.TransferStageFrozen, "attempt": row.Attempt + 1, "revision": row.Revision + 1, "needs_reconciliation": false, "execution_id": nil, "worker_fence": nil, "lease_until": nil, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
				return err
			}
		}
		row, err = s.readJob(tx, actor, id, false, true)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		if err != nil {
			return err
		}
		return recordTransferCommand(tx, actor, key, hash, action, emit, emitEvent, result)
	})
	return result, err
}

var _ application.TransferRepository = (*TransferStore)(nil)
