package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func (s *PurgeStore) cancelPendingItem(ctx context.Context, tx *gorm.DB, row purgeJobRow, actor identityapp.Principal, index int, library libraryRow, facts application.LibraryProjectFacts) (bool, error) {
	state, frozen, err := readPurgeItem(tx, row.ID, index)
	if err != nil {
		return false, err
	}
	if state.Status == domain.PurgeItemStatusSucceeded || state.Status == domain.PurgeItemStatusBlocked || state.Status == domain.PurgeItemStatusCancelled {
		return true, nil
	}
	var startedCount int64
	if err := tx.Model(&purgeObjectRow{}).Where("job_id = ? AND item_index = ? AND removal_started = ?", row.ID, index, true).Limit(1).Count(&startedCount).Error; err != nil {
		return false, err
	}
	if startedCount > 0 {
		return false, nil
	}
	if err := validatePurgeItem(tx, row, frozen); err != nil {
		return false, err
	}
	if frozen.Asset != nil {
		if err := updateOne(tx, "media.media_asset", map[string]any{"id": frozen.Asset.ID, "revision": frozen.Asset.Revision, "is_delete": true}, map[string]any{"is_delete": false, "delete_time": nil, "purge_after": nil, "revision": frozen.Asset.Revision + 1, "update_time": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
			return false, err
		}
	}
	if err := updateOne(tx, "media.purge_item", map[string]any{"job_id": row.ID, "item_index": index}, map[string]any{"status": domain.PurgeItemStatusCancelled, "failure_code": domain.PurgeFailureCancelled}, application.ErrLibraryConflict); err != nil {
		return false, err
	}
	if err := updateOne(tx, "media.library", map[string]any{"id": library.ID, "revision": library.Revision}, map[string]any{"revision": library.Revision + 1, "update_time": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
		return false, err
	}
	if row.ProjectID != nil {
		if _, err := s.project(tx).TouchContent(ctx, actor, facts.ProjectID, facts.Revision); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ControlPurge is permanent and currently authorized. Cancel never recreates
// deleted bytes: any started item must finish its original cleanup instead.
func (s *PurgeStore) ControlPurge(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, revision int64, action application.PurgeAction) (domain.PurgeJob, error) {
	var result domain.PurgeJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if id == uuid.Nil || key == uuid.Nil || revision < 1 || action != application.PurgeActionCancel && action != application.PurgeActionReconcile {
		return result, domain.ErrInvalidLibrary
	}
	hash, err := purgeHash(struct {
		ID       uuid.UUID               `json:"id"`
		Revision int64                   `json:"revision"`
		Action   application.PurgeAction `json:"action"`
	}{id, revision, action})
	if err != nil {
		return result, err
	}
	var cached *domain.PurgeJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, key); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, row.scope(), false); err != nil {
			return err
		}
		cached, err = purgeReplay(tx, actor, key, hash, action)
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, key); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, actor, row.scope(), true)
		if err != nil {
			return err
		}
		library, err := readLibrary(tx, actor, row.scope(), true)
		if err != nil {
			return err
		}
		replay, err := purgeReplay(tx, actor, key, hash, action)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		row, err = s.readJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return domain.ErrPurgeConflict
		}
		if row.Status == domain.PurgeStatusSucceeded || row.Status == domain.PurgeStatusFailed || row.Status == domain.PurgeStatusPartialFailed || row.Status == domain.PurgeStatusCancelled {
			return domain.ErrPurgeConflict
		}
		emit := true
		if action == application.PurgeActionReconcile {
			if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Status != domain.PurgeStatusNeedsReconciliation && row.Status != domain.PurgeStatusCancelRequested && row.Status != domain.PurgeStatusRunning {
				return domain.ErrPurgeConflict
			}
			if err := tx.Model(&purgeItemRow{}).Where("job_id = ? AND status IN ?", id, []domain.PurgeItemStatus{domain.PurgeItemStatusRunning, domain.PurgeItemStatusNeedsReconciliation}).Update("status", domain.PurgeItemStatusQueued).Error; err != nil {
				return err
			}
			if err := updateOne(tx, "media.purge_job", map[string]any{"id": id, "revision": revision}, map[string]any{"status": domain.PurgeStatusQueued, "attempt": row.Attempt + 1, "revision": revision + 1, "needs_reconciliation": false, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
				return err
			}
		} else {
			unsettled := !row.ProcessEnded || row.ExecutionUnconfirmed
			if !unsettled {
				for index := 0; index < row.ItemCount; index++ {
					done, err := s.cancelPendingItem(ctx, tx, row, actor, index, library, facts)
					if err != nil {
						return err
					}
					unsettled = unsettled || !done
					library, err = readLibrary(tx, actor, row.scope(), true)
					if err != nil {
						return err
					}
					facts, err = s.access(ctx, tx, actor, row.scope(), true)
					if err != nil {
						return err
					}
				}
			}
			status, stage := domain.PurgeStatusCancelRequested, row.Stage
			if !unsettled {
				status, stage = domain.PurgeStatusCancelled, domain.PurgeStageCompleted
				emit = false
			}
			if err := updateOne(tx, "media.purge_job", map[string]any{"id": id, "revision": revision}, map[string]any{"cancellation_requested": true, "status": status, "stage": stage, "revision": revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
				return err
			}
		}
		row, err = s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		result, err = purgeView(tx, row)
		if err != nil {
			return err
		}
		return recordPurgeCommand(tx, actor, key, hash, action, result, emit)
	})
	return result, err
}

var _ application.PurgeRepository = (*PurgeStore)(nil)
