package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
)

func TestWalletEntryPartitionMovesDefaultRowsBeforeAttach(t *testing.T) {
	ctx, database := billingDB(t)
	if err := pgbilling.NewPartitionStore(database).EnsurePartitions(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("create current wallet-entry partitions: %v", err)
	}
	actor, _ := billingProject(t, database)
	var walletScope uuid.UUID
	if err := database.Raw(`SELECT billing.ensure_wallet_account(?::uuid)`, actor.ScopeID).Row().Scan(&walletScope); err != nil || walletScope != actor.ScopeID {
		t.Fatalf("ensure wallet account = %s, error = %v", walletScope, err)
	}
	lotID, entryID, sourceID := uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO billing.wallet_lot
		(id,scope_id,source_kind,source_id,issued_micros,available_micros,expires_at,reason)
		VALUES (?::uuid,?::uuid,'promotional',?,100000,100000,?,'partition test')`,
		lotID, actor.ScopeID, sourceID, time.Now().UTC().Add(24*time.Hour)).Error; err != nil {
		t.Fatalf("create wallet lot: %v", err)
	}
	var latestPartition string
	if err := database.Raw(`SELECT c.relname FROM pg_inherits AS i
		JOIN pg_class AS c ON c.oid=i.inhrelid
		JOIN pg_namespace AS n ON n.oid=c.relnamespace
		WHERE i.inhparent='billing.wallet_entry'::regclass AND n.nspname='billing' AND c.relname ~ '^wallet_entry_[0-9]{6}$'
		ORDER BY c.relname DESC LIMIT 1`).Scan(&latestPartition).Error; err != nil || latestPartition == "" {
		t.Fatalf("read latest wallet-entry month partition %q: %v", latestPartition, err)
	}
	lastMonth, err := time.Parse("wallet_entry_200601", latestPartition)
	if err != nil {
		t.Fatalf("parse latest wallet-entry month partition %q: %v", latestPartition, err)
	}
	entryMonth := lastMonth.AddDate(0, 1, 0)
	entryTime := entryMonth.Add(24 * time.Hour)
	if err := database.Exec(`INSERT INTO billing.wallet_entry(id,scope_id,lot_id,kind,amount_micros,created_at)
		VALUES (?::uuid,?::uuid,?::uuid,'issue',100000,?)`, entryID, actor.ScopeID, lotID, entryTime).Error; err != nil {
		t.Fatalf("insert wallet entry in default partition: %v", err)
	}
	var before string
	if err := database.Raw(`SELECT tableoid::regclass::text FROM billing.wallet_entry WHERE id=?::uuid`, entryID).Scan(&before).Error; err != nil || before != "billing.wallet_entry_default" {
		t.Fatalf("wallet entry before partition maintenance = %q, error = %v", before, err)
	}
	if err := pgbilling.NewPartitionStore(database).EnsurePartitions(ctx, entryMonth.AddDate(0, -3, 0)); err != nil {
		t.Fatalf("ensure wallet entry partitions: %v", err)
	}
	var after string
	if err := database.Raw(`SELECT tableoid::regclass::text FROM billing.wallet_entry WHERE id=?::uuid`, entryID).Scan(&after).Error; err != nil || after != "billing.wallet_entry_"+entryMonth.Format("200601") {
		t.Fatalf("wallet entry after partition maintenance = %q, error = %v", after, err)
	}
	var availableDelta, fundingDelta int64
	if err := database.Raw(`SELECT available_delta_micros,funding_delta_micros
		FROM billing.wallet_entry WHERE id=?::uuid`, entryID).Row().Scan(&availableDelta, &fundingDelta); err != nil || availableDelta != 100000 || fundingDelta != -100000 {
		t.Fatalf("moved journal delta = available %d funding %d, error = %v", availableDelta, fundingDelta, err)
	}
}
