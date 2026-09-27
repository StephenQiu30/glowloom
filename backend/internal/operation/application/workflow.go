package application

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrInvalidTransitionInput means the requested persisted state write is unsafe.
	ErrInvalidTransitionInput = errors.New("invalid workflow transition input")
	// ErrTransitionConflict means the stored state differs from the expected state.
	ErrTransitionConflict = errors.New("operation transition conflict")
	// ErrInvalidFinalizationInput means terminal facts cannot be settled safely.
	ErrInvalidFinalizationInput = errors.New("invalid workflow finalization input")
	// ErrWorkflowModelUnavailable means the frozen model or price version is gone.
	ErrWorkflowModelUnavailable = errors.New("workflow model version unavailable")
	// ErrWorkflowInputNotReady means frozen media is missing or unapproved.
	ErrWorkflowInputNotReady = errors.New("workflow input media not ready")
	// ErrWorkflowConsentUnavailable rejects a consent that cannot be verified.
	ErrWorkflowConsentUnavailable = errors.New("workflow media consent cannot be verified")
)

// WorkflowProvider is the nonsecret, frozen provider configuration required by
// OperationWorkflow. Credentials are loaded inside a short-lived Activity.
type WorkflowProvider struct {
	ProviderKey     string
	AdapterKey      string
	ModelKey        string
	ProviderModelID string
	Queue           string
	SupportsQuery   bool
	SupportsCancel  bool
	ExpectedMaxMS   int32
	Moderation      string
}

// WorkflowOutput is the durable result state of one sequence.
type WorkflowOutput struct {
	ID               uuid.UUID
	SeqNo            int32
	Kind             string
	MediaAssetID     *uuid.UUID
	JSONPayload      json.RawMessage
	ModerationStatus string
	ModerationReason *string
}

// WorkflowOperation contains only nonsecret facts used to resume an Operation.
// ProviderRequestKey is deterministic even before its first database write.
type WorkflowOperation struct {
	Operation          domain.Operation
	Inputs             []domain.OperationInput
	Outputs            []WorkflowOutput
	Provider           WorkflowProvider
	PriceUnit          string
	ProviderRequestKey string
	ProviderTaskID     *string
}

// TransitionInput identifies one conditional, nonterminal workflow state change.
// ProviderTaskID is retained only in private operation history for recovery.
type TransitionInput struct {
	OperationID        uuid.UUID
	From               []domain.Status
	To                 domain.Status
	Reason             string
	ProviderRequestKey *string
	ProviderTaskID     *string
}

// FinalizeInput closes one confirmed operation after billing has settled in
// the same database transaction. SettledMicros is the customer's charge.
type FinalizeInput struct {
	OperationID   uuid.UUID
	From          []domain.Status
	To            domain.Status
	SettledMicros int64
	FailureCode   string
	Retryable     *bool
	Reason        string
}

// FinalizePreparation locks the Operation before the billing transaction and
// identifies a replay so billing can be skipped entirely.
type FinalizePreparation struct {
	ProjectID        uuid.UUID
	ReservationID    uuid.UUID
	AlreadyFinalized bool
	SettledMicros    *int64
}
