package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type storageWorkRow struct {
	ID, OwnerID, WorkID               uuid.UUID
	Kind                              application.StorageWorkKind
	ProfileID, Bucket, ConfigRevision string
	State                             application.StorageWorkState
	ClosedAt                          *time.Time
	CreatedAt                         time.Time
}

func (storageWorkRow) TableName() string { return "media.storage_work_intent" }

func (r storageWorkRow) intent() application.StorageWorkIntent {
	return application.StorageWorkIntent{ID: r.ID, Owner: application.StorageWorkOwner{Kind: r.Kind, OwnerID: r.OwnerID, WorkID: r.WorkID}, ProfileID: r.ProfileID, Bucket: r.Bucket, ConfigRevision: r.ConfigRevision, State: r.State}
}

func readStorageWork(tx *gorm.DB, owner application.StorageWorkOwner) (application.StorageWorkIntent, bool, error) {
	var row storageWorkRow
	result := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("kind = ? AND owner_id = ? AND work_id = ?", owner.Kind, owner.OwnerID, owner.WorkID).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return application.StorageWorkIntent{}, false, nil
	}
	return row.intent(), result.RowsAffected == 1, result.Error
}

func readStorageWorkByID(tx *gorm.DB, id uuid.UUID, strength string) (storageWorkRow, error) {
	var row storageWorkRow
	if id == uuid.Nil {
		return row, application.ErrStorageRouteConflict
	}
	result := tx.Where("id = ?", id)
	if strength != "" {
		result = result.Clauses(clause.Locking{Strength: strength})
	}
	if err := result.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, application.ErrStorageRouteConflict
		}
		return row, err
	}
	return row, nil
}

// PinStorageWork is part of the caller's authorized admission transaction. A
// retry returns the original intent even if the environment route later changes.
func (s *StorageRoutes) PinStorageWork(ctx context.Context, owner application.StorageWorkOwner) (application.StorageWorkIntent, error) {
	var intent application.StorageWorkIntent
	if s == nil || s.db == nil {
		return intent, application.ErrUnavailable
	}
	if err := owner.Validate(); err != nil {
		return intent, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prior, found, err := readStorageWork(tx, owner)
		if err != nil {
			return err
		}
		if found {
			intent = prior
			return nil
		}
		route, err := readStorageRoute(tx, true)
		if err != nil {
			return err
		}
		candidate := storageWorkRow{ID: uuid.New(), Kind: owner.Kind, OwnerID: owner.OwnerID, WorkID: owner.WorkID, ProfileID: route.ProfileID, Bucket: route.Bucket, ConfigRevision: route.Revision, State: application.StorageWorkOpen}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}, {Name: "owner_id"}, {Name: "work_id"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		intent, found, err = readStorageWork(tx, owner)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrUnavailable
		}
		return nil
	})
	return intent, err
}

// ReadStorageWork never creates a new target during execution or recovery.
func (s *StorageRoutes) ReadStorageWork(ctx context.Context, owner application.StorageWorkOwner) (application.StorageWorkIntent, error) {
	if s == nil || s.db == nil {
		return application.StorageWorkIntent{}, application.ErrUnavailable
	}
	if err := owner.Validate(); err != nil {
		return application.StorageWorkIntent{}, err
	}
	intent, found, err := readStorageWork(s.db.WithContext(ctx), owner)
	if err != nil {
		return intent, err
	}
	if !found {
		return intent, application.ErrStorageNotReady
	}
	return intent, nil
}

// ReserveWorkLocation records ownership on the frozen target before SDK writes.
func (s *StorageRoutes) ReserveWorkLocation(ctx context.Context, id uuid.UUID, key string) (domain.ObjectLocation, error) {
	var location domain.ObjectLocation
	if s == nil || s.db == nil {
		return location, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "SHARE")
		if err != nil {
			return err
		}
		if intent.State != application.StorageWorkOpen {
			return application.ErrStorageRouteConflict
		}
		ref := domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}
		if err := ref.Validate(); err != nil {
			return err
		}
		candidate := objectLocationRow{ID: uuid.New(), ProfileID: ref.ProfileID, Bucket: ref.Bucket, ObjectKey: ref.Key, State: domain.ObjectLocationReserved, OwnerIntentID: &id}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "profile_id"}, {Name: "bucket"}, {Name: "object_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		var row objectLocationRow
		if err := tx.Where("profile_id = ? AND bucket = ? AND object_key = ?", ref.ProfileID, ref.Bucket, ref.Key).Take(&row).Error; err != nil {
			return err
		}
		if row.OwnerIntentID == nil || *row.OwnerIntentID != id {
			return application.ErrStorageRouteConflict
		}
		location = row.domain()
		return nil
	})
	return location, err
}

// BeginWorkWrite records the bounded execution deadline before SDK I/O.
func (s *StorageRoutes) BeginWorkWrite(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	deadline, bounded := ctx.Deadline()
	if !bounded || ctx.Err() != nil || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Minute || location.ID == uuid.Nil || location.Ref.Validate() != nil {
		return application.ErrStorageRouteConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "SHARE")
		if err != nil {
			return err
		}
		if intent.State != application.StorageWorkOpen {
			return application.ErrStorageRouteConflict
		}
		var row objectLocationRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state IN ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationUnknown}).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrStorageRouteConflict
			}
			return read.Error
		}
		if row.WriteUntil != nil && row.WriteUntil.After(deadline) {
			deadline = *row.WriteUntil
		}
		row.WriteUntil = &deadline
		return tx.Model(&row).Select("write_until").Updates(&row).Error
	})
}

