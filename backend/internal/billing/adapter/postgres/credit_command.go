package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

const (
	creditCommandTopic       = "glowloom.billing.command.v1"
	creditCommandResultTopic = "glowloom.billing.command_result.v1"
)

type creditCommandRow struct {
	ID                 uuid.UUID
	ScopeID            uuid.UUID
	CallerID           string
	IdempotencyKeyHash string
	RequestHash        string
	Action             string
	SourceID           uuid.UUID
	ReferenceID        *uuid.UUID
	AmountMicros       int64
	ExpiresAt          *time.Time
	ModelKey           *string
	Reason             string
	Status             string
	Result             []byte
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type creditCommandEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	CommandID  uuid.UUID `json:"command_id"`
	ScopeID    uuid.UUID `json:"scope_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type creditCommandResultEvent struct {
	EventID    uuid.UUID       `json:"event_id"`
	EventType  string          `json:"event_type"`
	CommandID  uuid.UUID       `json:"command_id"`
	ScopeID    uuid.UUID       `json:"scope_id"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// SubmitCreditCommand idempotently records a command and its Outbox event.
func (s *Store) SubmitCreditCommand(ctx context.Context, command application.CreditCommand) (application.CreditCommandReceipt, error) {
	now := time.Now().UTC()
	if err := command.Validate(now); err != nil {
		return application.CreditCommandReceipt{}, err
	}
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	keyHash := hex.EncodeToString(keyDigest[:])
	requestData, err := json.Marshal(struct {
		ScopeID     uuid.UUID                `json:"scope_id"`
		Action      application.CreditAction `json:"action"`
		SourceID    uuid.UUID                `json:"source_id"`
		ReferenceID *uuid.UUID               `json:"reference_id,omitempty"`
		Amount      int64                    `json:"amount_micros"`
		ExpiresAt   *time.Time               `json:"expires_at,omitempty"`
		ModelKey    *string                  `json:"model_key,omitempty"`
		Reason      string                   `json:"reason"`
	}{command.ScopeID, command.Action, command.SourceID, command.ReferenceID, command.AmountMicros, command.ExpiresAt, command.ModelKey, command.Reason})
	if err != nil {
		return application.CreditCommandReceipt{}, fmt.Errorf("encode credit command digest: %w", err)
	}
	requestDigest := sha256.Sum256(requestData)
	requestHash := hex.EncodeToString(requestDigest[:])
	commandID, err := uuid.NewV7()
	if err != nil {
		return application.CreditCommandReceipt{}, fmt.Errorf("create credit command ID: %w", err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return application.CreditCommandReceipt{}, fmt.Errorf("create credit command event ID: %w", err)
	}
	var receipt application.CreditCommandReceipt
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var walletScope uuid.UUID
		if err := tx.Raw(`SELECT billing.ensure_wallet_account(?::uuid)`, command.ScopeID).Row().Scan(&walletScope); err != nil {
			return fmt.Errorf("ensure credit account: %w", err)
		}
		if walletScope != command.ScopeID {
			return application.ErrInvalidCreditCommand
		}
		inserted := tx.Exec(`
			INSERT INTO billing.credit_command
			  (id, scope_id, caller_id, idempotency_key_hash, request_hash, action, source_id,
			   reference_id, amount_micros, expires_at, model_key, reason, status)
			VALUES (?::uuid, ?::uuid, ?, ?, ?, ?, ?::uuid, ?::uuid, ?, ?, ?, ?, 'accepted')
			ON CONFLICT DO NOTHING
		`, commandID, command.ScopeID, command.CallerID, keyHash, requestHash, string(command.Action), command.SourceID,
			command.ReferenceID, command.AmountMicros, command.ExpiresAt, command.ModelKey, command.Reason)
		if inserted.Error != nil {
			return fmt.Errorf("accept credit command: %w", inserted.Error)
		}
		if inserted.RowsAffected == 0 {
			var existing creditCommandRow
			query := tx.Raw(`SELECT id,scope_id,caller_id,idempotency_key_hash,request_hash,action,source_id,
				reference_id,amount_micros,expires_at,model_key,reason,status,result,created_at,updated_at
				FROM billing.credit_command WHERE caller_id=? AND idempotency_key_hash=?`, command.CallerID, keyHash).Scan(&existing)
			if query.Error != nil {
				return fmt.Errorf("read credit command idempotency receipt: %w", query.Error)
			}
			if query.RowsAffected == 0 {
				if command.Action == application.CreditCommit || command.Action == application.CreditRelease {
					query = tx.Raw(`SELECT id,scope_id,caller_id,idempotency_key_hash,request_hash,action,source_id,
						reference_id,amount_micros,expires_at,model_key,reason,status,result,created_at,updated_at
						FROM billing.credit_command WHERE scope_id=? AND action=? AND reference_id=?::uuid
						ORDER BY created_at LIMIT 1`, command.ScopeID, string(command.Action), command.ReferenceID).Scan(&existing)
				} else {
					query = tx.Raw(`SELECT id,scope_id,caller_id,idempotency_key_hash,request_hash,action,source_id,
					reference_id,amount_micros,expires_at,model_key,reason,status,result,created_at,updated_at
					FROM billing.credit_command WHERE scope_id=? AND action=? AND source_id=?
					ORDER BY created_at LIMIT 1`, command.ScopeID, string(command.Action), command.SourceID).Scan(&existing)
				}
				if query.Error != nil {
					return fmt.Errorf("read credit source receipt: %w", query.Error)
				}
			}
			if query.RowsAffected == 0 {
				return application.ErrCreditCommandConflict
			}
			if existing.ScopeID != command.ScopeID || existing.RequestHash != requestHash || existing.CallerID != command.CallerID {
				if existing.CallerID == command.CallerID && existing.IdempotencyKeyHash == keyHash {
					return application.ErrCreditIdempotencyConflict
				}
				return application.ErrCreditCommandConflict
			}
			receipt = commandReceipt(existing)
			return nil
		}
		event := creditCommandEvent{EventID: eventID, EventType: creditCommandTopic, CommandID: commandID, ScopeID: command.ScopeID, OccurredAt: now}
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode credit command event: %w", err)
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, eventID, creditCommandTopic, command.ScopeID, string(payload))); err != nil {
			return fmt.Errorf("queue accepted credit command: %w", err)
		}
		receipt = application.CreditCommandReceipt{ID: commandID, ScopeID: command.ScopeID, Action: command.Action,
			Status: "accepted", SourceID: command.SourceID, ReferenceID: command.ReferenceID,
			AmountMicros: command.AmountMicros, CreatedAt: now, UpdatedAt: now}
		return nil
	})
	if err != nil {
		return application.CreditCommandReceipt{}, err
	}
	return receipt, nil
}

