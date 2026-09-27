package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var errSettlementStateConflict = operationapp.ErrTransitionConflict

// operationFinalizer coordinates two context-owned stores under one database
// transaction. Neither store reads or writes the other's business tables.
type operationFinalizer struct {
	db        *gorm.DB
	operation *pgoperation.Store
	billing   *pgbilling.Store
}

// NewOperationFinalizer composes an atomic terminal writer for the flow worker.
func NewOperationFinalizer(db *gorm.DB, operation *pgoperation.Store, billing *pgbilling.Store) operationflow.Finalizer {
	return &operationFinalizer{db: db, operation: operation, billing: billing}
}

func (f *operationFinalizer) FinalizeOperation(ctx context.Context, input operationflow.SettlementInput) error {
	id, err := uuid.Parse(input.OperationID)
	if err != nil || id.String() != input.OperationID || input.ActualCostMicros < 0 {
		return operationflow.ErrInvalidOperationInput
	}
	loaded, err := f.operation.LoadWorkflowOperation(ctx, id)
	if err != nil {
		return fmt.Errorf("load operation for settlement: %w", err)
	}
	var retryable *bool
	if input.To == "failed" {
		retryable = &input.Retryable
	}
	var modelKey *string
	if loaded.Provider.ModelKey != "" {
		modelKey = &loaded.Provider.ModelKey
	}
	finalize := operationapp.FinalizeInput{
		OperationID: id, From: []operationdomain.Status{input.From},
		To: input.To, FailureCode: input.FailureCode,
		Retryable: retryable, Reason: input.Reason,
	}
	settle := billingapp.SettleInput{
		ProjectID: loaded.Operation.ProjectID, OperationID: id,
		ActualCostMicros: input.ActualCostMicros,
		CapAtReservation: loaded.PriceUnit == "per_1k_tokens",
		Capability:       loaded.Operation.Capability, ModelKey: modelKey,
		Region:     loaded.Operation.Region,
		OccurredAt: time.Now().UTC(),
	}
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prepared, err := f.operation.PrepareFinalizationInTransaction(ctx, tx, finalize)
		if err != nil {
			return fmt.Errorf("prepare operation finalization: %w", err)
		}
		if prepared.ProjectID != settle.ProjectID {
			return errSettlementStateConflict
		}
		if !prepared.AlreadyFinalized {
			providerCost, err := f.operation.ProviderCostInTransaction(ctx, tx, id)
			if err != nil {
				return fmt.Errorf("establish provider cost: %w", err)
			}
			if providerCost.ActualCostMicros != input.ActualCostMicros {
				return operationapp.ErrProviderCallConflict
			}
			if providerCost.ManualNotExecuted &&
				(input.From != operationdomain.StatusManual || input.To != operationdomain.StatusFailed ||
					input.FailureCode != "provider_not_executed") {
				return operationapp.ErrProviderCallConflict
			}
			settle.ActualCostMicros = providerCost.ActualCostMicros
		}
		result, err := f.billing.SettleInTransaction(ctx, tx, settle)
		if err != nil {
			if errors.Is(err, pgbilling.ErrSettlementConflict) || errors.Is(err, billingapp.ErrInvalidSettlement) {
				return operationapp.ErrTransitionConflict
			}
			return fmt.Errorf("settle operation reservation: %w", err)
		}
		if prepared.AlreadyFinalized != result.AlreadySettled {
			return errSettlementStateConflict
		}
		finalize.SettledMicros = result.ChargeMicros
		if err := f.operation.FinalizeInTransaction(ctx, tx, finalize); err != nil {
			return fmt.Errorf("finalize operation state: %w", err)
		}
		return nil
	})
}

var _ operationflow.Finalizer = (*operationFinalizer)(nil)
