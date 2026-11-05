package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func (s *TransferStore) workerActor(tx *gorm.DB, id uuid.UUID) (identityapp.Principal, error) {
	var row struct {
		ActorID, OrgID uuid.UUID
	}
	read := tx.Model(&transferJobRow{}).Select("actor_id", "org_id").Where("id = ?", id).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return identityapp.Principal{}, application.ErrNotFound
	}
	if read.Error != nil {
		return identityapp.Principal{}, read.Error
	}
	return identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID}, nil
}

func (s *TransferStore) withLease(ctx context.Context, lease application.TransferLease, ended, allowCancel bool, consume func(*gorm.DB, transferJobRow, libraryRow, libraryRow) error) error {
	if s == nil || s.db == nil || lease.Fence == uuid.Nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(lease.Work.ExecutionID)
	if err != nil {
		return domain.ErrTransferConflict
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, lease.Actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, lease.Actor, lease.Work.JobID, false, true)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, lease.Actor, source, target, true); err != nil {
			return err
		}
		sourceLibrary, targetLibrary, err := s.ensureTransferLibraries(tx, lease.Actor, source, target)
		if err != nil {
			return err
		}
		row, err = s.readJob(tx, lease.Actor, lease.Work.JobID, true, true)
		if err != nil {
			return err
		}
		if row.Attempt != lease.Work.Attempt || row.WorkerFence == nil || *row.WorkerFence != lease.Fence || row.ExecutionID == nil || *row.ExecutionID != execution || row.LeaseUntil == nil || !row.LeaseUntil.After(s.clock()) || row.ProcessEnded != ended {
			return domain.ErrTransferConflict
		}
		if !allowCancel && row.Status == domain.TransferStatusCancelRequested {
			return domain.ErrTransferCancelled
		}
		if !allowCancel && row.Status != domain.TransferStatusRunning {
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
	err = taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		actor, err := s.workerActor(tx, work.JobID)
		if err != nil {
			return err
		}
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, work.JobID, false, true)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, true); err != nil {
			return err
		}
		if _, _, err := s.ensureTransferLibraries(tx, actor, source, target); err != nil {
			return err
		}
		row, err = s.readJob(tx, actor, work.JobID, true, true)
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
		if row.Status == domain.TransferStatusSucceeded || row.Status == domain.TransferStatusCancelled || row.Status == domain.TransferStatusFailed || row.Status == domain.TransferStatusPartialFailed {
			result.Done = true
			return nil
		}
		if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Status != domain.TransferStatusQueued && row.Status != domain.TransferStatusCancelRequested {
			return domain.ErrTransferConflict
		}
		var frozen []transferItemRow
		if err := tx.Select("frozen").Where("job_id = ?", row.ID).Order("item_index").Find(&frozen).Error; err != nil {
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
		for index, item := range items {
			if _, _, err := readTransferAttempt(tx, row, index, item); err != nil {
				return err
			}
		}
		result.Fence = uuid.New()
		status := domain.TransferStatusRunning
		if row.CancellationRequested {
			status = domain.TransferStatusCancelRequested
		}
		return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "revision": row.Revision}, map[string]any{"status": status, "stage": domain.TransferStageCopying, "worker_fence": result.Fence, "execution_id": execution, "lease_until": s.clock().Add(45 * time.Second).UTC(), "process_ended": false, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
	return result, err
}

// HeartbeatTransfer renews cancellation cleanup under the same live fence. The
// cancellation signal is returned after commit; object write gates stay closed.
func (s *TransferStore) HeartbeatTransfer(ctx context.Context, lease application.TransferLease) error {
	cancelled := false
	err := s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		cancelled = row.CancellationRequested
		return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "worker_fence": lease.Fence}, map[string]any{"lease_until": s.clock().Add(45 * time.Second).UTC(), "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
	if err != nil {
		return err
	}
	if cancelled {
		return domain.ErrTransferCancelled
	}
	return nil
}

type transferItemRow struct {
	JobID                        uuid.UUID
	ItemIndex                    int
	SourceItemID, TargetItemID   uuid.UUID
	SourceAssetID, TargetAssetID *uuid.UUID
	Frozen                       []byte
	Status                       domain.TransferItemStatus
	FailureCode                  *string
}

func (transferItemRow) TableName() string { return "media.transfer_item" }

func readTransferItem(tx *gorm.DB, id uuid.UUID, index int) (transferItemRow, application.FrozenTransferItem, error) {
	var row transferItemRow
	if index < 0 || index > 199 {
		return row, application.FrozenTransferItem{}, application.ErrNotFound
	}
	read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ?", id, index).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return row, application.FrozenTransferItem{}, application.ErrUnavailable
	}
	if read.Error != nil {
		return row, application.FrozenTransferItem{}, read.Error
	}
	var item application.FrozenTransferItem
	if strictCopyMedia(row.Frozen, &item) != nil {
		return row, item, application.ErrUnavailable
	}
	return row, item, nil
}

