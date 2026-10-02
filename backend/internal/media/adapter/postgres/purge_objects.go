package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// PurgeObjects returns only exact persisted keys under a currently live fence.
func (s *PurgeStore) PurgeObjects(ctx context.Context, lease application.PurgeLease, index int) ([]application.PurgeObject, error) {
	var result []application.PurgeObject
	err := s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, _ libraryRow, _ application.LibraryProjectFacts) error {
		state, frozen, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status != "running" && state.Status != "needs_reconciliation" {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		result = make([]application.PurgeObject, 0, 17)
		if err := tx.Raw(`SELECT object_key,byte_size,sha256,verified,removal_started,removed FROM media.purge_object WHERE job_id=? AND item_index=? ORDER BY rendition_kind FOR UPDATE`, row.ID, index).Scan(&result).Error; err != nil {
			return err
		}
		if frozen.Asset == nil && len(result) != 0 || frozen.Asset != nil && len(result) != len(frozen.Renditions)+1 {
			return application.ErrUnavailable
		}
		if frozen.Asset != nil {
			keys := map[string]bool{frozen.Asset.ObjectKey: true}
			for _, r := range frozen.Renditions {
				keys[r.ObjectKey] = true
			}
			for _, object := range result {
				if !keys[object.ObjectKey] {
					return application.ErrObjectMismatch
				}
				delete(keys, object.ObjectKey)
			}
			if len(keys) != 0 {
				return application.ErrObjectMismatch
			}
		}
		return nil
	})
	return result, err
}

func (s *PurgeStore) withObject(ctx context.Context, lease application.PurgeLease, index int, key string, consume func(*gorm.DB, purgeJobRow, application.PurgeObject) error) error {
	return s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, _ libraryRow, _ application.LibraryProjectFacts) error {
		state, frozen, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status != "running" && state.Status != "needs_reconciliation" {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		allowed := frozen.Asset != nil && frozen.Asset.ObjectKey == key
		for _, r := range frozen.Renditions {
			allowed = allowed || r.ObjectKey == key
		}
		if !allowed {
			return application.ErrObjectMismatch
		}
		var object application.PurgeObject
		read := tx.Raw(`SELECT object_key,byte_size,sha256,verified,removal_started,removed FROM media.purge_object WHERE job_id=? AND item_index=? AND object_key=? FOR UPDATE`, row.ID, index, key).Scan(&object)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrObjectMismatch
		}
		return consume(tx, row, object)
	})
}

// RecordPurgeDigest closes actual original/rendition bytes before any deletion.
func (s *PurgeStore) RecordPurgeDigest(ctx context.Context, lease application.PurgeLease, index int, key, sha string, size int64) error {
	if !validTransferDigest(sha, size) {
		return application.ErrObjectMismatch
	}
	return s.withObject(ctx, lease, index, key, func(tx *gorm.DB, row purgeJobRow, object application.PurgeObject) error {
		if object.SHA256 != nil && *object.SHA256 != sha || object.ByteSize != nil && *object.ByteSize != size || object.Removed {
			return application.ErrObjectMismatch
		}
		return libraryChanged(tx, `UPDATE media.purge_object SET sha256=?,byte_size=?,verified=true WHERE job_id=? AND item_index=? AND object_key=?`, sha, size, row.ID, index, key)
	})
}

// BeginPurgeRemoval commits exact uncertainty before object-storage mutation.
func (s *PurgeStore) BeginPurgeRemoval(ctx context.Context, lease application.PurgeLease, index int, key string) error {
	return s.withObject(ctx, lease, index, key, func(tx *gorm.DB, row purgeJobRow, object application.PurgeObject) error {
		if row.CancellationRequested {
			var started bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_object WHERE job_id=? AND item_index=? AND removal_started)`, row.ID, index).Scan(&started).Error; err != nil {
				return err
			}
			if !started {
				return domain.ErrPurgeCancelled
			}
		}
		if !object.Verified || object.SHA256 == nil || object.ByteSize == nil || object.RemovalStarted || object.Removed {
			return application.ErrObjectMismatch
		}
		var unverified bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_object WHERE job_id=? AND item_index=? AND NOT verified)`, row.ID, index).Scan(&unverified).Error; err != nil {
			return err
		}
		if unverified {
			return application.ErrObjectMismatch
		}
		if err := libraryChanged(tx, `UPDATE media.purge_object SET removal_started=true WHERE job_id=? AND item_index=? AND object_key=?`, row.ID, index, key); err != nil {
			return err
		}
		return libraryChanged(tx, `UPDATE media.purge_job SET stage='removing',updated_at=? WHERE id=?`, s.clock().UTC(), row.ID)
	})
}

