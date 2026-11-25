package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ReadWritableWorkLocation checks the durable fence immediately before SDK I/O.
// Business authorization belongs to the owner that admitted this exact location.
func (s *StorageRoutes) ReadWritableWorkLocation(ctx context.Context, id uuid.UUID) (domain.ObjectLocation, error) {
	return s.readWorkLocationForIO(ctx, id, false)
}

// ReadRemovableWorkLocation never grants deletion from a bare key or active route.
func (s *StorageRoutes) ReadRemovableWorkLocation(ctx context.Context, id uuid.UUID) (domain.ObjectLocation, error) {
	return s.readWorkLocationForIO(ctx, id, true)
}

func (s *StorageRoutes) readWorkLocationForIO(ctx context.Context, id uuid.UUID, remove bool) (domain.ObjectLocation, error) {
	if s == nil || s.db == nil || id == uuid.Nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	var row objectLocationRow
	query := s.db.WithContext(ctx).Table("media.object_location AS l").
		Select("l.*").
		Joins("JOIN media.storage_work_intent AS w ON w.id = l.owner_intent_id AND w.profile_id = l.profile_id AND w.bucket = l.bucket").
		Where("l.id = ?", id)
	if remove {
		query = query.Where("(w.state = ? OR (w.state = ? AND w.kind IN ?)) AND l.state = ? AND (l.write_until IS NULL OR l.write_until < clock_timestamp())",
			application.StorageWorkCancelled,
			application.StorageWorkAccepted,
			[]application.StorageWorkKind{application.StorageWorkKindProjectCopy, application.StorageWorkKindScript},
			domain.ObjectLocationDeletePending)
	} else {
		query = query.Where("w.state = ? AND l.state IN ? AND l.write_until > clock_timestamp()",
			application.StorageWorkOpen,
			[]domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationUnknown})
	}
	if err := query.Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ObjectLocation{}, application.ErrStorageRouteConflict
	} else if err != nil {
		return domain.ObjectLocation{}, err
	}
	return row.domain(), nil
}
