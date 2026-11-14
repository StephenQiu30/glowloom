package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// CreditSettlementOutcomeTopic carries the trusted terminal fact for a frozen hold.
const CreditSettlementOutcomeTopic = "glowloom.billing.settlement_outcome.v1"

var (
	// ErrInvalidCreditCommand means the command is incomplete or malformed.
	ErrInvalidCreditCommand = errors.New("invalid credit command")
	// ErrCreditIdempotencyConflict means a caller key was reused for a different request.
	ErrCreditIdempotencyConflict = errors.New("credit idempotency key reused with different command")
	// ErrCreditCommandNotFound means the scoped caller cannot read this command.
	ErrCreditCommandNotFound = errors.New("credit command not found")
	// ErrCreditVersionConflict means a compare-and-swap lost a concurrent update.
	ErrCreditVersionConflict = errors.New("credit account version conflict")
	// ErrCreditCommandConflict means the source or hold already has an incompatible command.
	ErrCreditCommandConflict = errors.New("credit command conflicts with an existing source")
	// ErrCreditOutcomeConflict means a hold received conflicting terminal outcomes.
	ErrCreditOutcomeConflict = errors.New("credit settlement outcome conflicts with the existing saga")
	// ErrCreditFreezePending prevents operation dispatch before its queued freeze succeeds.
	ErrCreditFreezePending = errors.New("operation credit freeze is not confirmed")
)

// CreditAction names one supported credit ledger command.
type CreditAction string

const (
	// CreditOperationCallerID identifies trusted operation confirmations.
	CreditOperationCallerID = "system:billing-operation"
	// CreditSagaCallerID identifies trusted automatic hold settlement commands.
	CreditSagaCallerID = "system:billing-saga"

	// CreditGrant adds a source-backed credit lot to an account.
	CreditGrant CreditAction = "grant"
	// CreditDebit consumes available credits immediately.
	CreditDebit CreditAction = "debit"
	// CreditFreeze moves available credits into a hold.
	CreditFreeze CreditAction = "freeze"
	// CreditCommit settles part or all of a hold.
	CreditCommit CreditAction = "commit"
	// CreditRelease returns an uncommitted hold to available balance.
	CreditRelease CreditAction = "release"
	// CreditRead authorizes a service caller to read one mapped wallet scope.
	CreditRead CreditAction = "read"
)

// CreditCommand is an authorized server-side write request. ScopeID and
// CallerID must be resolved from authenticated identity, never request JSON.
type CreditCommand struct {
	CallerID       string
	IdempotencyKey string
	ScopeID        uuid.UUID
	Action         CreditAction
	SourceID       uuid.UUID
	ReferenceID    *uuid.UUID
	AmountMicros   int64
	ExpiresAt      *time.Time
	ModelKey       *string
	Reason         string
}

