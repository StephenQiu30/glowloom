package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// MarkQueuedWorkflowOperationCancelling claims a child before any provider
// call, even if it already holds a launch slot. The status transition races
// safely with the child's move to submitting and provider-call recording.
// A true result can be followed by the independent, replayable zero-cost
// settlement; the parent retries after a crash between these steps.
func (s *Store) MarkQueuedWorkflowOperationCancelling(ctx context.Context, batchID, operationID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if batchID == uuid.Nil || operationID == uuid.Nil {
		return false, application.ErrInvalidWorkflowBatch
	}
	var claimed bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch struct {
			ProjectID         uuid.UUID
			Status            string
			CancelRequestedAt *time.Time
		}
		read := tx.Raw(`
			SELECT project_id, status, cancel_requested_at FROM operation.batch
			WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, batchID.String()).Scan(&batch)
		if read.Error != nil {
			return fmt.Errorf("lock batch cancellation intent: %w", read.Error)
		}
		if read.RowsAffected != 1 || batch.CancelRequestedAt == nil ||
			(batch.Status != string(domain.BatchStatusRunning) && batch.Status != string(domain.BatchStatusCancelled)) {
			return application.ErrInvalidWorkflowBatch
		}
		var item struct {
			Status        string
			ReservationID *uuid.UUID
			WorkflowID    *string
		}
		read = tx.Raw(`
			SELECT status, reservation_id, workflow_id FROM operation.operation
			WHERE id = ?::uuid AND batch_id = ?::uuid AND project_id = ?::uuid AND NOT is_delete FOR UPDATE
		`, operationID.String(), batchID.String(), batch.ProjectID.String()).Scan(&item)
		if read.Error != nil {
			return fmt.Errorf("lock queued batch child: %w", read.Error)
		}
		if read.RowsAffected != 1 || item.ReservationID == nil || *item.ReservationID == uuid.Nil ||
			item.WorkflowID == nil || *item.WorkflowID != "operation/"+operationID.String() {
			return application.ErrInvalidWorkflowBatch
		}
		var called bool
		if err := tx.Raw(`SELECT EXISTS (
			SELECT 1 FROM operation.provider_call WHERE operation_id = ?::uuid AND NOT is_delete
		)`, operationID.String()).Scan(&called).Error; err != nil {
			return fmt.Errorf("check queued child provider calls: %w", err)
		}
		if called {
			return nil
		}
		switch domain.Status(item.Status) {
		case domain.StatusConfirmed, domain.StatusSubmitting:
			from := domain.Status(item.Status)
			_, err := NewStore(tx).TransitionWorkflowOperation(ctx, application.TransitionInput{
				OperationID: operationID, From: []domain.Status{from},
				To: domain.StatusCancelling, Reason: "batch_cancel_queued",
			})
			if err != nil {
				return fmt.Errorf("mark queued child cancelling: %w", err)
			}
			claimed = true
		case domain.StatusCancelling, domain.StatusCancelled:
			claimed = true
		default:
			return application.ErrInvalidWorkflowBatch
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("claim queued batch cancellation: %w", err)
	}
	return claimed, nil
}
