package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// ApplyCreditFreezeCommandResult resolves failures that occur before an
// operation is dispatched and releases its project budget in the same tx.
func (s *Store) ApplyCreditFreezeCommandResult(ctx context.Context, result billingapp.CreditCommandResult) error {
	if s == nil || s.db == nil || result.EventID == uuid.Nil || result.CommandID == uuid.Nil ||
		result.ScopeID == uuid.Nil || result.OccurredAt.IsZero() || result.Status == "" {
		return billingapp.ErrCreditOutcomeConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command struct {
			ScopeID  uuid.UUID
			SourceID uuid.UUID
			Status   string
		}
		read := tx.Raw(`SELECT scope_id,source_id,status FROM billing.credit_command
			WHERE id=?::uuid AND caller_id=? AND action='freeze' FOR SHARE`,
			result.CommandID, billingapp.CreditOperationCallerID).Scan(&command)
		if read.Error != nil {
			return fmt.Errorf("read operation freeze result command: %w", read.Error)
		}
		if read.RowsAffected == 0 {
			return nil
		}
		if command.ScopeID != result.ScopeID || command.Status != result.Status {
			return billingapp.ErrCreditOutcomeConflict
		}
		var operation struct {
			ProjectID     uuid.UUID
			Status        string
			SettledMicros *int64
		}
		read = tx.Raw(`SELECT project_id,status,settled_micros FROM operation.operation
			WHERE id=?::uuid AND NOT is_delete FOR UPDATE`, command.SourceID).Scan(&operation)
		if read.Error != nil {
			return fmt.Errorf("lock operation for credit freeze result: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return application.ErrTransitionConflict
		}
		status := domain.Status(operation.Status)
		if result.Status == "succeeded" {
			if status == domain.StatusUnknown {
				return s.billingStore(tx).QueueOperationCreditSettlementOutcomeInTransaction(
					ctx, tx, command.SourceID, "unknown", 0, result.OccurredAt,
				)
			}
			if status.IsTerminal() {
				if operation.SettledMicros == nil {
					return application.ErrTransitionConflict
				}
				return s.billingStore(tx).QueueOperationCreditSettlementOutcomeInTransaction(
					ctx, tx, command.SourceID, "succeeded", *operation.SettledMicros, result.OccurredAt,
				)
			}
			return nil
		}
		if result.Status != "insufficient" && result.Status != "failed" {
			return billingapp.ErrCreditOutcomeConflict
		}
		if status.IsTerminal() {
			return nil
		}
		if status != domain.StatusConfirmed && status != domain.StatusCancelling {
			return application.ErrTransitionConflict
		}
		facts, err := s.ReadSettlementFactsInTransaction(ctx, tx, command.SourceID)
		if err != nil {
			return fmt.Errorf("read failed credit freeze operation: %w", err)
		}
		failed := status == domain.StatusConfirmed
		to := domain.StatusCancelled
		reason := "cancelled_before_credit_freeze"
		failureCode := ""
		var retryable *bool
		if failed {
			to = domain.StatusFailed
			reason = "credit_freeze_failed"
			failureCode = "credit_freeze_failed"
			if result.Status == "insufficient" {
				failureCode = "insufficient_credits"
			}
			noRetry := false
			retryable = &noRetry
		}
		finalize := application.FinalizeInput{
			OperationID: command.SourceID, From: []domain.Status{status}, To: to,
			FailureCode: failureCode, Retryable: retryable, Reason: reason,
		}
		prepared, err := s.PrepareFinalizationInTransaction(ctx, tx, finalize)
		if err != nil {
			return fmt.Errorf("prepare operation credit freeze failure: %w", err)
		}
		if prepared.ProjectID != facts.ProjectID || prepared.AlreadyFinalized {
			return application.ErrTransitionConflict
		}
		modelKey := (*string)(nil)
		if facts.ModelKey != "" {
			modelKey = &facts.ModelKey
		}
		settled, err := s.billingStore(tx).SettleInTransaction(ctx, tx, billingapp.SettleInput{
			ProjectID: facts.ProjectID, OperationID: command.SourceID,
			ActualCostMicros: 0, CapAtReservation: facts.PriceUnit == "per_1k_tokens",
			Capability: facts.Capability, ModelKey: modelKey, Region: facts.Region,
			OccurredAt: result.OccurredAt.UTC(),
		})
		if err != nil {
			return fmt.Errorf("release project reservation after credit freeze failure: %w", err)
		}
		if settled.ChargeMicros != 0 || settled.AlreadySettled {
			return application.ErrTransitionConflict
		}
		finalize.SettledMicros = 0
		if err := s.FinalizeInTransaction(ctx, tx, finalize); err != nil {
			return fmt.Errorf("finalize operation after credit freeze failure: %w", err)
		}
		if err := s.ReleaseBatchLaunchInTransaction(ctx, tx, command.SourceID); err != nil {
			return fmt.Errorf("release batch slot after credit freeze failure: %w", err)
		}
		return nil
	})
}
