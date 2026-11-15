package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

var _ application.SessionRevocationRecoveryStore = (*SessionRevocationStore)(nil)

// LoadSessionRevocationRequestByID is the restricted owner's internal recovery
// reader. Public callers must use the separate original credential/proof lookup.
func (s *SessionRevocationStore) LoadSessionRevocationRequestByID(ctx context.Context, id uuid.UUID) (application.SessionRevocationRequest, error) {
	if id == uuid.Nil {
		return application.SessionRevocationRequest{}, application.ErrSessionRevocationUnavailable
	}
	var result application.SessionRevocationRequest
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = readSessionRevocationRequest(tx, id, false)
		return err
	})
	return result, err
}

// EnqueuePendingSessionRevocations repairs only missing original-request wakeups
// in a bounded owner-local transaction. Existing completed or failed tasks are
// never reopened; no credential or fresh authorization is involved.
func (s *SessionRevocationStore) EnqueuePendingSessionRevocations(ctx context.Context, limit int, at time.Time) (int, error) {
	if limit < 1 || limit > 1000 || at.IsZero() {
		return 0, application.ErrSessionRevocationUnavailable
	}
	count := 0
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		result := tx.Exec(`WITH pending AS (
			SELECT r.id FROM identity.session_revocation_request r
			WHERE r.completed_at IS NULL AND NOT EXISTS (
				SELECT 1 FROM infra.task_work w WHERE w.service_owner=infra.runtime_owner()
				AND w.kind=? AND w.business_id=r.id::text)
			ORDER BY r.requested_at,r.id LIMIT ?)
			INSERT INTO infra.task_work(kind,business_id,next_run_at,reference)
			SELECT ?,id::text,?,'null'::jsonb FROM pending
			ON CONFLICT(service_owner,kind,business_id) DO NOTHING`,
			application.SessionRevocationRecoveryTask, limit, application.SessionRevocationRecoveryTask, at.UTC())
		if result.Error != nil {
			return fmt.Errorf("reconcile original session recovery tasks: %w", result.Error)
		}
		count = int(result.RowsAffected)
		return nil
	})
	return count, err
}
