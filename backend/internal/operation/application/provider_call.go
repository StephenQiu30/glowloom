package application

import (
	"encoding/json"
	"errors"
	"strings"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProviderCall means a workflow supplied incomplete call evidence.
	ErrInvalidProviderCall = errors.New("invalid provider call evidence")
	// ErrProviderCallConflict means a replay disagrees with previously stored facts.
	ErrProviderCallConflict = errors.New("provider call evidence conflict")
	// ErrProviderCostUnknown prevents terminal settlement while the charge is uncertain.
	ErrProviderCostUnknown = errors.New("provider charge is not established")
	// ErrManualResolutionUnverified means the administrator has not recorded
	// evidence that an uncertain provider request was never executed.
	ErrManualResolutionUnverified = errors.New("manual provider resolution is unverified")
)

// ProviderCallOutcome records transport certainty for one persisted request.
type ProviderCallOutcome string

// ProviderCallState is the bounded state evidence allowed for one call action.
type ProviderCallState string

// Provider call outcomes classify a completed, rejected or uncertain request.
const (
	ProviderCallOutcomeOK      ProviderCallOutcome = "ok"
	ProviderCallOutcomeError   ProviderCallOutcome = "error"
	ProviderCallOutcomeTimeout ProviderCallOutcome = "timeout"
	ProviderCallOutcomeUnknown ProviderCallOutcome = "unknown"
)

// Provider call states record durable dispatch and response progress.
const (
	ProviderCallStateUnset             ProviderCallState = ""
	ProviderCallStateAccepted          ProviderCallState = "accepted"
	ProviderCallStateCompleted         ProviderCallState = "completed"
	ProviderCallStateRejected          ProviderCallState = "rejected"
	ProviderCallStateNotSubmitted      ProviderCallState = "not_submitted"
	ProviderCallStatePending           ProviderCallState = "pending"
	ProviderCallStateRunning           ProviderCallState = "running"
	ProviderCallStateSucceeded         ProviderCallState = "succeeded"
	ProviderCallStateFailed            ProviderCallState = "failed"
	ProviderCallStateNotFound          ProviderCallState = "not_found"
	ProviderCallStateConfirmedNotExist ProviderCallState = "confirmed_not_exist"
	ProviderCallStateCancelled         ProviderCallState = "cancelled"
	ProviderCallStateNotCancelled      ProviderCallState = "not_cancelled"
	ProviderCallStateUnknown           ProviderCallState = "unknown"
)

// ManualNotExecutedInput is written by an authenticated administrator command,
// before the workflow receives its resolve_manual signal.
type ManualNotExecutedInput struct {
	Actor       identity.Principal
	OperationID uuid.UUID
	AdminID     uuid.UUID
	Evidence    string
}

// Validate requires a bounded, nonempty explanation and exact identities.
func (i ManualNotExecutedInput) Validate() error {
	if i.OperationID == uuid.Nil || i.AdminID == uuid.Nil ||
		strings.TrimSpace(i.Evidence) == "" ||
		strings.TrimSpace(i.Evidence) != i.Evidence || len(i.Evidence) > 2000 {
		return ErrInvalidProviderCall
	}
	return nil
}

// BeginProviderCallInput identifies one attempt before any provider request is sent.
// Attempt numbers are scoped to operation and action and must be stable on replay.
type BeginProviderCallInput struct {
	OperationID      uuid.UUID `json:"operation_id"`
	Action           string    `json:"action"`
	Attempt          int32     `json:"attempt"`
	ProviderTaskID   *string   `json:"provider_task_id,omitempty"`
	DispatchRequired bool      `json:"dispatch_required,omitempty"`
}

// CompleteProviderCallInput records a redacted result for a begun attempt.
// Usage contains normalized, non-sensitive billable quantities from a trusted
// provider adapter. The mock adapter is explicitly free.
type CompleteProviderCallInput struct {
	ErrorCode      string              `json:"error_code,omitempty"`
	Managed        bool                `json:"managed,omitempty"`
	OperationID    uuid.UUID           `json:"operation_id"`
	Action         string              `json:"action"`
	Attempt        int32               `json:"attempt"`
	Outcome        ProviderCallOutcome `json:"outcome"`
	State          ProviderCallState   `json:"state"`
	ProviderTaskID *string             `json:"provider_task_id,omitempty"`
	Usage          json.RawMessage     `json:"usage,omitempty"`
	Receipt        *ProviderReceipt    `json:"receipt,omitempty"`
}

