package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func (s *PurgeStore) cancelPendingItem(ctx context.Context, tx *gorm.DB, row purgeJobRow, actor identityapp.Principal, index int, library libraryRow, facts application.LibraryProjectFacts) (bool, error) {
	state, frozen, err := readPurgeItem(tx, row.ID, index)
	if err != nil {
		return false, err
	}
	if state.Status == "succeeded" || state.Status == "blocked" || state.Status == "cancelled" {
		return true, nil
	}
	var started bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_object WHERE job_id=? AND item_index=? AND removal_started)`, row.ID, index).Scan(&started).Error; err != nil {
		return false, err
	}
	if started {
		return false, nil
	}
	if err := validatePurgeItem(tx, row, frozen); err != nil {
		return false, err
	}
	if frozen.Asset != nil {
		if err := libraryChanged(tx, `UPDATE media.media_asset SET is_delete=false,delete_time=NULL,purge_after=NULL,revision=revision+1,update_time=? WHERE id=? AND revision=? AND is_delete`, s.clock().UTC(), frozen.Asset.ID, frozen.Asset.Revision); err != nil {
			return false, err
		}
	}
	if err := libraryChanged(tx, `UPDATE media.purge_item SET status='cancelled',failure_code='cancelled' WHERE job_id=? AND item_index=?`, row.ID, index); err != nil {
		return false, err
	}
	if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, s.clock().UTC(), library.ID, library.Revision); err != nil {
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
func (s *PurgeStore) ControlPurge(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, revision int64, action string) (domain.PurgeJob, error) {
	var result domain.PurgeJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if id == uuid.Nil || key == uuid.Nil || revision < 1 || action != "cancel" && action != "reconcile" {
		return result, domain.ErrInvalidLibrary
	}
	hash, err := purgeHash(struct {
		ID       uuid.UUID `json:"id"`
		Revision int64     `json:"revision"`
		Action   string    `json:"action"`
	}{id, revision, action})
	if err != nil {
		return result, err
	}
	var cached *domain.PurgeJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
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
		if err := requireCurrentActor(tx, actor); err != nil {
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
		if row.Status == "succeeded" || row.Status == "failed" || row.Status == "partial_failed" || row.Status == "cancelled" {
			return domain.ErrPurgeConflict
		}
		emit := true
		if action == "reconcile" {
			if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Status != "needs_reconciliation" && row.Status != "cancel_requested" && row.Status != "running" {
				return domain.ErrPurgeConflict
			}
			if err := tx.Exec(`UPDATE media.purge_item SET status='queued' WHERE job_id=? AND status IN ('running','needs_reconciliation')`, id).Error; err != nil {
				return err
			}
			if err := libraryChanged(tx, `UPDATE media.purge_job SET status='queued',attempt=attempt+1,revision=revision+1,needs_reconciliation=false,updated_at=? WHERE id=? AND revision=?`, s.clock().UTC(), id, revision); err != nil {
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
			status, stage := "cancel_requested", row.Stage
			if !unsettled {
				status, stage = "cancelled", "completed"
				emit = false
			}
			if err := libraryChanged(tx, `UPDATE media.purge_job SET cancellation_requested=true,status=?,stage=?,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, status, stage, s.clock().UTC(), id, revision); err != nil {
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
