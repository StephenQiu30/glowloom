package workflow

import (
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// runManagedProvider shares operation, dispatch and billing state with the rest
// of the product. Provider activities receive IDs, never Keys or signed URLs.
func runManagedProvider(ctx, flowCtx workflow.Context, loaded application.WorkflowOperation) error {
	if loaded.Provider.Queue != "provider" || loaded.Provider.SupportsCancel || loaded.Operation.ModelProfileVersionID == nil || loaded.Operation.PriceRuleVersionID == nil {
		return ErrUnsupportedOperation
	}
	id := loaded.Operation.ID
	status := loaded.Operation.Status
	providerCtx := providerActivityContext(ctx, "provider", 10*time.Minute, 1)
	step := func(action string) (application.ManagedProgress, error) {
		var p application.ManagedProgress
		err := workflow.ExecuteActivity(providerCtx, "provider.managed", application.ManagedStep{OperationID: id, Action: action}).Get(providerCtx, &p)
		return p, err
	}
	calls := &providerCalls{flowCtx: flowCtx, operationID: id}
	// A completed output without enough billable usage is evidence of execution,
	// never evidence for a refund or another submission. Keep its reservation and
	// wait for verified reconciliation before retrying the local accounting step.
	completeManaged := func(input application.CompleteProviderCallInput) error {
		for {
			err := calls.complete(input)
			if err == nil {
				return nil
			}
			var appErr *temporal.ApplicationError
			if !errors.As(err, &appErr) || appErr.Type() != "provider_cost_unknown" {
				return err
			}
			for status != domain.StatusManual {
				next := domain.StatusReconciling
				if status == domain.StatusSubmitting {
					next = domain.StatusUnknown
				}
				if status == domain.StatusReconciling {
					next = domain.StatusManual
				}
				if err := transition(flowCtx, id, []domain.Status{status}, next, "provider_cost_unknown", nil, nil); err != nil {
					return err
				}
				status = next
			}
			if _, err := waitImageManualResolution(ctx); err != nil {
				return err
			}
		}
	}

	var progress application.ManagedProgress
	if status == domain.StatusConfirmed {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitting, "submit", &loaded.ProviderRequestKey, nil); err != nil {
			return err
		}
		status = domain.StatusSubmitting
		if err := workflow.ExecuteActivity(flowCtx, "flow.BeginProviderCall", application.BeginProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, DispatchRequired: true}).Get(flowCtx, nil); err != nil {
			return err
		}
		var err error
		progress, err = step("submit")
		if err != nil {
			if isApplicationErrorType(err, "execution_authority_revoked") {
				return err
			}
			progress = application.ManagedProgress{State: "unknown"}
		}
		for progress.State == "waiting" {
			if err := workflow.Sleep(ctx, 5*time.Second); err != nil {
				return err
			}
			progress, err = step("submit")
			if err != nil {
				if isApplicationErrorType(err, "execution_authority_revoked") {
					return err
				}
				progress = application.ManagedProgress{State: "unknown"}
			}
		}
		switch progress.State {
		case "not_submitted", "rejected":
			if err := calls.complete(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "error", State: progress.State}); err != nil {
				return err
			}
			if progress.ErrorCode == "provider_dispatch_cancelled" {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusCancelling, "user_cancel_before_dispatch", nil, nil); err != nil {
					return err
				}
				return settleOperation(flowCtx, id.String(), domain.StatusCancelling, domain.StatusCancelled, 0, "", false)
			}
			return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, "provider_"+progress.State, false)
		case "accepted", "succeeded":
		default:
			if err := calls.complete(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "unknown", State: "unknown"}); err != nil {
				return err
			}
		}
	}
	if status == domain.StatusSubmitting {
		if progress.State != "accepted" && progress.State != "succeeded" {
			var err error
			progress, err = step("recover")
			if err != nil {
				if isApplicationErrorType(err, "execution_authority_revoked") {
					return err
				}
				progress = application.ManagedProgress{State: "unknown"}
			}
		}
		switch {
		case progress.State == "accepted":
			if err := calls.complete(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "ok", State: "accepted", ProviderTaskID: &progress.TaskID}); err != nil {
				return err
			}
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitted, "provider_accepted", nil, &progress.TaskID); err != nil {
				return err
			}
			status = domain.StatusSubmitted
		case progress.State == "succeeded" && progress.TaskID == "":
			if err := completeManaged(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}); err != nil {
				return err
			}
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSucceeded, "provider_succeeded", nil, nil); err != nil {
				return err
			}
			status = domain.StatusSucceeded
		default:
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusUnknown, "provider_result_unknown", nil, nil); err != nil {
				return err
			}
			status = domain.StatusUnknown
		}
	}
	if status == domain.StatusUnknown {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling, "recover_provider_receipt", nil, nil); err != nil {
			return err
		}
		status = domain.StatusReconciling
	}
	attempt := loaded.LastQueryAttempt
	deadline := workflow.Now(ctx).Add(time.Duration(max(loaded.Provider.ExpectedMaxMS, 60000)) * 2 * time.Millisecond)
	for status == domain.StatusSubmitted || status == domain.StatusReconciling || status == domain.StatusManual {
		if status == domain.StatusManual {
			resolution, err := waitImageManualResolution(ctx)
			if err != nil {
				return err
			}
			if resolution.Outcome == "not_executed" {
				if err := workflow.ExecuteActivity(flowCtx, "flow.CheckManualNotExecuted", id.String()).Get(flowCtx, nil); err == nil {
					return settleOperation(flowCtx, id.String(), status, domain.StatusFailed, 0, "provider_not_executed", false)
				}
			}
		}
		recovered, err := step("recover")
		if isApplicationErrorType(err, "execution_authority_revoked") {
			return err
		}
		if err == nil && recovered.State == "succeeded" && recovered.TaskID == "" {
			if err := completeManaged(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}); err != nil {
				return err
			}
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSucceeded, "recovered_provider_result", nil, nil); err != nil {
				return err
			}
			status = domain.StatusSucceeded
			break
		}
		taskID := recovered.TaskID
		if taskID != "" {
			if err := calls.complete(application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "ok", State: "accepted", ProviderTaskID: &taskID}); err != nil {
				return err
			}
			if status == domain.StatusReconciling {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitted, "recovered_provider_task", nil, &taskID); err != nil {
					return err
				}
				status = domain.StatusSubmitted
			}
			attempt++
			// Resume after the last persisted query attempt across explicit restarts.
			// BeginProviderCall rejects conflicting evidence; query itself is read-only.
			if err := calls.begin("query", attempt, &taskID); err != nil {
				return err
			}
			result, queryErr := step("query")
			evidence := application.CompleteProviderCallInput{OperationID: id, Action: "query", Attempt: attempt, ProviderTaskID: &taskID, Outcome: "unknown", State: "unknown"}
			if queryErr == nil {
				switch result.State {
				case "pending", "running":
					evidence.Outcome = "ok"
					evidence.State = result.State
				case "succeeded":
					evidence.Outcome = "ok"
					evidence.State = "succeeded"
					evidence.Managed = true
				}
			}
			complete := calls.complete
			if evidence.Managed {
				complete = completeManaged
			}
			if err := complete(evidence); err != nil {
				return err
			}
			if isApplicationErrorType(queryErr, "execution_authority_revoked") {
				return queryErr
			}
			if queryErr == nil && result.State == "failed" && status == domain.StatusSubmitted {
				// Generation failure is known, supplier charge is not. Preserve the
				// original task and hold rather than inventing a zero-cost refund.
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling, "provider_failed_cost_unknown", nil, nil); err != nil {
					return err
				}
				status = domain.StatusReconciling
			}
			if evidence.State == "succeeded" {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSucceeded, "provider_succeeded", nil, nil); err != nil {
					return err
				}
				status = domain.StatusSucceeded
				break
			}
		}
		if status == domain.StatusSubmitted && workflow.Now(ctx).Before(deadline) {
			if err := workflow.Sleep(ctx, 10*time.Second); err != nil {
				return err
			}
			continue
		}
		if status == domain.StatusSubmitted {
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling, "provider_poll_unresolved", nil, nil); err != nil {
				return err
			}
			status = domain.StatusReconciling
		}
		if status == domain.StatusReconciling {
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusManual, "provider_evidence_unavailable", nil, nil); err != nil {
				return err
			}
			status = domain.StatusManual
		}
	}
	if status == domain.StatusSucceeded {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusIngesting, "ingest", nil, nil); err != nil {
			return err
		}
		status = domain.StatusIngesting
	}
	if status != domain.StatusIngesting {
		return ErrUnsupportedOperation
	}
	mediaCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 20 * time.Minute, RetryPolicy: &temporal.RetryPolicy{InitialInterval: 10 * time.Second, MaximumInterval: 10 * time.Minute}})
	if err := workflow.ExecuteActivity(mediaCtx, "media.IngestManaged", id).Get(mediaCtx, nil); err != nil {
		return err
	}
	var cost application.ProviderCost
	if err := workflow.ExecuteActivity(flowCtx, "flow.LoadProviderCost", id.String()).Get(flowCtx, &cost); err != nil {
		return err
	}
	return settleOperation(flowCtx, id.String(), status, domain.StatusCompleted, cost.ActualCostMicros, "", false)
}
