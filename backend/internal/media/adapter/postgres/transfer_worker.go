package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func (s *TransferStore) workerActor(tx *gorm.DB, id uuid.UUID) (identityapp.Principal, error) {
	var row struct {
		ActorID, OrgID uuid.UUID
		Role           string
	}
	read := tx.Raw(`SELECT j.actor_id,j.org_id,u.role FROM media.transfer_job j JOIN identity."user" u ON u.id=j.actor_id AND u.org_id=j.org_id WHERE j.id=?`, id).Scan(&row)
	if read.Error != nil {
		return identityapp.Principal{}, read.Error
	}
	if read.RowsAffected != 1 {
		return identityapp.Principal{}, application.ErrNotFound
	}
	return identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID, Role: identitydomain.Role(row.Role)}, nil
}

func (s *TransferStore) withLease(ctx context.Context, lease application.TransferLease, ended, allowCancel bool, consume func(*gorm.DB, transferJobRow, libraryRow, libraryRow) error) error {
	if s == nil || s.db == nil || lease.Fence == uuid.Nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(lease.Work.ExecutionID)
	if err != nil {
		return domain.ErrTransferConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, lease.Actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, lease.Actor, lease.Work.JobID, false)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, lease.Actor, source, target, true); err != nil {
			return err
		}
		sourceLibrary, targetLibrary, err := ensureTransferLibraries(tx, lease.Actor, source, target)
		if err != nil {
			return err
		}
		row, err = s.readJob(tx, lease.Actor, lease.Work.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != lease.Work.Attempt || row.WorkerFence == nil || *row.WorkerFence != lease.Fence || row.ExecutionID == nil || *row.ExecutionID != execution || row.LeaseUntil == nil || !row.LeaseUntil.After(s.clock()) || row.ProcessEnded != ended {
			return domain.ErrTransferConflict
		}
		if !allowCancel && row.Status == "cancel_requested" {
			return domain.ErrTransferCancelled
		}
		if !allowCancel && row.Status != "running" {
			return domain.ErrTransferConflict
		}
		return consume(tx, row, sourceLibrary, targetLibrary)
	})
}