// ReadCreditCommand returns the receipt owned by the caller within one scope.
func (s *Store) ReadCreditCommand(ctx context.Context, callerID string, scopeID, commandID uuid.UUID) (application.CreditCommandReceipt, error) {
	var row creditCommandRow
	result := s.db.WithContext(ctx).Raw(`SELECT id,scope_id,caller_id,idempotency_key_hash,request_hash,action,source_id,
		reference_id,amount_micros,expires_at,model_key,reason,status,result,created_at,updated_at
		FROM billing.credit_command WHERE id=?::uuid AND caller_id=? AND scope_id=?::uuid`, commandID, callerID, scopeID).Scan(&row)
	if result.Error != nil {
		return application.CreditCommandReceipt{}, fmt.Errorf("read credit command: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return application.CreditCommandReceipt{}, application.ErrCreditCommandNotFound
	}
	return commandReceipt(row), nil
}

// ProcessCreditCommand applies one accepted command and commits its ledger effects.
func (s *Store) ProcessCreditCommand(ctx context.Context, commandID uuid.UUID) error {
	if commandID == uuid.Nil {
		return application.ErrInvalidCreditCommand
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command creditCommandRow
		row := tx.Raw(`SELECT id,scope_id,caller_id,idempotency_key_hash,request_hash,action,source_id,
			reference_id,amount_micros,expires_at,model_key,reason,status,result,created_at,updated_at
			FROM billing.credit_command WHERE id=?::uuid FOR UPDATE`, commandID).Scan(&command)
		if row.Error != nil {
			return fmt.Errorf("lock credit command: %w", row.Error)
		}
		if row.RowsAffected == 0 {
			return application.ErrCreditCommandNotFound
		}
		if command.Status != "accepted" {
			return nil
		}
		version, err := advanceWalletVersion(tx, command.ScopeID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		result, status, err := applyCreditCommand(tx, command, version, now)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrWalletInsufficient):
				status = "insufficient"
				result, err = json.Marshal(map[string]any{"code": "insufficient_credits", "account_version": version})
				if err != nil {
					return fmt.Errorf("encode insufficient credit receipt: %w", err)
				}
			case errors.Is(err, application.ErrCreditCommandConflict):
				status = "failed"
				result, err = json.Marshal(map[string]any{"code": "credit_command_conflict", "account_version": version})
				if err != nil {
					return fmt.Errorf("encode failed credit receipt: %w", err)
				}
			default:
				return err
			}
		}
		updated := tx.Exec(`UPDATE billing.credit_command SET status=?,result=?::jsonb,updated_at=? WHERE id=?::uuid AND status='accepted'`, status, string(result), now, commandID)
		if updated.Error != nil {
			return fmt.Errorf("complete credit command: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return application.ErrCreditCommandConflict
		}
		resultEventID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("create credit command result event ID: %w", err)
		}
		payload, err := json.Marshal(creditCommandResultEvent{EventID: resultEventID, EventType: creditCommandResultTopic, CommandID: command.ID,
			ScopeID: command.ScopeID, Status: status, Result: result, OccurredAt: now})
		if err != nil {
			return fmt.Errorf("encode credit command result event: %w", err)
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, resultEventID, creditCommandResultTopic, command.ScopeID, string(payload))); err != nil {
			return fmt.Errorf("queue credit command result: %w", err)
		}
		return nil
	})
}

