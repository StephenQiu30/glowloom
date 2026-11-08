package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

const creditSagaCallerID = "billing-credit-saga"

type creditSagaRow struct {
	HoldID       uuid.UUID
	ScopeID      uuid.UUID
	Outcome      string
	AmountMicros int64
	Status       string
	CommandID    *uuid.UUID
}

type creditSagaEventRow struct {
	EventType   string
	ScopeID     uuid.UUID
	HoldID      uuid.UUID
	CommandID   *uuid.UUID
	RequestHash string
}

// ApplyCreditSettlementOutcome persists a trusted outcome and queues its
// idempotent commit/release command in the same transaction. Unknown outcomes
// are recorded while the hold remains frozen for later reconciliation.
func (s *Store) ApplyCreditSettlementOutcome(ctx context.Context, outcome application.CreditSettlementOutcome) error {
	if outcome.EventID == uuid.Nil || outcome.ScopeID == uuid.Nil || outcome.HoldID == uuid.Nil || outcome.OccurredAt.IsZero() || outcome.AmountMicros < 0 {
		return application.ErrCreditOutcomeConflict
	}
	if outcome.Outcome != "unknown" && outcome.Outcome != "succeeded" && outcome.Outcome != "failed" {
		return application.ErrCreditOutcomeConflict
	}
	if outcome.Outcome != "succeeded" && outcome.AmountMicros != 0 {
		return application.ErrCreditOutcomeConflict
	}
	hash, err := creditSagaEventHash(struct {
		EventID      uuid.UUID `json:"event_id"`
		ScopeID      uuid.UUID `json:"scope_id"`
		HoldID       uuid.UUID `json:"hold_id"`
		Outcome      string    `json:"outcome"`
		AmountMicros int64     `json:"amount_micros"`
		OccurredAt   time.Time `json:"occurred_at"`
	}{outcome.EventID, outcome.ScopeID, outcome.HoldID, outcome.Outcome, outcome.AmountMicros, outcome.OccurredAt.UTC()})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hold struct {
			ID           uuid.UUID
			ScopeID      uuid.UUID
			AmountMicros int64
			Status       string
		}
		query := tx.Raw(`SELECT id,scope_id,amount_micros,status FROM billing.credit_hold
			WHERE id=?::uuid AND scope_id=?::uuid FOR UPDATE`, outcome.HoldID, outcome.ScopeID).Scan(&hold)
		if query.Error != nil {
			return fmt.Errorf("lock credit saga hold: %w", query.Error)
		}
		if query.RowsAffected == 0 {
			return application.ErrCreditCommandNotFound
		}
		inserted := tx.Exec(`INSERT INTO billing.credit_saga_event
			(event_id,event_type,scope_id,hold_id,request_hash,created_at)
			VALUES (?::uuid,'settlement_outcome',?::uuid,?::uuid,?,?) ON CONFLICT DO NOTHING`,
			outcome.EventID, outcome.ScopeID, outcome.HoldID, hash, now)
		if inserted.Error != nil {
			return fmt.Errorf("record credit settlement outcome: %w", inserted.Error)
		}
		if inserted.RowsAffected == 0 {
			var existing creditSagaEventRow
			result := tx.Raw(`SELECT event_type,scope_id,hold_id,request_hash FROM billing.credit_saga_event WHERE event_id=?::uuid`, outcome.EventID).Scan(&existing)
			if result.Error != nil {
				return fmt.Errorf("read duplicate credit settlement outcome: %w", result.Error)
			}
			if result.RowsAffected != 1 || existing.EventType != "settlement_outcome" || existing.ScopeID != outcome.ScopeID || existing.HoldID != outcome.HoldID || existing.RequestHash != hash {
				return application.ErrCreditOutcomeConflict
			}
			return nil
		}

		var saga creditSagaRow
		result := tx.Raw(`SELECT hold_id,scope_id,outcome,amount_micros,status,command_id
			FROM billing.credit_saga WHERE hold_id=?::uuid FOR UPDATE`, outcome.HoldID).Scan(&saga)
		if result.Error != nil {
			return fmt.Errorf("lock credit saga state: %w", result.Error)
		}
		hasSaga := result.RowsAffected == 1
		if hasSaga && saga.Outcome != "unknown" {
			if outcome.Outcome == "unknown" {
				return nil
			}
			if saga.Outcome != outcome.Outcome || saga.AmountMicros != outcome.AmountMicros {
				return application.ErrCreditOutcomeConflict
			}
			return nil
		}
		if outcome.Outcome == "unknown" {
			if !hasSaga {
				if err := exactlyOne(tx.Exec(`INSERT INTO billing.credit_saga
					(hold_id,scope_id,outcome,amount_micros,status,created_at,updated_at)
					VALUES (?::uuid,?::uuid,'unknown',0,'unknown',?,?)`, outcome.HoldID, outcome.ScopeID, now, now)); err != nil {
					return fmt.Errorf("record unknown credit saga outcome: %w", err)
				}
			}
			return nil
		}
		if hold.Status != "held" {
			return application.ErrCreditOutcomeConflict
		}
		if outcome.Outcome == "succeeded" && outcome.AmountMicros > hold.AmountMicros {
			return application.ErrCreditOutcomeConflict
		}

		action := application.CreditRelease
		amount := hold.AmountMicros
		if outcome.Outcome == "succeeded" && outcome.AmountMicros > 0 {
			action = application.CreditCommit
			amount = outcome.AmountMicros
		}
		holdID := outcome.HoldID
		command := application.CreditCommand{
			CallerID: creditSagaCallerID, IdempotencyKey: "credit-saga/" + holdID.String() + "/settle",
			ScopeID: outcome.ScopeID, Action: action, SourceID: holdID, ReferenceID: &holdID,
			AmountMicros: amount, Reason: "automatic credit saga settlement: " + outcome.Outcome,
		}
		receipt, err := submitCreditCommandTx(tx, command, now)
		if err != nil {
			return fmt.Errorf("queue automatic credit saga settlement: %w", err)
		}
		if hasSaga {
			updated := tx.Exec(`UPDATE billing.credit_saga SET outcome=?,amount_micros=?,status='command_accepted',command_id=?,updated_at=?
				WHERE hold_id=?::uuid AND outcome='unknown' AND status='unknown'`, outcome.Outcome, outcome.AmountMicros, receipt.ID, now, outcome.HoldID)
			if updated.Error != nil {
				return fmt.Errorf("advance unknown credit saga: %w", updated.Error)
			}
			if updated.RowsAffected != 1 {
				return application.ErrCreditOutcomeConflict
			}
		} else if err := exactlyOne(tx.Exec(`INSERT INTO billing.credit_saga
			(hold_id,scope_id,outcome,amount_micros,status,command_id,created_at,updated_at)
			VALUES (?::uuid,?::uuid,?,?, 'command_accepted',?::uuid,?,?)`,
			outcome.HoldID, outcome.ScopeID, outcome.Outcome, outcome.AmountMicros, receipt.ID, now, now)); err != nil {
			return fmt.Errorf("create credit saga state: %w", err)
		}
		return nil
	})
}

