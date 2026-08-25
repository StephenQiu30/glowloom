package temporal

import (
	"context"
	"errors"
	"strings"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const controlReasonPrefix = "lanverse-control:"

type controlHistory struct {
	observedInputHash string
	found             bool
	terminal          string
}

func (runtime *Client) Control(
	ctx context.Context,
	request domain.ControlRequest,
) (domain.ControlObservation, error) {
	if runtime == nil || runtime.client == nil || !validControlRequest(request) {
		return domain.ControlObservation{}, errors.New("invalid Temporal workflow control request")
	}
	if request.Action == domain.ControlActionPause || request.Action == domain.ControlActionResume {
		return runtime.signalControl(ctx, request)
	}
	history, historyErr := runtime.findControl(ctx, request)
	if history.found {
		return reconciledControl(request.InputHash, history, false), nil
	}
	if historyErr != nil {
		var notFound *serviceerror.NotFound
		if errors.As(historyErr, &notFound) {
			return domain.ControlObservation{Outcome: domain.ControlOutcomeConflict}, nil
		}
	}
	_, controlErr := runtime.client.WorkflowService().RequestCancelWorkflowExecution(
		ctx,
		&workflowservice.RequestCancelWorkflowExecutionRequest{
			Namespace: runtime.namespace,
			WorkflowExecution: &commonpb.WorkflowExecution{
				WorkflowId: request.TemporalWorkflowID,
			},
			Identity:  signalIdentity,
			RequestId: request.ControlID,
			Reason:    controlReason(request),
		},
	)
	if controlErr == nil {
		history, reconcileErr := runtime.awaitControlHistory(ctx, request)
		if history.found {
			return reconciledControl(request.InputHash, history, true), nil
		}
		return domain.ControlObservation{Outcome: domain.ControlOutcomeUnknown}, reconcileErr
	}
	history, reconcileErr := runtime.findControl(ctx, request)
	if history.found {
		return reconciledControl(request.InputHash, history, false), nil
	}
	var notFound *serviceerror.NotFound
	if errors.As(controlErr, &notFound) {
		return domain.ControlObservation{Outcome: domain.ControlOutcomeConflict}, nil
	}
	return domain.ControlObservation{Outcome: domain.ControlOutcomeUnknown}, errors.Join(controlErr, reconcileErr)
}

func (runtime *Client) signalControl(
	ctx context.Context,
	request domain.ControlRequest,
) (domain.ControlObservation, error) {
	observedHash, found, historyErr := runtime.findControlSignal(ctx, request)
	if found {
		return reconciledControlSignal(request.InputHash, observedHash, false), nil
	}
	if historyErr != nil {
		var notFound *serviceerror.NotFound
		if errors.As(historyErr, &notFound) {
			return domain.ControlObservation{Outcome: domain.ControlOutcomeConflict}, nil
		}
	}
	payloads, err := runtime.dataConverter.ToPayloads(WorkflowControlSignal{
		WorkflowRunID: request.WorkflowRunID, ControlID: request.ControlID,
		Action: request.Action, InputHash: request.InputHash,
	})
	if err != nil {
		return domain.ControlObservation{}, err
	}
	_, signalErr := runtime.client.WorkflowService().SignalWorkflowExecution(
		ctx,
		&workflowservice.SignalWorkflowExecutionRequest{
			Namespace: runtime.namespace,
			WorkflowExecution: &commonpb.WorkflowExecution{
				WorkflowId: request.TemporalWorkflowID,
			},
			SignalName: WorkflowControlSignalName,
			Input:      payloads,
			Identity:   signalIdentity,
			RequestId:  request.ControlID,
		},
	)
	if signalErr == nil {
		observedHash, found, reconcileErr := runtime.awaitControlSignalHistory(ctx, request)
		if found {
			return reconciledControlSignal(request.InputHash, observedHash, true), nil
		}
		return domain.ControlObservation{Outcome: domain.ControlOutcomeUnknown}, reconcileErr
	}
	observedHash, found, reconcileErr := runtime.findControlSignal(ctx, request)
	if found {
		return reconciledControlSignal(request.InputHash, observedHash, false), nil
	}
	var notFound *serviceerror.NotFound
	if errors.As(signalErr, &notFound) {
		return domain.ControlObservation{Outcome: domain.ControlOutcomeConflict}, nil
	}
	return domain.ControlObservation{Outcome: domain.ControlOutcomeUnknown}, errors.Join(signalErr, reconcileErr)
}

func (runtime *Client) awaitControlSignalHistory(
	ctx context.Context,
	request domain.ControlRequest,
) (string, bool, error) {
	reconcileContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		observedHash, found, err := runtime.findControlSignal(reconcileContext, request)
		if found || err != nil {
			return observedHash, found, err
		}
		select {
		case <-reconcileContext.Done():
			return "", false, reconcileContext.Err()
		case <-ticker.C:
		}
	}
}