func applyCreditCommand(tx *gorm.DB, command creditCommandRow, accountVersion int64, now time.Time) ([]byte, string, error) {
	var available int64
	if err := tx.Raw(`SELECT COALESCE(sum(available_micros),0) FROM billing.wallet_lot
		WHERE scope_id=? AND available_micros>0 AND (expires_at IS NULL OR expires_at>?)`, command.ScopeID, now).Scan(&available).Error; err != nil {
		return nil, "", fmt.Errorf("read credit availability: %w", err)
	}
	switch command.Action {
	case string(application.CreditGrant):
		if command.ExpiresAt == nil || !command.ExpiresAt.After(now) {
			return nil, "", application.ErrCreditCommandConflict
		}
		lotID, err := uuid.NewV7()
		if err != nil {
			return nil, "", fmt.Errorf("create credit lot ID: %w", err)
		}
		var insertedLot uuid.UUID
		if err := tx.Raw(`SELECT billing.insert_credit_grant_lot(?::uuid,?::uuid,?)`, lotID, command.ID, now).Row().Scan(&insertedLot); err != nil {
			return nil, "", fmt.Errorf("issue promotional credit lot: %w", err)
		}
		if insertedLot != lotID {
			return nil, "", application.ErrCreditCommandConflict
		}
		if err := insertCommandWalletEntry(tx, command, lotID, nil, "issue", command.AmountMicros, now); err != nil {
			return nil, "", err
		}
		available += command.AmountMicros
		return creditResult(accountVersion, available, 0, nil)
	case string(application.CreditDebit), string(application.CreditFreeze):
		lots, err := availableWalletLots(tx, command.ScopeID)
		if err != nil {
			return nil, "", fmt.Errorf("read eligible credit lots: %w", err)
		}
		eligible := int64(0)
		for _, lot := range lots {
			if lot.ModelKey != nil && (command.ModelKey == nil || *lot.ModelKey != *command.ModelKey) {
				continue
			}
			if eligible > math.MaxInt64-lot.AvailableMicros {
				return nil, "", application.ErrCreditCommandConflict
			}
			eligible += lot.AvailableMicros
		}
		if eligible < command.AmountMicros {
			return nil, "", domain.ErrWalletInsufficient
		}
		remaining := command.AmountMicros
		var holdID *uuid.UUID
		if command.Action == string(application.CreditFreeze) {
			hold := command.ID
			if err := exactlyOne(tx.Exec(`INSERT INTO billing.credit_hold(id,scope_id,source_id,command_id,amount_micros,status,created_at)
				VALUES (?::uuid,?::uuid,?::uuid,?,?,'held',?)`, hold, command.ScopeID, command.SourceID, command.ID, command.AmountMicros, now)); err != nil {
				return nil, "", fmt.Errorf("create credit freeze: %w", err)
			}
			holdID = &hold
		}
		for index := range lots {
			lot := lots[index]
			if lot.ModelKey != nil && (command.ModelKey == nil || *lot.ModelKey != *command.ModelKey) {
				continue
			}
			amount := min(remaining, lot.AvailableMicros)
			if amount == 0 {
				continue
			}
			if err := setLotAvailableCAS(tx, lot, lot.AvailableMicros-amount); err != nil {
				return nil, "", err
			}
			kind := "redeem"
			if holdID != nil {
				kind = "hold"
				if err := exactlyOne(tx.Exec(`INSERT INTO billing.credit_hold_allocation(hold_id,scope_id,lot_id,ordinal,amount_micros) VALUES(?,?,?,?,?)`, *holdID, command.ScopeID, lot.ID, index+1, amount)); err != nil {
					return nil, "", fmt.Errorf("allocate credit freeze: %w", err)
				}
			}
			if err := insertCommandWalletEntry(tx, command, lot.ID, holdID, kind, amount, now); err != nil {
				return nil, "", err
			}
			remaining -= amount
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return nil, "", domain.ErrWalletInsufficient
		}
		available -= command.AmountMicros
		held := int64(0)
		if holdID != nil {
			held = command.AmountMicros
		}
		return creditResult(accountVersion, available, held, holdID)
	case string(application.CreditCommit), string(application.CreditRelease):
		var hold struct {
			ID           uuid.UUID
			ScopeID      uuid.UUID
			AmountMicros int64
			Status       string
		}
		row := tx.Raw(`SELECT id,scope_id,amount_micros,status FROM billing.credit_hold WHERE id=?::uuid FOR UPDATE`, command.ReferenceID).Scan(&hold)
		if row.Error != nil {
			return nil, "", fmt.Errorf("lock credit freeze: %w", row.Error)
		}
		if row.RowsAffected == 0 || hold.ScopeID != command.ScopeID || hold.Status != "held" {
			return nil, "", application.ErrCreditCommandConflict
		}
		if command.Action == string(application.CreditRelease) && command.AmountMicros != hold.AmountMicros || command.Action == string(application.CreditCommit) && command.AmountMicros > hold.AmountMicros {
			return nil, "", application.ErrCreditCommandConflict
		}
		var allocations []struct {
			LotID           uuid.UUID
			AmountMicros    int64
			AvailableMicros int64
			Version         int64
			ExpiresAt       *time.Time
			RootLotID       *uuid.UUID
			ModelKey        *string
		}
		if err := tx.Raw(`SELECT a.lot_id,a.amount_micros,l.available_micros,l.version,l.expires_at,l.root_lot_id,l.model_key
			FROM billing.credit_hold_allocation a JOIN billing.wallet_lot l ON l.id=a.lot_id
			WHERE a.hold_id=?::uuid AND a.scope_id=?::uuid ORDER BY a.ordinal`, hold.ID, hold.ScopeID).Scan(&allocations).Error; err != nil {
			return nil, "", fmt.Errorf("read credit freeze allocations: %w", err)
		}
		remaining := int64(0)
		if command.Action == string(application.CreditCommit) {
			remaining = command.AmountMicros
		}
		for _, allocation := range allocations {
			used := int64(0)
			if command.Action == string(application.CreditCommit) {
				used = min(remaining, allocation.AmountMicros)
				remaining -= used
			}
			if used > 0 {
				if err := insertCommandWalletEntry(tx, command, allocation.LotID, &hold.ID, "redeem", used, now); err != nil {
					return nil, "", err
				}
			}
			refund := allocation.AmountMicros - used
			if refund == 0 {
				continue
			}
			if allocation.ExpiresAt == nil || allocation.ExpiresAt.After(now) {
				lot := walletLot{ID: allocation.LotID, AvailableMicros: allocation.AvailableMicros, Version: allocation.Version}
				if lot.AvailableMicros > math.MaxInt64-refund {
					return nil, "", application.ErrCreditCommandConflict
				}
				if err := setLotAvailableCAS(tx, lot, lot.AvailableMicros+refund); err != nil {
					return nil, "", err
				}
				if err := insertCommandWalletEntry(tx, command, allocation.LotID, &hold.ID, "release", refund, now); err != nil {
					return nil, "", err
				}
				available += refund
			} else {
				if err := insertCommandWalletEntry(tx, command, allocation.LotID, &hold.ID, "expire", refund, now); err != nil {
					return nil, "", err
				}
				restored, err := restoreExpiredCreditHold(tx, command, hold.ID, allocation.LotID, allocation.RootLotID, allocation.ModelKey, refund, now)
				if err != nil {
					return nil, "", err
				}
				if restored {
					available += refund
				}
			}
		}
		if remaining != 0 {
			return nil, "", application.ErrCreditCommandConflict
		}
		status := "committed"
		if command.Action == string(application.CreditRelease) {
			status = "released"
		}
		updated := tx.Exec(`UPDATE billing.credit_hold SET status=?,closed_at=? WHERE id=?::uuid AND status='held'`, status, now, hold.ID)
		if updated.Error != nil {
			return nil, "", fmt.Errorf("close credit freeze: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return nil, "", application.ErrCreditCommandConflict
		}
		return creditResult(accountVersion, available, 0, &hold.ID)
	default:
		return nil, "", application.ErrInvalidCreditCommand
	}
}

func insertCommandWalletEntry(tx *gorm.DB, command creditCommandRow, lotID uuid.UUID, holdID *uuid.UUID, kind string, amount int64, at time.Time) error {
	entryID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("create wallet entry ID: %w", err)
	}
	row := tx.Exec(`INSERT INTO billing.wallet_entry(id,scope_id,lot_id,command_id,hold_id,kind,amount_micros,created_at)
		VALUES (?::uuid,?::uuid,?::uuid,?::uuid,?::uuid,?,?,?)`, entryID, command.ScopeID, lotID, command.ID, holdID, kind, amount, at)
	if row.Error != nil {
		return fmt.Errorf("append credit %s ledger entry: %w", kind, row.Error)
	}
	if row.RowsAffected != 1 {
		return fmt.Errorf("append credit %s ledger entry: inserted %d rows", kind, row.RowsAffected)
	}
	return nil
}

