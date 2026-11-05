package postgres

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func insertTransferAsset(tx *gorm.DB, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if asset.Validate() != nil || asset.IsDelete || asset.Status != domain.StatusReady || asset.ModerationStatus != domain.ModerationPassed || asset.ContainsRealPerson || asset.ConsentRecordID != nil {
		return application.ErrProjectCopyMediaUnavailable
	}
	record := mediaAssetRecordFromDomain(asset)
	if err := tx.Create(&record).Error; err != nil {
		return err
	}
	for _, r := range renditions {
		if r.Validate() != nil || r.MediaAssetID != asset.ID || r.IsDelete {
			return application.ErrProjectCopyMediaUnavailable
		}
		record := mediaRenditionRecordFromDomain(r)
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

// PublishTransferItem atomically registers a complete independently owned item.
// The source is unchanged; failed/unknown objects never enter formal media reads.
func (s *TransferStore) PublishTransferItem(ctx context.Context, lease application.TransferLease, index int) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, source, target libraryRow) error {
		state, initial, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == domain.TransferItemStatusSucceeded {
			return nil
		}
		if state.Status != domain.TransferItemStatusRunning {
			return domain.ErrTransferConflict
		}
		output, item, err := readTransferAttempt(tx, row, index, initial)
		if err != nil {
			return err
		}
		if err := validateTransferSource(tx, lease.Actor, row, source, item); err != nil {
			return err
		}
		if err := validateTransferFolder(tx, row, target); err != nil {
			return err
		}
		var objects []transferObjectRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ? AND output_attempt = ?", row.ID, index, output.OutputAttempt).Order("rendition_kind").Find(&objects).Error; err != nil {
			return err
		}
		if len(objects) != len(item.Objects) {
			return application.ErrObjectMismatch
		}
		byKey := make(map[string]transferObjectRow, len(objects))
		for _, object := range objects {
			if object.Status != application.ProjectCopyObjectVerified || !object.SourceVerified || !object.WriteStarted || object.SHA256 == nil || object.ByteSize == nil || !validTransferDigest(*object.SHA256, *object.ByteSize) {
				return application.ErrObjectMismatch
			}
			matches := false
			for _, frozen := range item.Objects {
				matches = matches || transferObjectMatches(object, frozen)
			}
			if !matches {
				return application.ErrObjectMismatch
			}
			byKey[object.TargetObjectKey] = object
		}
		if len(item.Objects) > 0 && item.Objects[0].TargetLocation != nil {
			if err := acceptCopyLocations(ctx, tx, transferWorkOwner(row.ID, row.TargetLibraryID, index, output.OutputAttempt), item.Objects); err != nil {
				return err
			}
		}
		if item.TargetAsset != nil {
			a := item.TargetAsset
			if a.ObjectLocation != nil {
				actual, err := NewStorageRoutes(tx).ReadLocation(ctx, a.ObjectLocation.ID)
				if err != nil {
					return err
				}
				a.ObjectLocation = &actual
			}
			object, ok := byKey[a.ObjectKey]
			if !ok || object.RenditionKind != "" || object.SHA256 == nil || a.SHA256 == nil || *object.SHA256 != *a.SHA256 || object.ByteSize == nil || *object.ByteSize != a.ByteSize {
				return application.ErrObjectMismatch
			}
			for i := range item.TargetRenditions {
				r := &item.TargetRenditions[i]
				object, ok := byKey[r.ObjectKey]
				if !ok || object.RenditionKind != string(r.Kind) {
					return application.ErrObjectMismatch
				}
				r.ByteSize = object.ByteSize
				if r.ObjectLocation != nil {
					actual, err := NewStorageRoutes(tx).ReadLocation(ctx, r.ObjectLocation.ID)
					if err != nil {
						return err
					}
					r.ObjectLocation = &actual
				}
			}
			if err := insertTransferAsset(tx, *a, item.TargetRenditions); err != nil {
				return &application.TransferItemError{Code: "registration_failed", Cause: err}
			}
		}
		if item.TargetItem.LibraryID != target.ID || item.TargetItem.ID == item.SourceItem.ID || item.TargetItem.Validate() != nil {
			return application.ErrUnavailable
		}
		if err := persistLibraryItem(tx, item.TargetItem, 0); err != nil {
			return &application.TransferItemError{Code: "registration_failed", Cause: err}
		}
		if err := updateOne(tx, "media.library", map[string]any{"id": target.ID, "revision": target.Revision}, map[string]any{"revision": target.Revision + 1, "update_time": s.clock().UTC()}, application.ErrLibraryConflict); err != nil {
			return err
		}
		_, targetScope := row.scopes()
		if targetScope.Kind == domain.LibraryProject {
			owner := s.project(tx)
			facts, err := owner.Authorize(ctx, lease.Actor, *targetScope.ProjectID, true)
			if err != nil {
				return err
			}
			if _, err := owner.TouchContent(ctx, lease.Actor, facts.ProjectID, facts.Revision); err != nil {
				return err
			}
		}
		if err := updateOne(tx, "media.transfer_item", map[string]any{"job_id": row.ID, "item_index": index, "status": domain.TransferItemStatusRunning}, map[string]any{"status": domain.TransferItemStatusSucceeded, "failure_code": nil}, application.ErrLibraryConflict); err != nil {
			return err
		}
		return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "worker_fence": lease.Fence}, map[string]any{"stage": domain.TransferStageRegistering, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
}

var _ application.TransferWorkerRepository = (*TransferStore)(nil)
