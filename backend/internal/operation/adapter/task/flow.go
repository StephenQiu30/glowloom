package task

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// OperationStore owns operation execution, recovery and publication facts.
type OperationStore interface {
	LoadWorkflowOperation(context.Context, uuid.UUID) (app.WorkflowOperation, error)
	TaskEvidence(context.Context, uuid.UUID) (app.TaskEvidence, error)
	CheckSubmissionInputs(context.Context, uuid.UUID) error
	TransitionWorkflowOperation(context.Context, app.TransitionInput) (domain.Status, error)
	ScheduleOperationStep(context.Context, string, uuid.UUID) error
	BeginProviderCall(context.Context, app.BeginProviderCallInput) error
	CompleteProviderCall(context.Context, app.CompleteProviderCallInput) error
	CheckManualNotExecuted(context.Context, uuid.UUID) error
	LoadProviderCost(context.Context, uuid.UUID) (app.ProviderCost, error)
	LoadProviderImageIdentity(context.Context, uuid.UUID) (app.ProviderDispatchIdentity, bool, error)
}
type imageRecovery interface {
	RecoverImage(context.Context, app.ProviderDispatchIdentity) (*app.ProviderReceipt, error)
}

// FlowStore owns durable operation flow transitions.
type FlowStore interface {
	OperationStore
	BatchStore
	CanDeliverWorkflowControl(context.Context, app.WorkflowControlDelivery) (bool, error)
	ScheduleOriginalDelivery(context.Context, app.WorkflowControlDelivery) error
}
type operationStep func(context.Context, app.WorkflowOperation, app.TaskEvidence) (time.Time, error)

func operationHandler(store OperationStore, run operationStep, finalizer OperationFinalizer) taskruntime.Handler {
	return func(ctx context.Context, j taskruntime.Job) (next time.Time, err error) {
		id, err := uuid.Parse(j.BusinessID)
		if err != nil {
			return next, ErrInvalidOperationInput
		}
		defer func() {
			if errors.Is(err, app.ErrExecutionAuthorityRevoked) && finalizer != nil {
				next = time.Time{}
				err = finalizer.ResolveRevokedOperation(ctx, id.String())
			}
		}()
		l, err := store.LoadWorkflowOperation(ctx, id)
		if err != nil {
			if errors.Is(err, app.ErrCreditFreezePending) {
				return time.Now().Add(2 * time.Second), nil
			}
			return next, err
		}
		if l.Operation.ID != id || l.Operation.Validate() != nil {
			return next, ErrInvalidOperationInput
		}
		if l.Operation.Status.IsTerminal() {
			return next, nil
		}
		e, err := store.TaskEvidence(ctx, id)
		if err != nil {
			return next, err
		}
		if !e.Admitted {
			return next, app.ErrInvalidWorkflowBatch
		}
		return run(ctx, l, e)
	}
}
func change(ctx context.Context, s OperationStore, l app.WorkflowOperation, to domain.Status, reason string, taskID *string) error {
	i := app.TransitionInput{OperationID: l.Operation.ID, From: []domain.Status{l.Operation.Status}, To: to, Reason: reason, ProviderTaskID: taskID}
	if to == domain.StatusSubmitting {
		i.ProviderRequestKey = &l.ProviderRequestKey
	}
	_, err := s.TransitionWorkflowOperation(ctx, i)
	return err
}

type flow struct {
	store     FlowStore
	finalizer OperationFinalizer
	recovery  imageRecovery
}

