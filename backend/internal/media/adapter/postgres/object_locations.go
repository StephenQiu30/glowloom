package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type objectLocationRow struct {
	OwnerIntentID                *uuid.UUID
	OwnerSessionID               *uuid.UUID
	WriteUntil                   *time.Time
	ID                           uuid.UUID
	ProfileID, Bucket, ObjectKey string
	State                        domain.ObjectLocationState
	ByteSize                     *int64
	SHA256, MIMEType             *string
	CreatedAt, UpdatedAt         time.Time
}

func (objectLocationRow) TableName() string { return "media.object_location" }

func (r objectLocationRow) domain() domain.ObjectLocation {
	return domain.ObjectLocation{ID: r.ID, Ref: domain.ObjectRef{ProfileID: r.ProfileID, Bucket: r.Bucket, Key: r.ObjectKey}, State: r.State, ByteSize: r.ByteSize, SHA256: r.SHA256, MIMEType: r.MIMEType}
}

// ReserveLocation registers one immutable physical identity without claiming bytes exist.
func (s *StorageRoutes) ReserveLocation(ctx context.Context, ref domain.ObjectRef) (domain.ObjectLocation, error) {
	if s == nil || s.db == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return domain.ObjectLocation{}, err
	}
	query := s.db.WithContext(ctx)
	candidate := objectLocationRow{ID: uuid.New(), ProfileID: ref.ProfileID, Bucket: ref.Bucket, ObjectKey: ref.Key, State: domain.ObjectLocationReserved}
	if err := query.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "profile_id"}, {Name: "bucket"}, {Name: "object_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
		return domain.ObjectLocation{}, fmt.Errorf("reserve physical object location: %w", err)
	}
	var row objectLocationRow
	if err := query.Where("profile_id = ? AND bucket = ? AND object_key = ?", ref.ProfileID, ref.Bucket, ref.Key).Take(&row).Error; err != nil {
		return domain.ObjectLocation{}, fmt.Errorf("read reserved physical object location: %w", err)
	}
	if row.ID == uuid.Nil || row.domain().Ref != ref {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	return row.domain(), nil
}

// ReadLocation resolves a stable location ID without consulting the active route.
func (s *StorageRoutes) ReadLocation(ctx context.Context, id uuid.UUID) (domain.ObjectLocation, error) {
	if s == nil || s.db == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	if id == uuid.Nil {
		return domain.ObjectLocation{}, domain.ErrInvalidObjectRef
	}
	var row objectLocationRow
	if err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ObjectLocation{}, application.ErrNotFound
		}
		return domain.ObjectLocation{}, fmt.Errorf("read physical object location: %w", err)
	}
	location := row.domain()
	if err := location.Ref.Validate(); err != nil {
		return domain.ObjectLocation{}, err
	}
	return location, nil
}

// rowLocation keeps an unresolved FK visible so an unmigrated reader cannot
// silently discard the stable location identity from an object row.
func rowLocation(id *uuid.UUID, loaded *domain.ObjectLocation) *domain.ObjectLocation {
	if loaded != nil {
		return loaded
	}
	if id != nil {
		return &domain.ObjectLocation{ID: *id}
	}
	return nil
}

func readRowLocation(tx *gorm.DB, id *uuid.UUID, key string) (*domain.ObjectLocation, error) {
	if id == nil {
		return nil, nil
	}
	if *id == uuid.Nil {
		return nil, application.ErrUnavailable
	}
	var row objectLocationRow
	read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND object_key = ?", *id, key).Take(&row)
	if read.Error != nil {
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return nil, application.ErrUnavailable
		}
		return nil, fmt.Errorf("read stored media location: %w", read.Error)
	}
	location := row.domain()
	if location.Ref.Validate() != nil {
		return nil, application.ErrUnavailable
	}
	return &location, nil
}
func (r *assetRow) loadLocation(tx *gorm.DB) error {
	location, err := readRowLocation(tx, r.ObjectLocationID, r.ObjectKey)
	if err == nil {
		r.location = location
	}
	return err
}
func (r *renditionRow) loadLocation(tx *gorm.DB) error {
	location, err := readRowLocation(tx, r.ObjectLocationID, r.ObjectKey)
	if err == nil {
		r.location = location
	}
	return err
}
