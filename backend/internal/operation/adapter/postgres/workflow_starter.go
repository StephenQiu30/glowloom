package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConfirmedSingle verifies an event against the committed single-operation row.
func (s *Store) ConfirmedSingle(ctx context.Context, orgID, projectID, operationID, reservationID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if orgID == uuid.Nil || projectID == uuid.Nil || operationID == uuid.Nil || reservationID == uuid.Nil {
		return false, nil
	}
	var eligible bool
	err := s.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
		  SELECT 1 FROM operation.operation AS o
		  JOIN workspace.project AS p ON p.id = o.project_id
		  WHERE o.id = ?::uuid AND o.project_id = ?::uuid AND p.org_id = ?::uuid
		    AND o.reservation_id = ?::uuid AND o.status = 'confirmed'
		    AND o.batch_id IS NULL AND o.target_type IS DISTINCT FROM 'agent_session'
		    AND o.workflow_id = 'operation/' || o.id::text
		    AND NOT o.is_delete AND NOT p.is_delete
		)`, operationID.String(), projectID.String(), orgID.String(), reservationID.String()).Scan(&eligible).Error
	if err != nil {
		return false, fmt.Errorf("find confirmed single operation: %w", err)
	}
	return eligible, nil
}

// StaleConfirmedSingles lists old confirmed candidates for an idempotent retry.
func (s *Store) StaleConfirmedSingles(ctx context.Context, before time.Time, limit, offset int) ([]uuid.UUID, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if before.IsZero() || limit < 1 || limit > 100 || offset < 0 {
		return nil, fmt.Errorf("invalid stale confirmation query")
	}
	var rows []struct{ ID uuid.UUID }
	err := s.db.WithContext(ctx).Raw(`
		SELECT o.id FROM operation.operation AS o
		JOIN workspace.project AS p ON p.id = o.project_id
		WHERE o.status = 'confirmed' AND o.confirmed_at < ?
		  AND o.batch_id IS NULL AND o.target_type IS DISTINCT FROM 'agent_session'
		  AND o.workflow_id = 'operation/' || o.id::text
		  AND NOT o.is_delete AND NOT p.is_delete
		ORDER BY o.confirmed_at, o.id LIMIT ? OFFSET ?`, before.UTC(), limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list stale confirmed singles: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

// ConfirmedBatch verifies the committed parent and every selected child.
func (s *Store) ConfirmedBatch(ctx context.Context, orgID, projectID, batchID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if orgID == uuid.Nil || projectID == uuid.Nil || batchID == uuid.Nil {
		return false, nil
	}
	var eligible bool
	err := s.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
		  SELECT 1 FROM operation.batch AS b
		  JOIN workspace.project AS p ON p.id = b.project_id
		  WHERE b.id = ?::uuid AND b.project_id = ?::uuid AND p.org_id = ?::uuid
		    AND b.status = 'confirmed' AND b.total_count BETWEEN 1 AND 300
		    AND b.workflow_id = 'batch/' || b.id::text
		    AND NOT b.is_delete AND NOT p.is_delete
		    AND b.total_count = (
		      SELECT count(*) FROM operation.operation AS o
		      WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
		        AND o.status = 'confirmed' AND o.reservation_id IS NOT NULL
		        AND o.workflow_id = 'operation/' || o.id::text
		        AND o.target_type IS DISTINCT FROM 'agent_session'
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM operation.operation AS o
		      WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
		        AND o.status NOT IN ('confirmed', 'expired')
		    )
		)`, batchID.String(), projectID.String(), orgID.String()).Scan(&eligible).Error
	if err != nil {
		return false, fmt.Errorf("find confirmed batch: %w", err)
	}
	return eligible, nil
}

// StaleConfirmedBatches lists old parents, never their children, for retry.
func (s *Store) StaleConfirmedBatches(ctx context.Context, before time.Time, limit, offset int) ([]uuid.UUID, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if before.IsZero() || limit < 1 || limit > 100 || offset < 0 {
		return nil, fmt.Errorf("invalid stale batch confirmation query")
	}
	var rows []struct{ ID uuid.UUID }
	err := s.db.WithContext(ctx).Raw(`
		SELECT b.id FROM operation.batch AS b
		JOIN workspace.project AS p ON p.id = b.project_id
		WHERE b.status = 'confirmed' AND b.update_time < ?
		  AND b.total_count BETWEEN 1 AND 300
		  AND b.workflow_id = 'batch/' || b.id::text
		  AND NOT b.is_delete AND NOT p.is_delete
		  AND b.total_count = (
		    SELECT count(*) FROM operation.operation AS o
		    WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
		      AND o.status = 'confirmed' AND o.reservation_id IS NOT NULL
		      AND o.workflow_id = 'operation/' || o.id::text
		      AND o.target_type IS DISTINCT FROM 'agent_session'
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM operation.operation AS o
		    WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
		      AND o.status NOT IN ('confirmed', 'expired')
		  )
		ORDER BY b.update_time, b.id LIMIT ? OFFSET ?`, before.UTC(), limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list stale confirmed batches: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}