func restoreExpiredCreditHold(tx *gorm.DB, command creditCommandRow, holdID, originalLotID uuid.UUID, rootLotID *uuid.UUID, modelKey *string, amount int64, at time.Time) (bool, error) {
	root := originalLotID
	if rootLotID != nil {
		root = *rootLotID
	}
	if err := tx.Exec(`INSERT INTO billing.wallet_expiry_grace(root_lot_id,expires_at) VALUES (?,?) ON CONFLICT (root_lot_id) DO NOTHING`, root, at.Add(7*24*time.Hour)).Error; err != nil {
		return false, fmt.Errorf("preserve expired credit freeze: %w", err)
	}
	var deadline time.Time
	if err := tx.Raw(`SELECT expires_at FROM billing.wallet_expiry_grace WHERE root_lot_id=?`, root).Scan(&deadline).Error; err != nil {
		return false, fmt.Errorf("read expired credit grace: %w", err)
	}
	if !deadline.After(at) {
		return false, nil
	}
	lotID, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("create expired credit return lot ID: %w", err)
	}
	var returned *uuid.UUID
	if err := tx.Raw(`SELECT billing.insert_credit_return_lot(?,?,?,?,?,?,?,?)`, lotID, command.ScopeID, holdID, amount, deadline, root, modelKey, at).Row().Scan(&returned); err != nil {
		return false, fmt.Errorf("restore expired frozen credit: %w", err)
	}
	if returned == nil || *returned != lotID {
		return false, application.ErrCreditCommandConflict
	}
	if err := insertCommandWalletEntry(tx, command, lotID, &holdID, "restore", amount, at); err != nil {
		return false, err
	}
	return true, nil
}

func creditResult(version, available, held int64, holdID *uuid.UUID) ([]byte, string, error) {
	data, err := json.Marshal(struct {
		AccountVersion  int64      `json:"account_version"`
		AvailableMicros int64      `json:"available_micros"`
		HeldMicros      int64      `json:"held_micros"`
		HoldID          *uuid.UUID `json:"hold_id,omitempty"`
	}{version, available, held, holdID})
	if err != nil {
		return nil, "", fmt.Errorf("encode credit command result: %w", err)
	}
	return data, "succeeded", nil
}

func commandReceipt(row creditCommandRow) application.CreditCommandReceipt {
	return application.CreditCommandReceipt{ID: row.ID, ScopeID: row.ScopeID, Action: application.CreditAction(row.Action), Status: row.Status,
		SourceID: row.SourceID, ReferenceID: row.ReferenceID, AmountMicros: row.AmountMicros,
		Result: row.Result, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func exactlyOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("expected one row, wrote %d", result.RowsAffected)
	}
	return nil
}