// ApplyCreditCommandResult advances the saga only after the ledger command
// result event has been committed to the outbox and consumed from Kafka.
func (s *Store) ApplyCreditCommandResult(ctx context.Context, result application.CreditCommandResult) error {
	if result.EventID == uuid.Nil || result.CommandID == uuid.Nil || result.ScopeID == uuid.Nil || result.Status == "" || result.OccurredAt.IsZero() {
		return application.ErrCreditOutcomeConflict
	}
	hash, err := creditSagaEventHash(struct {
		EventID    uuid.UUID       `json:"event_id"`
		CommandID  uuid.UUID       `json:"command_id"`
		ScopeID    uuid.UUID       `json:"scope_id"`
		Status     string          `json:"status"`
		Result     json.RawMessage `json:"result,omitempty"`
		OccurredAt time.Time       `json:"occurred_at"`
	}{result.EventID, result.CommandID, result.ScopeID, result.Status, result.Result, result.OccurredAt.UTC()})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var saga creditSagaRow
		query := tx.Raw(`SELECT hold_id,scope_id,outcome,amount_micros,status,command_id
			FROM billing.credit_saga WHERE command_id=?::uuid AND scope_id=?::uuid FOR UPDATE`, result.CommandID, result.ScopeID).Scan(&saga)
		if query.Error != nil {
			return fmt.Errorf("lock credit saga command result: %w", query.Error)
		}
		if query.RowsAffected == 0 {
			return nil
		}
		inserted := tx.Exec(`INSERT INTO billing.credit_saga_event
			(event_id,event_type,scope_id,hold_id,command_id,request_hash,created_at)
			VALUES (?::uuid,'command_result',?::uuid,?::uuid,?::uuid,?,?) ON CONFLICT DO NOTHING`,
			result.EventID, result.ScopeID, saga.HoldID, result.CommandID, hash, now)
		if inserted.Error != nil {
			return fmt.Errorf("record credit saga command result: %w", inserted.Error)
		}
		if inserted.RowsAffected == 0 {
			var existing creditSagaEventRow
			read := tx.Raw(`SELECT event_type,scope_id,hold_id,command_id,request_hash FROM billing.credit_saga_event WHERE event_id=?::uuid`, result.EventID).Scan(&existing)
			if read.Error != nil {
				return fmt.Errorf("read duplicate credit saga command result: %w", read.Error)
			}
			if read.RowsAffected != 1 || existing.EventType != "command_result" || existing.ScopeID != result.ScopeID || existing.HoldID != saga.HoldID || existing.CommandID == nil || *existing.CommandID != result.CommandID || existing.RequestHash != hash {
				return application.ErrCreditOutcomeConflict
			}
			return nil
		}
		var command struct {
			Action string
			Status string
		}
		read := tx.Raw(`SELECT action,status FROM billing.credit_command WHERE id=?::uuid AND scope_id=?::uuid`, result.CommandID, result.ScopeID).Scan(&command)
		if read.Error != nil {
			return fmt.Errorf("read credit saga command result: %w", read.Error)
		}
		if read.RowsAffected != 1 || command.Status != result.Status {
			return application.ErrCreditOutcomeConflict
		}
		status := "reconcile_required"
		if result.Status == "succeeded" {
			switch command.Action {
			case string(application.CreditCommit):
				status = "committed"
			case string(application.CreditRelease):
				status = "released"
			}
		}
		updated := tx.Exec(`UPDATE billing.credit_saga SET status=?,updated_at=? WHERE hold_id=?::uuid AND command_id=?::uuid`, status, now, saga.HoldID, result.CommandID)
		if updated.Error != nil {
			return fmt.Errorf("complete credit saga: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return application.ErrCreditOutcomeConflict
		}
		return nil
	})
}

func creditSagaEventHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode credit saga event digest: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

var _ application.CreditSagaStore = (*Store)(nil)
