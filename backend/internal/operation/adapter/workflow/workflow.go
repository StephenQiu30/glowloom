package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrInvalidOperationInput means the durable workflow request is malformed.
	ErrInvalidOperationInput = errors.New("invalid operation workflow input")
	// ErrUnsupportedOperation means this workflow skeleton has no safe adapter path.
	ErrUnsupportedOperation = errors.New("unsupported operation workflow capability")
	// ErrInvalidProviderResult means an agent activity returned inconsistent facts.
	ErrInvalidProviderResult = errors.New("invalid provider activity result")
)

// OperationInput starts a single confirmed, non-batch generation.
type OperationInput struct {
	OperationID string `json:"operation_id"`
}

// ManualResolution is sent after an administrator records evidence for an
// unresolved provider request. The command layer owns authorization and audit.
type ManualResolution struct {
	Outcome        string `json:"outcome"`
	ProviderTaskID string `json:"provider_task_id,omitempty"`
}

// IngestOutput is the media queue's durable output identity.
type IngestOutput struct {
	OutputID     string `json:"output_id"`
	MediaAssetID string `json:"media_asset_id"`
	Kind         string `json:"kind"`
}

// ModerationOutput records only the mock review decision; the media activity
// persists it after checking the output belongs to this operation.
type ModerationOutput struct {
	Status   string   `json:"status"`
	Labels   []string `json:"labels"`
	Provider string   `json:"provider"`
}

// SettlementInput asks the flow activity to enter a terminal state and settle
// its reservation in one database transaction.
type SettlementInput struct {
	OperationID      string        `json:"operation_id"`
	From             domain.Status `json:"from"`
	To               domain.Status `json:"to"`
	ActualCostMicros int64         `json:"actual_cost_micros"`
	FailureCode      string        `json:"failure_code,omitempty"`
	Retryable        bool          `json:"retryable"`
	Reason           string        `json:"reason,omitempty"`
}

// OperationWorkflow runs the mock provider path. Every database and network
// call is an Activity; workflow code uses only deterministic Temporal APIs.
func OperationWorkflow(ctx workflow.Context, input OperationInput) error {
	id, err := uuid.Parse(input.OperationID)
	if err != nil || id.String() != input.OperationID {
		return ErrInvalidOperationInput
	}
	flowCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue: "flow", StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second, MaximumInterval: time.Minute,
		},
	})
	var loaded application.WorkflowOperation
	if err := workflow.ExecuteActivity(flowCtx, "flow.LoadOperation", input.OperationID).Get(flowCtx, &loaded); err != nil {
		return err
	}
	if loaded.Operation.ID != id || loaded.Operation.Status.IsTerminal() {
		if loaded.Operation.ID == id && loaded.Operation.Status.IsTerminal() {
			return nil
		}
		return ErrInvalidOperationInput
	}
	if loaded.Operation.BatchID != nil || loaded.Operation.TargetType == "agent_session" ||
		loaded.Operation.ReusedFromID != nil || loaded.Provider.AdapterKey != "mock" ||
		loaded.Provider.Queue != "agent.mock" || !loaded.Provider.SupportsQuery {
		return ErrUnsupportedOperation
	}
	mockStatus, err := mockModerationStatus(loaded.Operation.Params)
	if err != nil {
		if loaded.Operation.Status == domain.StatusConfirmed {
			return settleOperation(flowCtx, input.OperationID, domain.StatusConfirmed,
				domain.StatusFailed, 0, "invalid_mock_parameters", false)
		}
		return err
	}
	if loaded.Operation.Status == domain.StatusConfirmed {
		if err := workflow.ExecuteActivity(flowCtx, "flow.CheckConsent", input.OperationID).Get(flowCtx, nil); err != nil {
			if code := preSubmitFailureCode(err); code != "" {
				return settleOperation(flowCtx, input.OperationID, domain.StatusConfirmed,
					domain.StatusFailed, 0, code, false)
			}
			return err
		}
	}
	return runMockProvider(ctx, flowCtx, loaded, mockStatus)
}

