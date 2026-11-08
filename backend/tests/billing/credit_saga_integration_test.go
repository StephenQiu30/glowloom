package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingevent "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/event"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

func TestCreditSagaUnknownOutcomeHoldsThenFailureAutomaticallyReleases(t *testing.T) {
	ctx, database, scopeID, store, holdID := newCreditSagaFixture(t)
	unknown := billingapp.CreditSettlementOutcome{
		EventID: uuid.New(), ScopeID: scopeID, HoldID: holdID, Outcome: "unknown", OccurredAt: time.Now().UTC(),
	}
	if err := sendCreditOutcomeAsApp(t, ctx, database, unknown); err != nil {
		t.Fatalf("apply unknown settlement outcome: %v", err)
	}
	var saga struct {
		Status string
	}
	if err := database.Raw(`SELECT status FROM billing.credit_saga WHERE hold_id=?::uuid`, holdID).Scan(&saga).Error; err != nil || saga.Status != "unknown" {
		t.Fatalf("unknown saga state = %q, error = %v", saga.Status, err)
	}
	var queued int64
	if err := database.Raw(`SELECT count(*) FROM billing.credit_command WHERE caller_id='billing-credit-saga' AND reference_id=?::uuid`, holdID).Scan(&queued).Error; err != nil || queued != 0 {
		t.Fatalf("unknown outcome queued %d settlement commands, error = %v", queued, err)
	}

	failure := billingapp.CreditSettlementOutcome{
		EventID: uuid.New(), ScopeID: scopeID, HoldID: holdID, Outcome: "failed", OccurredAt: time.Now().UTC(),
	}
	if err := sendCreditOutcomeAsApp(t, ctx, database, failure); err != nil {
		t.Fatalf("apply failure settlement outcome: %v", err)
	}
	command := readCreditSagaCommand(t, database, holdID)
	if command.Action != string(billingapp.CreditRelease) || command.AmountMicros != 5*billingdomain.MicrosPerCredit || command.Status != "accepted" {
		t.Fatalf("automatic compensation = %+v", command)
	}
	processCreditCommandAsApp(t, ctx, database, command.ID)
	consumeCreditCommandResultAsApp(t, ctx, database, command.ID)
	if err := sendCreditOutcomeAsApp(t, ctx, database, failure); err != nil {
		t.Fatalf("replay failure outcome: %v", err)
	}
	if err := sendCreditOutcomeAsApp(t, ctx, database, billingapp.CreditSettlementOutcome{
		EventID: uuid.New(), ScopeID: scopeID, HoldID: holdID, Outcome: "succeeded",
		AmountMicros: 3 * billingdomain.MicrosPerCredit, OccurredAt: time.Now().UTC(),
	}); !errors.Is(err, billingapp.ErrCreditOutcomeConflict) {
		t.Fatalf("conflicting terminal outcome = %v", err)
	}
	var status string
	if err := database.Raw(`SELECT status FROM billing.credit_saga WHERE hold_id=?::uuid`, holdID).Scan(&status).Error; err != nil || status != "released" {
		t.Fatalf("final saga status = %q, error = %v", status, err)
	}
	var holdStatus string
	if err := database.Raw(`SELECT status FROM billing.credit_hold WHERE id=?::uuid`, holdID).Scan(&holdStatus).Error; err != nil || holdStatus != "released" {
		t.Fatalf("compensated hold status = %q, error = %v", holdStatus, err)
	}
	view, _, err := store.ReadWalletWithBalance(ctx, scopeID, false, nil, nil)
	if err != nil || view.GeneralCredits != "8" || view.HeldCredits != "0" || view.RedeemedToday != "0" {
		t.Fatalf("balance after automatic release = available %q held %q redeemed %q: %v", view.GeneralCredits, view.HeldCredits, view.RedeemedToday, err)
	}
}

func TestCreditSagaSuccessCommitsActualChargeAndReleasesRemainder(t *testing.T) {
	ctx, database, scopeID, store, holdID := newCreditSagaFixture(t)
	outcome := billingapp.CreditSettlementOutcome{
		EventID: uuid.New(), ScopeID: scopeID, HoldID: holdID, Outcome: "succeeded",
		AmountMicros: 3 * billingdomain.MicrosPerCredit, OccurredAt: time.Now().UTC(),
	}
	if err := sendCreditOutcomeAsApp(t, ctx, database, outcome); err != nil {
		t.Fatalf("apply success settlement outcome: %v", err)
	}
	command := readCreditSagaCommand(t, database, holdID)
	if command.Action != string(billingapp.CreditCommit) || command.AmountMicros != 3*billingdomain.MicrosPerCredit {
		t.Fatalf("automatic commit = %+v", command)
	}
	processCreditCommandAsApp(t, ctx, database, command.ID)
	consumeCreditCommandResultAsApp(t, ctx, database, command.ID)
	var status string
	if err := database.Raw(`SELECT status FROM billing.credit_saga WHERE hold_id=?::uuid`, holdID).Scan(&status).Error; err != nil || status != "committed" {
		t.Fatalf("final saga status = %q, error = %v", status, err)
	}
	view, _, err := store.ReadWalletWithBalance(ctx, scopeID, false, nil, nil)
	if err != nil || view.GeneralCredits != "5" || view.HeldCredits != "0" || view.RedeemedToday != "3" {
		t.Fatalf("balance after commit = available %q held %q redeemed %q: %v", view.GeneralCredits, view.HeldCredits, view.RedeemedToday, err)
	}
}

