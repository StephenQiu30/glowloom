package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// PurgeObjects returns only exact persisted keys under a currently live fence.
func (s *PurgeStore) PurgeObjects(ctx context.Context, lease application.PurgeLease, index int) ([]application.PurgeObject, error) {
	var result []application.PurgeObject
	err := s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, _ libraryRow, _ application.LibraryProjectFacts) error {
		state, frozen, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status != domain.PurgeItemStatusRunning && state.Status != domain.PurgeItemStatusNeedsReconciliation {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		if err := s.requirePurgeUnreferenced(ctx, tx, lease.Actor, row, frozen); err != nil {
			return err
		}
		result = make([]application.PurgeObject, 0, 17)
		var rows []purgeObjectRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ?", row.ID, index).Order("rendition_kind").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			result = append(result, r.object())
		}
		if err := loadPurgeLocations(tx, row.ID, index, result); err != nil {
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
		if state.Status != domain.PurgeItemStatusRunning && state.Status != domain.PurgeItemStatusNeedsReconciliation {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		if err := s.requirePurgeUnreferenced(ctx, tx, lease.Actor, row, frozen); err != nil {
			return err
		}
		allowed := frozen.Asset != nil && frozen.Asset.ObjectKey == key
		for _, r := range frozen.Renditions {
			allowed = allowed || r.ObjectKey == key
		}
		if !allowed {
			return application.ErrObjectMismatch
		}
		var objectRow purgeObjectRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ? AND object_key = ?", row.ID, index, key).Take(&objectRow)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrObjectMismatch
			}
			return read.Error
		}
		object := objectRow.object()
		if err := loadPurgeLocations(tx, row.ID, index, []application.PurgeObject{object}); err != nil {
			return err
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
		return updatePurgeObject(tx, row.ID, index, key, map[string]any{"sha256": sha, "byte_size": size, "verified": true})
	})
}

// BeginPurgeRemoval commits exact uncertainty before object-storage mutation.
func (s *PurgeStore) BeginPurgeRemoval(ctx context.Context, lease application.PurgeLease, index int, key string) error {
	return s.withObject(ctx, lease, index, key, func(tx *gorm.DB, row purgeJobRow, object application.PurgeObject) error {
		if row.CancellationRequested {
			var startedCount int64
			if err := tx.Model(&purgeObjectRow{}).Where("job_id = ? AND item_index = ? AND removal_started = ?", row.ID, index, true).Limit(1).Count(&startedCount).Error; err != nil {
				return err
			}
			if startedCount == 0 {
				return domain.ErrPurgeCancelled
			}
		}
		if !object.Verified || object.SHA256 == nil || object.ByteSize == nil || object.RemovalStarted || object.Removed {
			return application.ErrObjectMismatch
		}
		var unverified bool
		var unverifiedCount int64
		if err := tx.Model(&purgeObjectRow{}).Where("job_id = ? AND item_index = ? AND verified = ?", row.ID, index, false).Limit(1).Count(&unverifiedCount).Error; err != nil {
			return err
		}
		unverified = unverifiedCount > 0
		if unverified {
			return application.ErrObjectMismatch
		}
		if err := updatePurgeObject(tx, row.ID, index, key, map[string]any{"removal_started": true}); err != nil {
			return err
		}
		return updatePurgeJob(tx, row.ID, nil, map[string]any{"stage": domain.PurgeStageRemoving, "updated_at": s.clock().UTC()})
	})
}