// ConfirmPurgeRemoval records only absence after a persisted owning intent.
func (s *PurgeStore) ConfirmPurgeRemoval(ctx context.Context, lease application.PurgeLease, index int, key string) error {
	return s.withObject(ctx, lease, index, key, func(tx *gorm.DB, row purgeJobRow, object application.PurgeObject) error {
		if !object.Verified || !object.RemovalStarted {
			return application.ErrObjectMismatch
		}
		return libraryChanged(tx, `UPDATE media.purge_object SET removed=true WHERE job_id=? AND item_index=? AND object_key=?`, row.ID, index, key)
	})
}

// CompletePurgeItem retires visible metadata only after every object's real
// absence receipt. No private source title or plain text enters the tombstone.
func (s *PurgeStore) CompletePurgeItem(ctx context.Context, lease application.PurgeLease, index int) error {
	return s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, library libraryRow, facts application.LibraryProjectFacts) error {
		state, frozen, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status != "running" && state.Status != "needs_reconciliation" {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		var proof struct {
			Count  int
			Unsafe bool
		}
		if err := tx.Raw(`SELECT count(*) AS count,COALESCE(bool_or(NOT removed),false) AS unsafe FROM media.purge_object WHERE job_id=? AND item_index=?`, row.ID, index).Scan(&proof).Error; err != nil {
			return err
		}
		expected := 0
		if frozen.Asset != nil {
			expected = len(frozen.Renditions) + 1
		}
		if proof.Unsafe || proof.Count != expected {
			return domain.ErrPurgeConflict
		}
		now := s.clock().UTC()
		if err := libraryChanged(tx, `UPDATE media.library_item SET plain_text=CASE WHEN plain_text IS NULL THEN NULL ELSE '' END,folder_id=NULL,title='已永久清理',tags='[]'::jsonb,source_label='',note='',favorite=false,catalog_state='removed',trashed_at=NULL,purged_at=?,revision=revision+1,update_time=? WHERE id=? AND library_id=? AND revision=? AND purged_at IS NULL`, now, now, frozen.ItemID, library.ID, frozen.CatalogRevision); err != nil {
			return err
		}
		if frozen.Asset != nil {
			if err := tx.Exec(`UPDATE media.rendition SET is_delete=true,update_time=? WHERE media_asset_id=? AND NOT is_delete`, now, frozen.Asset.ID).Error; err != nil {
				return err
			}
		}
		if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, now, library.ID, library.Revision); err != nil {
			return err
		}
		if row.ProjectID != nil {
			if _, err := s.project(tx).TouchContent(ctx, lease.Actor, facts.ProjectID, facts.Revision); err != nil {
				return err
			}
		}
		return libraryChanged(tx, `UPDATE media.purge_item SET status='succeeded',failure_code=NULL WHERE job_id=? AND item_index=?`, row.ID, index)
	})
}

// FinishPurge never reports success for a hidden but physically retained item.
func (s *PurgeStore) FinishPurge(ctx context.Context, lease application.PurgeLease) (domain.PurgeJob, error) {
	var result domain.PurgeJob
	err := s.withLease(ctx, lease, true, func(tx *gorm.DB, row purgeJobRow, _ libraryRow, _ application.LibraryProjectFacts) error {
		job, err := purgeView(tx, row)
		if err != nil {
			return err
		}
		success, blocked, unsettled := 0, 0, false
		for _, item := range job.Items {
			switch item.Status {
			case "succeeded":
				success++
			case "blocked":
				blocked++
			case "cancelled":
			default:
				unsettled = true
			}
		}
		status, stage := "failed", "completed"
		switch {
		case unsettled:
			status, stage = "needs_reconciliation", "removing"
		case success == len(job.Items):
			status = "succeeded"
		case success > 0:
			status = "partial_failed"
		case row.CancellationRequested:
			status = "cancelled"
		case blocked == len(job.Items):
			status = "failed"
		}
		if err := libraryChanged(tx, `UPDATE media.purge_job SET status=?,stage=?,needs_reconciliation=?,revision=revision+1,updated_at=? WHERE id=? AND worker_fence=?`, status, stage, unsettled, s.clock().UTC(), row.ID, lease.Fence); err != nil {
			return err
		}
		row, err = s.readJob(tx, lease.Actor, row.ID, false)
		if err != nil {
			return err
		}
		result, err = purgeView(tx, row)
		return err
	})
	return result, err
}

var _ application.PurgeWorkerRepository = (*PurgeStore)(nil)