// RegisterFlow registers handlers for admitted operation flow steps.
func RegisterFlow(h map[string]taskruntime.Handler, s FlowStore, f OperationFinalizer, r imageRecovery) {
	runner := flow{s, f, r}
	h["operation.run"] = operationHandler(s, runner.step, f)
	h["operation.batch"] = runner.batch
	h["operation.control"] = taskruntime.WithReference(time.Minute, runner.control)
}
func (f flow) finish(ctx context.Context, l app.WorkflowOperation, to domain.Status, cost int64, code string, retry bool) (time.Time, error) {
	return time.Time{}, f.finalizer.FinalizeOperation(ctx, SettlementInput{OperationID: l.Operation.ID.String(), From: l.Operation.Status, To: to, ActualCostMicros: cost, FailureCode: code, Retryable: retry})
}
func (f flow) step(ctx context.Context, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
	id := l.Operation.ID
	soon := time.Now().Add(time.Second)
	switch l.Operation.Status {
	case domain.StatusConfirmed:
		if e.CancelRequested {
			return soon, change(ctx, f.store, l, domain.StatusCancelling, "user_cancel_before_dispatch", nil)
		}
		if err := f.store.CheckSubmissionInputs(ctx, id); err != nil {
			switch {
			case errors.Is(err, app.ErrWorkflowInputNotReady):
				return f.finish(ctx, l, domain.StatusFailed, 0, "input_not_ready", false)
			case errors.Is(err, app.ErrWorkflowConsentUnavailable):
				return f.finish(ctx, l, domain.StatusFailed, 0, "consent_unavailable", false)
			default:
				return soon, err
			}
		}
		if l.Operation.ReusedFromID != nil {
			err := f.finalizer.CompleteFromReuse(ctx, id.String())
			if errors.Is(err, app.ErrReuseSourceUnavailable) {
				return f.finish(ctx, l, domain.StatusFailed, 0, "reuse_source_unavailable", false)
			}
			return time.Time{}, err
		}
		if l.Provider.AdapterKey == "mock" {
			if _, err := mockModerationStatus(l.Operation.Params); err != nil {
				return f.finish(ctx, l, domain.StatusFailed, 0, "invalid_mock_parameters", false)
			}
		}
		return soon, change(ctx, f.store, l, domain.StatusSubmitting, "submit", nil)
	case domain.StatusCancelling:
		if e.SubmitTaskID == nil || e.CancelState == app.ProviderCallStateCancelled {
			return f.finish(ctx, l, domain.StatusCancelled, 0, "", false)
		}
		if e.CancelState == app.ProviderCallStateNotCancelled || e.CancelState == app.ProviderCallStateUnknown {
			return soon, change(ctx, f.store, l, domain.StatusSubmitted, "provider_cancel_unconfirmed", nil)
		}
	case domain.StatusSucceeded:
		return soon, change(ctx, f.store, l, domain.StatusIngesting, "ingest", nil)
	case domain.StatusIngesting:
		if e.IngestFailed {
			cost, err := f.store.LoadProviderCost(ctx, id)
			if err != nil {
				return soon, err
			}
			return f.finish(ctx, l, domain.StatusFailed, cost.ActualCostMicros, "media_ingest_unavailable", true)
		}
		if !e.Ingested {
			return soon, f.store.ScheduleOperationStep(ctx, "operation.ingest", id)
		}
		cost, err := f.store.LoadProviderCost(ctx, id)
		if err != nil {
			return soon, err
		}

		return f.finish(ctx, l, domain.StatusCompleted, cost.ActualCostMicros, "", false)
	case domain.StatusSubmitted:
		if e.QueryState == app.ProviderCallStateFailed {
			if e.QueryFailureCode == "" {
				e.QueryFailureCode = "provider_rejected"
			}
			return f.finish(ctx, l, domain.StatusFailed, 0, e.QueryFailureCode, false)
		}
		if e.CancelRequested && l.Provider.SupportsCancel && e.CancelState == app.ProviderCallStateUnset {
			return soon, change(ctx, f.store, l, domain.StatusCancelling, "cancel_requested", nil)
		}
	case domain.StatusUnknown:
		return soon, change(ctx, f.store, l, domain.StatusReconciling, "recover_provider_receipt", nil)
	case domain.StatusManual:
		if err := f.store.CheckManualNotExecuted(ctx, id); err == nil {
			return f.finish(ctx, l, domain.StatusFailed, 0, "provider_not_executed", true)
		} else if !errors.Is(err, app.ErrManualResolutionUnverified) && !errors.Is(err, app.ErrProviderCallConflict) {
			return soon, err
		}
		if l.Provider.AdapterKey == "codex" {
			if err := f.recoverImage(ctx, id); err != nil {
				return soon, err
			}
			return time.Now().Add(time.Minute), nil
		}
	case domain.StatusReconciling:
		if l.Provider.AdapterKey == "codex" {
			if err := f.recoverImage(ctx, id); err != nil {
				return soon, err
			}
			return soon, change(ctx, f.store, l, domain.StatusManual, "synchronous_charge_unverified", nil)
		}
	}
	if l.Operation.Status == domain.StatusSubmitting {
		switch e.SubmitState {
		case app.ProviderCallStateNotSubmitted, app.ProviderCallStateRejected:
			if e.DispatchCancelled {
				return soon, change(ctx, f.store, l, domain.StatusCancelling, "user_cancel_before_dispatch", nil)
			}
			code := e.FailureCode
			if l.Provider.AdapterKey == "mock" && e.SubmitState == app.ProviderCallStateRejected && code == "" {
				code = providerFailureCode(nil)
			}
			if code == "" {
				code = "provider_not_submitted"
			}
			return f.finish(ctx, l, domain.StatusFailed, 0, code, l.Provider.AdapterKey == "codex")
		case app.ProviderCallStateUnknown:
			return soon, change(ctx, f.store, l, domain.StatusUnknown, "provider_result_unknown", nil)
		case app.ProviderCallStateCompleted:
			if l.Provider.AdapterKey == "codex" {
				return soon, change(ctx, f.store, l, domain.StatusUnknown, "synchronous_result_or_cost_unknown", nil)
			}
			return soon, change(ctx, f.store, l, domain.StatusSucceeded, "provider_succeeded", nil)
		case app.ProviderCallStateAccepted:
			return soon, change(ctx, f.store, l, domain.StatusSubmitted, "provider_accepted", e.SubmitTaskID)
		}
	}
	kind := "operation.provider"
	switch {
	case l.Provider.AdapterKey == "codex":
		kind = "operation.codex"
	case l.Provider.AdapterKey == "mock":
		kind = "operation.mock"
	case !catalog.IsManagedCloud(l.Provider.AdapterKey):
		return time.Time{}, ErrUnsupportedOperation
	}
	return soon, f.store.ScheduleOperationStep(ctx, kind, id)
}
func (f flow) recoverImage(ctx context.Context, id uuid.UUID) error {
	if f.recovery == nil {
		return nil
	}
	identity, found, err := f.store.LoadProviderImageIdentity(ctx, id)
	if err != nil || !found {
		return err
	}
	receipt, err := f.recovery.RecoverImage(ctx, identity)
	if receipt != nil && (receipt.Validate() != nil || receipt.Identity != identity) {
		return app.ErrProviderCallConflict
	}
	return err
}