// ClaimTransfer never steals an expired lease from an unconfirmed physical owner.
func (s *TransferStore) ClaimTransfer(ctx context.Context, work application.TransferWorkID) (application.TransferLease, error) {
	var result application.TransferLease
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	execution, err := uuid.Parse(work.ExecutionID)
	if err != nil || execution == uuid.Nil || work.JobID == uuid.Nil || work.Attempt < 1 {
		return result, domain.ErrTransferConflict
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := s.workerActor(tx, work.JobID)
		if err != nil {
			return err
		}
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, work.JobID, false)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, true); err != nil {
			return err
		}
		if _, _, err := ensureTransferLibraries(tx, actor, source, target); err != nil {
			return err
		}
		row, err = s.readJob(tx, actor, work.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != work.Attempt {
			return domain.ErrTransferConflict
		}
		result = application.TransferLease{Work: work, Actor: actor, ItemCount: row.ItemCount}
		result.Job, err = transferView(tx, row)
		if err != nil {
			return err
		}
		if row.Status == "succeeded" || row.Status == "cancelled" || row.Status == "failed" || row.Status == "partial_failed" {
			result.Done = true
			return nil
		}
		if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Status != "queued" && row.Status != "cancel_requested" {
			return domain.ErrTransferConflict
		}
		var frozen []struct{ Frozen []byte }
		if err := tx.Raw(`SELECT frozen FROM media.transfer_item WHERE job_id=? ORDER BY item_index`, row.ID).Scan(&frozen).Error; err != nil {
			return err
		}
		items := make([]application.FrozenTransferItem, len(frozen))
		for i, r := range frozen {
			if strictCopyMedia(r.Frozen, &items[i]) != nil {
				return application.ErrUnavailable
			}
		}
		digest, _, err := transferFrozenBytes(items)
		if err != nil || digest != row.ManifestSHA256 || len(items) != row.ItemCount {
			return application.ErrUnavailable
		}
		result.Fence = uuid.New()
		status := "running"
		if row.CancellationRequested {
			status = "cancel_requested"
		}
		return libraryChanged(tx, `UPDATE media.transfer_job SET status=?,stage='copying',worker_fence=?,execution_id=?,lease_until=?,process_ended=false,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, status, result.Fence, execution, s.clock().Add(45*time.Second).UTC(), s.clock().UTC(), row.ID, row.Revision)
	})
	return result, err
}

// HeartbeatTransfer renews only the exact live owner and observes cancellation.
func (s *TransferStore) HeartbeatTransfer(ctx context.Context, lease application.TransferLease) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		return libraryChanged(tx, `UPDATE media.transfer_job SET lease_until=?,updated_at=? WHERE id=? AND worker_fence=?`, s.clock().Add(45*time.Second).UTC(), s.clock().UTC(), row.ID, lease.Fence)
	})
}

type transferItemRow struct {
	Frozen []byte
	Status string
}

func readTransferItem(tx *gorm.DB, id uuid.UUID, index int) (transferItemRow, application.FrozenTransferItem, error) {
	var row transferItemRow
	if index < 0 || index > 199 {
		return row, application.FrozenTransferItem{}, application.ErrNotFound
	}
	read := tx.Raw(`SELECT frozen,status FROM media.transfer_item WHERE job_id=? AND item_index=? FOR UPDATE`, id, index).Scan(&row)
	if read.Error != nil {
		return row, application.FrozenTransferItem{}, read.Error
	}
	var item application.FrozenTransferItem
	if read.RowsAffected != 1 || strictCopyMedia(row.Frozen, &item) != nil {
		return row, item, application.ErrUnavailable
	}
	return row, item, nil
}

func validateTransferSource(tx *gorm.DB, actor identityapp.Principal, row transferJobRow, source libraryRow, item application.FrozenTransferItem) error {
	scope, _ := row.scopes()
	items, err := lockLibraryItems(tx, actor, scope, source.ID, []uuid.UUID{item.SourceItem.ID})
	if err != nil || !equalCopyMedia(item.SourceItem, items[item.SourceItem.ID]) {
		return &application.TransferItemError{Code: "source_changed", Cause: err}
	}
	if item.SourceAsset == nil {
		return nil
	}
	file, err := transferSourceFile(tx, actor, scope, item.SourceAsset.ID)
	if err != nil || !equalCopyMedia(*item.SourceAsset, file.Asset) || len(file.Renditions) != len(item.SourceRenditions) {
		return &application.TransferItemError{Code: "source_changed", Cause: err}
	}
	byID := make(map[uuid.UUID]domain.Rendition, len(file.Renditions))
	for _, r := range file.Renditions {
		byID[r.ID] = r
	}
	for _, r := range item.SourceRenditions {
		if !equalCopyMedia(r, byID[r.ID]) {
			return &application.TransferItemError{Code: "source_changed"}
		}
	}
	return nil
}

func validateTransferFolder(tx *gorm.DB, row transferJobRow, target libraryRow) error {
	if row.TargetFolderID == nil {
		return nil
	}
	var present int
	read := tx.Raw(`SELECT 1 FROM media.library_folder WHERE id=? AND library_id=? AND revision=? FOR SHARE`, *row.TargetFolderID, target.ID, row.TargetFolderRevision).Scan(&present)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return &application.TransferItemError{Code: "target_folder_changed"}
	}
	return nil
}

// StartTransferItem verifies the saved source and target folder before any I/O.
func (s *TransferStore) StartTransferItem(ctx context.Context, lease application.TransferLease, index int) (bool, error) {
	done := false
	err := s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, source, target libraryRow) error {
		state, item, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" {
			done = true
			return nil
		}
		if err := validateTransferSource(tx, lease.Actor, row, source, item); err != nil {
			return err
		}
		if err := validateTransferFolder(tx, row, target); err != nil {
			return err
		}
		return libraryChanged(tx, `UPDATE media.transfer_item SET status='running',failure_code=NULL WHERE job_id=? AND item_index=? AND status IN ('queued','needs_reconciliation','running')`, row.ID, index)
	})
	return done, err
}

// EndTransferPhysical records cessation under the original fence even if actor
// rights were revoked. It grants no object access or business publication.
func (s *TransferStore) EndTransferPhysical(ctx context.Context, lease application.TransferLease) error {
	if s == nil || s.db == nil || lease.Fence == uuid.Nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(lease.Work.ExecutionID)
	if err != nil {
		return domain.ErrTransferConflict
	}
	return libraryChanged(s.db.WithContext(ctx), `UPDATE media.transfer_job SET process_ended=true,execution_unconfirmed=false,revision=revision+1,updated_at=? WHERE id=? AND attempt=? AND worker_fence=? AND execution_id=? AND org_id=? AND actor_id=?`, s.clock().UTC(), lease.Work.JobID, lease.Work.Attempt, lease.Fence, execution, lease.Actor.OrgID, lease.Actor.ID)
}

// FailTransferItem keeps unknown outcomes distinct from a completed failure.
func (s *TransferStore) FailTransferItem(ctx context.Context, lease application.TransferLease, index int, code string, unknown bool) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" {
			return domain.ErrTransferConflict
		}
		status := "failed"
		if unknown {
			status = "needs_reconciliation"
		} else if code == "cancelled" {
			if row.Status == "cancel_requested" {
				status = "cancelled"
			} else {
				code = "worker_interrupted"
			}
		}
		if err := libraryChanged(tx, `UPDATE media.transfer_item SET status=?,failure_code=? WHERE job_id=? AND item_index=?`, status, code, row.ID, index); err != nil {
			return err
		}
		if unknown {
			return libraryChanged(tx, `UPDATE media.transfer_job SET needs_reconciliation=true,stage='cleanup',revision=revision+1,updated_at=? WHERE id=? AND worker_fence=?`, s.clock().UTC(), row.ID, lease.Fence)
		}
		return nil
	})
}

// FinishTransfer exposes only actual per-item outcomes after physical cessation.
func (s *TransferStore) FinishTransfer(ctx context.Context, lease application.TransferLease) (domain.TransferJob, error) {
	var result domain.TransferJob
	err := s.withLease(ctx, lease, true, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		job, err := transferView(tx, row)
		if err != nil {
			return err
		}
		succeeded, unknown := 0, false
		for _, item := range job.Items {
			if item.Status == "succeeded" {
				succeeded++
			}
			unknown = unknown || item.Status == "needs_reconciliation"
		}
		if !unknown && row.NeedsReconciliation {
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_object o JOIN media.transfer_item i USING(job_id,item_index) WHERE o.job_id=? AND i.status<>'succeeded' AND o.write_started AND o.status<>'removed')`, row.ID).Scan(&unknown).Error; err != nil {
				return err
			}
		}
		status := "failed"
		switch {
		case unknown:
			status = "needs_reconciliation"
		case succeeded == len(job.Items):
			status = "succeeded"
		case succeeded > 0:
			status = "partial_failed"
		case row.Status == "cancel_requested":
			status = "cancelled"
		}
		if row.CancellationRequested && !unknown {
			var unsafe bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_object o JOIN media.transfer_item i USING(job_id,item_index) WHERE o.job_id=? AND i.status<>'succeeded' AND o.write_started AND o.status<>'removed')`, row.ID).Scan(&unsafe).Error; err != nil {
				return err
			}
			if unsafe {
				return domain.ErrTransferConflict
			}
			if err := tx.Exec(`UPDATE media.transfer_item SET status='cancelled',failure_code='cancelled' WHERE job_id=? AND status<>'succeeded'`, row.ID).Error; err != nil {
				return err
			}
		}
		stage := "completed"
		if unknown {
			stage = "cleanup"
		}
		if err := libraryChanged(tx, `UPDATE media.transfer_job SET status=?,stage=?,needs_reconciliation=?,revision=revision+1,updated_at=? WHERE id=? AND worker_fence=?`, status, stage, unknown, s.clock().UTC(), row.ID, lease.Fence); err != nil {
			return err
		}
		row, err = s.readJob(tx, lease.Actor, row.ID, false)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		return err
	})
	return result, err
}
