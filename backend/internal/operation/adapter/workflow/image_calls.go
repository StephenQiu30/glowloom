package workflow

import (
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func (c *providerCalls) submitImage(providerCtx workflow.Context, input ProviderSubmitInput) (ProviderSubmitOutput, error) {
	if err := workflow.ExecuteActivity(c.flowCtx, "flow.BeginProviderCall", application.BeginProviderCallInput{OperationID: c.operationID, Action: "submit", Attempt: input.Attempt, DispatchRequired: true}).Get(c.flowCtx, nil); err != nil {
		return ProviderSubmitOutput{}, err
	}
	var result ProviderSubmitOutput
	if err := workflow.ExecuteActivity(providerCtx, "provider.submit", input).Get(providerCtx, &result); err != nil {
		result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
	}
	outcome := "ok"
	switch result.Outcome {
	case ProviderSubmitCompleted:
		if !matchesImageReceipt(result, input) {
			result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
			outcome = "unknown"
		}
	case ProviderSubmitRejected, ProviderSubmitNotSubmitted:
		if result.ProviderTaskID != nil || result.Receipt != nil || result.Usage != nil {
			result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
			outcome = "unknown"
		} else {
			outcome = "error"
		}
	default:
		result = ProviderSubmitOutput{Outcome: ProviderSubmitUnknown}
		outcome = "unknown"
	}
	if err := c.complete(application.CompleteProviderCallInput{OperationID: c.operationID, Action: "submit", Attempt: input.Attempt, Outcome: outcome, State: string(result.Outcome), Receipt: result.Receipt, Usage: result.Usage}); err != nil {
		return ProviderSubmitOutput{}, err
	}
	return result, nil
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
