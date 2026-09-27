package application

import (
	"encoding/json"
	"errors"
	"strings"

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

// ManualNotExecutedInput is written by an authenticated administrator command,
// before the workflow receives its resolve_manual signal.
type ManualNotExecutedInput struct {
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
	OperationID    uuid.UUID `json:"operation_id"`
	Action         string    `json:"action"`
	Attempt        int32     `json:"attempt"`
	ProviderTaskID *string   `json:"provider_task_id,omitempty"`
}

// CompleteProviderCallInput records a redacted result for a begun attempt.
// Usage is an opaque provider fact; only an installed pricing adapter may turn
// it into a charge. The mock adapter is explicitly free.
type CompleteProviderCallInput struct {
	OperationID    uuid.UUID       `json:"operation_id"`
	Action         string          `json:"action"`
	Attempt        int32           `json:"attempt"`
	Outcome        string          `json:"outcome"`
	State          string          `json:"state"`
	ProviderTaskID *string         `json:"provider_task_id,omitempty"`
	Usage          json.RawMessage `json:"usage,omitempty"`
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
		!validProviderTaskID(i.ProviderTaskID) {
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
	switch i.Outcome {
	case "ok", "error", "timeout", "unknown":
	default:
		return ErrInvalidProviderCall
	}
	switch i.State {
	case "accepted", "rejected", "not_submitted", "pending", "running",
		"succeeded", "failed", "not_found", "confirmed_not_exist",
		"cancelled", "not_cancelled", "unknown":
	default:
		return ErrInvalidProviderCall
	}
	if (i.Outcome == "unknown" || i.Outcome == "timeout") != (i.State == "unknown") {
		return ErrInvalidProviderCall
	}
	switch i.Action {
	case "submit":
		if i.State != "accepted" && i.State != "rejected" &&
			i.State != "not_submitted" && i.State != "unknown" {
			return ErrInvalidProviderCall
		}
		if i.State == "accepted" && (i.Outcome != "ok" || i.ProviderTaskID == nil) {
			return ErrInvalidProviderCall
		}
	case "query":
		if i.State != "pending" && i.State != "running" && i.State != "succeeded" &&
			i.State != "failed" && i.State != "not_found" &&
			i.State != "confirmed_not_exist" && i.State != "unknown" {
			return ErrInvalidProviderCall
		}
	case "cancel":
		if i.State != "cancelled" && i.State != "not_cancelled" && i.State != "unknown" {
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