func runMockProvider(ctx, flowCtx workflow.Context, loaded application.WorkflowOperation, mockStatus string) error {
	id := loaded.Operation.ID
	requestKey := loaded.ProviderRequestKey
	if requestKey == "" {
		return ErrInvalidOperationInput
	}
	status := loaded.Operation.Status
	calls := &providerCalls{flowCtx: flowCtx, operationID: id}
	taskID := ""
	if loaded.ProviderTaskID != nil {
		taskID = *loaded.ProviderTaskID
	}
	var result ProviderQueryOutput
	if status == domain.StatusConfirmed {
		submitInput, err := makeMockSubmitInput(loaded)
		if err != nil {
			return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
				0, "unsupported_mock_input", false)
		}
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitting,
			"submit", &requestKey, nil); err != nil {
			if code := preSubmitFailureCode(err); code != "" {
				return settleOperation(flowCtx, id.String(), domain.StatusConfirmed,
					domain.StatusFailed, 0, code, false)
			}
			return err
		}
		status = domain.StatusSubmitting
		providerCtx := providerActivityContext(ctx, loaded.Provider.Queue, 60*time.Second, 1)
		for attempt := 0; attempt < 3; attempt++ {
			submitted, err := calls.submit(providerCtx, submitInput, int32(attempt+1))
			if err != nil {
				return err
			}
			switch submitted.Outcome {
			case ProviderSubmitAccepted:
				taskID = *submitted.ProviderTaskID
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitted,
					"provider_accepted", nil, &taskID); err != nil {
					return err
				}
				status = domain.StatusSubmitted
			case ProviderSubmitRejected:
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, providerFailureCode(submitted.Error), false)
			case ProviderSubmitNotSubmitted:
				if attempt == 2 {
					return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
						0, "provider_not_submitted", true)
				}
				if err := workflow.Sleep(ctx, time.Duration(attempt+1)*time.Second); err != nil {
					return err
				}
				continue
			case ProviderSubmitUnknown:
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusUnknown,
					"provider_result_unknown", nil, nil); err != nil {
					return err
				}
				status = domain.StatusUnknown
			default:
				return ErrInvalidProviderResult
			}
			break
		}
	}
	if status == domain.StatusSubmitting {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusUnknown,
			"recover_ambiguous_submit", nil, nil); err != nil {
			return err
		}
		status = domain.StatusUnknown
	}
	if status == domain.StatusUnknown {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling,
			"reconcile", nil, nil); err != nil {
			return err
		}
		status = domain.StatusReconciling
	}
	pollLimit := 2 * time.Duration(loaded.Provider.ExpectedMaxMS) * time.Millisecond
	if pollLimit <= 0 {
		pollLimit = 2 * time.Hour
	}
	for status == domain.StatusReconciling || status == domain.StatusManual || status == domain.StatusSubmitted {
		switch status {
		case domain.StatusReconciling:
			var found bool
			var err error
			taskID, result, found, err = reconcileMockProvider(ctx, calls, loaded.Provider.Queue, requestKey)
			if err != nil {
				return err
			}
			if !found {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusManual,
					"reconcile_unresolved", nil, nil); err != nil {
					return err
				}
				status = domain.StatusManual
				continue
			}
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSubmitted,
				"reconciled_found", nil, &taskID); err != nil {
				return err
			}
			status = domain.StatusSubmitted
		case domain.StatusManual:
			resolution, err := waitManualResolution(ctx)
			if err != nil {
				return err
			}
			switch resolution.Outcome {
			case "not_executed":
				if err := workflow.ExecuteActivity(flowCtx, "flow.CheckManualNotExecuted", id.String()).Get(flowCtx, nil); err != nil {
					if isApplicationErrorType(err, "manual_resolution_unverified") {
						continue
					}
					return err
				}
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, "provider_not_executed", true)
			case "succeeded":
				if resolution.ProviderTaskID == "" {
					continue
				}
				// A manual signal carries an untrusted task ID. Resolve the frozen
				// request key first so another operation's result cannot be
				// attached to this operation.
				verified, err := calls.query(ctx, loaded.Provider.Queue, ProviderQueryInput{
					ProviderRequestKey: &requestKey,
				})
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					continue
				}
				if verified.ProviderTaskID == nil || *verified.ProviderTaskID != resolution.ProviderTaskID {
					continue
				}
				taskID = resolution.ProviderTaskID
				result, err = pollMockProvider(ctx, calls, loaded.Provider.Queue, taskID, pollLimit)
				if errors.Is(err, errNeedsReconciliation) {
					continue // Keep the reserved amount until another documented decision.
				}
				if err != nil {
					return err
				}
				if result.State == ProviderQueryFailed {
					return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
						0, providerFailureCode(result.Error), false)
				}
				if result.State != ProviderQuerySucceeded || len(result.ResultURLs) == 0 {
					return ErrInvalidProviderResult
				}
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusIngesting,
					"manual_succeeded", nil, &taskID); err != nil {
					return err
				}
				status = domain.StatusIngesting
			default:
				continue
			}
		case domain.StatusSubmitted:
			if taskID == "" {
				if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling,
					"missing_provider_task_id", nil, nil); err != nil {
					return err
				}
				status = domain.StatusReconciling
				continue
			}
			if result.State != ProviderQuerySucceeded && result.State != ProviderQueryFailed {
				var err error
				result, err = pollMockProvider(ctx, calls, loaded.Provider.Queue, taskID, pollLimit)
				if errors.Is(err, errNeedsReconciliation) {
					if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusReconciling,
						"poll_result_unknown", nil, nil); err != nil {
						return err
					}
					status = domain.StatusReconciling
					continue
				}
				if err != nil {
					return err
				}
			}
			if result.State == ProviderQueryFailed {
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, providerFailureCode(result.Error), false)
			}
			if result.State == ProviderQuerySucceeded && len(result.ResultURLs) == 0 {
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, "provider_result_invalid", true)
			}
			if result.State != ProviderQuerySucceeded {
				return ErrInvalidProviderResult
			}
			if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusSucceeded,
				"provider_succeeded", nil, nil); err != nil {
				return err
			}
			status = domain.StatusSucceeded
		}
	}
	if status == domain.StatusSucceeded {
		if err := transition(flowCtx, id, []domain.Status{status}, domain.StatusIngesting,
			"ingest", nil, nil); err != nil {
			return err
		}
		status = domain.StatusIngesting
	}
	if status != domain.StatusIngesting {
		return fmt.Errorf("%w: status %s", ErrUnsupportedOperation, status)
	}
	if len(result.ResultURLs) == 0 {
		for taskID == "" {
			found, err := calls.query(ctx, loaded.Provider.Queue, ProviderQueryInput{
				ProviderRequestKey: &requestKey,
			})
			if err == nil && found.ProviderTaskID != nil && *found.ProviderTaskID != "" {
				taskID, result = *found.ProviderTaskID, found
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := workflow.Sleep(ctx, 10*time.Minute); err != nil {
				return err
			}
		}
		for len(result.ResultURLs) == 0 {
			var err error
			result, err = pollMockProvider(ctx, calls, loaded.Provider.Queue, taskID, pollLimit)
			if err != nil && !errors.Is(err, errNeedsReconciliation) {
				return err
			}
			if errors.Is(err, errNeedsReconciliation) {
				if err := workflow.Sleep(ctx, 10*time.Minute); err != nil {
					return err
				}
				continue
			}
			if result.State == ProviderQueryFailed {
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, providerFailureCode(result.Error), false)
			}
			if result.State == ProviderQuerySucceeded && len(result.ResultURLs) == 0 {
				return settleOperation(flowCtx, id.String(), status, domain.StatusFailed,
					0, "provider_result_invalid", true)
			}
		}
	}
	return ingestAndFinish(ctx, flowCtx, loaded, result.ResultURLs, mockStatus)
}

