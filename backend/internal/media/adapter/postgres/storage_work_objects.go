package postgres

import (
	"context"

	"github.com/google/uuid"

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
	predicate := `w.state='open' AND l.state IN ('reserved','present','unknown') AND l.write_until>clock_timestamp()`
	if remove {
		predicate = `(w.state='cancelled' OR (w.state='accepted' AND w.kind IN ('project-copy','script'))) AND l.state='delete_pending' AND (l.write_until IS NULL OR l.write_until<clock_timestamp())`
	}
	var row objectLocationRow
	read := s.db.WithContext(ctx).Raw(`SELECT l.* FROM media.object_location l JOIN media.storage_work_intent w ON w.id=l.owner_intent_id AND w.profile_id=l.profile_id AND w.bucket=l.bucket WHERE l.id=? AND `+predicate, id).Scan(&row)
	if read.Error != nil {
		return domain.ObjectLocation{}, read.Error
	}
	if read.RowsAffected != 1 {
		return domain.ObjectLocation{}, application.ErrStorageRouteConflict
	}
	return row.domain(), nil
}
