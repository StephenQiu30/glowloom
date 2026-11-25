package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// RecoverStale closes only an expired send window. It never dispatches, releases
// funds or fabricates supplier evidence. Late known receipts can still settle it.
func (s *Store) RecoverStale(ctx context.Context, id uuid.UUID) (time.Time, error) {
	var next time.Time
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			State    string
			Deadline time.Time
		}
		q := tx.Raw("SELECT state,deadline FROM ai.request WHERE id=? FOR UPDATE", id).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrUnavailable
		}
		if row.State != "sending" {
			return nil
		}
		var now time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
			return err
		}
		// The sender has up to 15 seconds after its deadline to commit a receipt.
		until := row.Deadline.Add(15 * time.Second)
		if now.Before(until) {
			next = until
			return nil
		}
		q = tx.Exec("UPDATE ai.attempt SET state='unknown' WHERE request_id=? AND state='sending' AND receipt IS NULL", id)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return nil
		} // A never-consumed ready claim belongs to ai.execute.
		return tx.Exec("UPDATE ai.request SET state='unknown',updated_at=clock_timestamp() WHERE id=? AND state='sending'", id).Error
	})
	return next, err
}

// ReadProjectResult is an internal project-authorized projection source. Raw SDK
// receipt endpoints use Read, which additionally restricts consumer visibility.
func (s *Store) ReadProjectResult(ctx context.Context, scope, project, id uuid.UUID) (domain.Record, error) {
	var out domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if project == uuid.Nil {
			return domain.ErrForbidden
		}
		if err := s.check(ctx, tx, scope, project, false); err != nil {
			return err
		}
		var present int
		q := tx.Raw("SELECT 1 FROM ai.request r JOIN ai.creation c ON c.id=r.id WHERE r.id=? AND r.scope_id=? AND r.project_id=? AND c.project_id=?", id, scope, project, project).Scan(&present)
		if q.Error != nil {
			return q.Error
		}
		if present != 1 {
			return domain.ErrForbidden
		}
		var err error
		out, err = scanRecord(tx, id)
		return err
	})
	return out, err
}