func transition(ctx workflow.Context, id uuid.UUID, from []domain.Status, to domain.Status,
	reason string, requestKey, taskID *string,
) error {
	input := application.TransitionInput{
		OperationID: id, From: from, To: to, Reason: reason,
		ProviderRequestKey: requestKey, ProviderTaskID: taskID,
	}
	return workflow.ExecuteActivity(ctx, "flow.Transition", input).Get(ctx, nil)
}

func settleOperation(ctx workflow.Context, id string, from, to domain.Status,
	actual int64, failureCode string, retryable bool,
) error {
	return workflow.ExecuteActivity(ctx, "flow.SettleOperation", SettlementInput{
		OperationID: id, From: from, To: to, ActualCostMicros: actual,
		FailureCode: failureCode, Retryable: retryable,
	}).Get(ctx, nil)
}

func preSubmitFailureCode(err error) string {
	for _, code := range []string{"consent_unavailable", "input_not_ready"} {
		if isApplicationErrorType(err, code) {
			return code
		}
	}
	return ""
}

func isApplicationErrorType(err error, target string) bool {
	var applicationErr *temporal.ApplicationError
	return errors.As(err, &applicationErr) && applicationErr.Type() == target
}

func waitManualResolution(ctx workflow.Context) (ManualResolution, error) {
	var resolution ManualResolution
	if !workflow.GetSignalChannel(ctx, "resolve_manual").Receive(ctx, &resolution) {
		return ManualResolution{}, ctx.Err()
	}
	return resolution, nil
}

func providerActivityContext(ctx workflow.Context, queue string, timeout time.Duration, attempts int32) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue: queue, StartToCloseTimeout: timeout,
		RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: attempts},
	})
}