type creditSagaCommand struct {
	ID           uuid.UUID
	Action       string
	AmountMicros int64
	Status       string
}

func newCreditSagaFixture(t *testing.T) (context.Context, *gorm.DB, uuid.UUID, *pgbilling.Store, uuid.UUID) {
	t.Helper()
	ctx, database := billingDB(t)
	actor, _ := billingProject(t, database)
	var walletScope uuid.UUID
	if err := database.Raw(`SELECT billing.ensure_wallet_account(?::uuid)`, actor.ScopeID).Row().Scan(&walletScope); err != nil || walletScope != actor.ScopeID {
		t.Fatalf("ensure wallet account = %s, error = %v", walletScope, err)
	}
	store := pgbilling.NewStore(database)
	callerID := "credit-saga-test-" + uuid.NewString()
	grant := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditGrant, uuid.New(), uuid.New(), 8*billingdomain.MicrosPerCredit)
	grant.ExpiresAt = timePointer(time.Now().UTC().Add(30 * 24 * time.Hour))
	acceptAndProcessCreditCommand(t, ctx, store, grant)
	freeze := testCreditCommand(callerID, actor.ScopeID, billingapp.CreditFreeze, uuid.New(), uuid.New(), 5*billingdomain.MicrosPerCredit)
	freezeReceipt := acceptAndProcessCreditCommand(t, ctx, store, freeze)
	var freezeResult struct {
		HoldID uuid.UUID `json:"hold_id"`
	}
	if err := json.Unmarshal(freezeReceipt.Result, &freezeResult); err != nil || freezeResult.HoldID == uuid.Nil {
		t.Fatalf("decode freeze result %s: %v", freezeReceipt.Result, err)
	}
	return ctx, database, actor.ScopeID, store, freezeResult.HoldID
}

func sendCreditOutcome(ctx context.Context, handler *billingevent.CreditEventHandler, outcome billingapp.CreditSettlementOutcome) error {
	payload, err := json.Marshal(struct {
		EventType string `json:"event_type"`
		billingapp.CreditSettlementOutcome
	}{EventType: billingevent.CreditSettlementOutcomeTopic, CreditSettlementOutcome: outcome})
	if err != nil {
		return err
	}
	return handler.Handle(ctx, inboxapp.Record{Topic: billingevent.CreditSettlementOutcomeTopic, Key: []byte(outcome.ScopeID.String()), Value: payload})
}

func sendCreditOutcomeAsApp(t *testing.T, ctx context.Context, database *gorm.DB, outcome billingapp.CreditSettlementOutcome) error {
	t.Helper()
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		store := pgbilling.NewStore(tx)
		return sendCreditOutcome(ctx, billingevent.NewCreditEventHandler(store, nil, store), outcome)
	})
}

func processCreditCommandAsApp(t *testing.T, ctx context.Context, database *gorm.DB, commandID uuid.UUID) {
	t.Helper()
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		return pgbilling.NewStore(tx).ProcessCreditCommand(ctx, commandID)
	}); err != nil {
		t.Fatalf("process credit command as app role: %v", err)
	}
}

func readCreditSagaCommand(t *testing.T, database *gorm.DB, holdID uuid.UUID) creditSagaCommand {
	t.Helper()
	var command creditSagaCommand
	if err := database.Raw(`SELECT id,action,amount_micros,status FROM billing.credit_command
		WHERE caller_id='billing-credit-saga' AND reference_id=?::uuid`, holdID).Scan(&command).Error; err != nil {
		t.Fatalf("read automatic credit saga command: %v", err)
	}
	if command.ID == uuid.Nil {
		t.Fatal("automatic credit saga command was not created")
	}
	return command
}

func consumeCreditCommandResultAsApp(t *testing.T, ctx context.Context, database *gorm.DB, commandID uuid.UUID) {
	t.Helper()
	var record struct {
		ScopeID uuid.UUID
		Payload []byte
	}
	if err := database.Raw(`SELECT partition_key AS scope_id,payload FROM infra.outbox
		WHERE topic=? AND payload->>'command_id'=? ORDER BY id DESC LIMIT 1`, billingevent.CreditCommandResultTopic, commandID.String()).Scan(&record).Error; err != nil {
		t.Fatalf("read credit command result outbox: %v", err)
	}
	if record.ScopeID == uuid.Nil || len(record.Payload) == 0 {
		t.Fatal("credit command result outbox event was not found")
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		store := pgbilling.NewStore(tx)
		handler := billingevent.NewCreditEventHandler(store, nil, store)
		return handler.Handle(ctx, inboxapp.Record{Topic: billingevent.CreditCommandResultTopic, Key: []byte(record.ScopeID.String()), Value: record.Payload})
	}); err != nil {
		t.Fatalf("consume credit command result as app role: %v", err)
	}
}
