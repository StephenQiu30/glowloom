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
func NewOperationFinalizer(db *gorm.DB, operation *pgoperation.Store, billing *pgbilling.Store) operationflow.OperationFinalizer {
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
		return f.operation.ReleaseBatchLaunchInTransaction(ctx, tx, id)
	})
}

// CompleteFromReuse commits copied output rows, a zero-cost settlement, and
// the completed state together. A replay of the completed operation is read only.
func (f *operationFinalizer) CompleteFromReuse(ctx context.Context, operationID string) error {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return operationflow.ErrInvalidOperationInput
	}
	loaded, err := f.operation.LoadWorkflowOperation(ctx, id)
	if err != nil {
		return fmt.Errorf("load reuse operation: %w", err)
	}
	if loaded.Operation.ReusedFromID == nil {
		return operationapp.ErrReuseSourceUnavailable
	}
	finalize := operationapp.FinalizeInput{
		OperationID: id, From: []operationdomain.Status{operationdomain.StatusConfirmed},
		To: operationdomain.StatusCompleted, Reason: "reused_result",
	}
	settle := billingapp.SettleInput{
		ProjectID: loaded.Operation.ProjectID, OperationID: id,
		ActualCostMicros: 0, Capability: loaded.Operation.Capability,
		Region: loaded.Operation.Region, OccurredAt: time.Now().UTC(),
	}
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prepared, err := f.operation.PrepareFinalizationInTransaction(ctx, tx, finalize)
		if err != nil {
			return fmt.Errorf("prepare reused operation: %w", err)
		}
		if prepared.ProjectID != settle.ProjectID {
			return errSettlementStateConflict
		}
		if prepared.AlreadyFinalized {
			if prepared.SettledMicros == nil || *prepared.SettledMicros != 0 {
				return errSettlementStateConflict
			}
			return f.operation.ReleaseBatchLaunchInTransaction(ctx, tx, id)
		}
		if err := f.operation.CopyReuseOutputsInTransaction(ctx, tx, id); err != nil {
			return fmt.Errorf("copy reused outputs: %w", err)
		}
		result, err := f.billing.SettleInTransaction(ctx, tx, settle)
		if err != nil {
			return fmt.Errorf("settle reused operation: %w", err)
		}
		if result.ChargeMicros != 0 || result.AlreadySettled {
			return errSettlementStateConflict
		}
		finalize.SettledMicros = 0
		if err := f.operation.FinalizeInTransaction(ctx, tx, finalize); err != nil {
			return fmt.Errorf("complete reused operation: %w", err)
		}
		return f.operation.ReleaseBatchLaunchInTransaction(ctx, tx, id)
	})
}

var _ operationflow.Finalizer = (*operationFinalizer)(nil)
var _ operationflow.BatchQueuedCanceler = (*operationFinalizer)(nil)

// CancelQueuedBatchOperation uses the durable batch intent to claim an
// unlaunched child, then performs the normal zero-cost settlement. A crash
// between the claim and settlement is retried from the cancelling state.
func (f *operationFinalizer) CancelQueuedBatchOperation(ctx context.Context, batchID, operationID uuid.UUID) (bool, error) {
	claimed, err := f.operation.MarkQueuedWorkflowOperationCancelling(ctx, batchID, operationID)
	if err != nil || !claimed {
		return claimed, err
	}
	if err := f.FinalizeOperation(ctx, operationflow.SettlementInput{
		OperationID: operationID.String(), From: operationdomain.StatusCancelling,
		To: operationdomain.StatusCancelled, ActualCostMicros: 0,
		Reason: "batch_cancel_queued",
	}); err != nil {
		return false, fmt.Errorf("settle queued batch cancellation: %w", err)
	}
	return true, nil
}
