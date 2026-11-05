package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func transferWorkOwner(job, library uuid.UUID, index, attempt int) application.StorageWorkOwner {
	return application.StorageWorkOwner{Kind: application.StorageWorkKindTransfer, OwnerID: library, WorkID: uuid.NewSHA1(job, []byte("item/"+strconv.Itoa(index)+"/attempt/"+strconv.Itoa(attempt)))}
}
func projectCopyWorkOwner(target, snapshot uuid.UUID) application.StorageWorkOwner {
	return application.StorageWorkOwner{Kind: application.StorageWorkKindProjectCopy, OwnerID: target, WorkID: snapshot}
}

// bindCopyLocations reserves the complete item before its immutable manifest is
// saved. Source facts must already identify an accepted original, never a default.
func bindCopyLocations(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, objects []application.ProjectCopyObject) error {
	if len(objects) == 0 {
		return nil
	}
	routes := NewStorageRoutes(tx)
	intent, err := routes.PinStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	if intent.State != application.StorageWorkOpen {
		return application.ErrStorageRouteConflict
	}
	for i := range objects {
		o := &objects[i]
		if o.SourceLocation == nil || o.SourceLocation.ID == uuid.Nil || o.SourceLocation.Ref.Validate() != nil || o.SourceLocation.Ref.Key != o.SourceObjectKey || o.SourceLocation.State != domain.ObjectLocationAccepted {
			return application.ErrStorageNotReady
		}
		target, err := routes.ReserveWorkLocation(ctx, intent.ID, o.TargetObjectKey)
		if err != nil {
			return err
		}
		o.TargetLocation = &target
	}
	return nil
}
func copyTargetLocation(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, id *uuid.UUID, key string) (application.StorageWorkIntent, domain.ObjectLocation, error) {
	if id == nil {
		return application.StorageWorkIntent{}, domain.ObjectLocation{}, application.ErrStorageNotReady
	}
	routes := NewStorageRoutes(tx)
	intent, err := routes.ReadStorageWork(ctx, owner)
	if err != nil {
		return intent, domain.ObjectLocation{}, err
	}
	var row objectLocationRow
	err = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND owner_intent_id = ? AND object_key = ? AND profile_id = ? AND bucket = ?", *id, intent.ID, key, intent.ProfileID, intent.Bucket).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return intent, domain.ObjectLocation{}, application.ErrStorageRouteConflict
	}
	if err != nil {
		return intent, domain.ObjectLocation{}, err
	}
	return intent, row.domain(), nil
}
func beginCopyLocation(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, id *uuid.UUID, key string) error {
	intent, location, err := copyTargetLocation(ctx, tx, owner, id, key)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return NewStorageRoutes(tx).BeginWorkWrite(bounded, intent.ID, location)
}
func confirmCopyLocation(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, id *uuid.UUID, key, mime, sha string, size int64) error {
	intent, location, err := copyTargetLocation(ctx, tx, owner, id, key)
	if err != nil {
		return err
	}
	if location.State == domain.ObjectLocationAccepted {
		if location.ByteSize == nil || *location.ByteSize != size || location.SHA256 == nil || *location.SHA256 != sha || location.MIMEType == nil || *location.MIMEType != mime {
			return application.ErrObjectMismatch
		}
		return nil
	}
	return NewStorageRoutes(tx).ConfirmWorkLocation(ctx, intent.ID, location, application.ObjectInfo{Size: size, ContentType: mime, SHA256: sha})
}
func acceptCopyLocations(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, objects []application.ProjectCopyObject) error {
	if len(objects) == 0 {
		return nil
	}
	intent, err := NewStorageRoutes(tx).ReadStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(objects))
	for _, o := range objects {
		if o.TargetLocation == nil {
			return application.ErrStorageNotReady
		}
		ids = append(ids, o.TargetLocation.ID)
	}
	return NewStorageRoutes(tx).AcceptStorageWork(ctx, intent.ID, ids)
}
func attachCopyAssetLocations(assets []application.ProjectCopySourceAsset, objects []application.ProjectCopyObject) error {
	byKey := make(map[string]*domain.ObjectLocation, len(objects))
	for _, o := range objects {
		byKey[o.TargetObjectKey] = o.TargetLocation
	}
	for i := range assets {
		a := &assets[i]
		if byKey[a.Asset.ObjectKey] == nil {
			return application.ErrStorageNotReady
		}
		a.Asset.ObjectLocation = byKey[a.Asset.ObjectKey]
		for j := range a.Renditions {
			if byKey[a.Renditions[j].ObjectKey] == nil {
				return application.ErrStorageNotReady
			}
			a.Renditions[j].ObjectLocation = byKey[a.Renditions[j].ObjectKey]
		}
	}
	return nil
}
func hydrateCopySourceAsset(tx *gorm.DB, row *assetRow) error {
	if err := row.loadLocation(tx); err != nil {
		return err
	}
	if row.location != nil && row.SHA256 == nil {
		row.SHA256 = row.location.SHA256
	}
	return nil
}
func hydrateCopySourceRendition(tx *gorm.DB, row *renditionRow) error {
	if err := row.loadLocation(tx); err != nil {
		return err
	}
	if row.location != nil && row.ByteSize == nil {
		row.ByteSize = row.location.ByteSize
	}
	return nil
}

func beginCopyRemoval(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, id *uuid.UUID, key string) error {
	intent, location, err := copyTargetLocation(ctx, tx, owner, id, key)
	if err != nil {
		return err
	}
	if location.State == domain.ObjectLocationDeleted {
		return nil
	}
	routes := NewStorageRoutes(tx)
	var allowed bool
	if intent.State == application.StorageWorkAccepted {
		allowed, err = routes.BeginPrivateCopyRemoval(ctx, intent.ID, location)
	} else {
		if err = routes.CancelStorageWork(ctx, intent.ID); err != nil {
			return err
		}
		allowed, err = routes.BeginWorkRemoval(ctx, intent.ID, location)
	}
	if err != nil {
		return err
	}
	// Preserve the cancellation/debt transaction even while the write fence is
	// draining. The mandatory RoutedCopyObjects checks removal permission again
	// immediately before SDK I/O and fails closed until delete_pending is granted.
	_ = allowed
	return nil
}
func confirmCopyRemoval(ctx context.Context, tx *gorm.DB, owner application.StorageWorkOwner, id *uuid.UUID, key string) error {
	intent, location, err := copyTargetLocation(ctx, tx, owner, id, key)
	if err != nil {
		return err
	}
	return NewStorageRoutes(tx).ConfirmWorkRemoval(ctx, intent.ID, location)
}

// ReadWorkLocationByRef never creates or reassigns an output during recovery.
func (s *StorageRoutes) ReadWorkLocationByRef(ctx context.Context, ref domain.ObjectRef) (domain.ObjectLocation, error) {
	if s == nil || s.db == nil || ref.Validate() != nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	var row objectLocationRow
	err := s.db.WithContext(ctx).Where("profile_id = ? AND bucket = ? AND object_key = ? AND owner_intent_id IS NOT NULL", ref.ProfileID, ref.Bucket, ref.Key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ObjectLocation{}, application.ErrStorageRouteConflict
	}
	if err != nil {
		return domain.ObjectLocation{}, err
	}
	return row.domain(), nil
}