// CreditCommandReceipt is stable across retries of the same caller key.
type CreditCommandReceipt struct {
	ID           uuid.UUID       `json:"command_id"`
	ScopeID      uuid.UUID       `json:"scope_id"`
	Action       CreditAction    `json:"action"`
	Status       string          `json:"status"`
	SourceID     uuid.UUID       `json:"source_id"`
	ReferenceID  *uuid.UUID      `json:"reference_id,omitempty"`
	AmountMicros int64           `json:"amount_micros"`
	Result       json.RawMessage `json:"result,omitempty" swaggertype:"object"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CreditSettlementOutcome is the trusted internal fact that closes a frozen hold.
// Unknown outcomes deliberately leave the hold open for later reconciliation.
type CreditSettlementOutcome struct {
	EventID      uuid.UUID `json:"event_id"`
	ScopeID      uuid.UUID `json:"scope_id"`
	HoldID       uuid.UUID `json:"hold_id"`
	Outcome      string    `json:"outcome"`
	AmountMicros int64     `json:"amount_micros"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// CreditCommandResult is emitted after the ledger command reaches a terminal status.
type CreditCommandResult struct {
	EventID    uuid.UUID       `json:"event_id"`
	CommandID  uuid.UUID       `json:"command_id"`
	ScopeID    uuid.UUID       `json:"scope_id"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// CreditSagaStore atomically turns trusted settlement outcomes into idempotent
// commit/release commands and tracks their ledger result.
type CreditSagaStore interface {
	ApplyCreditSettlementOutcome(context.Context, CreditSettlementOutcome) error
	ApplyCreditCommandResult(context.Context, CreditCommandResult) error
}

// CreditCommandStore persists accepted commands and applies their ledger effects.
type CreditCommandStore interface {
	SubmitCreditCommand(context.Context, CreditCommand) (CreditCommandReceipt, error)
	ReadCreditCommand(context.Context, string, uuid.UUID, uuid.UUID) (CreditCommandReceipt, error)
	ProcessCreditCommand(context.Context, uuid.UUID) error
}

// CreditCommandLimiter is deliberately fail-closed: a write command cannot
// bypass abuse controls when the shared limiter is unavailable.
type CreditCommandLimiter interface {
	AllowCreditCommand(context.Context, string, uuid.UUID, string, int64, time.Time) (time.Duration, error)
}

// CreditCommands validates and submits idempotent credit ledger commands.
type CreditCommands struct {
	store   CreditCommandStore
	limiter CreditCommandLimiter
	clock   func() time.Time
}

// NewCreditCommands constructs the command service with its persistence and limiter.
func NewCreditCommands(store CreditCommandStore, limiter CreditCommandLimiter, clock func() time.Time) *CreditCommands {
	if clock == nil {
		clock = time.Now
	}
	return &CreditCommands{store: store, limiter: limiter, clock: clock}
}

// Submit validates, rate-limits, and durably accepts a credit command.
func (s *CreditCommands) Submit(ctx context.Context, command CreditCommand, requestIP string) (CreditCommandReceipt, error) {
	if s == nil || s.store == nil || s.limiter == nil {
		return CreditCommandReceipt{}, ErrInvalidCreditCommand
	}
	if err := command.Validate(s.clock().UTC()); err != nil {
		return CreditCommandReceipt{}, err
	}
	wait, err := s.limiter.AllowCreditCommand(ctx, command.CallerID, command.ScopeID, requestIP, command.AmountMicros, s.clock().UTC())
	if err != nil {
		return CreditCommandReceipt{}, err
	}
	if wait > 0 {
		return CreditCommandReceipt{}, &CreditRateLimitedError{RetryAfter: wait}
	}
	return s.store.SubmitCreditCommand(ctx, command)
}

// Read returns a previously accepted command within its caller and scope.
func (s *CreditCommands) Read(ctx context.Context, callerID string, scopeID, commandID uuid.UUID) (CreditCommandReceipt, error) {
	if s == nil || s.store == nil || strings.TrimSpace(callerID) == "" || scopeID == uuid.Nil || commandID == uuid.Nil {
		return CreditCommandReceipt{}, ErrInvalidCreditCommand
	}
	return s.store.ReadCreditCommand(ctx, callerID, scopeID, commandID)
}

// Validate checks the command identity, source, amount, and action-specific fields.
func (c CreditCommand) Validate(now time.Time) error {
	if strings.TrimSpace(c.CallerID) == "" || len(c.CallerID) > 200 || strings.TrimSpace(c.IdempotencyKey) == "" || len(c.IdempotencyKey) > 200 ||
		c.ScopeID == uuid.Nil || c.SourceID == uuid.Nil || c.AmountMicros <= 0 || utf8.RuneCountInString(c.Reason) < 1 || utf8.RuneCountInString(c.Reason) > 500 {
		return ErrInvalidCreditCommand
	}
	if c.ModelKey != nil && (strings.TrimSpace(*c.ModelKey) == "" || utf8.RuneCountInString(*c.ModelKey) > 256) {
		return ErrInvalidCreditCommand
	}
	switch c.Action {
	case CreditGrant:
		if c.ExpiresAt == nil || !c.ExpiresAt.After(now) || c.ReferenceID != nil {
			return ErrInvalidCreditCommand
		}
	case CreditDebit, CreditFreeze:
		if c.ExpiresAt != nil || c.ReferenceID != nil {
			return ErrInvalidCreditCommand
		}
	case CreditCommit:
		if c.ExpiresAt != nil || c.ReferenceID == nil || *c.ReferenceID == uuid.Nil {
			return ErrInvalidCreditCommand
		}
	case CreditRelease:
		if c.ExpiresAt != nil || c.ReferenceID == nil || *c.ReferenceID == uuid.Nil {
			return ErrInvalidCreditCommand
		}
	default:
		return ErrInvalidCreditCommand
	}
	return nil
}

// CreditRateLimitedError reports the minimum delay before a caller can retry.
type CreditRateLimitedError struct{ RetryAfter time.Duration }

// Error returns a safe message without exposing limiter keys or request details.
func (e *CreditRateLimitedError) Error() string { return "credit command rate limit exceeded" }
