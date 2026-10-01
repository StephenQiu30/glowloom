package workflow

import (
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// runCodexImage isolates the first synchronous protocol from legacy mock
// commands. A generated image does not establish a Codex-plan charge. Until a
// trusted image pricing and review contract exists, it retains the reservation.
func runCodexImage(ctx, flowCtx workflow.Context, loaded application.WorkflowOperation) error {
	if loaded.Provider.Queue != "agent.codex" || loaded.Provider.SupportsQuery || loaded.Provider.SupportsCancel || loaded.ProviderTaskID != nil ||
		loaded.Operation.Capability != "image.generate" || loaded.Operation.Mode != "text_to_image" || loaded.Operation.OutputCount != 1 ||
		loaded.Operation.ModelProfileVersionID == nil || loaded.Operation.PriceRuleVersionID == nil || loaded.ProviderRequestKey == "" {
		return ErrUnsupportedOperation
	}
	status := loaded.Operation.Status
	id := loaded.Operation.ID
	if status == domain.StatusConfirmed {
		input, err := makeMockSubmitInput(loaded)
		if err != nil {
			return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, "unsupported_codex_input", false)
		}
		input.ProjectID = loaded.Operation.ProjectID.String()
		input.ModelProfileVersionID = loaded.Operation.ModelProfileVersionID.String()
		input.PriceRuleVersionID = loaded.Operation.PriceRuleVersionID.String()
		input.Attempt = 1
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitting, "submit", &loaded.ProviderRequestKey, nil); err != nil {
			if code := preSubmitFailureCode(err); code != "" {
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, code, false)
			}
			return err
		}
		status = domain.StatusSubmitting
		calls := &providerCalls{flowCtx: flowCtx, operationID: id}
		providerCtx := providerActivityContext(ctx, loaded.Provider.Queue, 10*time.Minute, 1)
		result, err := calls.submitImage(providerCtx, input)
		if err != nil {
			return err
		}
		switch result.Outcome {
		case ProviderSubmitCompleted:
			// Even explicit zero or token usage cannot establish an image charge.
			// Read durable evidence so missing pricing has a distinct audit reason.
			if err := imageChargeGate(flowCtx, id.String()); err != nil {
				return err
			}
		case ProviderSubmitNotSubmitted:
			if result.Error != nil && result.Error.Code == "provider_dispatch_cancelled" {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusCancelling, "user_cancel_before_dispatch", nil, nil); err != nil {
					return err
				}
				return settleOperation(flowCtx, id.String(), domain.StatusCancelling, domain.StatusCancelled, 0, "", false)
			}
			return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, "provider_not_submitted", true)
		case ProviderSubmitRejected:
			return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, providerFailureCode(result.Error), false)
		case ProviderSubmitUnknown:
		default:
			return ErrInvalidProviderResult
		}
	}
	if status == domain.StatusSubmitting {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusUnknown, "synchronous_result_or_cost_unknown", nil, nil); err != nil {
			return err
		}
		status = domain.StatusUnknown
	}
	if status == domain.StatusUnknown {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling, "reconcile_synchronous_receipt", nil, nil); err != nil {
			return err
		}
		status = domain.StatusReconciling
	}
	if status == domain.StatusReconciling {
		if err := recoverImageEvidence(flowCtx, loaded); err != nil {
			return err
		}
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusManual, "synchronous_charge_unverified", nil, nil); err != nil {
			return err
		}
		status = domain.StatusManual
	}
	if status != domain.StatusManual {
		return ErrUnsupportedOperation
	}
	for {
		resolution, err := waitImageManualResolution(ctx)
		if err != nil {
			return err
		}
		if resolution.Outcome == "not_executed" {
			if err := workflow.ExecuteActivity(flowCtx, "flow.CheckManualNotExecuted", id.String()).Get(flowCtx, nil); err != nil {
				if isApplicationErrorType(err, "manual_resolution_unverified") {
					continue
				}
				return err
			}
			if err := settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, "provider_not_executed", true); err != nil {
				if isApplicationErrorType(err, "manual_resolution_unverified") {
					continue
				}
				return err
			}
			return nil
		}
		if resolution.Outcome == "succeeded" {
			// Signals trigger the same receipt/cost check; their task ID or extra
			// fields never authorize a query, a second turn, or a zero settlement.
			if err := recoverImageEvidence(flowCtx, loaded); err != nil {
				return err
			}
		}
	}
}

func waitImageManualResolution(ctx workflow.Context) (ManualResolution, error) {
	var resolution ManualResolution
	channel := workflow.GetSignalChannel(ctx, "resolve_manual")
	err := workflow.Await(ctx, func() bool { return channel.ReceiveAsync(&resolution) })
	return resolution, err
}

func imageChargeGate(flowCtx workflow.Context, operationID string) error {
	var cost application.ProviderCost
	err := workflow.ExecuteActivity(flowCtx, "flow.LoadProviderCost", operationID).Get(flowCtx, &cost)
	if err != nil && !isApplicationErrorType(err, "provider_cost_unknown") && !isApplicationErrorType(err, "manual_resolution_unverified") {
		return err
	}
	// A successfully read amount also cannot open the Codex path until the
	// image-specific price mapping and real review gate are implemented.
	return nil
}

func recoverImageEvidence(flowCtx workflow.Context, loaded application.WorkflowOperation) error {
	var receipt *application.ProviderReceipt
	err := workflow.ExecuteActivity(flowCtx, "flow.RecoverProviderImage", loaded.Operation.ID.String()).Get(flowCtx, &receipt)
	if err != nil {
		if isApplicationErrorType(err, "image_recovery_unavailable") || isApplicationErrorType(err, "illegal_transition") {
			return nil // Retain the unresolved call and reservation for manual work.
		}
		return err
	}
	if receipt == nil {
		return nil
	}
	i := receipt.Identity
	if receipt.Validate() != nil || i.ProjectID != loaded.Operation.ProjectID || i.OperationID != loaded.Operation.ID || i.RequestKey != loaded.ProviderRequestKey ||
		i.ModelProfileVersionID != *loaded.Operation.ModelProfileVersionID || i.PriceRuleVersionID != *loaded.Operation.PriceRuleVersionID {
		return nil // Untrusted recovery output cannot establish a billable result.
	}
	return imageChargeGate(flowCtx, loaded.Operation.ID.String())
}