// ProviderCost is the amount established by durable, redacted provider calls.
type ProviderCost struct {
	ActualCostMicros  int64 `json:"actual_cost_micros"`
	HasSubmission     bool  `json:"has_submission"`
	ManualNotExecuted bool  `json:"manual_not_executed"`
}

// Validate checks the stable call identity before writing an unknown record.
func (i BeginProviderCallInput) Validate() error {
	if i.OperationID == uuid.Nil || i.Attempt < 1 || !providerCallAction(i.Action) ||
		!validProviderTaskID(i.ProviderTaskID) || (i.Action == "cancel" && i.ProviderTaskID == nil) ||
		(i.DispatchRequired && (i.Action != "submit" || i.ProviderTaskID != nil)) {
		return ErrInvalidProviderCall
	}
	return nil
}

// Validate limits call outcomes to redacted, internally consistent facts.
func (i CompleteProviderCallInput) Validate() error {
	if i.OperationID == uuid.Nil || i.Attempt < 1 || !providerCallAction(i.Action) ||
		!validProviderTaskID(i.ProviderTaskID) || len(i.State) > 64 ||
		(i.Usage != nil && (len(i.Usage) > 16*1024 || !json.Valid(i.Usage) || i.Usage[0] != '{')) {
		return ErrInvalidProviderCall
	}
	if i.Managed && (i.Receipt != nil || len(i.Usage) > 0 || (i.State != ProviderCallStateCompleted && i.State != ProviderCallStateSucceeded)) {
		return ErrInvalidProviderCall
	}
	switch i.Outcome {
	case ProviderCallOutcomeOK, ProviderCallOutcomeError, ProviderCallOutcomeTimeout, ProviderCallOutcomeUnknown:
	default:
		return ErrInvalidProviderCall
	}
	switch i.State {
	case ProviderCallStateAccepted, ProviderCallStateCompleted, ProviderCallStateRejected, ProviderCallStateNotSubmitted, ProviderCallStatePending, ProviderCallStateRunning,
		ProviderCallStateSucceeded, ProviderCallStateFailed, ProviderCallStateNotFound, ProviderCallStateConfirmedNotExist,
		ProviderCallStateCancelled, ProviderCallStateNotCancelled, ProviderCallStateUnknown:
	default:
		return ErrInvalidProviderCall
	}
	if (i.Outcome == ProviderCallOutcomeUnknown || i.Outcome == ProviderCallOutcomeTimeout) != (i.State == ProviderCallStateUnknown) {
		return ErrInvalidProviderCall
	}
	if i.State == ProviderCallStateCompleted {
		if i.Action != "submit" || i.Outcome != ProviderCallOutcomeOK || i.ProviderTaskID != nil ||
			(!i.Managed && (i.Receipt == nil || i.Receipt.Validate() != nil ||
				i.Receipt.Identity.OperationID != i.OperationID ||
				i.Receipt.Identity.Action != i.Action || i.Receipt.Identity.Attempt != i.Attempt)) || (i.Managed && i.Receipt != nil) {
			return ErrInvalidProviderCall
		}
		if i.Usage != nil {
			if _, err := ParseBillableUsage(i.Usage); err != nil {
				return ErrInvalidProviderCall
			}
		}
	} else if i.Receipt != nil {
		return ErrInvalidProviderCall
	}
	switch i.Action {
	case "submit":
		if i.State != ProviderCallStateAccepted && i.State != ProviderCallStateCompleted && i.State != ProviderCallStateRejected &&
			i.State != ProviderCallStateNotSubmitted && i.State != ProviderCallStateUnknown {
			return ErrInvalidProviderCall
		}
		if i.State == ProviderCallStateAccepted && (i.Outcome != ProviderCallOutcomeOK || i.ProviderTaskID == nil) {
			return ErrInvalidProviderCall
		}
	case "query":
		if i.State != ProviderCallStatePending && i.State != ProviderCallStateRunning && i.State != ProviderCallStateSucceeded &&
			i.State != ProviderCallStateFailed && i.State != ProviderCallStateNotFound &&
			i.State != ProviderCallStateConfirmedNotExist && i.State != ProviderCallStateUnknown {
			return ErrInvalidProviderCall
		}
	case "cancel":
		if i.State != ProviderCallStateCancelled && i.State != ProviderCallStateNotCancelled && i.State != ProviderCallStateUnknown {
			return ErrInvalidProviderCall
		}
	}
	return nil
}

func providerCallAction(action string) bool {
	return action == "submit" || action == "query" || action == "cancel"
}

func validProviderTaskID(taskID *string) bool {
	return taskID == nil || (*taskID != "" && len(*taskID) <= 256 &&
		strings.TrimSpace(*taskID) == *taskID)
}