func reconcileMockProvider(ctx workflow.Context, calls *providerCalls, queue, requestKey string) (string, ProviderQueryOutput, bool, error) {
	for _, delay := range []time.Duration{time.Minute, 5 * time.Minute, 10 * time.Minute,
		20 * time.Minute, 30 * time.Minute, 50 * time.Minute} {
		if err := workflow.Sleep(ctx, delay); err != nil {
			return "", ProviderQueryOutput{}, false, err
		}
		result, err := calls.query(ctx, queue, ProviderQueryInput{
			ProviderRequestKey: &requestKey,
		})
		if err != nil {
			if ctx.Err() != nil {
				return "", ProviderQueryOutput{}, false, ctx.Err()
			}
			continue
		}
		if result.State == ProviderQueryNotFound {
			continue // not_found is not proof that the request was never accepted.
		}
		if result.ProviderTaskID == nil || *result.ProviderTaskID == "" {
			continue
		}
		return *result.ProviderTaskID, result, true, nil
	}
	return "", ProviderQueryOutput{}, false, nil
}

var errNeedsReconciliation = errors.New("provider result needs reconciliation")

func pollMockProvider(ctx workflow.Context, calls *providerCalls, queue, taskID string, limit time.Duration) (ProviderQueryOutput, error) {
	start := workflow.Now(ctx)
	backoff := []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second,
		15 * time.Second, 30 * time.Second}
	for attempt := 0; ; attempt++ {
		delay := 30 * time.Second
		if attempt < len(backoff) {
			delay = backoff[attempt]
		}
		if err := workflow.Sleep(ctx, delay); err != nil {
			return ProviderQueryOutput{}, err
		}
		result, err := calls.query(ctx, queue, ProviderQueryInput{
			ProviderTaskID: &taskID,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ProviderQueryOutput{}, ctx.Err()
			}
			return ProviderQueryOutput{}, errNeedsReconciliation
		}
		if result.State == ProviderQuerySucceeded || result.State == ProviderQueryFailed {
			return result, nil
		}
		if result.State == ProviderQueryNotFound || workflow.Now(ctx).Sub(start) > limit {
			return ProviderQueryOutput{}, errNeedsReconciliation
		}
		if result.State != ProviderQueryPending && result.State != ProviderQueryRunning {
			return ProviderQueryOutput{}, errNeedsReconciliation
		}
	}
}

func ingestAndFinish(ctx, flowCtx workflow.Context, loaded application.WorkflowOperation,
	urls []string, mockStatus string,
) error {
	if len(urls) == 0 || len(urls) > int(loaded.Operation.OutputCount) {
		return settleOperation(flowCtx, loaded.Operation.ID.String(), domain.StatusIngesting,
			domain.StatusFailed, 0, "provider_result_invalid", true)
	}
	mediaCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue: "media", StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout: 15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second, MaximumInterval: 5 * time.Minute, MaximumAttempts: 8,
		},
	})
	moderationCtx := providerActivityContext(ctx, "agent", 60*time.Second, 3)
	moderationDeadline := workflow.Now(ctx).Add(24 * time.Hour)
	moderationUnavailable := false
	for index, url := range urls {
		ingested, failureCode, err := ingestOutput(ctx, mediaCtx, loaded.Operation.ID,
			index+1, url)
		if err != nil {
			return err
		}
		if failureCode != "" {
			return settleOperation(flowCtx, loaded.Operation.ID.String(), domain.StatusIngesting,
				domain.StatusFailed, 0, failureCode, false)
		}
		unavailable, err := reviewOutput(ctx, moderationCtx, mediaCtx, loaded.Operation.ID,
			ingested, mockStatus, moderationDeadline)
		if err != nil {
			return err
		}
		moderationUnavailable = moderationUnavailable || unavailable
	}
	if moderationUnavailable {
		return settleOperation(flowCtx, loaded.Operation.ID.String(), domain.StatusIngesting,
			domain.StatusFailed, 0, "moderation_unavailable", true)
	}
	// The mock provider incurs no external spend. A reservation still closes in
	// the same transaction as completion, producing one release entry.
	return settleOperation(flowCtx, loaded.Operation.ID.String(), domain.StatusIngesting,
		domain.StatusCompleted, 0, "", false)
}

