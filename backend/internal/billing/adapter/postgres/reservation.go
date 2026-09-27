package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FindReservation scopes a held or closed reservation to the current actor's
// organization and the specified live project.
func (s *Store) FindReservation(ctx context.Context, actor identityapp.Principal, projectID, reservationID uuid.UUID) (domain.Reservation, error) {
	if s == nil || s.db == nil {
		return domain.Reservation{}, ErrUnavailable
	}
	if projectID == uuid.Nil || reservationID == uuid.Nil {
		return domain.Reservation{}, ErrNotFound
	}
	var row struct {
		ID           uuid.UUID
		ProjectID    uuid.UUID
		OperationID  uuid.UUID
		AmountMicros int64
		Status       string
		CreateTime   time.Time
		ClosedAt     *time.Time
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT r.id, r.project_id, r.operation_id, r.amount_micros,
			       r.status, r.create_time, r.closed_at
			FROM billing.reservation AS r
			JOIN workspace.project AS p ON p.id = r.project_id
			WHERE r.id = ?::uuid AND r.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT r.is_delete AND NOT p.is_delete
		`, reservationID.String(), projectID.String(), actor.OrgID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read reservation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("find reservation: %w", err)
	}
	reservation := domain.Reservation{
		ID: row.ID, ProjectID: row.ProjectID, OperationID: row.OperationID,
		AmountMicros: row.AmountMicros, Status: domain.ReservationStatus(row.Status),
		CreateTime: row.CreateTime, ClosedAt: row.ClosedAt,
	}
	if err := reservation.Validate(); err != nil {
		return domain.Reservation{}, fmt.Errorf("validate stored reservation: %w", err)
	}
	return reservation, nil
}
