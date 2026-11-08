// Package event handles Kafka events for credit commands and wallet projections.
package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

const (
	// CreditCommandTopic carries accepted credit commands to the ledger consumer.
	CreditCommandTopic = "glowloom.billing.command.v1"
	// CreditBalanceChangedTopic invalidates stale cached wallet balances.
	CreditBalanceChangedTopic = "glowloom.billing.wallet_balance_changed.v1"
	// CreditDeadLetterTopic stores credit events that exhausted consumer retries.
	CreditDeadLetterTopic = "glowloom.billing.dlq.v1"
)

// CreditCommandProcessor applies an accepted command to the wallet ledger.
type CreditCommandProcessor interface {
	ProcessCreditCommand(context.Context, uuid.UUID) error
}

// WalletBalanceInvalidator advances the cache version watermark for a scope.
type WalletBalanceInvalidator interface {
	Invalidate(context.Context, uuid.UUID, int64) error
}

// CreditEventHandler routes credit command and balance-change events.
type CreditEventHandler struct {
	commands CreditCommandProcessor
	balances WalletBalanceInvalidator
}

// NewCreditEventHandler builds a handler for command and cache events.
func NewCreditEventHandler(commands CreditCommandProcessor, balances WalletBalanceInvalidator) *CreditEventHandler {
	return &CreditEventHandler{commands: commands, balances: balances}
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
	default:
		return fmt.Errorf("unsupported credit event topic %s", record.Topic)
	}
}

var _ application.Handler = (*CreditEventHandler)(nil)
