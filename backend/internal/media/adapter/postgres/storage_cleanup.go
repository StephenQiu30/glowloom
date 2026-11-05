package postgres

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// PendingWorkRemovals lists bounded durable debts. Each removal still reacquires
// the intent/location fence and verifies current owning references before I/O.
func (s *StorageRoutes) PendingWorkRemovals(ctx context.Context, limit int) ([]application.StorageRemoval, error) {
	if s == nil || s.db == nil || limit < 1 || limit > 1000 {
		return nil, application.ErrUnavailable
	}
	var rows []objectLocationRow
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Table("media.object_location AS l").Select("l.*").
		Joins("JOIN media.storage_work_intent AS w ON w.id = l.owner_intent_id").
		Where("w.state = ? AND w.closed_at < ?", application.StorageWorkCancelled, now.Add(-10*time.Minute)).
		Where("l.state IN ?", []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationUnknown, domain.ObjectLocationDeletePending}).
		Where("l.write_until IS NULL OR l.write_until < ?", now).
		Order("w.closed_at").Order("l.id").Limit(limit).Find(&rows).Error
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