func ingestOutput(ctx, mediaCtx workflow.Context, operationID uuid.UUID,
	seqNo int, url string,
) (IngestOutput, string, error) {
	for {
		var ingested IngestOutput
		err := workflow.ExecuteActivity(mediaCtx, "media.Ingest", map[string]any{
			"operation_id": operationID.String(), "seq_no": seqNo, "url": url,
		}).Get(mediaCtx, &ingested)
		if ctx.Err() != nil {
			return IngestOutput{}, "", ctx.Err()
		}
		if err == nil && ingested.OutputID != "" && ingested.MediaAssetID != "" && ingested.Kind != "" {
			return ingested, "", nil
		}
		if temporal.IsCanceledError(err) {
			return IngestOutput{}, "", err
		}
		if code := permanentMediaFailureCode(err); code != "" {
			return IngestOutput{}, code, nil
		}
		// Activity retries were exhausted. Only retry transfer and registration;
		// the provider request must never be submitted again.
		if err := workflow.Sleep(ctx, 10*time.Minute); err != nil {
			return IngestOutput{}, "", err
		}
	}
}

func permanentMediaFailureCode(err error) string {
	var applicationErr *temporal.ApplicationError
	if !errors.As(err, &applicationErr) {
		return ""
	}
	switch applicationErr.Type() {
	case "unsupported_media", "provider_result_invalid", "result_expired":
		return applicationErr.Type()
	default:
		return ""
	}
}

func reviewOutput(ctx, moderationCtx, mediaCtx workflow.Context, operationID uuid.UUID,
	output IngestOutput, mockStatus string, deadline time.Time,
) (bool, error) {
	status := ""
	reason := ""
	for status == "" {
		if !workflow.Now(ctx).Before(deadline) {
			status = "rejected"
			reason = "moderation_unavailable"
			break
		}
		var reviewed ModerationOutput
		err := workflow.ExecuteActivity(moderationCtx, "moderation.check", map[string]any{
			"operation_id": operationID.String(), "output_id": output.OutputID,
			"asset_id": output.MediaAssetID, "kind": output.Kind,
			"adapter_key": "mock", "mock_status": mockStatus,
		}).Get(moderationCtx, &reviewed)
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if temporal.IsCanceledError(err) {
			return false, err
		}
		if err == nil && (reviewed.Status == "passed" || reviewed.Status == "rejected") {
			status = reviewed.Status
			break
		}
		if err := workflow.Sleep(ctx, 10*time.Minute); err != nil {
			return false, err
		}
	}
	for {
		if err := workflow.ExecuteActivity(mediaCtx, "media.RecordModeration", map[string]any{
			"operation_id": operationID.String(), "output_id": output.OutputID,
			"status": status, "reason": reason,
		}).Get(mediaCtx, nil); err == nil {
			return reason == "moderation_unavailable", nil
		}
		// A database outage must not let the Workflow close while this output is pending.
		if err := workflow.Sleep(ctx, 10*time.Minute); err != nil {
			return false, err
		}
	}
}

func makeMockSubmitInput(loaded application.WorkflowOperation) (ProviderSubmitInput, error) {
	var params map[string]any
	if err := json.Unmarshal(loaded.Operation.Params, &params); err != nil {
		return ProviderSubmitInput{}, fmt.Errorf("decode frozen mock params: %w", err)
	}
	inputs := make([]ProviderInput, 0, len(loaded.Inputs))
	for _, item := range loaded.Inputs {
		if item.MediaAssetID != nil || item.MaskAssetID != nil || item.RefID != nil {
			return ProviderSubmitInput{}, ErrUnsupportedOperation // signed input URLs pending.
		}
		inputs = append(inputs, ProviderInput{Role: item.Role, Text: item.TextValue})
	}
	return ProviderSubmitInput{
		OperationID: loaded.Operation.ID.String(), ProviderRequestKey: loaded.ProviderRequestKey,
		AdapterKey: loaded.Provider.AdapterKey, ProviderModelID: loaded.Provider.ProviderModelID,
		Capability: loaded.Operation.Capability, Mode: loaded.Operation.Mode,
		Params: params, Inputs: inputs, OutputCount: int(loaded.Operation.OutputCount),
	}, nil
}

func mockModerationStatus(raw json.RawMessage) (string, error) {
	var params struct {
		Status string `json:"mock_moderation_status"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", ErrInvalidOperationInput
	}
	if params.Status != "passed" && params.Status != "rejected" {
		return "", ErrInvalidOperationInput
	}
	return params.Status, nil
}

func providerFailureCode(providerErr *ProviderError) string {
	if providerErr != nil && providerErr.Code != "" {
		return providerErr.Code
	}
	return "provider_failed"
}
