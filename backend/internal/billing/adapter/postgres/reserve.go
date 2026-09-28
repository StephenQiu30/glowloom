package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

// ReserveInTransaction locks the project budget and writes one held
// reservation, budget revision, and reserve ledger entry. The caller must
// recheck and lock the quoted operation and attach ReservationID in the same
// transaction; this method neither commits nor starts a workflow.
func (s *Store) ReserveInTransaction(ctx context.Context, tx *gorm.DB, input application.ReserveInput) (application.ReserveResult, error) {
	if s == nil || s.db == nil || tx == nil {
		return application.ReserveResult{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.ReserveResult{}, err
	}
	tx = tx.WithContext(ctx)
	var budget domain.Budget
	row := tx.Raw(`
		SELECT id, project_id, limit_micros, reserved_micros,
		       settled_micros, is_overrun, revision
		FROM billing.budget
		WHERE project_id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, input.ProjectID.String()).Scan(&budget)
	if row.Error != nil {
		return application.ReserveResult{}, fmt.Errorf("lock confirmation budget: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.ReserveResult{}, ErrNotFound
	}
	previousRevision := budget.Revision
	if err := budget.Reserve(input.AmountMicros); err != nil {
		return application.ReserveResult{}, fmt.Errorf("reserve confirmation budget: %w", err)
	}
	availableMicros, err := budget.AvailableMicros()
	if err != nil {
		return application.ReserveResult{}, fmt.Errorf("calculate confirmation balance: %w", err)
	}
	reservationID := uuid.New()
	row = tx.Exec(`
		INSERT INTO billing.reservation
		  (id, project_id, operation_id, amount_micros, status, create_time)
		VALUES (?::uuid, ?::uuid, ?::uuid, ?, 'held', ?)
	`, reservationID.String(), input.ProjectID.String(), input.OperationID.String(),
		input.AmountMicros, input.OccurredAt.UTC())
	if row.Error != nil {
		return application.ReserveResult{}, fmt.Errorf("insert confirmation reservation: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.ReserveResult{}, fmt.Errorf("insert confirmation reservation: wrote %d rows", row.RowsAffected)
	}
	row = tx.Exec(`
		UPDATE billing.budget
		SET reserved_micros = ?, revision = revision + 1
		WHERE id = ?::uuid AND project_id = ?::uuid AND revision = ? AND NOT is_delete
	`, budget.ReservedMicros, budget.ID.String(), input.ProjectID.String(), previousRevision)
	if row.Error != nil {
		return application.ReserveResult{}, fmt.Errorf("update confirmation budget: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.ReserveResult{}, domain.ErrBudgetRevision
	}
	row = tx.Exec(`
		INSERT INTO billing.ledger_entry
		  (id, project_id, entry_type, amount_micros, operation_id,
		   episode_id, shot_id, model_key, region, create_time, create_by)
		VALUES (?::uuid, ?::uuid, 'reserve', ?, ?::uuid,
		        ?::uuid, ?::uuid, ?, ?, ?, ?::uuid)
	`, uuid.NewString(), input.ProjectID.String(), input.AmountMicros, input.OperationID.String(),
		input.EpisodeID, input.ShotID, input.ModelKey, input.Region,
		input.OccurredAt.UTC(), input.ActorID.String())
	if row.Error != nil {
		return application.ReserveResult{}, fmt.Errorf("insert confirmation reserve ledger: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.ReserveResult{}, fmt.Errorf("insert confirmation reserve ledger: wrote %d rows", row.RowsAffected)
	}
	return application.ReserveResult{
		ReservationID: reservationID, AvailableMicros: availableMicros,
		BudgetRevision: budget.Revision,
	}, nil
}
