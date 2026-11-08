package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

func TestCreditCommandsGrantDebitFreezeCommitAndRelease(t *testing.T) {
	ctx, database := billingDB(t)
	actor, _ := billingProject(t, database)
	var walletScope uuid.UUID
	if err := database.Raw(`SELECT billing.ensure_wallet_account(?::uuid)`, actor.ScopeID).Row().Scan(&walletScope); err != nil || walletScope != actor.ScopeID {
		t.Fatalf("ensure wallet account = %s, error = %v", walletScope, err)
	}
	store := pgbilling.NewStore(database)
	callerID := "credit-test-" + uuid.NewString()
	grant := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditGrant, uuid.New(), uuid.New(), 10*billingdomain.MicrosPerCredit)
	grant.ExpiresAt = timePointer(time.Now().UTC().Add(30 * 24 * time.Hour))
	accepted, err := store.SubmitCreditCommand(ctx, grant)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accept grant = (%+v, %v)", accepted, err)
	}
	replayed, err := store.SubmitCreditCommand(ctx, grant)
	if err != nil || replayed.ID != accepted.ID {
		t.Fatalf("replay grant = (%+v, %v), want command %s", replayed, err, accepted.ID)
	}
	changed := grant
	changed.AmountMicros++
	if _, err := store.SubmitCreditCommand(ctx, changed); !errors.Is(err, billingapp.ErrCreditIdempotencyConflict) {
		t.Fatalf("reuse idempotency key with changed request = %v", err)
	}
	processCreditCommand(t, ctx, store, accepted.ID)
	processCreditCommand(t, ctx, store, accepted.ID)
	assertCreditCommandStatus(t, ctx, store, callerID, actor.ScopeID, accepted.ID, "succeeded")

	debit := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditDebit, uuid.New(), uuid.New(), 2*billingdomain.MicrosPerCredit)
	debitReceipt := acceptAndProcessCreditCommand(t, ctx, store, debit)
	assertCreditCommandStatus(t, ctx, store, callerID, actor.ScopeID, debitReceipt.ID, "succeeded")

	freeze := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditFreeze, uuid.New(), uuid.New(), 4*billingdomain.MicrosPerCredit)
	freezeReceipt := acceptAndProcessCreditCommand(t, ctx, store, freeze)
	var freezeResult struct {
		HoldID uuid.UUID `json:"hold_id"`
	}
	if err := json.Unmarshal(freezeReceipt.Result, &freezeResult); err != nil || freezeResult.HoldID == uuid.Nil {
		t.Fatalf("decode freeze result %s: %v", freezeReceipt.Result, err)
	}
	view, _, err := store.ReadWalletWithBalance(ctx, actor.ScopeID, true, nil, nil)
	if err != nil || view.GeneralCredits != "4" || view.HeldCredits != "4" {
		t.Fatalf("balance after freeze = available %q held %q: %v", view.GeneralCredits, view.HeldCredits, err)
	}

	commit := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditCommit, freezeResult.HoldID, uuid.New(), 3*billingdomain.MicrosPerCredit)
	commit.ReferenceID = &freezeResult.HoldID
	commitReceipt := acceptAndProcessCreditCommand(t, ctx, store, commit)
	assertCreditCommandStatus(t, ctx, store, callerID, actor.ScopeID, commitReceipt.ID, "succeeded")

	secondFreeze := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditFreeze, uuid.New(), uuid.New(), 2*billingdomain.MicrosPerCredit)
	secondFreezeReceipt := acceptAndProcessCreditCommand(t, ctx, store, secondFreeze)
	var secondFreezeResult struct {
		HoldID uuid.UUID `json:"hold_id"`
	}
	if err := json.Unmarshal(secondFreezeReceipt.Result, &secondFreezeResult); err != nil || secondFreezeResult.HoldID == uuid.Nil {
		t.Fatalf("decode second freeze result %s: %v", secondFreezeReceipt.Result, err)
	}
	release := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditRelease, secondFreezeResult.HoldID, uuid.New(), 2*billingdomain.MicrosPerCredit)
	release.ReferenceID = &secondFreezeResult.HoldID
	releaseReceipt := acceptAndProcessCreditCommand(t, ctx, store, release)
	assertCreditCommandStatus(t, ctx, store, callerID, actor.ScopeID, releaseReceipt.ID, "succeeded")

	view, _, err = store.ReadWalletWithBalance(ctx, actor.ScopeID, true, nil, nil)
	if err != nil || view.GeneralCredits != "5" || view.HeldCredits != "0" || view.RedeemedToday != "5" {
		t.Fatalf("final balance = available %q held %q redeemed %q: %v", view.GeneralCredits, view.HeldCredits, view.RedeemedToday, err)
	}
	var entries int64
	if err := database.Raw(`SELECT count(*) FROM billing.wallet_entry WHERE scope_id=?::uuid`, actor.ScopeID).Scan(&entries).Error; err != nil || entries != 7 {
		t.Fatalf("immutable credit movements = %d, error = %v; want 7", entries, err)
	}
	var projection struct {
		Available int64
		Held      int64
		Redeemed  int64
		Expired   int64
		Funding   int64
	}
	if err := database.Raw(`SELECT sum(available_delta_micros) AS available,
		sum(held_delta_micros) AS held,sum(redeemed_delta_micros) AS redeemed,
		sum(expired_delta_micros) AS expired,sum(funding_delta_micros) AS funding
		FROM billing.wallet_entry WHERE scope_id=?::uuid`, actor.ScopeID).Scan(&projection).Error; err != nil {
		t.Fatalf("rebuild wallet buckets from journal: %v", err)
	}
	if projection.Available != 500000 || projection.Held != 0 || projection.Redeemed != 500000 || projection.Expired != 0 || projection.Funding != -1000000 {
		t.Fatalf("journal projection = %+v", projection)
	}
}

