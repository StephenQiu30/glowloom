package postgres

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

const walletBalanceChangedTopic = "glowloom.billing.wallet_balance_changed.v1"

type walletBalanceChangedEvent struct {
	EventID        uuid.UUID `json:"event_id"`
	EventType      string    `json:"event_type"`
	ScopeID        uuid.UUID `json:"scope_id"`
	AccountVersion int64     `json:"account_version"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func advanceWalletVersion(tx *gorm.DB, scope uuid.UUID) (int64, error) {
	var current struct{ Version int64 }
	row := tx.Raw(`SELECT version FROM billing.wallet_account WHERE scope_id=?::uuid`, scope).Scan(&current)
	if row.Error != nil {
		return 0, fmt.Errorf("read wallet account version: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return 0, ErrNotFound
	}
	if current.Version < 1 || current.Version == math.MaxInt64 {
		return 0, application.ErrCreditVersionConflict
	}
	version := current.Version + 1
	updated := tx.Exec(`UPDATE billing.wallet_account SET version=? WHERE scope_id=?::uuid AND version=?`, version, scope, current.Version)
	if updated.Error != nil {
		return 0, fmt.Errorf("advance wallet account version: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return 0, application.ErrCreditVersionConflict
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return 0, fmt.Errorf("create wallet balance event ID: %w", err)
	}
	now := time.Now().UTC()
	payload, err := json.Marshal(walletBalanceChangedEvent{EventID: eventID, EventType: walletBalanceChangedTopic,
		ScopeID: scope, AccountVersion: version, OccurredAt: now})
	if err != nil {
		return 0, fmt.Errorf("encode wallet balance event: %w", err)
	}
	if err := exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, eventID, walletBalanceChangedTopic, scope, string(payload))); err != nil {
		return 0, fmt.Errorf("record wallet balance event: %w", err)
	}
	return version, nil
}

func setLotAvailableCAS(tx *gorm.DB, lot walletLot, next int64) error {
	if lot.ID == uuid.Nil || lot.Version < 1 || next < 0 || lot.Version == math.MaxInt64 {
		return ErrSettlementConflict
	}
	result := tx.Exec(`UPDATE billing.wallet_lot SET available_micros=?,version=? WHERE id=?::uuid AND version=?`, next, lot.Version+1, lot.ID, lot.Version)
	if result.Error != nil {
		return fmt.Errorf("update wallet lot with optimistic version: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrCreditVersionConflict
	}
	return nil
}
