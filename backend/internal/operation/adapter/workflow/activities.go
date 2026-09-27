package workflow

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// Store is the operation workflow's narrow database boundary.
type Store interface {
	LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error)
	CheckSubmissionInputs(context.Context, uuid.UUID) error
	TransitionWorkflowOperation(context.Context, application.TransitionInput) (domain.Status, error)
	BeginProviderCall(context.Context, application.BeginProviderCallInput) error
	CompleteProviderCall(context.Context, application.CompleteProviderCallInput) error
	LoadProviderCost(context.Context, uuid.UUID) (application.ProviderCost, error)
	CheckManualNotExecuted(context.Context, uuid.UUID) error
}

// Finalizer enters a terminal state and closes billing in one transaction.
type Finalizer interface {
	FinalizeOperation(context.Context, SettlementInput) error
}

// Activities is the flow queue's database Activity adapter.
type Activities struct {
	store     Store
	finalizer Finalizer
}

// NewActivities injects the operation store and atomic terminal writer.
func NewActivities(store Store, finalizer Finalizer) *Activities {
	return &Activities{store: store, finalizer: finalizer}
}

// LoadOperation reads the persisted workflow snapshot without credentials.
func (a *Activities) LoadOperation(ctx context.Context, operationID string) (application.WorkflowOperation, error) {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return application.WorkflowOperation{}, ErrInvalidOperationInput
	}
	loaded, err := a.store.LoadWorkflowOperation(ctx, id)
	return loaded, permanentActivityError(err)
}

// CheckConsent rechecks frozen media before a provider request can be sent.
func (a *Activities) CheckConsent(ctx context.Context, operationID string) error {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return ErrInvalidOperationInput
	}
	return permanentActivityError(a.store.CheckSubmissionInputs(ctx, id))
}

// Transition conditionally writes one nonterminal state and its event.
func (a *Activities) Transition(ctx context.Context, input application.TransitionInput) (domain.Status, error) {
	status, err := a.store.TransitionWorkflowOperation(ctx, input)
	return status, permanentActivityError(err)
}

// SettleOperation atomically enters a terminal state and settles the budget.
func (a *Activities) SettleOperation(ctx context.Context, input SettlementInput) error {
	return permanentActivityError(a.finalizer.FinalizeOperation(ctx, input))
}

// BeginProviderCall records an unknown attempt before an external call.
func (a *Activities) BeginProviderCall(ctx context.Context, input application.BeginProviderCallInput) error {
	return permanentActivityError(a.store.BeginProviderCall(ctx, input))
}

// CompleteProviderCall records a redacted result for that attempt.
func (a *Activities) CompleteProviderCall(ctx context.Context, input application.CompleteProviderCallInput) error {
	return permanentActivityError(a.store.CompleteProviderCall(ctx, input))
}

// LoadProviderCost reports a conclusive charge from durable provider evidence.
func (a *Activities) LoadProviderCost(ctx context.Context, operationID string) (application.ProviderCost, error) {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return application.ProviderCost{}, ErrInvalidOperationInput
	}
	cost, err := a.store.LoadProviderCost(ctx, id)
	return cost, permanentActivityError(err)
}

// CheckManualNotExecuted verifies that an authenticated administrator command
// recorded its private evidence before the workflow trusts a manual signal.
func (a *Activities) CheckManualNotExecuted(ctx context.Context, operationID string) error {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return ErrInvalidOperationInput
	}
	return permanentActivityError(a.store.CheckManualNotExecuted(ctx, id))
}

func permanentActivityError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, application.ErrWorkflowInputNotReady) {
		return temporal.NewNonRetryableApplicationError("frozen input media is not ready", "input_not_ready", err)
	}
	if errors.Is(err, application.ErrWorkflowConsentUnavailable) {
		return temporal.NewNonRetryableApplicationError("frozen media consent cannot be verified", "consent_unavailable", err)
	}
	if errors.Is(err, application.ErrManualResolutionUnverified) {
		return temporal.NewNonRetryableApplicationError("administrator resolution has not been recorded", "manual_resolution_unverified", err)
	}
	if errors.Is(err, domain.ErrIllegalTransition) || errors.Is(err, domain.ErrInvalidStatus) ||
		errors.Is(err, domain.ErrInvalidOperation) || errors.Is(err, ErrInvalidOperationInput) ||
		errors.Is(err, application.ErrInvalidTransitionInput) ||
		errors.Is(err, application.ErrTransitionConflict) ||
		errors.Is(err, application.ErrInvalidFinalizationInput) ||
		errors.Is(err, application.ErrWorkflowModelUnavailable) ||
		errors.Is(err, application.ErrInvalidProviderCall) ||
		errors.Is(err, application.ErrProviderCallConflict) ||
		errors.Is(err, application.ErrProviderCostUnknown) {
		return temporal.NewNonRetryableApplicationError("invalid operation workflow state", "illegal_transition", err)
	}
	return err
}

// Register installs one deterministic workflow and its flow queue activities.
func Register(w worker.Worker, activities *Activities) {
	w.RegisterWorkflowWithOptions(OperationWorkflow, workflow.RegisterOptions{Name: "OperationWorkflow"})
	w.RegisterActivityWithOptions(activities.LoadOperation, activity.RegisterOptions{Name: "flow.LoadOperation"})
	w.RegisterActivityWithOptions(activities.CheckConsent, activity.RegisterOptions{Name: "flow.CheckConsent"})
	w.RegisterActivityWithOptions(activities.Transition, activity.RegisterOptions{Name: "flow.Transition"})
	w.RegisterActivityWithOptions(activities.SettleOperation, activity.RegisterOptions{Name: "flow.SettleOperation"})
	w.RegisterActivityWithOptions(activities.BeginProviderCall, activity.RegisterOptions{Name: "flow.BeginProviderCall"})
	w.RegisterActivityWithOptions(activities.CompleteProviderCall, activity.RegisterOptions{Name: "flow.CompleteProviderCall"})
	w.RegisterActivityWithOptions(activities.LoadProviderCost, activity.RegisterOptions{Name: "flow.LoadProviderCost"})
	w.RegisterActivityWithOptions(activities.CheckManualNotExecuted, activity.RegisterOptions{Name: "flow.CheckManualNotExecuted"})
}