func TestConcurrentCreditCommandsCannotOverdrawOneAccount(t *testing.T) {
	ctx, database := billingDB(t)
	actor, _ := billingProject(t, database)
	var walletScope uuid.UUID
	if err := database.Raw(`SELECT billing.ensure_wallet_account(?::uuid)`, actor.ScopeID).Row().Scan(&walletScope); err != nil || walletScope != actor.ScopeID {
		t.Fatalf("ensure wallet account = %s, error = %v", walletScope, err)
	}
	store := pgbilling.NewStore(database)
	callerID := "credit-race-" + uuid.NewString()
	grant := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditGrant, uuid.New(), uuid.New(), 10*billingdomain.MicrosPerCredit)
	grant.ExpiresAt = timePointer(time.Now().UTC().Add(30 * 24 * time.Hour))
	grantReceipt := acceptAndProcessCreditCommand(t, ctx, store, grant)
	assertCreditCommandStatus(t, ctx, store, callerID, actor.ScopeID, grantReceipt.ID, "succeeded")

	commands := []billingapp.CreditCommand{
		testCreditCommand(callerID, actor.ScopeID, billingapp.CreditDebit, uuid.New(), uuid.New(), 8*billingdomain.MicrosPerCredit),
		testCreditCommand(callerID, actor.ScopeID, billingapp.CreditDebit, uuid.New(), uuid.New(), 8*billingdomain.MicrosPerCredit),
	}
	receipts := make([]billingapp.CreditCommandReceipt, len(commands))
	for index, command := range commands {
		receipt, err := store.SubmitCreditCommand(ctx, command)
		if err != nil {
			t.Fatalf("accept concurrent debit %d: %v", index, err)
		}
		receipts[index] = receipt
	}
	start := make(chan struct{})
	errorsByCommand := make([]error, len(receipts))
	var wait sync.WaitGroup
	for index, receipt := range receipts {
		wait.Add(1)
		go func(index int, commandID uuid.UUID) {
			defer wait.Done()
			<-start
			errorsByCommand[index] = store.ProcessCreditCommand(ctx, commandID)
		}(index, receipt.ID)
	}
	close(start)
	wait.Wait()
	for index, err := range errorsByCommand {
		if errors.Is(err, billingapp.ErrCreditVersionConflict) {
			err = store.ProcessCreditCommand(ctx, receipts[index].ID)
		}
		if err != nil {
			t.Fatalf("process debit %d: %v", index, err)
		}
	}
	succeeded, insufficient := 0, 0
	for _, receipt := range receipts {
		var current billingapp.CreditCommandReceipt
		current, err := store.ReadCreditCommand(ctx, callerID, actor.ScopeID, receipt.ID)
		if err != nil {
			t.Fatalf("read debit result: %v", err)
		}
		switch current.Status {
		case "succeeded":
			succeeded++
		case "insufficient":
			insufficient++
		default:
			t.Fatalf("debit status = %q", current.Status)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("concurrent results = %d succeeded, %d insufficient", succeeded, insufficient)
	}
	view, _, err := store.ReadWalletWithBalance(ctx, actor.ScopeID, false, nil, nil)
	if err != nil || view.GeneralCredits != "2" || view.HeldCredits != "0" {
		t.Fatalf("balance after concurrent debits = available %q held %q: %v", view.GeneralCredits, view.HeldCredits, err)
	}
}

func acceptAndProcessCreditCommand(t *testing.T, ctx context.Context, store *pgbilling.Store, command billingapp.CreditCommand) billingapp.CreditCommandReceipt {
	t.Helper()
	receipt, err := store.SubmitCreditCommand(ctx, command)
	if err != nil {
		t.Fatalf("accept %s command: %v", command.Action, err)
	}
	processCreditCommand(t, ctx, store, receipt.ID)
	updated, err := store.ReadCreditCommand(ctx, command.CallerID, command.ScopeID, receipt.ID)
	if err != nil {
		t.Fatalf("read %s command: %v", command.Action, err)
	}
	return updated
}

func processCreditCommand(t *testing.T, ctx context.Context, store *pgbilling.Store, commandID uuid.UUID) {
	t.Helper()
	if err := store.ProcessCreditCommand(ctx, commandID); err != nil {
		t.Fatalf("process command %s: %v", commandID, err)
	}
}

func assertCreditCommandStatus(t *testing.T, ctx context.Context, store *pgbilling.Store, callerID string, scopeID, commandID uuid.UUID, status string) {
	t.Helper()
	current, err := store.ReadCreditCommand(ctx, callerID, scopeID, commandID)
	if err != nil || current.Status != status {
		t.Fatalf("command %s status = %q, error = %v; want %q", commandID, current.Status, err, status)
	}
}

func testCreditCommand(callerID string, scopeID uuid.UUID, action billingapp.CreditAction, sourceID, idempotencySeed uuid.UUID, amount int64) billingapp.CreditCommand {
	return billingapp.CreditCommand{
		CallerID: callerID, IdempotencyKey: "credit-key-" + idempotencySeed.String(), ScopeID: scopeID,
		Action: action, SourceID: sourceID, AmountMicros: amount, Reason: "integration test",
	}
}

func timePointer(value time.Time) *time.Time { return &value }
