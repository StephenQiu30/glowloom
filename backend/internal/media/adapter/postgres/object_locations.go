package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type objectLocationRow struct {
	OwnerSessionID                      *uuid.UUID
	ID                                  uuid.UUID
	ProfileID, Bucket, ObjectKey, State string
	ByteSize                            *int64
	SHA256, MIMEType                    *string
}

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
	var row objectLocationRow
	read := s.db.WithContext(ctx).Raw(`INSERT INTO media.object_location(id,profile_id,bucket,object_key) VALUES(?,?,?,?) ON CONFLICT(profile_id,bucket,object_key) DO UPDATE SET updated_at=media.object_location.updated_at RETURNING *`, uuid.New(), ref.ProfileID, ref.Bucket, ref.Key).Scan(&row)
	if read.Error != nil {
		return domain.ObjectLocation{}, fmt.Errorf("reserve physical object location: %w", read.Error)
	}
	if read.RowsAffected != 1 || row.ID == uuid.Nil || row.domain().Ref != ref {
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
	read := s.db.WithContext(ctx).Raw(`SELECT * FROM media.object_location WHERE id=?`, id).Scan(&row)
	if read.Error != nil {
		return domain.ObjectLocation{}, fmt.Errorf("read physical object location: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return domain.ObjectLocation{}, application.ErrNotFound
	}
	location := row.domain()
	if err := location.Ref.Validate(); err != nil {
		return domain.ObjectLocation{}, err
	}
	return location, nil
}