// ConfirmPurgeRemoval records only absence after a persisted owning intent.
func (s *PurgeStore) ConfirmPurgeRemoval(ctx context.Context, lease application.PurgeLease, index int, key string) error {
	return s.withObject(ctx, lease, index, key, func(tx *gorm.DB, row purgeJobRow, object application.PurgeObject) error {
		if !object.Verified || !object.RemovalStarted {
			return application.ErrObjectMismatch
		}
		return updatePurgeObject(tx, row.ID, index, key, map[string]any{"removed": true})
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
		if state.Status != domain.PurgeItemStatusRunning && state.Status != domain.PurgeItemStatusNeedsReconciliation {
			return domain.ErrPurgeConflict
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		if err := s.requirePurgeUnreferenced(ctx, tx, lease.Actor, row, frozen); err != nil {
			return err
		}
		var objects []purgeObjectRow
		if err := tx.Where("job_id = ? AND item_index = ?", row.ID, index).Find(&objects).Error; err != nil {
			return err
		}
		expected := 0
		if frozen.Asset != nil {
			expected = len(frozen.Renditions) + 1
		}
		unsafe := false
		for _, object := range objects {
			unsafe = unsafe || !object.Removed
		}
		if unsafe || len(objects) != expected {
			return domain.ErrPurgeConflict
		}
		now := s.clock().UTC()
		var item libraryItemRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND library_id = ? AND revision = ? AND purged_at IS NULL", frozen.ItemID, library.ID, frozen.CatalogRevision).Take(&item)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrLibraryConflict
			}
			return read.Error
		}
		updates := map[string]any{"folder_id": nil, "title": "已永久清理", "tags": jsonDocument([]byte("[]")), "source_label": "", "note": "", "favorite": false, "catalog_state": domain.LibraryItemStateRemoved, "trashed_at": nil, "purged_at": now, "revision": item.Revision + 1, "update_time": now}
		if item.PlainText != nil {
			updates["plain_text"] = ""
		}
		if err := updateOne(tx, "media.library_item", map[string]any{"id": frozen.ItemID, "library_id": library.ID, "revision": frozen.CatalogRevision, "purged_at": nil}, updates, application.ErrLibraryConflict); err != nil {
			return err
		}
		if frozen.Asset != nil {
			if err := tx.Model(&renditionRow{}).Where("media_asset_id = ? AND is_delete = ?", frozen.Asset.ID, false).Updates(map[string]any{"is_delete": true, "update_time": now}).Error; err != nil {
				return err
			}
		}
		if err := updateOne(tx, "media.library", map[string]any{"id": library.ID, "revision": library.Revision}, map[string]any{"revision": library.Revision + 1, "update_time": now}, application.ErrLibraryConflict); err != nil {
			return err
		}
		if row.ProjectID != nil {
			if _, err := s.project(tx).TouchContent(ctx, lease.Actor, facts.ProjectID, facts.Revision); err != nil {
				return err
			}
		}
		return updateOne(tx, "media.purge_item", map[string]any{"job_id": row.ID, "item_index": index}, map[string]any{"status": domain.PurgeItemStatusSucceeded, "failure_code": nil}, application.ErrLibraryConflict)
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
			case domain.PurgeItemStatusSucceeded:
				success++
			case domain.PurgeItemStatusBlocked:
				blocked++
			case domain.PurgeItemStatusCancelled:
			default:
				unsettled = true
			}
		}
		status, stage := domain.PurgeStatusFailed, domain.PurgeStageCompleted
		switch {
		case unsettled:
			status, stage = domain.PurgeStatusNeedsReconciliation, domain.PurgeStageRemoving
		case success == len(job.Items):
			status = domain.PurgeStatusSucceeded
		case success > 0:
			status = domain.PurgeStatusPartialFailed
		case row.CancellationRequested:
			status = domain.PurgeStatusCancelled
		case blocked == len(job.Items):
			status = domain.PurgeStatusFailed
		}
		if err := updateOne(tx, "media.purge_job", map[string]any{"id": row.ID, "worker_fence": lease.Fence}, map[string]any{"status": status, "stage": stage, "needs_reconciliation": unsettled, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
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

func loadPurgeLocations(tx *gorm.DB, job uuid.UUID, index int, objects []application.PurgeObject) error {
	for i := range objects {
		var row purgeObjectRow
		if err := tx.Model(&purgeObjectRow{}).Select("object_location_id").Where("job_id = ? AND item_index = ? AND object_key = ?", job, index, objects[i].ObjectKey).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrPurgeConflict
			}
			return err
		}
		if row.ObjectLocationID == nil {
			return domain.ErrPurgeConflict
		}
		l, err := NewStorageRoutes(tx).ReadLocation(tx.Statement.Context, *row.ObjectLocationID)
		if err != nil {
			return err
		}
		if l.Ref.Key != objects[i].ObjectKey {
			return application.ErrObjectMismatch
		}
		objects[i].Location = &l
	}
	return nil
}

func locationID(l *domain.ObjectLocation) *uuid.UUID {
	if l == nil {
		return nil
	}
	id := l.ID
	return &id
}

type purgeObjectRow struct {
	JobID                             uuid.UUID
	ItemIndex                         int
	ObjectLocationID                  *uuid.UUID
	ObjectKey                         string
	RenditionKind                     string
	ByteSize                          *int64
	SHA256                            *string
	Verified, RemovalStarted, Removed bool
}

func (purgeObjectRow) TableName() string { return "media.purge_object" }

func updatePurgeObject(tx *gorm.DB, jobID uuid.UUID, index int, key string, values map[string]any) error {
	return updateOne(tx, "media.purge_object", map[string]any{"job_id": jobID, "item_index": index, "object_key": key}, values, application.ErrLibraryConflict)
}

func updatePurgeJob(tx *gorm.DB, id uuid.UUID, filters map[string]any, values map[string]any) error {
	where := map[string]any{"id": id}
	for key, value := range filters {
		where[key] = value
	}
	return updateOne(tx, "media.purge_job", where, values, application.ErrLibraryConflict)
}

func (r purgeObjectRow) object() application.PurgeObject {
	return application.PurgeObject{ObjectKey: r.ObjectKey, ByteSize: r.ByteSize, SHA256: r.SHA256, Verified: r.Verified, RemovalStarted: r.RemovalStarted, Removed: r.Removed}
}
