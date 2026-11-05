package task

import (
	"context"
	"errors"
	"time"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type managedProvider interface {
	Step(context.Context, app.ManagedStep) (app.ManagedProgress, error)
}
type imageProvider interface {
	Submit(context.Context, ProviderSubmitInput) (ProviderSubmitOutput, error)
}

// RegisterProvider registers provider dispatch and polling handlers.
func RegisterProvider(h map[string]taskruntime.Handler, s OperationStore, p managedProvider) {
	h["operation.mock"] = operationHandler(s, func(ctx context.Context, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
		return mockStep(ctx, s, l, e)
	}, nil)
	h["operation.provider"] = operationHandler(s, func(ctx context.Context, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
		return managedStep(ctx, s, p, l, e)
	}, nil)
}
func managedStep(ctx context.Context, s OperationStore, p managedProvider, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
	op := l.Operation
	soon := time.Now().Add(time.Second)
	if op.Status == domain.StatusSucceeded || op.Status == domain.StatusIngesting {
		return time.Time{}, nil
	}
	if p == nil {
		return soon, app.ErrWorkflowModelUnavailable
	}
	if op.Status == domain.StatusSubmitting {
		if e.SubmitState != app.ProviderCallStateUnset {
			return soon, nil
		}
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, DispatchRequired: true}); err != nil {
			return soon, err
		}
		action := "submit"
		if e.DispatchStartedAt != nil {
			action = "recover"
		}
		v, err := p.Step(ctx, app.ManagedStep{OperationID: op.ID, Action: action})
		if err != nil {
			return soon, err
		}
		if v.State == app.ManagedProgressWaiting {
			return time.Now().Add(5 * time.Second), nil
		}
		ev := app.CompleteProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, Outcome: app.ProviderCallOutcomeUnknown, State: app.ProviderCallStateUnknown, ErrorCode: v.ErrorCode}
		switch v.State {
		case app.ManagedProgressAccepted:
			ev.State = app.ProviderCallStateAccepted
			ev.Outcome = app.ProviderCallOutcomeOK
			ev.ProviderTaskID = &v.TaskID
		case app.ManagedProgressRejected:
			ev.State = app.ProviderCallStateUnknown
			ev.Outcome = app.ProviderCallOutcomeUnknown
		case app.ManagedProgressSucceeded:
			ev.State = app.ProviderCallStateCompleted
			ev.Outcome = app.ProviderCallOutcomeOK
			ev.Managed = true
		case app.ManagedProgressNotSubmitted:
			ev.State = app.ProviderCallStateNotSubmitted
			ev.Outcome = app.ProviderCallOutcomeError
		}
		err = s.CompleteProviderCall(ctx, ev)
		if errors.Is(err, app.ErrProviderCostUnknown) {
			return soon, change(ctx, s, l, domain.StatusUnknown, "provider_cost_unknown", nil)
		}
		return soon, err
	}
	if op.Status != domain.StatusSubmitted && op.Status != domain.StatusManual && op.Status != domain.StatusReconciling {
		return soon, nil
	}
	taskID := l.ProviderTaskID
	if taskID == nil {
		taskID = e.SubmitTaskID
	}
	action := "recover"
	if taskID != nil {
		action = "query"
	}
	attempt := l.LastQueryAttempt + 1
	if action == "query" {
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "query", Attempt: attempt, ProviderTaskID: taskID}); err != nil {
			return soon, err
		}
	}
	v, err := p.Step(ctx, app.ManagedStep{OperationID: op.ID, Action: action})
	if err != nil {
		return soon, err
	}
	if action == "query" {
		ev := app.CompleteProviderCallInput{OperationID: op.ID, Action: "query", Attempt: attempt, ProviderTaskID: taskID, Outcome: app.ProviderCallOutcomeUnknown, State: app.ProviderCallStateUnknown}
		switch v.State {
		case app.ManagedProgressSucceeded:
			ev.Outcome = app.ProviderCallOutcomeOK
			ev.State = app.ProviderCallStateSucceeded
			ev.Managed = true
		case app.ManagedProgressFailed:
			ev.Outcome = app.ProviderCallOutcomeOK
			ev.State = app.ProviderCallStateFailed
			ev.ErrorCode = v.ErrorCode
		case app.ManagedProgressPending, app.ManagedProgressRunning:
			ev.Outcome = app.ProviderCallOutcomeOK
			ev.State = app.ProviderCallState(v.State)
		}
		if err := s.CompleteProviderCall(ctx, ev); err != nil {
			if !errors.Is(err, app.ErrProviderCostUnknown) {
				return soon, err
			}
			v.State = app.ManagedProgressUnknown
		}
	}
	if v.State == app.ManagedProgressSucceeded {
		if action == "recover" {
			err := s.CompleteProviderCall(ctx, app.CompleteProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, Outcome: app.ProviderCallOutcomeOK, State: app.ProviderCallStateCompleted, Managed: true})
			if err != nil {
				return soon, err
			}
		}
		return soon, change(ctx, s, l, domain.StatusSucceeded, "provider_succeeded", nil)
	}
	if v.State == app.ManagedProgressAccepted && op.Status == domain.StatusReconciling {
		if err := s.CompleteProviderCall(ctx, app.CompleteProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, Outcome: app.ProviderCallOutcomeOK, State: app.ProviderCallStateAccepted, ProviderTaskID: &v.TaskID}); err != nil {
			return soon, err
		}
		return soon, change(ctx, s, l, domain.StatusSubmitted, "provider_accepted", &v.TaskID)
	}
	if op.Status == domain.StatusSubmitted {
		if v.State == app.ManagedProgressFailed {
			return soon, nil
		}
		if v.State != app.ManagedProgressFailed && op.ConfirmedAt != nil && time.Now().Before(op.ConfirmedAt.Add(time.Duration(max(60000, l.Provider.ExpectedMaxMS))*2*time.Millisecond)) {
			return time.Now().Add(10 * time.Second), nil
		}
		return soon, change(ctx, s, l, domain.StatusReconciling, "provider_poll_unresolved", nil)
	}
	if op.Status == domain.StatusReconciling {
		return soon, change(ctx, s, l, domain.StatusManual, "provider_evidence_unavailable", nil)
	}
	return time.Now().Add(time.Minute), nil
}