func (runtime *Client) findControlSignal(
	ctx context.Context,
	request domain.ControlRequest,
) (string, bool, error) {
	iterator := runtime.client.GetWorkflowHistory(
		ctx, request.TemporalWorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			return "", false, err
		}
		attributes := event.GetWorkflowExecutionSignaledEventAttributes()
		if attributes == nil {
			continue
		}
		if attributes.GetSignalName() != WorkflowControlSignalName {
			if attributes.GetRequestId() == request.ControlID {
				return "", true, nil
			}
			continue
		}
		var signal WorkflowControlSignal
		if err = runtime.dataConverter.FromPayloads(attributes.GetInput(), &signal); err != nil {
			if attributes.GetRequestId() == request.ControlID {
				return "", true, nil
			}
			continue
		}
		if signal.ControlID != request.ControlID {
			if attributes.GetRequestId() == request.ControlID {
				return "", true, nil
			}
			continue
		}
		observed := domain.ControlRequest{
			TemporalWorkflowID: request.TemporalWorkflowID, ControlID: signal.ControlID,
			WorkflowRunID: signal.WorkflowRunID, Action: signal.Action,
		}
		observedHash, hashErr := platformcommand.InputHash(observed)
		if hashErr != nil {
			return "", false, hashErr
		}
		if signal.InputHash != observedHash {
			return signal.InputHash, true, nil
		}
		return observedHash, true, nil
	}
	return "", false, nil
}

func reconciledControlSignal(expectedHash, observedHash string, signaledNow bool) domain.ControlObservation {
	outcome := domain.ControlOutcomeConflict
	if expectedHash == observedHash {
		outcome = domain.ControlOutcomeAlreadyApplied
		if signaledNow {
			outcome = domain.ControlOutcomeApplied
		}
	}
	return domain.ControlObservation{Outcome: outcome, ObservedInputHash: observedHash}
}

func (runtime *Client) awaitControlHistory(
	ctx context.Context,
	request domain.ControlRequest,
) (controlHistory, error) {
	reconcileContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var latest controlHistory
	for {
		history, err := runtime.findControl(reconcileContext, request)
		if err != nil {
			return history, err
		}
		latest = history
		if history.found && history.terminal != "" {
			return history, nil
		}
		select {
		case <-reconcileContext.Done():
			if latest.found {
				return latest, nil
			}
			return latest, reconcileContext.Err()
		case <-ticker.C:
		}
	}
}

func (runtime *Client) findControl(
	ctx context.Context,
	request domain.ControlRequest,
) (controlHistory, error) {
	iterator := runtime.client.GetWorkflowHistory(
		ctx, request.TemporalWorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	result := controlHistory{}
	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			return result, err
		}
		if attributes := event.GetWorkflowExecutionCancelRequestedEventAttributes(); attributes != nil {
			observedHash, found := parseControlReason(attributes.GetCause(), request.ControlID)
			if found {
				result.found = true
				result.observedInputHash = observedHash
			}
		}
		switch {
		case event.GetWorkflowExecutionCanceledEventAttributes() != nil:
			result.terminal = "canceled"
		case event.GetWorkflowExecutionCompletedEventAttributes() != nil:
			result.terminal = "completed"
		case event.GetWorkflowExecutionFailedEventAttributes() != nil:
			result.terminal = "failed"
		case event.GetWorkflowExecutionTerminatedEventAttributes() != nil:
			result.terminal = "terminated"
		case event.GetWorkflowExecutionTimedOutEventAttributes() != nil:
			result.terminal = "timed_out"
		}
	}
	return result, nil
}

func reconciledControl(expectedHash string, history controlHistory, requestedNow bool) domain.ControlObservation {
	observation := domain.ControlObservation{ObservedInputHash: history.observedInputHash}
	switch {
	case history.observedInputHash != expectedHash:
		observation.Outcome = domain.ControlOutcomeConflict
	case history.terminal == "canceled" && requestedNow:
		observation.Outcome = domain.ControlOutcomeApplied
	case history.terminal == "canceled":
		observation.Outcome = domain.ControlOutcomeAlreadyApplied
	case history.terminal != "":
		observation.Outcome = domain.ControlOutcomeConflict
	default:
		observation.Outcome = domain.ControlOutcomeRequested
	}
	return observation
}

func controlReason(request domain.ControlRequest) string {
	return controlReasonPrefix + request.ControlID + ":" + request.InputHash
}

func parseControlReason(reason, controlID string) (string, bool) {
	prefix := controlReasonPrefix + controlID + ":"
	if !strings.HasPrefix(reason, prefix) {
		return "", false
	}
	return strings.TrimPrefix(reason, prefix), true
}

func validControlRequest(request domain.ControlRequest) bool {
	if strings.TrimSpace(request.TemporalWorkflowID) == "" || strings.TrimSpace(request.ControlID) == "" ||
		strings.TrimSpace(request.WorkflowRunID) == "" ||
		len(request.InputHash) != 64 {
		return false
	}
	switch request.Action {
	case domain.ControlActionCancel, domain.ControlActionPause, domain.ControlActionResume:
	default:
		return false
	}
	expectedHash, err := platformcommand.InputHash(domain.ControlRequest{
		TemporalWorkflowID: request.TemporalWorkflowID, ControlID: request.ControlID,
		WorkflowRunID: request.WorkflowRunID, Action: request.Action,
	})
	return err == nil && expectedHash == request.InputHash
}

var _ workflowapp.WorkflowController = (*Client)(nil)
