package task

import (
	"context"
	"encoding/json"
	"time"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// The catalog's explicit free mock adapter uses only its frozen fixture parameters.
// The mock request identity and task ID are reproducible after a process restart.
func mockStep(ctx context.Context, s OperationStore, l app.WorkflowOperation, e app.TaskEvidence) (time.Time, error) {
	op := l.Operation
	soon := time.Now().Add(time.Second)
	if l.Provider.AdapterKey != "mock" || l.Provider.Queue != "agent.mock" || !l.Provider.SupportsQuery {
		return time.Time{}, ErrUnsupportedOperation
	}
	var params mockParams
	if json.Unmarshal(op.Params, &params) != nil {
		return soon, ErrInvalidOperationInput
	}
	taskID := "mock-" + op.ID.String()
	switch op.Status {
	case domain.StatusSubmitting:
		if e.SubmitState != app.ProviderCallStateUnset {
			return soon, nil
		}
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1}); err != nil {
			return soon, err
		}
		ev := app.CompleteProviderCallInput{OperationID: op.ID, Action: "submit", Attempt: 1, Outcome: app.ProviderCallOutcomeOK, State: app.ProviderCallStateAccepted, ProviderTaskID: &taskID}
		switch params.Outcome {
		case ProviderSubmitRejected, ProviderSubmitNotSubmitted:
			ev.Outcome = app.ProviderCallOutcomeError
			ev.State = app.ProviderCallState(params.Outcome)
			ev.ProviderTaskID = nil
		case ProviderSubmitUnknown:
			ev.Outcome = app.ProviderCallOutcomeUnknown
			ev.State = app.ProviderCallStateUnknown
			ev.ProviderTaskID = nil
		}
		return soon, s.CompleteProviderCall(ctx, ev)
	case domain.StatusSubmitted, domain.StatusReconciling, domain.StatusManual:
		if op.Status == domain.StatusSubmitted && e.CancelRequested && l.Provider.SupportsCancel && e.CancelState == app.ProviderCallStateUnset {
			return soon, change(ctx, s, l, domain.StatusCancelling, "cancel_requested", nil)
		}
		attempt := l.LastQueryAttempt + 1
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "query", Attempt: attempt, ProviderTaskID: l.ProviderTaskID}); err != nil {
			return soon, err
		}
		state := app.ProviderCallStateSucceeded
		due := time.Now()
		if op.ConfirmedAt != nil {
			due = op.ConfirmedAt.Add(time.Duration(params.DelayMS) * time.Millisecond)
			if due.After(time.Now()) {
				state = app.ProviderCallStatePending
			}
		}
		if err := s.CompleteProviderCall(ctx, app.CompleteProviderCallInput{OperationID: op.ID, Action: "query", Attempt: attempt, Outcome: app.ProviderCallOutcomeOK, State: state, ProviderTaskID: &taskID}); err != nil {
			return soon, err
		}
		if op.Status == domain.StatusReconciling {
			return soon, change(ctx, s, l, domain.StatusSubmitted, "provider_accepted", &taskID)
		}
		if state == app.ProviderCallStatePending {
			return due, nil
		}
		return soon, change(ctx, s, l, domain.StatusSucceeded, "provider_succeeded", nil)
	case domain.StatusCancelling:
		if e.CancelState != app.ProviderCallStateUnset {
			return soon, nil
		}
		if err := s.BeginProviderCall(ctx, app.BeginProviderCallInput{OperationID: op.ID, Action: "cancel", Attempt: 1, ProviderTaskID: &taskID}); err != nil {
			return soon, err
		}
		return soon, s.CompleteProviderCall(ctx, app.CompleteProviderCallInput{OperationID: op.ID, Action: "cancel", Attempt: 1, Outcome: app.ProviderCallOutcomeOK, State: app.ProviderCallStateCancelled, ProviderTaskID: &taskID})
	case domain.StatusSucceeded, domain.StatusIngesting:
		return time.Time{}, nil
	default:
		return soon, nil
	}
}
