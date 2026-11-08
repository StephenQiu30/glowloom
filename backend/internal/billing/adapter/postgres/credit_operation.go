package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

type operationCreditFreeze struct {
	HoldID  *uuid.UUID
	ScopeID uuid.UUID
	Status  string
}

type creditSettlementOutcomeEvent struct {
	EventType string `json:"event_type"`
	application.CreditSettlementOutcome
}

func findOperationCreditFreeze(tx *gorm.DB, operationID uuid.UUID) (operationCreditFreeze, bool, error) {
	var freezes []operationCreditFreeze
	result := tx.Raw(`SELECT NULLIF(result->>'hold_id','')::uuid AS hold_id, scope_id, status FROM billing.credit_command
		WHERE caller_id=? AND action='freeze' AND source_id=?::uuid`,
		application.CreditOperationCallerID, operationID).Scan(&freezes)
	if result.Error != nil {
		return operationCreditFreeze{}, false, fmt.Errorf("read operation credit freeze: %w", result.Error)
	}
	if len(freezes) > 1 {
		return operationCreditFreeze{}, false, application.ErrCreditOutcomeConflict
	}
	if len(freezes) == 0 {
		return operationCreditFreeze{}, false, nil
	}
	return freezes[0], true, nil
}

// QueueOperationCreditSettlementOutcomeInTransaction writes the operation's
// immutable SAGA fact to Outbox in the caller-owned transaction.
func (s *Store) QueueOperationCreditSettlementOutcomeInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	operationID uuid.UUID,
	outcome string,
	amountMicros int64,
	occurredAt time.Time,
) error {
	if s == nil || s.db == nil || tx == nil || operationID == uuid.Nil || occurredAt.IsZero() {
		return ErrUnavailable
	}
	if outcome != "unknown" && outcome != "succeeded" && outcome != "failed" || amountMicros < 0 || outcome != "succeeded" && amountMicros != 0 {
		return application.ErrCreditOutcomeConflict
	}
	freeze, found, err := findOperationCreditFreeze(tx.WithContext(ctx), operationID)
	if err != nil || !found {
		return err
	}
	if freeze.Status == "insufficient" || freeze.Status == "failed" {
		return nil
	}
	if freeze.Status != "succeeded" {
		return application.ErrCreditFreezePending
	}
	if freeze.HoldID == nil || *freeze.HoldID == uuid.Nil {
		return application.ErrCreditOutcomeConflict
	}
	return queueCreditSettlementOutcomeTx(tx.WithContext(ctx), freeze.ScopeID, *freeze.HoldID, outcome, amountMicros, occurredAt.UTC())
}

func queueCreditSettlementOutcomeTx(
	tx *gorm.DB,
	scopeID, holdID uuid.UUID,
	outcome string,
	amountMicros int64,
	occurredAt time.Time,
) error {
	eventID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("glowloom.credit.operation/%s/%s/%d", holdID, outcome, amountMicros)))
	event := creditSettlementOutcomeEvent{
		EventType: application.CreditSettlementOutcomeTopic,
		CreditSettlementOutcome: application.CreditSettlementOutcome{
			EventID: eventID, ScopeID: scopeID, HoldID: holdID,
			Outcome: outcome, AmountMicros: amountMicros, OccurredAt: occurredAt.UTC(),
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode operation credit settlement outcome: %w", err)
	}
	result := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)
		VALUES(?::uuid,?,?,?::jsonb) ON CONFLICT (id) DO NOTHING`,
		eventID, application.CreditSettlementOutcomeTopic, scopeID, string(payload))
	if result.Error != nil {
		return fmt.Errorf("queue operation credit settlement outcome: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existingRaw []byte
	read := tx.Raw(`SELECT payload FROM infra.outbox WHERE id=?::uuid`, eventID).Scan(&existingRaw)
	if read.Error != nil {
		return fmt.Errorf("verify operation credit settlement outcome replay: %w", read.Error)
	}
	var existing creditSettlementOutcomeEvent
	if read.RowsAffected != 1 || json.Unmarshal(existingRaw, &existing) != nil ||
		existing.EventType != event.EventType || existing.EventID != event.EventID ||
		existing.ScopeID != scopeID || existing.HoldID != holdID ||
		existing.Outcome != outcome || existing.AmountMicros != amountMicros {
		return application.ErrCreditOutcomeConflict
	}
	return nil
}
