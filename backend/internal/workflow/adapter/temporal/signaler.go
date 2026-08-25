package temporal

import (
	"context"
	"errors"
	"strings"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const signalIdentity = "lanverse-backend"

func (runtime *Client) Signal(ctx context.Context, request domain.SignalRequest) (domain.SignalObservation, error) {
	if runtime == nil || runtime.client == nil || !validSignalRequest(request) {
		return domain.SignalObservation{}, errors.New("invalid Temporal workflow signal request")
	}
	observedHash, found, _ := runtime.findSignal(ctx, request)
	if found {
		return reconciledSignal(request.InputHash, observedHash), nil
	}
	payloads, err := runtime.dataConverter.ToPayloads(HumanGateSignal{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: request.NodeRunID,
		SignalID: request.SignalID, SignalIntentID: request.SignalIntentID, Decision: request.Decision,
		OwnerReceiptID: request.OwnerReceiptID, Output: request.Output, OutputHash: request.OutputHash,
	})
	if err != nil {
		return domain.SignalObservation{}, err
	}
	_, signalErr := runtime.client.WorkflowService().SignalWorkflowExecution(ctx, &workflowservice.SignalWorkflowExecutionRequest{
		Namespace: runtime.namespace,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: request.TemporalWorkflowID,
		},
		SignalName: HumanGateSignalName,
		Input:      payloads,
		Identity:   signalIdentity,
		RequestId:  request.SignalID,
	})
	if signalErr == nil {
		observedHash, found, reconcileErr := runtime.awaitSignalHistory(ctx, request)
		if !found {
			return domain.SignalObservation{Outcome: domain.SignalOutcomeUnknown}, reconcileErr
		}
		if found && observedHash != request.InputHash {
			return reconciledSignal(request.InputHash, observedHash), nil
		}
		return domain.SignalObservation{Outcome: domain.SignalOutcomeSignaled, ObservedInputHash: request.InputHash}, nil
	}
	observedHash, found, reconcileErr := runtime.findSignal(ctx, request)
	if found {
		return reconciledSignal(request.InputHash, observedHash), nil
	}
	return domain.SignalObservation{Outcome: domain.SignalOutcomeUnknown}, errors.Join(signalErr, reconcileErr)
}

func (runtime *Client) awaitSignalHistory(ctx context.Context, request domain.SignalRequest) (string, bool, error) {
	reconcileContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		observedHash, found, err := runtime.findSignal(reconcileContext, request)
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

func (runtime *Client) findSignal(ctx context.Context, request domain.SignalRequest) (string, bool, error) {
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
		if attributes.GetSignalName() != HumanGateSignalName {
			if attributes.GetRequestId() == request.SignalID {
				return "", true, nil
			}
			continue
		}
		var signal HumanGateSignal
		if err = runtime.dataConverter.FromPayloads(attributes.GetInput(), &signal); err != nil {
			if attributes.GetRequestId() == request.SignalID {
				return "", true, nil
			}
			continue
		}
		if signal.SignalID != request.SignalID {
			continue
		}
		observed := domain.SignalRequest{
			TemporalWorkflowID: request.TemporalWorkflowID, SignalID: signal.SignalID,
			SignalIntentID: signal.SignalIntentID, WorkflowRunID: signal.WorkflowRunID,
			NodeRunID: signal.NodeRunID, Decision: signal.Decision,
			OwnerReceiptID: signal.OwnerReceiptID, Output: signal.Output, OutputHash: signal.OutputHash,
		}
		observedHash, hashErr := platformcommand.InputHash(observed)
		if hashErr != nil {
			return "", false, hashErr
		}
		return observedHash, true, nil
	}
	return "", false, nil
}

func reconciledSignal(expectedHash, observedHash string) domain.SignalObservation {
	outcome := domain.SignalOutcomeConflict
	if expectedHash == observedHash {
		outcome = domain.SignalOutcomeAlreadyApplied
	}
	return domain.SignalObservation{Outcome: outcome, ObservedInputHash: observedHash}
}

func validSignalRequest(request domain.SignalRequest) bool {
	if strings.TrimSpace(request.TemporalWorkflowID) == "" || strings.TrimSpace(request.SignalID) == "" ||
		strings.TrimSpace(request.SignalIntentID) == "" || strings.TrimSpace(request.WorkflowRunID) == "" ||
		strings.TrimSpace(request.NodeRunID) == "" || len(request.InputHash) != 64 {
		return false
	}
	switch request.Decision {
	case "APPROVED", "SELECTED":
		_, _, outputHash, outputErr := domain.BuildNodeOutput(request.Output)
		if outputErr != nil || strings.TrimSpace(request.OwnerReceiptID) == "" || request.OutputHash != outputHash {
			return false
		}
	case "REJECTED", "CHANGES_REQUESTED":
		if request.OwnerReceiptID != "" || request.OutputHash != "" || request.Output.SchemaVersion != "" || len(request.Output.Bindings) != 0 {
			return false
		}
	default:
		return false
	}
	expected := request
	expected.InputHash = ""
	expectedHash, err := platformcommand.InputHash(expected)
	return err == nil && expectedHash == request.InputHash
}

var (
	_ workflowapp.WorkflowStarter  = (*Client)(nil)
	_ workflowapp.WorkflowSignaler = (*Client)(nil)
)
