// Package workflow defines the JSON boundary between Go operation workflows and agent activities.
package workflow

// ProviderSubmitInput is sent to provider.submit on the provider's agent queue.
type ProviderSubmitInput struct {
	OperationID        string              `json:"operation_id"`
	ProviderRequestKey string              `json:"provider_request_key"`
	AdapterKey         string              `json:"adapter_key"`
	ProviderModelID    string              `json:"provider_model_id"`
	Capability         string              `json:"capability"`
	Mode               string              `json:"mode"`
	Params             map[string]any      `json:"params"`
	Inputs             []ProviderInput     `json:"inputs"`
	OutputCount        int                 `json:"output_count"`
	Credential         *ProviderCredential `json:"credential,omitempty"`
}

// ProviderInput is one role-bound input sent to a provider.
type ProviderInput struct {
	Role       string  `json:"role"`
	MediaURL   *string `json:"media_url,omitempty"`
	MediaType  *string `json:"media_type,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Text       *string `json:"text,omitempty"`
}

// ProviderCredential carries an encrypted credential for the selected provider.
// The workflow must not log or persist its ciphertext outside the activity payload.
type ProviderCredential struct {
	ID         string `json:"id"`
	KeyID      string `json:"key_id"`
	Ciphertext string `json:"ciphertext"`
}

// ProviderError is the provider-facing failure summary returned by an activity.
type ProviderError struct {
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

// ProviderSubmitOutcome separates a confirmed submission from an uncertain result.
type ProviderSubmitOutcome string

// Provider submission outcomes distinguish a definite result from unknown delivery.
const (
	ProviderSubmitAccepted     ProviderSubmitOutcome = "accepted"
	ProviderSubmitRejected     ProviderSubmitOutcome = "rejected"
	ProviderSubmitNotSubmitted ProviderSubmitOutcome = "not_submitted"
	ProviderSubmitUnknown      ProviderSubmitOutcome = "unknown"
)

// ProviderSubmitOutput is returned by provider.submit.
type ProviderSubmitOutput struct {
	Outcome        ProviderSubmitOutcome `json:"outcome"`
	ProviderTaskID *string               `json:"provider_task_id"`
	Error          *ProviderError        `json:"error"`
}

// ProviderQueryInput identifies a provider task by exactly one of its two keys.
type ProviderQueryInput struct {
	ProviderTaskID     *string `json:"provider_task_id,omitempty"`
	ProviderRequestKey *string `json:"provider_request_key,omitempty"`
}

// ProviderQueryState is the provider's current task state.
type ProviderQueryState string

// Provider query states reflect the provider's latest known task state.
const (
	ProviderQueryPending   ProviderQueryState = "pending"
	ProviderQueryRunning   ProviderQueryState = "running"
	ProviderQuerySucceeded ProviderQueryState = "succeeded"
	ProviderQueryFailed    ProviderQueryState = "failed"
	ProviderQueryNotFound  ProviderQueryState = "not_found"
)

// ProviderQueryOutput is returned by provider.query.
type ProviderQueryOutput struct {
	State          ProviderQueryState `json:"state"`
	ProviderTaskID *string            `json:"provider_task_id"`
	ResultURLs     []string           `json:"result_urls"`
	Usage          map[string]any     `json:"usage"`
	Error          *ProviderError     `json:"error"`
}

// ProviderCancelInput identifies the task passed to provider.cancel.
type ProviderCancelInput struct {
	ProviderTaskID string `json:"provider_task_id"`
}

// ProviderCancelOutcome records whether the provider confirmed cancellation.
type ProviderCancelOutcome string

// Provider cancellation outcomes reflect whether cancellation took effect.
const (
	ProviderCancelConfirmed  ProviderCancelOutcome = "cancelled"
	ProviderCancelNotApplied ProviderCancelOutcome = "not_cancelled"
	ProviderCancelUnknown    ProviderCancelOutcome = "unknown"
)

// ProviderCancelOutput is returned by provider.cancel.
type ProviderCancelOutput struct {
	Outcome ProviderCancelOutcome `json:"outcome"`
	Usage   map[string]any        `json:"usage,omitempty"`
}
