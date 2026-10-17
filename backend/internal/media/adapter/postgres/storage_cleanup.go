package postgres

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// PendingWorkRemovals lists bounded durable debts. Each removal still reacquires
// the intent/location fence and verifies current owning references before I/O.
func (s *StorageRoutes) PendingWorkRemovals(ctx context.Context, limit int) ([]application.StorageRemoval, error) {
	if s == nil || s.db == nil || limit < 1 || limit > 1000 {
		return nil, application.ErrUnavailable
	}
	var rows []objectLocationRow
	err := s.db.WithContext(ctx).Raw(`SELECT l.* FROM media.object_location l JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE w.state='cancelled' AND w.closed_at<clock_timestamp()-interval '10 minutes' AND l.state IN ('reserved','present','unknown','delete_pending') AND (l.write_until IS NULL OR l.write_until<clock_timestamp()) ORDER BY w.closed_at,l.id LIMIT ?`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]application.StorageRemoval, 0, len(rows))
	for _, row := range rows {
		if row.OwnerIntentID == nil {
			return nil, application.ErrStorageIdentityConflict
		}
		result = append(result, application.StorageRemoval{IntentID: *row.OwnerIntentID, Location: row.domain()})
	}
	return result, nil
}
