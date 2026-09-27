package workflow

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// providerCalls gives every logical provider attempt a durable identity before
// dispatch. Its counters live in deterministic Workflow state.
type providerCalls struct {
	flowCtx      workflow.Context
	operationID  uuid.UUID
	queryAttempt int32
}

func (c *providerCalls) begin(action string, attempt int32, taskID *string) error {
	return workflow.ExecuteActivity(c.flowCtx, "flow.BeginProviderCall", application.BeginProviderCallInput{
		OperationID: c.operationID, Action: action, Attempt: attempt, ProviderTaskID: taskID,
	}).Get(c.flowCtx, nil)
}

func (c *providerCalls) complete(input application.CompleteProviderCallInput) error {
	return workflow.ExecuteActivity(c.flowCtx, "flow.CompleteProviderCall", input).Get(c.flowCtx, nil)
}

func (c *providerCalls) submit(providerCtx workflow.Context, input ProviderSubmitInput,
	attempt int32,
) (ProviderSubmitOutput, error) {
	if err := c.begin("submit", attempt, nil); err != nil {
		return ProviderSubmitOutput{}, err
	}
	var result ProviderSubmitOutput
	if err := workflow.ExecuteActivity(providerCtx, "provider.submit", input).Get(providerCtx, &result); err != nil {
		// Once dispatch is possible, even a timeout may have incurred a charge.
		result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
	}
	state := string(result.Outcome)
	outcome := "ok"
	if result.Outcome == ProviderSubmitAccepted && (result.ProviderTaskID == nil || *result.ProviderTaskID == "") {
		result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
		state = "unknown"
	}
	switch result.Outcome {
	case ProviderSubmitAccepted:
	case ProviderSubmitRejected, ProviderSubmitNotSubmitted:
		outcome = "error"
	case ProviderSubmitUnknown:
		outcome = "unknown"
	default:
		result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
		state, outcome = "unknown", "unknown"
	}
	if err := c.complete(application.CompleteProviderCallInput{
		OperationID: c.operationID, Action: "submit", Attempt: attempt,
		Outcome: outcome, State: state, ProviderTaskID: result.ProviderTaskID,
	}); err != nil {
		return ProviderSubmitOutput{}, err
	}
	return result, nil
}

// query records each dispatched read separately. A transient query failure is
// safe to retry; it never authorizes a second paid submit.
func (c *providerCalls) query(ctx workflow.Context, queue string,
	input ProviderQueryInput,
) (ProviderQueryOutput, error) {
	queryCtx := providerActivityContext(ctx, queue, 30*time.Second, 1)
	for retry := 0; retry < 10; retry++ {
		c.queryAttempt++
		attempt := c.queryAttempt
		if err := c.begin("query", attempt, input.ProviderTaskID); err != nil {
			return ProviderQueryOutput{}, err
		}
		var result ProviderQueryOutput
		err := workflow.ExecuteActivity(queryCtx, "provider.query", input).Get(queryCtx, &result)
		if ctx.Err() != nil {
			return ProviderQueryOutput{}, ctx.Err()
		}
		outcome, state := "ok", string(result.State)
		if err != nil {
			outcome, state = "timeout", "unknown"
		} else if !validQueryState(result.State) {
			outcome, state = "unknown", "unknown"
		}
		taskID := result.ProviderTaskID
		if taskID == nil {
			taskID = input.ProviderTaskID
		}
		var usage json.RawMessage
		if result.Usage != nil {
			usage, err = json.Marshal(result.Usage)
			if err != nil {
				return ProviderQueryOutput{}, ErrInvalidProviderResult
			}
		}
		if completeErr := c.complete(application.CompleteProviderCallInput{
			OperationID: c.operationID, Action: "query", Attempt: attempt,
			Outcome: outcome, State: state, ProviderTaskID: taskID, Usage: usage,
		}); completeErr != nil {
			return ProviderQueryOutput{}, completeErr
		}
		if err == nil && outcome == "ok" {
			return result, nil
		}
		if retry < 9 {
			delay := time.Duration(1<<min(retry, 5)) * time.Second
			if err := workflow.Sleep(ctx, delay); err != nil {
				return ProviderQueryOutput{}, err
			}
		}
	}
	return ProviderQueryOutput{}, errNeedsReconciliation
}

func validQueryState(state ProviderQueryState) bool {
	switch state {
	case ProviderQueryPending, ProviderQueryRunning, ProviderQuerySucceeded,
		ProviderQueryFailed, ProviderQueryNotFound:
		return true
	default:
		return false
	}
}
