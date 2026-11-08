// Package event handles Kafka events for credit commands and wallet projections.
package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

const (
	// CreditCommandTopic carries accepted credit commands to the ledger consumer.
	CreditCommandTopic = "glowloom.billing.command.v1"
	// CreditBalanceChangedTopic invalidates stale cached wallet balances.
	CreditBalanceChangedTopic = "glowloom.billing.wallet_balance_changed.v1"
	// CreditDeadLetterTopic stores credit events that exhausted consumer retries.
	CreditDeadLetterTopic = "glowloom.billing.dlq.v1"
	// CreditCommandResultTopic carries command completion events from the ledger.
	CreditCommandResultTopic = "glowloom.billing.command_result.v1"
	// CreditSettlementOutcomeTopic carries a trusted operation settlement fact.
	CreditSettlementOutcomeTopic = billingapp.CreditSettlementOutcomeTopic
)

// CreditCommandProcessor applies an accepted command to the wallet ledger.
type CreditCommandProcessor interface {
	ProcessCreditCommand(context.Context, uuid.UUID) error
}

// WalletBalanceInvalidator advances the cache version watermark for a scope.
type WalletBalanceInvalidator interface {
	Invalidate(context.Context, uuid.UUID, int64) error
}

// CreditSagaProcessor turns settlement facts into ledger commands and tracks
// their durable results.
type CreditSagaProcessor interface {
	ApplyCreditSettlementOutcome(context.Context, billingapp.CreditSettlementOutcome) error
	ApplyCreditCommandResult(context.Context, billingapp.CreditCommandResult) error
}

// CreditOperationFreezeResultProcessor releases pre-dispatch reservations when
// their asynchronous wallet freeze fails.
type CreditOperationFreezeResultProcessor interface {
	ApplyCreditFreezeCommandResult(context.Context, billingapp.CreditCommandResult) error
}

// CreditEventHandler routes commands, settlement outcomes, command results, and balance changes.
type CreditEventHandler struct {
	commands   CreditCommandProcessor
	balances   WalletBalanceInvalidator
	sagas      CreditSagaProcessor
	operations CreditOperationFreezeResultProcessor
}

// NewCreditEventHandler builds a handler for command and cache events.
func NewCreditEventHandler(commands CreditCommandProcessor, balances WalletBalanceInvalidator, sagas CreditSagaProcessor, operations CreditOperationFreezeResultProcessor) *CreditEventHandler {
	return &CreditEventHandler{commands: commands, balances: balances, sagas: sagas, operations: operations}
}

// Handle validates an event envelope and applies its idempotent effect.
func (h *CreditEventHandler) Handle(ctx context.Context, record application.Record) error {
	switch record.Topic {
	case CreditCommandTopic:
		if h.commands == nil {
			return fmt.Errorf("credit command processor unavailable")
		}
		var event struct {
			EventID   uuid.UUID `json:"event_id"`
			EventType string    `json:"event_type"`
			CommandID uuid.UUID `json:"command_id"`
			ScopeID   uuid.UUID `json:"scope_id"`
		}
		if err := json.Unmarshal(record.Value, &event); err != nil {
			return fmt.Errorf("decode credit command event: %w", err)
		}
		if event.EventID == uuid.Nil || event.EventType != record.Topic || event.CommandID == uuid.Nil || event.ScopeID == uuid.Nil || string(record.Key) != event.ScopeID.String() {
			return fmt.Errorf("invalid credit command event")
		}
		return h.commands.ProcessCreditCommand(ctx, event.CommandID)
	case CreditBalanceChangedTopic:
		if h.balances == nil {
			return fmt.Errorf("credit balance invalidator unavailable")
		}
		var event struct {
			EventID        uuid.UUID `json:"event_id"`
			EventType      string    `json:"event_type"`
			ScopeID        uuid.UUID `json:"scope_id"`
			AccountVersion int64     `json:"account_version"`
		}
		if err := json.Unmarshal(record.Value, &event); err != nil {
			return fmt.Errorf("decode wallet balance event: %w", err)
		}
		if event.EventID == uuid.Nil || event.EventType != record.Topic || event.ScopeID == uuid.Nil || event.AccountVersion < 1 || string(record.Key) != event.ScopeID.String() {
			return fmt.Errorf("invalid wallet balance event")
		}
		return h.balances.Invalidate(ctx, event.ScopeID, event.AccountVersion)
	case CreditSettlementOutcomeTopic:
		if h.sagas == nil {
			return fmt.Errorf("credit saga processor unavailable")
		}
		var event struct {
			EventType string `json:"event_type"`
			billingapp.CreditSettlementOutcome
		}
		if err := json.Unmarshal(record.Value, &event); err != nil {
			return fmt.Errorf("decode credit settlement outcome: %w", err)
		}
		outcome := event.CreditSettlementOutcome
		if outcome.EventID == uuid.Nil || event.EventType != record.Topic || outcome.ScopeID == uuid.Nil || outcome.HoldID == uuid.Nil || outcome.OccurredAt.IsZero() || string(record.Key) != outcome.ScopeID.String() {
			return fmt.Errorf("invalid credit settlement outcome")
		}
		return h.sagas.ApplyCreditSettlementOutcome(ctx, outcome)
	case CreditCommandResultTopic:
		if h.sagas == nil {
			return fmt.Errorf("credit saga processor unavailable")
		}
		var event struct {
			EventType string `json:"event_type"`
			billingapp.CreditCommandResult
		}
		if err := json.Unmarshal(record.Value, &event); err != nil {
			return fmt.Errorf("decode credit command result: %w", err)
		}
		result := event.CreditCommandResult
		if result.EventID == uuid.Nil || event.EventType != record.Topic || result.CommandID == uuid.Nil || result.ScopeID == uuid.Nil || result.Status == "" || result.OccurredAt.IsZero() || string(record.Key) != result.ScopeID.String() {
			return fmt.Errorf("invalid credit command result")
		}
		if err := h.sagas.ApplyCreditCommandResult(ctx, result); err != nil {
			return err
		}
		if h.operations != nil {
			return h.operations.ApplyCreditFreezeCommandResult(ctx, result)
		}
		return nil
	default:
		return fmt.Errorf("unsupported credit event topic %s", record.Topic)
	}
}

var _ application.Handler = (*CreditEventHandler)(nil)
