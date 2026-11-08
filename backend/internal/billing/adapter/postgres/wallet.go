package postgres

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

type walletLot struct {
	ID              uuid.UUID
	ModelKey        *string
	AvailableMicros int64
	Version         int64
}

// holdWallet follows the existing project budget lock. Every writer of a
// scope's lots takes its account lock before reading or changing a lot.
func holdWallet(tx *gorm.DB, input application.ReserveInput, reservationID uuid.UUID) error {
	if input.AmountMicros == 0 && input.ReusedResult {
		return nil
	}
	payer, err := walletPayer(tx, input.ProjectID)
	if err != nil {
		return err
	}
	if payer == uuid.Nil {
		if input.AmountMicros > 0 {
			return domain.ErrWalletInsufficient
		}
		return nil
	}
	if payer != input.Actor.ScopeID {
		return domain.ErrWalletPayerChanged
	}
	if !input.CommercialPriceReady {
		return domain.ErrWalletPriceUnavailable
	}
	if input.AmountMicros == 0 {
		return nil
	}
	if err := lockWallet(tx, payer); err != nil {
		return err
	}
	lots, err := availableWalletLots(tx, payer)
	if err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO billing.wallet_hold(reservation_id,scope_id) VALUES (?,?)`, reservationID, payer).Error; err != nil {
		return err
	}
	remaining := input.AmountMicros
	for index, lot := range lots {
		if lot.ModelKey != nil && (input.ModelKey == nil || *lot.ModelKey != *input.ModelKey) {
			continue
		}
		amount := min(remaining, lot.AvailableMicros)
		if err := changeLotBalance(tx, lot.ID, -amount); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO billing.wallet_allocation(reservation_id,scope_id,lot_id,ordinal,amount_micros)
			VALUES (?,?,?,?,?)`, reservationID, payer, lot.ID, index+1, amount).Error; err != nil {
			return err
		}
		if err := walletEntry(tx, payer, lot.ID, reservationID, "hold", amount, input.OccurredAt); err != nil {
			return err
		}
		remaining -= amount
		if remaining == 0 {
			return nil
		}
	}
	return domain.ErrWalletInsufficient
}

func walletPayer(tx *gorm.DB, projectID uuid.UUID) (uuid.UUID, error) {
	var payer struct {
		ScopeID, OwnerScopeID uuid.UUID
	}
	read := tx.Raw(`SELECT coalesce(w.scope_id, a.scope_id) AS scope_id, p.owner_scope_id
        FROM workspace.project p
        LEFT JOIN billing.project_wallet w ON w.project_id=p.id
        LEFT JOIN billing.wallet_account a ON a.scope_id=p.owner_scope_id
        WHERE p.id=? AND NOT p.is_delete AND coalesce(w.scope_id, a.scope_id) IS NOT NULL`, projectID).Scan(&payer)
	if read.Error != nil || read.RowsAffected == 0 {
		return uuid.Nil, read.Error
	}
	if payer.ScopeID != payer.OwnerScopeID {
		return uuid.Nil, domain.ErrWalletPayerChanged
	}
	return payer.ScopeID, nil
}

func availableWalletLots(tx *gorm.DB, scope uuid.UUID) ([]walletLot, error) {
	var lots []walletLot
	err := tx.Raw(`SELECT id,available_micros,model_key,version FROM billing.wallet_lot
		WHERE scope_id=? AND available_micros>0 AND (expires_at IS NULL OR expires_at>clock_timestamp())
		ORDER BY expires_at NULLS LAST,
		  CASE source_kind WHEN 'promotional' THEN 0 WHEN 'expiry_return' THEN 0 WHEN 'subscription' THEN 1 ELSE 2 END,id`,
		scope).Scan(&lots).Error
	return lots, err
}

// CheckWalletQuoteInTransaction simulates the same lot order as confirmation.
// The caller has already checked project access; confirmation still takes locks.
func (s *Store) CheckWalletQuoteInTransaction(tx *gorm.DB, projectID uuid.UUID, items []application.WalletDemand) error {
	scope, err := walletPayer(tx, projectID)
	if err != nil {
		return err
	}
	if scope == uuid.Nil {
		for _, item := range items {
			if item.Micros > 0 {
				return domain.ErrWalletInsufficient
			}
		}
		return nil
	}
	lots, err := availableWalletLots(tx, scope)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Micros == 0 && item.ReusedResult {
			continue
		}
		if !item.CommercialPriceReady {
			return domain.ErrWalletPriceUnavailable
		}
		remaining := item.Micros
		for i := range lots {
			if lots[i].ModelKey != nil && *lots[i].ModelKey != item.ModelKey {
				continue
			}
			amount := min(remaining, lots[i].AvailableMicros)
			lots[i].AvailableMicros -= amount
			remaining -= amount
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return domain.ErrWalletInsufficient
		}
	}
	return nil
}