// RegisterCodex registers handlers for admitted Codex generation work.
func RegisterCodex(h map[string]taskruntime.Handler, s OperationStore, p imageProvider) {
	h["operation.codex"] = operationHandler(s, func(ctx context.Context, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
		op := l.Operation
		if op.Status != domain.StatusSubmitting || e.SubmitState != app.ProviderCallStateUnset {
			return time.Time{}, nil
		}
		if p == nil {
			return time.Time{}, app.ErrWorkflowModelUnavailable
		}
		if op.ModelProfileVersionID == nil || op.PriceRuleVersionID == nil || l.Provider.AdapterKey != "codex" || l.Provider.Queue != "agent.codex" {
			return time.Time{}, ErrUnsupportedOperation
		}
		input, err := makeMockSubmitInput(l)
		if err != nil {
			return time.Time{}, err
		}
		input.ProjectID = op.ProjectID.String()
		input.ModelProfileVersionID = op.ModelProfileVersionID.String()
		input.PriceRuleVersionID = op.PriceRuleVersionID.String()
		input.Attempt = 1
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, DispatchRequired: true}); err != nil {
			return time.Time{}, err
		}
		result := ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
		if e.DispatchStartedAt == nil {
			result, err = p.Submit(ctx, input)
			if err != nil {
				return time.Time{}, err
			}
		}
		ev := app.CompleteProviderCallInput{OperationID: op.ID, Attempt: 1, Action: "submit", Outcome: app.ProviderCallOutcomeUnknown, State: app.ProviderCallStateUnknown}
		if result.Error != nil {
			ev.ErrorCode = result.Error.Code
		}
		switch result.Outcome {
		case ProviderSubmitCompleted:
			if matchesImageReceipt(result, input) {
				ev.Outcome = app.ProviderCallOutcomeOK
				ev.State = app.ProviderCallStateCompleted
				ev.Receipt = result.Receipt
			}
		case ProviderSubmitNotSubmitted:
			if result.Receipt == nil && result.ProviderTaskID == nil && result.Usage == nil {
				ev.Outcome = app.ProviderCallOutcomeError
				ev.State = app.ProviderCallStateNotSubmitted
			}
		}
		return time.Time{}, s.CompleteProviderCall(ctx, ev)
	}, nil)
}
func matchesImageReceipt(result ProviderSubmitOutput, input ProviderSubmitInput) bool {
	if result.ProviderTaskID != nil || result.Receipt == nil || result.Receipt.Validate() != nil {
		return false
	}
	i := result.Receipt.Identity
	if i.OperationID.String() != input.OperationID || i.ProjectID.String() != input.ProjectID || i.RequestKey != input.ProviderRequestKey || i.Attempt != input.Attempt ||
		i.ModelProfileVersionID.String() != input.ModelProfileVersionID || i.PriceRuleVersionID.String() != input.PriceRuleVersionID {
		return false
	}
	return result.Usage == nil // Codex token usage is not trusted image pricing.
}
