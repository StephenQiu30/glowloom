package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// FindWorkLocation resolves an existing frozen output, including cancellation
// recovery, without admitting a new object on the current default route.
func (s *StorageRoutes) FindWorkLocation(ctx context.Context, intent uuid.UUID, key string) (domain.ObjectLocation, bool, error) {
	if s == nil || s.db == nil || intent == uuid.Nil {
		return domain.ObjectLocation{}, false, application.ErrStorageIdentityConflict
	}
	var row objectLocationRow
	q := s.db.WithContext(ctx).Raw(`SELECT * FROM media.object_location WHERE owner_intent_id=? AND object_key=?`, intent, key).Scan(&row)
	if q.Error != nil {
		return domain.ObjectLocation{}, false, q.Error
	}
	if q.RowsAffected == 0 {
		return domain.ObjectLocation{}, false, nil
	}
	loc := row.domain()
	if loc.Ref.Validate() != nil {
		return domain.ObjectLocation{}, false, application.ErrStorageIdentityConflict
	}
	return loc, true, nil
}