func lockWallet(tx *gorm.DB, scope uuid.UUID) error {
	_, err := advanceWalletVersion(tx, scope)
	return err
}

func changeLotBalance(tx *gorm.DB, lot uuid.UUID, delta int64) error {
	var current walletLot
	if err := tx.Raw(`SELECT id,available_micros,version FROM billing.wallet_lot WHERE id=?::uuid`, lot).Scan(&current).Error; err != nil {
		return err
	}
	if current.ID == uuid.Nil || delta > 0 && current.AvailableMicros > math.MaxInt64-delta || delta < 0 && (delta == math.MinInt64 || current.AvailableMicros < -delta) {
		return ErrSettlementConflict
	}
	return setLotAvailableCAS(tx, current, current.AvailableMicros+delta)
}

func walletEntry(tx *gorm.DB, scope, lot, reservation uuid.UUID, kind string, amount int64, at time.Time) error {
	return tx.Exec(`INSERT INTO billing.wallet_entry(scope_id,lot_id,reservation_id,kind,amount_micros,created_at)
		VALUES (?,?,?,?,?,?)`, scope, lot, reservation, kind, amount, at.UTC()).Error
}

// settleWallet uses only the original allocation, including when the project
// has changed owner or a lot has expired since confirmation.
func settleWallet(tx *gorm.DB, reservation domain.Reservation, charge int64, at time.Time) error {
	var hold struct{ ScopeID uuid.UUID }
	row := tx.Raw(`SELECT scope_id FROM billing.wallet_hold WHERE reservation_id=?`, reservation.ID).Scan(&hold)
	if row.Error != nil || row.RowsAffected == 0 {
		return row.Error
	}
	scope := hold.ScopeID
	if err := lockWallet(tx, scope); err != nil {
		return err
	}
	var allocations []struct {
		LotID        uuid.UUID
		AmountMicros int64
		ExpiresAt    *time.Time
		RootLotID    *uuid.UUID
	}
	if err := tx.Raw(`SELECT a.lot_id,a.amount_micros,l.expires_at,l.root_lot_id
		FROM billing.wallet_allocation a JOIN billing.wallet_lot l ON l.id=a.lot_id
		WHERE a.reservation_id=? ORDER BY a.ordinal`, reservation.ID).Scan(&allocations).Error; err != nil {
		return err
	}
	remaining, allocated := charge, reservation.AmountMicros
	expired := make(map[uuid.UUID]int64)
	for _, a := range allocations {
		if a.AmountMicros <= 0 || a.AmountMicros > allocated {
			return ErrSettlementConflict
		}
		allocated -= a.AmountMicros
		used := min(remaining, a.AmountMicros)
		remaining -= used
		if used > 0 {
			if err := walletEntry(tx, scope, a.LotID, reservation.ID, "redeem", used, at); err != nil {
				return err
			}
		}
		unused := a.AmountMicros - used
		if unused == 0 {
			continue
		}
		if a.ExpiresAt == nil || at.Before(*a.ExpiresAt) {
			if err := changeLotBalance(tx, a.LotID, unused); err != nil {
				return err
			}
			if err := walletEntry(tx, scope, a.LotID, reservation.ID, "release", unused, at); err != nil {
				return err
			}
			continue
		}
		root := a.LotID
		if a.RootLotID != nil {
			root = *a.RootLotID
		}
		if err := walletEntry(tx, scope, a.LotID, reservation.ID, "expire", unused, at); err != nil {
			return err
		}
		expired[root] += unused
	}
	if allocated != 0 || remaining != 0 {
		return ErrSettlementConflict
	}
	for root, amount := range expired {
		if err := restoreExpiredWalletHold(tx, scope, root, reservation.ID, amount, at); err != nil {
			return err
		}
	}
	return nil
}

func restoreExpiredWalletHold(tx *gorm.DB, scope, root, reservation uuid.UUID, amount int64, at time.Time) error {
	if err := tx.Exec(`INSERT INTO billing.wallet_expiry_grace(root_lot_id,expires_at)
		VALUES (?,?) ON CONFLICT (root_lot_id) DO NOTHING`, root, at.Add(7*24*time.Hour)).Error; err != nil {
		return err
	}
	var deadline time.Time
	if err := tx.Raw(`SELECT expires_at FROM billing.wallet_expiry_grace WHERE root_lot_id=?`, root).Scan(&deadline).Error; err != nil {
		return err
	}
	if !deadline.After(at) {
		return nil
	}
	lot := uuid.New()
	var restored *uuid.UUID
	err := tx.Raw(`SELECT billing.insert_wallet_return_lot(?,?,?,?,?,?,?)`, lot, scope, reservation, amount, deadline, root, at).Row().Scan(&restored)
	if err != nil {
		return fmt.Errorf("restore expired wallet hold: %w", err)
	}
	if restored == nil || *restored != lot {
		return ErrSettlementConflict
	}
	return walletEntry(tx, scope, lot, reservation, "restore", amount, at)
}