// ConfirmWorkLocation records verified metadata only while the task remains open.
func (s *StorageRoutes) ConfirmWorkLocation(ctx context.Context, id uuid.UUID, location domain.ObjectLocation, info application.ObjectInfo) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if location.ID == uuid.Nil || location.Ref.Validate() != nil || info.Size < 0 || info.Size > 5<<30 || info.ContentType == "" || !storageDigest(info.SHA256) {
		return application.ErrObjectMismatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "SHARE")
		if err != nil {
			return err
		}
		if intent.State != application.StorageWorkOpen {
			return application.ErrStorageRouteConflict
		}
		var row objectLocationRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state IN ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationUnknown}).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrStorageRouteConflict
			}
			return read.Error
		}
		if row.ByteSize != nil && *row.ByteSize != info.Size || row.SHA256 != nil && *row.SHA256 != info.SHA256 || row.MIMEType != nil && *row.MIMEType != info.ContentType {
			return application.ErrStorageRouteConflict
		}
		row.State, row.ByteSize, row.SHA256, row.MIMEType = domain.ObjectLocationPresent, &info.Size, &info.SHA256, &info.ContentType
		return tx.Model(&row).Select("state", "byte_size", "sha256", "mime_type").Updates(&row).Error
	})
}

// AcceptStorageWork atomically accepts the exact complete output set.
func (s *StorageRoutes) AcceptStorageWork(ctx context.Context, id uuid.UUID, ids []uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if len(ids) < 1 || len(ids) > 15000 {
		return application.ErrStorageRouteConflict
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, object := range ids {
		if object == uuid.Nil || seen[object] {
			return application.ErrStorageRouteConflict
		}
		seen[object] = true
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if intent.State == application.StorageWorkCancelled {
			return application.ErrStorageRouteConflict
		}
		var rows []objectLocationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_intent_id = ?", id).Order("id").Limit(15001).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return application.ErrStorageRouteConflict
		}
		for _, row := range rows {
			if !seen[row.ID] || row.State != domain.ObjectLocationPresent && row.State != domain.ObjectLocationAccepted {
				return application.ErrStorageRouteConflict
			}
		}
		if err := tx.Model(&objectLocationRow{}).Where("owner_intent_id = ?", id).Update("state", domain.ObjectLocationAccepted).Error; err != nil {
			return err
		}
		intent.State = application.StorageWorkAccepted
		if intent.ClosedAt == nil {
			now := time.Now().UTC()
			intent.ClosedAt = &now
		}
		return tx.Model(&intent).Select("state", "closed_at").Updates(&intent).Error
	})
}

// CancelStorageWork closes future writes without declaring unknown bytes removed.
func (s *StorageRoutes) CancelStorageWork(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if intent.State == application.StorageWorkCancelled {
			return nil
		}
		if intent.State != application.StorageWorkOpen {
			return application.ErrStorageRouteConflict
		}
		intent.State = application.StorageWorkCancelled
		if intent.ClosedAt == nil {
			now := time.Now().UTC()
			intent.ClosedAt = &now
		}
		return tx.Model(&intent).Select("state", "closed_at").Updates(&intent).Error
	})
}

// BeginWorkRemoval requires cancellation and drains the bounded provider write.
func (s *StorageRoutes) BeginWorkRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	removable := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if intent.State != application.StorageWorkCancelled || intent.ClosedAt == nil {
			return nil
		}
		var locations []objectLocationRow
		if err := tx.Where("owner_intent_id = ?", id).Find(&locations).Error; err != nil {
			return err
		}
		hasWriteFence := false
		for _, row := range locations {
			hasWriteFence = hasWriteFence || row.WriteUntil != nil
		}
		if hasWriteFence && time.Since(*intent.ClosedAt) < 10*time.Minute {
			return nil
		}
		var row objectLocationRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state IN ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationUnknown, domain.ObjectLocationDeletePending}).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return nil
			}
			return read.Error
		}
		if row.WriteUntil != nil && !row.WriteUntil.Before(time.Now().UTC()) {
			return nil
		}
		for _, column := range storageOwnerColumns {
			live, err := hasLiveStorageReference(tx, column, location.ID)
			if err != nil {
				return err
			}
			if live {
				private, err := privateWorkReference(tx, column, location, intent)
				if err != nil {
					return err
				}
				if !private {
					return nil
				}
			}
		}
		row.State = domain.ObjectLocationDeletePending
		if err := tx.Model(&row).Select("state").Updates(&row).Error; err != nil {
			return err
		}
		removable = true
		return nil
	})
	return removable, err
}

// ConfirmWorkRemoval clears debt only for the same fenced cancelled work object.
func (s *StorageRoutes) ConfirmWorkRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	result := s.db.WithContext(ctx).Model(&objectLocationRow{}).
		Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state IN ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationDeletePending, domain.ObjectLocationDeleted}).
		Update("state", domain.ObjectLocationDeleted)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrStorageRouteConflict
	}
	return nil
}

// BeginAcceptedWorkRemoval retires one completed private object after its owner expires it.
func (s *StorageRoutes) BeginAcceptedWorkRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil || id == uuid.Nil || location.ID == uuid.Nil || location.Ref.Validate() != nil {
		return false, application.ErrUnavailable
	}
	removable := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		intent, err := readStorageWorkByID(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if intent.State != application.StorageWorkAccepted {
			return application.ErrStorageRouteConflict
		}
		var row objectLocationRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state IN ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationAccepted, domain.ObjectLocationDeletePending}).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return nil
			}
			return read.Error
		}
		if row.WriteUntil != nil && !row.WriteUntil.Before(time.Now().UTC()) {
			return nil
		}
		row.State = domain.ObjectLocationDeletePending
		if err := tx.Model(&row).Select("state").Updates(&row).Error; err != nil {
			return err
		}
		removable = true
		return nil
	})
	return removable, err
}
