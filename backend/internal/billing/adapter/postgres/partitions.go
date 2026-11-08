package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// PartitionStore manages monthly partitions for immutable wallet entries.
type PartitionStore struct{ db *gorm.DB }

// NewPartitionStore constructs a wallet entry partition maintainer.
func NewPartitionStore(db *gorm.DB) *PartitionStore { return &PartitionStore{db: db} }

// EnsurePartitions creates the current and next three UTC wallet-entry months.
// The default partition remains writable while missing month data is moved.
func (s *PartitionStore) EnsurePartitions(ctx context.Context, now time.Time) error {
	current := now.UTC()
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE billing.wallet_entry IN ACCESS EXCLUSIVE MODE").Error; err != nil {
			return fmt.Errorf("lock wallet entries for partition maintenance: %w", err)
		}
		for offset := range 4 {
			if err := ensureWalletEntryMonth(tx, first.AddDate(0, offset, 0)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("ensure wallet entry partitions: %w", err)
	}
	return nil
}

func ensureWalletEntryMonth(tx *gorm.DB, month time.Time) error {
	name := "wallet_entry_" + month.Format("200601")
	end := month.AddDate(0, 1, 0)
	var attached bool
	if err := tx.Raw(`SELECT EXISTS (
		SELECT 1 FROM pg_inherits AS i JOIN pg_class AS c ON c.oid=i.inhrelid
		JOIN pg_namespace AS n ON n.oid=c.relnamespace
		WHERE i.inhparent='billing.wallet_entry'::regclass AND n.nspname='billing' AND c.relname=?
	)`, name).Scan(&attached).Error; err != nil {
		return fmt.Errorf("check wallet entry partition %s: %w", name, err)
	}
	if attached {
		return nil
	}
	if err := tx.Exec("CREATE TABLE billing." + name + " (LIKE billing.wallet_entry INCLUDING ALL)").Error; err != nil {
		return fmt.Errorf("create wallet entry partition %s: %w", name, err)
	}
	move := `WITH moved AS (
		DELETE FROM billing.wallet_entry_default WHERE created_at >= ? AND created_at < ?
		RETURNING id,scope_id,lot_id,reservation_id,command_id,hold_id,kind,amount_micros,
			available_delta_micros,held_delta_micros,redeemed_delta_micros,expired_delta_micros,funding_delta_micros,created_at
	) INSERT INTO billing.` + name + ` (id,scope_id,lot_id,reservation_id,command_id,hold_id,kind,amount_micros,
		available_delta_micros,held_delta_micros,redeemed_delta_micros,expired_delta_micros,funding_delta_micros,created_at)
	SELECT id,scope_id,lot_id,reservation_id,command_id,hold_id,kind,amount_micros,
		available_delta_micros,held_delta_micros,redeemed_delta_micros,expired_delta_micros,funding_delta_micros,created_at FROM moved`
	if err := tx.Exec(move, month, end).Error; err != nil {
		return fmt.Errorf("move wallet entries into %s: %w", name, err)
	}
	attach := fmt.Sprintf("ALTER TABLE billing.wallet_entry ATTACH PARTITION billing.%s FOR VALUES FROM ('%s') TO ('%s')", name, month.Format(time.RFC3339), end.Format(time.RFC3339))
	if err := tx.Exec(attach).Error; err != nil {
		return fmt.Errorf("attach wallet entry partition %s: %w", name, err)
	}
	return nil
}
