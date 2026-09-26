// Package domain defines invariants for generation operations.
package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidStatus means an operation status is not part of the domain model.
	ErrInvalidStatus = errors.New("invalid operation status")
	// ErrIllegalTransition means the requested status change violates the lifecycle.
	ErrIllegalTransition = errors.New("illegal operation transition")
)

// Status is the persisted lifecycle state of a generation operation.
type Status string

// Operation status values match the persisted operation lifecycle.
const (
	StatusDraft       Status = "draft"
	StatusQuoted      Status = "quoted"
	StatusConfirmed   Status = "confirmed"
	StatusSubmitting  Status = "submitting"
	StatusSubmitted   Status = "submitted"
	StatusSucceeded   Status = "succeeded"
	StatusIngesting   Status = "ingesting"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusUnknown     Status = "unknown"
	StatusReconciling Status = "reconciling"
	StatusManual      Status = "manual"
	StatusCancelling  Status = "cancelling"
	StatusCancelled   Status = "cancelled"
	StatusExpired     Status = "expired"
)

func (s Status) valid() bool {
	switch s {
	case StatusDraft, StatusQuoted, StatusConfirmed, StatusSubmitting,
		StatusSubmitted, StatusSucceeded, StatusIngesting, StatusCompleted,
		StatusFailed, StatusUnknown, StatusReconciling, StatusManual,
		StatusCancelling, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether no further lifecycle transition is allowed.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

// CanTransitionTo validates a lifecycle change. Repeating the same valid state is
// accepted so that an Activity replay can recognize a previously applied write.
// The caller must separately prove reuse, provider submission, cancellation, and
// quote conditions; settlement and event writes belong in the same database transaction.
func (s Status) CanTransitionTo(next Status) error {
	if !s.valid() || !next.valid() {
		return fmt.Errorf("%w: %q -> %q", ErrInvalidStatus, s, next)
	}
	if s == next {
		return nil
	}

	allowed := false
	switch s {
	case StatusDraft:
		allowed = next == StatusQuoted
	case StatusQuoted:
		allowed = next == StatusConfirmed || next == StatusExpired
	case StatusConfirmed:
		allowed = next == StatusSubmitting || next == StatusCompleted ||
			next == StatusFailed || next == StatusCancelling
	case StatusSubmitting:
		allowed = next == StatusSubmitted || next == StatusUnknown ||
			next == StatusFailed || next == StatusCancelling
	case StatusSubmitted:
		allowed = next == StatusSucceeded || next == StatusFailed ||
			next == StatusReconciling || next == StatusCancelling
	case StatusSucceeded:
		allowed = next == StatusIngesting
	case StatusIngesting:
		allowed = next == StatusCompleted || next == StatusFailed
	case StatusUnknown:
		allowed = next == StatusReconciling
	case StatusReconciling:
		allowed = next == StatusSubmitted || next == StatusFailed || next == StatusManual
	case StatusManual:
		allowed = next == StatusIngesting || next == StatusFailed
	case StatusCancelling:
		allowed = next == StatusCancelled || next == StatusSubmitted
	}
	if !allowed {
		return fmt.Errorf("%w: %q -> %q", ErrIllegalTransition, s, next)
	}
	return nil
}