func validateTransferSource(tx *gorm.DB, actor identityapp.Principal, row transferJobRow, source libraryRow, item application.FrozenTransferItem) error {
	scope, _ := row.scopes()
	if row.SourceOrgID == nil || row.SourceScopeID == nil {
		return application.ErrNotFound
	}
	actor.OrgID, actor.ScopeID = *row.SourceOrgID, *row.SourceScopeID
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
	var folder libraryFolderRow
	read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND library_id = ? AND revision = ?", *row.TargetFolderID, target.ID, row.TargetFolderRevision).Take(&folder)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return &application.TransferItemError{Code: "target_folder_changed"}
	}
	if read.Error != nil {
		return read.Error
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
		if state.Status == domain.TransferItemStatusSucceeded {
			done = true
			return nil
		}
		if err := validateTransferSource(tx, lease.Actor, row, source, item); err != nil {
			return err
		}
		if err := validateTransferFolder(tx, row, target); err != nil {
			return err
		}
		return updateOne(tx, "media.transfer_item", map[string]any{"job_id": row.ID, "item_index": index, "status": []domain.TransferItemStatus{domain.TransferItemStatusQueued, domain.TransferItemStatusNeedsReconciliation, domain.TransferItemStatusRunning}}, map[string]any{"status": domain.TransferItemStatusRunning, "failure_code": nil}, application.ErrLibraryConflict)
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
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var row transferJobRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND attempt = ? AND worker_fence = ? AND execution_id = ? AND org_id = ? AND actor_id = ?", lease.Work.JobID, lease.Work.Attempt, lease.Fence, execution, lease.Actor.OrgID, lease.Actor.ID).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return application.ErrLibraryConflict
		}
		if read.Error != nil {
			return read.Error
		}
		return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "revision": row.Revision}, map[string]any{"process_ended": true, "execution_unconfirmed": false, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
}

// FailTransferItem keeps unknown outcomes distinct from a completed failure.
func (s *TransferStore) FailTransferItem(ctx context.Context, lease application.TransferLease, index int, code string, unknown bool) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == domain.TransferItemStatusSucceeded {
			return domain.ErrTransferConflict
		}
		status := domain.TransferItemStatusFailed
		if unknown {
			status = domain.TransferItemStatusNeedsReconciliation
		} else if code == domain.TransferFailureCancelled {
			if row.Status == domain.TransferStatusCancelRequested {
				status = domain.TransferItemStatusCancelled
			} else {
				code = "worker_interrupted"
			}
		}
		if err := updateOne(tx, "media.transfer_item", map[string]any{"job_id": row.ID, "item_index": index}, map[string]any{"status": status, "failure_code": code}, application.ErrLibraryConflict); err != nil {
			return err
		}
		if unknown {
			return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "worker_fence": lease.Fence}, map[string]any{"needs_reconciliation": true, "stage": domain.TransferStageCleanup, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
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
			if item.Status == domain.TransferItemStatusSucceeded {
				succeeded++
			}
			unknown = unknown || item.Status == domain.TransferItemStatusNeedsReconciliation
		}
		if !unknown {
			pending := tx.Table("media.transfer_object AS o").Joins("JOIN media.transfer_item AS i ON i.job_id = o.job_id AND i.item_index = o.item_index").Where("o.job_id = ? AND i.status <> ? AND o.status <> ?", row.ID, domain.TransferItemStatusSucceeded, application.ProjectCopyObjectRemoved)
			unknown, err = storageExists(pending)
			if err != nil {
				return err
			}
		}
		status := domain.TransferStatusFailed
		switch {
		case unknown:
			status = domain.TransferStatusNeedsReconciliation
		case succeeded == len(job.Items):
			status = domain.TransferStatusSucceeded
		case succeeded > 0:
			status = domain.TransferStatusPartialFailed
		case row.Status == domain.TransferStatusCancelRequested:
			status = domain.TransferStatusCancelled
		}
		if row.CancellationRequested && !unknown {
			if err := tx.Model(&transferItemRow{}).Where("job_id = ? AND status <> ?", row.ID, domain.TransferItemStatusSucceeded).Updates(map[string]any{"status": domain.TransferItemStatusCancelled, "failure_code": domain.TransferFailureCancelled}).Error; err != nil {
				return err
			}
		}
		stage := domain.TransferStageCompleted
		if unknown {
			stage = domain.TransferStageCleanup
		}
		if err := updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "worker_fence": lease.Fence}, map[string]any{"status": status, "stage": stage, "needs_reconciliation": unknown, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
			return err
		}
		row, err = s.readJob(tx, lease.Actor, row.ID, false, true)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		return err
	})
	return result, err
}
