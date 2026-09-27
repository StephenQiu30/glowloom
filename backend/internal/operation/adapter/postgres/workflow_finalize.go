package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// ErrInvalidFinalizationInput means the operation cannot be closed safely.
var ErrInvalidFinalizationInput = application.ErrInvalidFinalizationInput

// PrepareFinalizationInTransaction locks and validates the operation before
// its reservation is settled. Keep the returned transaction open through
// billing settlement and FinalizeInTransaction.
func (s *Store) PrepareFinalizationInTransaction(ctx context.Context, tx *gorm.DB, input application.FinalizeInput) (application.FinalizePreparation, error) {
	if s == nil || s.db == nil || tx == nil {
		return application.FinalizePreparation{}, ErrUnavailable
	}
	if err := validateFinalizationInput(input); err != nil {
		return application.FinalizePreparation{}, err
	}
	var row struct {
		ProjectID     uuid.UUID
		ReservationID *uuid.UUID
		Status        string
		SettledMicros *int64
	}
	result := tx.WithContext(ctx).Raw(`
		SELECT project_id, reservation_id, status, settled_micros
		FROM operation.operation
		WHERE id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, input.OperationID.String()).Scan(&row)
	if result.Error != nil {
		return application.FinalizePreparation{}, fmt.Errorf("lock operation for finalization: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.FinalizePreparation{}, ErrNotFound
	}
	preparation := application.FinalizePreparation{ProjectID: row.ProjectID, SettledMicros: row.SettledMicros}
	if row.ReservationID == nil || *row.ReservationID == uuid.Nil {
		return application.FinalizePreparation{}, ErrInvalidFinalizationInput
	}
	preparation.ReservationID = *row.ReservationID
	current := domain.Status(row.Status)
	if current == input.To {
		if row.SettledMicros == nil {
			return application.FinalizePreparation{}, ErrTransitionConflict
		}
		preparation.AlreadyFinalized = true
		return preparation, nil
	}
	if row.SettledMicros != nil || !containsTransitionFrom(input.From, current) {
		return application.FinalizePreparation{}, ErrTransitionConflict
	}
	if err := current.CanTransitionTo(input.To); err != nil {
		return application.FinalizePreparation{}, err
	}
	return preparation, nil
}

// FinalizeInTransaction completes the operation after billing settles its
// reservation in the same caller-owned transaction. The caller rolls back the
// entire transaction on any returned error.
func (s *Store) FinalizeInTransaction(ctx context.Context, tx *gorm.DB, input application.FinalizeInput) error {
	preparation, err := s.PrepareFinalizationInTransaction(ctx, tx, input)
	if err != nil {
		return err
	}
	if preparation.AlreadyFinalized {
		if preparation.SettledMicros == nil || *preparation.SettledMicros != input.SettledMicros {
			return ErrTransitionConflict
		}
		return nil
	}
	var row workflowTransitionRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, project_id, batch_id, target_type, target_id, status,
		       provider_request_key
		FROM operation.operation
		WHERE id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, input.OperationID.String()).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("read operation finalization metadata: %w", result.Error)
	}
	if result.RowsAffected != 1 || row.TargetType == nil || *row.TargetType == "" {
		return ErrInvalidFinalizationInput
	}
	from := domain.Status(row.Status)
	result = tx.WithContext(ctx).Exec(`
		UPDATE operation.operation
		SET status = ?, settled_micros = ?, finished_at = now(),
		    failure_code = NULLIF(?, ''), retryable = ?, update_time = now()
		WHERE id = ?::uuid AND status = ? AND settled_micros IS NULL AND NOT is_delete
	`, string(input.To), input.SettledMicros, input.FailureCode, input.Retryable,
		input.OperationID.String(), row.Status)
	if result.Error != nil {
		return fmt.Errorf("finalize workflow operation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrTransitionConflict
	}
	transition := application.TransitionInput{
		OperationID: input.OperationID, From: input.From, To: input.To, Reason: input.Reason,
	}
	detail := map[string]any{"settled_micros": input.SettledMicros}
	if input.FailureCode != "" {
		detail["failure_code"] = input.FailureCode
	}
	if err := appendWorkflowTransition(tx.WithContext(ctx), row, from, transition, detail); err != nil {
		return err
	}
	if err := appendWorkflowTerminalEvent(tx.WithContext(ctx), row, input); err != nil {
		return err
	}
	return nil
}

func validateFinalizationInput(input application.FinalizeInput) error {
	if input.OperationID == uuid.Nil || len(input.From) == 0 || len(input.From) > 8 ||
		!input.To.IsTerminal() || input.To == domain.StatusExpired || input.SettledMicros < 0 ||
		len(input.Reason) > 128 || len(input.FailureCode) > 128 ||
		strings.TrimSpace(input.FailureCode) != input.FailureCode ||
		(input.To != domain.StatusFailed && input.FailureCode != "") ||
		(input.To == domain.StatusFailed && input.FailureCode == "") {
		return ErrInvalidFinalizationInput
	}
	for _, from := range input.From {
		if from.IsTerminal() || from == input.To || from.CanTransitionTo(input.To) != nil {
			return ErrInvalidFinalizationInput
		}
	}
	return nil
}

func appendWorkflowTerminalEvent(tx *gorm.DB, row workflowTransitionRow, input application.FinalizeInput) error {
	var topic string
	switch input.To {
	case domain.StatusCompleted:
		topic = "lanverse.operation.completed.v1"
	case domain.StatusFailed:
		topic = "lanverse.operation.failed.v1"
	default:
		return nil
	}
	return appendWorkflowSpecialEvent(tx, row, topic, struct {
		Status        string `json:"status"`
		FailureCode   string `json:"failure_code,omitempty"`
		SettledMicros int64  `json:"settled_micros"`
	}{Status: string(input.To), FailureCode: input.FailureCode, SettledMicros: input.SettledMicros})
}
