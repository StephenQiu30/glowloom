package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func seedDispatchCall(t *testing.T, database *gorm.DB, dispatchRequired bool) (application.ProviderDispatchIdentity, *pgoperation.Store) {
	t.Helper()
	opID, store := seedProviderCallOperation(t, database)
	requestKey := "operation/" + opID.String()
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting,
		ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatalf("begin dispatch operation: %v", err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1, DispatchRequired: dispatchRequired,
	}); err != nil {
		t.Fatalf("begin dispatch call: %v", err)
	}
	var identity application.ProviderDispatchIdentity
	if err := database.Raw(`
		SELECT project_id, id AS operation_id, provider_request_key AS request_key,
		       model_profile_version_id, price_rule_version_id
		FROM operation.operation WHERE id = ?::uuid
	`, opID.String()).Scan(&identity).Error; err != nil {
		t.Fatalf("read dispatch identity: %v", err)
	}
	identity.Action, identity.Attempt = "submit", 1
	return identity, store
}

func dispatchReceipt(identity application.ProviderDispatchIdentity) application.ProviderReceipt {
	return application.ProviderReceipt{
		Version: 1, Identity: identity,
		ManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Outputs: []application.ProviderReceiptOutput{{
			Sequence: 1, SizeBytes: 123, MIMEType: "image/png",
			SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		}},
	}
}

func TestM1DispatchClaimHasOneConcurrentSender(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	const workers = 24
	results := make(chan application.ProviderDispatchResult, workers)
	failures := make(chan error, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			<-start
			result, err := store.ClaimProviderDispatch(t.Context(), identity)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		})
	}
	close(start)
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Errorf("concurrent claim: %v", err)
	}
	var claimed, returned int
	for result := range results {
		returned++
		if result.Claimed {
			claimed++
		}
		if result.DispatchStartedAt == nil || result.Receipt != nil {
			t.Errorf("unexpected dispatch evidence: %+v", result)
		}
	}
	if claimed != 1 || returned != workers {
		t.Fatalf("claim winners %d and replies %d; want 1 and %d", claimed, returned, workers)
	}
	if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
		t.Fatalf("dispatch changed budget: %+v, %v", state, err)
	}
}

func TestM1DispatchClaimRejectsMismatchesAndLegacyRows(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	for _, tc := range []struct {
		name   string
		change func(*application.ProviderDispatchIdentity)
	}{
		{"project", func(i *application.ProviderDispatchIdentity) { i.ProjectID = uuid.New() }},
		{"operation", func(i *application.ProviderDispatchIdentity) { i.OperationID = uuid.New() }},
		{"attempt", func(i *application.ProviderDispatchIdentity) { i.Attempt = 2 }},
		{"request key", func(i *application.ProviderDispatchIdentity) { i.RequestKey += "-other" }},
		{"model", func(i *application.ProviderDispatchIdentity) { i.ModelProfileVersionID = uuid.New() }},
		{"price", func(i *application.ProviderDispatchIdentity) { i.PriceRuleVersionID = uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := identity
			tc.change(&changed)
			if result, err := store.ClaimProviderDispatch(t.Context(), changed); err == nil || result.Claimed {
				t.Fatalf("mismatched identity claimed: %+v, %v", result, err)
			}
		})
	}
	legacy, legacyStore := seedDispatchCall(t, database, false)
	if result, err := legacyStore.ClaimProviderDispatch(t.Context(), legacy); !errors.Is(err, application.ErrProviderCallConflict) || result.Claimed {
		t.Fatalf("legacy NULL timestamp granted send: %+v, %v", result, err)
	}
	if err := legacyStore.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: legacy.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("legacy replay upgraded dispatch contract: %v", err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("new dispatch replay omitted required marker: %v", err)
	}
	if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
		t.Fatalf("dispatch changed budget: %+v, %v", state, err)
	}
	if state, err := readDispatchBudget(t.Context(), database, legacy.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
		t.Fatalf("dispatch changed budget: %+v, %v", state, err)
	}
}

func TestM1DispatchClaimDoesNotResendAndReplaysReceipt(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	first, err := store.ClaimProviderDispatch(t.Context(), identity)
	if err != nil || !first.Claimed {
		t.Fatalf("first claim: %+v, %v", first, err)
	}
	for range 2 {
		duplicate, err := store.ClaimProviderDispatch(t.Context(), identity)
		if err != nil || duplicate.Claimed || duplicate.DispatchStartedAt == nil || duplicate.Receipt != nil {
			t.Fatalf("uncertain request was resent: %+v, %v", duplicate, err)
		}
		if !duplicate.DispatchStartedAt.Equal(*first.DispatchStartedAt) {
			t.Fatal("duplicate dispatch replaced its timestamp")
		}
	}
	receipt := dispatchReceipt(identity)
	complete := application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
		Outcome: "ok", State: "completed", Receipt: &receipt, Usage: json.RawMessage(`{"output_count":1}`),
	}
	for range 2 {
		if err := store.CompleteProviderCall(t.Context(), complete); err != nil {
			t.Fatalf("complete receipt/replay: %v", err)
		}
	}
	replayed, err := store.ClaimProviderDispatch(t.Context(), identity)
	if err != nil || replayed.Claimed || replayed.Receipt == nil {
		t.Fatalf("durable receipt not recovered: %+v, %v", replayed, err)
	}
	if replayed.Receipt.ManifestSHA256 != receipt.ManifestSHA256 || replayed.Receipt.Identity != identity {
		t.Fatalf("recovered different receipt: %+v", replayed.Receipt)
	}
	var charge struct{ CostMicros *int64 }
	if err := database.Raw(`SELECT cost_micros FROM operation.provider_call WHERE operation_id = ?::uuid`, identity.OperationID.String()).Scan(&charge).Error; err != nil {
		t.Fatal(err)
	}
	if charge.CostMicros != nil {
		t.Fatalf("unimplemented synchronous pricing was treated as %d", *charge.CostMicros)
	}
	if _, err := store.LoadProviderCost(t.Context(), identity.OperationID); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("completed output released unknown charge: %v", err)
	}
	changed := receipt
	changed.ManifestSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	complete.Receipt = &changed
	if err := store.CompleteProviderCall(t.Context(), complete); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("different manifest replaced receipt: %v", err)
	}
	complete.Receipt = &receipt
	complete.Usage = json.RawMessage(`{"output_count":2}`)
	if err := store.CompleteProviderCall(t.Context(), complete); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("different usage replaced receipt: %v", err)
	}
	if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
		t.Fatalf("dispatch changed budget: %+v, %v", state, err)
	}
}

func TestM1DispatchClaimRequiresConclusivePriorNonSubmission(t *testing.T) {
	for _, state := range []string{"unknown", "accepted", "rejected", "completed", "not_submitted"} {
		t.Run(state, func(t *testing.T) {
			database := operationStoreDB(t)
			identity, store := seedDispatchCall(t, database, true)
			if _, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil {
				t.Fatal(err)
			}
			outcome := "ok"
			var taskID *string
			var receipt *application.ProviderReceipt
			if state == "unknown" {
				outcome = "unknown"
			}
			if state == "not_submitted" {
				outcome = "error"
			}
			if state == "accepted" {
				task := "provider-task"
				taskID = &task
			}
			if state == "completed" {
				r := dispatchReceipt(identity)
				receipt = &r
			}
			if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
				OperationID: identity.OperationID, Action: "submit", Attempt: 1,
				Outcome: outcome, State: state, ProviderTaskID: taskID, Receipt: receipt,
			}); err != nil {
				t.Fatal(err)
			}
			err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
				OperationID: identity.OperationID, Action: "submit", Attempt: 2, DispatchRequired: true,
			})
			if state == "not_submitted" {
				if err != nil {
					t.Fatalf("proven non-submission blocked a new attempt: %v", err)
				}
				identity.Attempt = 2
				if result, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil || !result.Claimed {
					t.Fatalf("new proven-safe attempt did not claim: %+v, %v", result, err)
				}
			} else if !errors.Is(err, application.ErrProviderCallConflict) {
				t.Fatalf("prior %s granted a new paid attempt: %v", state, err)
			}
			if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
				t.Fatalf("dispatch changed budget: %+v, %v", state, err)
			}
		})
	}
}

func TestM1DispatchClaimRejectsMissingDuplicateAndClosedCalls(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"missing call", `DELETE FROM operation.provider_call WHERE operation_id = ?::uuid`},
		{"duplicate call", `
			INSERT INTO operation.provider_call
			  (id, project_id, operation_id, attempt, action, provider_key,
			   request_summary, outcome, region, request_key,
			   model_profile_version_id, price_rule_version_id)
			SELECT gen_random_uuid(), project_id, operation_id, attempt, action, provider_key,
			       request_summary, outcome, region, request_key,
			       model_profile_version_id, price_rule_version_id
			FROM operation.provider_call WHERE operation_id = ?::uuid
		`},
		{"closed operation", `UPDATE operation.operation SET status = 'cancelled' WHERE id = ?::uuid`},
		{"missing frozen call model", `UPDATE operation.provider_call SET model_profile_version_id = NULL WHERE operation_id = ?::uuid`},
		{"missing frozen call price", `UPDATE operation.provider_call SET price_rule_version_id = NULL WHERE operation_id = ?::uuid`},
		{"missing frozen call key", `UPDATE operation.provider_call SET request_key = NULL WHERE operation_id = ?::uuid`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := operationStoreDB(t)
			identity, store := seedDispatchCall(t, database, true)
			if err := database.Exec(tc.sql, identity.OperationID.String()).Error; err != nil {
				t.Fatal(err)
			}
			if result, err := store.ClaimProviderDispatch(t.Context(), identity); !errors.Is(err, application.ErrProviderCallConflict) || result.Claimed {
				t.Fatalf("invalid durable call granted send: %+v, %v", result, err)
			}
			if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
				t.Fatalf("invalid claim changed budget: %+v, %v", state, err)
			}
		})
	}
}

func TestM1DispatchClaimRejectsSkippedAttempts(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	// Remove only this test's unsent row to test a first attempt above one.
	if err := database.Exec(`DELETE FROM operation.provider_call WHERE operation_id = ?::uuid`, identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 2, DispatchRequired: true,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("first attempt above one granted: %v", err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
		Outcome: "error", State: "not_submitted",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 3, DispatchRequired: true,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("skipped next attempt granted: %v", err)
	}
	if result, err := store.ClaimProviderDispatch(t.Context(), identity); !errors.Is(err, application.ErrProviderCallConflict) || result.Claimed {
		t.Fatalf("resolved unsent attempt later granted send: %+v, %v", result, err)
	}
}

func TestM1DispatchClaimUnderRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	identity, _ := seedDispatchCall(t, database, true)
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		store := pgoperation.NewStore(tx)
		if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
			OperationID: identity.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true,
		}); err != nil {
			return err
		}
		result, err := store.ClaimProviderDispatch(t.Context(), identity)
		if err != nil {
			return err
		}
		if !result.Claimed {
			return errors.New("runtime role did not acquire the first dispatch")
		}
		receipt := dispatchReceipt(identity)
		return store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
			OperationID: identity.OperationID, Action: "submit", Attempt: 1,
			Outcome: "ok", State: "completed", Receipt: &receipt,
		})
	}); err != nil {
		t.Fatalf("dispatch under runtime role: %v", err)
	}
}

func TestM1CompletedEvidenceBlocksManualNotExecuted(t *testing.T) {
	for _, cost := range []struct {
		name  string
		value any
	}{{"unknown cost", nil}, {"zero cost", int64(0)}} {
		t.Run(cost.name, func(t *testing.T) {
			database := operationStoreDB(t)
			opID, store := seedProviderCallOperation(t, database)
			requestKey := "operation/" + opID.String()
			for _, transition := range []application.TransitionInput{
				{OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey},
				{OperationID: opID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusUnknown},
				{OperationID: opID, From: []domain.Status{domain.StatusUnknown}, To: domain.StatusReconciling},
				{OperationID: opID, From: []domain.Status{domain.StatusReconciling}, To: domain.StatusManual},
			} {
				if _, err := store.TransitionWorkflowOperation(t.Context(), transition); err != nil {
					t.Fatalf("transition to manual: %v", err)
				}
			}
			if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
				OperationID: opID, Action: "submit", Attempt: 1,
			}); err != nil {
				t.Fatal(err)
			}
			// Model completed evidence retained from an earlier dispatch. Neither a
			// missing usage charge nor zero cost proves the provider did not execute.
			if err := database.Exec(`
				UPDATE operation.provider_call
				SET outcome = 'ok', response_summary = '{"state":"completed"}'::jsonb,
				    cost_micros = ?
				WHERE operation_id = ?::uuid
			`, cost.value, opID.String()).Error; err != nil {
				t.Fatal(err)
			}
			adminID := seedDispatchAdministrator(t, database, opID)
			if err := store.RecordManualNotExecuted(t.Context(), application.ManualNotExecutedInput{
				OperationID: opID, AdminID: adminID, Evidence: "request did not execute",
			}); !errors.Is(err, application.ErrProviderCallConflict) {
				t.Errorf("completed execution accepted as not executed: %v", err)
			}
			if err := database.Exec(`
				INSERT INTO operation.operation_event
				  (id, operation_id, from_status, to_status, reason, detail)
				VALUES (?::uuid, ?::uuid, 'manual', 'manual', 'manual_not_executed', '{}'::jsonb)
			`, uuid.NewString(), opID.String()).Error; err != nil {
				t.Fatal(err)
			}
			if err := store.CheckManualNotExecuted(t.Context(), opID); !errors.Is(err, application.ErrManualResolutionUnverified) {
				t.Errorf("historical no-execution decision trusted completed call: %v", err)
			}
			if _, err := store.LoadProviderCost(t.Context(), opID); !errors.Is(err, application.ErrManualResolutionUnverified) {
				t.Errorf("completed execution charge accepted for no-execution settlement: %v", err)
			}
			finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
			if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
				OperationID: opID.String(), From: domain.StatusManual, To: domain.StatusFailed,
				FailureCode: "provider_not_executed", ActualCostMicros: 0,
			}); !errors.Is(err, application.ErrManualResolutionUnverified) {
				t.Errorf("completed provider execution settled as not executed: %v", err)
			}
			if state, err := readDispatchBudget(t.Context(), database, opID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
				t.Fatalf("dispatch changed budget: %+v, %v", state, err)
			}
		})
	}
}

func TestM1CompletedEvidenceInvalidatesManualDecisionReplay(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	if _, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	for _, transition := range []application.TransitionInput{
		{OperationID: identity.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusUnknown},
		{OperationID: identity.OperationID, From: []domain.Status{domain.StatusUnknown}, To: domain.StatusReconciling},
		{OperationID: identity.OperationID, From: []domain.Status{domain.StatusReconciling}, To: domain.StatusManual},
	} {
		if _, err := store.TransitionWorkflowOperation(t.Context(), transition); err != nil {
			t.Fatal(err)
		}
	}
	decision := application.ManualNotExecutedInput{
		OperationID: identity.OperationID,
		AdminID:     seedDispatchAdministrator(t, database, identity.OperationID), Evidence: "provider audit found no execution",
	}
	if err := store.RecordManualNotExecuted(t.Context(), decision); err != nil {
		t.Fatalf("record initial decision: %v", err)
	}
	receipt := dispatchReceipt(identity)
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1, Outcome: "ok",
		State: "completed", Receipt: &receipt,
	}); err != nil {
		t.Fatalf("persist late completed receipt: %v", err)
	}
	if err := store.RecordManualNotExecuted(t.Context(), decision); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("old decision replay bypassed completed evidence: %v", err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: identity.OperationID.String(), From: domain.StatusManual, To: domain.StatusFailed,
		FailureCode: "provider_not_executed", ActualCostMicros: 0,
	}); !errors.Is(err, application.ErrManualResolutionUnverified) {
		t.Fatalf("old decision settled late completed output: %v", err)
	}
	if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
		t.Fatalf("late receipt changed budget: %+v, %v", state, err)
	}
}

func TestM1CompletedReceiptRejectsTerminalMutation(t *testing.T) {
	for _, status := range []string{"completed", "failed", "cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			database := operationStoreDB(t)
			identity, store := seedDispatchCall(t, database, true)
			if _, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil {
				t.Fatal(err)
			}
			if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
				OperationID: identity.OperationID, Action: "submit", Attempt: 1,
				Outcome: "unknown", State: "unknown",
			}); err != nil {
				t.Fatal(err)
			}
			// Isolate a terminal operation race without asserting that this
			// synthetic state is a valid product settlement.
			if err := database.Exec(`UPDATE operation.operation SET status = ? WHERE id = ?::uuid`, status, identity.OperationID.String()).Error; err != nil {
				t.Fatal(err)
			}
			var before string
			if err := database.Raw(`SELECT row_to_json(c)::text FROM operation.provider_call AS c WHERE operation_id = ?::uuid`, identity.OperationID.String()).Scan(&before).Error; err != nil {
				t.Fatal(err)
			}
			receipt := dispatchReceipt(identity)
			if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
				OperationID: identity.OperationID, Action: "submit", Attempt: 1,
				Outcome: "ok", State: "completed", Receipt: &receipt,
			}); !errors.Is(err, application.ErrProviderCallConflict) {
				t.Errorf("terminal operation accepted new receipt: %v", err)
			}
			var after string
			if err := database.Raw(`SELECT row_to_json(c)::text FROM operation.provider_call AS c WHERE operation_id = ?::uuid`, identity.OperationID.String()).Scan(&after).Error; err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Error("terminal completion modified durable call evidence")
			}
			if state, err := readDispatchBudget(t.Context(), database, identity.OperationID); err != nil || state != (dispatchBudgetSnapshot{Status: "held", ReservedMicros: 10}) {
				t.Fatalf("terminal completion changed budget: %+v, %v", state, err)
			}
		})
	}
}

func TestM1CompletedReceiptTerminalReplayIsReadOnly(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	if _, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	receipt := dispatchReceipt(identity)
	completed := application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
		Outcome: "ok", State: "completed", Receipt: &receipt,
	}
	if err := store.CompleteProviderCall(t.Context(), completed); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE operation.operation SET status = 'completed' WHERE id = ?::uuid`, identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	var before string
	if err := database.Raw(`SELECT row_to_json(c)::text FROM operation.provider_call AS c WHERE operation_id = ?::uuid`, identity.OperationID.String()).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteProviderCall(t.Context(), completed); err != nil {
		t.Fatalf("identical terminal receipt replay: %v", err)
	}
	if result, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil || result.Claimed || result.Receipt == nil {
		t.Fatalf("terminal receipt recovery resent request: %+v, %v", result, err)
	}
	var after string
	if err := database.Raw(`SELECT row_to_json(c)::text FROM operation.provider_call AS c WHERE operation_id = ?::uuid`, identity.OperationID.String()).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("terminal replay modified durable call evidence")
	}
}

func TestM1DispatchMigrationUpDownUpRetainsEvidence(t *testing.T) {
	dsn := os.Getenv("LV_TEST_PROVIDER_DISPATCH_MIGRATION_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_PROVIDER_DISPATCH_MIGRATION_DB_DSN to a disposable database migrated through 202609300011")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open provider dispatch migration database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	database := conn.DB.WithContext(ctx)
	var before struct {
		Name         string
		Calls        int64
		NewFields    int64
		BusinessRows bool
	}
	if err := database.Raw(`
		SELECT current_database() AS name,
		       (SELECT count(*) FROM operation.provider_call) AS calls,
		       (SELECT count(*) FROM information_schema.columns
		        WHERE table_schema = 'operation' AND table_name = 'provider_call'
		          AND column_name IN ('dispatch_started_at', 'receipt')) AS new_fields,
		       (EXISTS (SELECT 1 FROM workspace.project)
		        OR EXISTS (SELECT 1 FROM operation.operation)
		        OR EXISTS (SELECT 1 FROM catalog.provider)
		        OR EXISTS (SELECT 1 FROM identity."user")) AS business_rows
	`).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before.Name == "postgres" || before.Name == "template0" || before.Name == "template1" || before.Calls != 0 || before.NewFields != 0 || before.BusinessRows {
		t.Fatal("migration test requires an unused disposable operation database without dispatch fields")
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "202610010010_provider_dispatch_receipt.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "202610010010_provider_dispatch_receipt.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := seedDispatchCall(t, database, false)
	if err := database.Exec(string(up)).Error; err != nil {
		t.Fatalf("migration up: %v", err)
	}
	var legacyState struct {
		DispatchStartedAt *time.Time
		Receipt           []byte
		Outcome           string
	}
	if err := database.Raw(`
		SELECT dispatch_started_at, receipt, outcome FROM operation.provider_call WHERE operation_id = ?::uuid
	`, legacy.OperationID.String()).Scan(&legacyState).Error; err != nil {
		t.Fatal(err)
	}
	if legacyState.DispatchStartedAt != nil || len(legacyState.Receipt) != 0 || legacyState.Outcome != "unknown" {
		t.Fatalf("migration invented legacy execution evidence: %+v", legacyState)
	}
	if err := database.Exec(string(down)).Error; err != nil {
		t.Fatalf("unused migration down: %v", err)
	}
	if err := database.Exec(string(up)).Error; err != nil {
		t.Fatalf("migration reapply up: %v", err)
	}
	identity, store := seedDispatchCall(t, database, true)
	claimed, err := store.ClaimProviderDispatch(t.Context(), identity)
	if err != nil || !claimed.Claimed {
		t.Fatalf("claim after migration: %+v, %v", claimed, err)
	}
	if err := database.Exec(string(down)).Error; err == nil {
		t.Fatal("migration down erased a sent request")
	}
	receipt := dispatchReceipt(identity)
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
		Outcome: "ok", State: "completed", Receipt: &receipt,
	}); err != nil {
		t.Fatalf("persist receipt after refused rollback: %v", err)
	}
	if err := database.Exec(string(down)).Error; err == nil {
		t.Fatal("migration down erased a completed receipt")
	}
	replayed, err := store.ClaimProviderDispatch(t.Context(), identity)
	if err != nil || replayed.Claimed || replayed.Receipt == nil || replayed.DispatchStartedAt == nil ||
		!replayed.DispatchStartedAt.Equal(*claimed.DispatchStartedAt) {
		t.Fatalf("refused rollback changed dispatch evidence: %+v, %v", replayed, err)
	}
}

func TestM1DispatchReceiptDatabaseConstraints(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	if _, err := store.ClaimProviderDispatch(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	receipt := dispatchReceipt(identity)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unknown field", func(r map[string]any) { r["body"] = "untrusted" }},
		{"missing identity", func(r map[string]any) { delete(r, "identity") }},
		{"missing output", func(r map[string]any) { delete(r, "outputs") }},
		{"null identity", func(r map[string]any) { r["identity"] = nil }},
		{"wrong project", func(r map[string]any) { r["identity"].(map[string]any)["project_id"] = uuid.NewString() }},
		{"unknown identity field", func(r map[string]any) { r["identity"].(map[string]any)["secret"] = "untrusted" }},
		{"wrong version", func(r map[string]any) { r["version"] = 2 }},
		{"string version", func(r map[string]any) { r["version"] = "1" }},
		{"empty digest", func(r map[string]any) { r["manifest_sha256"] = "" }},
		{"zero output size", func(r map[string]any) { r["outputs"].([]any)[0].(map[string]any)["size_bytes"] = 0 }},
		{"oversized output", func(r map[string]any) { r["outputs"].([]any)[0].(map[string]any)["size_bytes"] = 33554433 }},
		{"fractional output size", func(r map[string]any) { r["outputs"].([]any)[0].(map[string]any)["size_bytes"] = 1.5 }},
		{"invalid MIME", func(r map[string]any) { r["outputs"].([]any)[0].(map[string]any)["mime_type"] = "text/plain" }},
		{"unknown output field", func(r map[string]any) { r["outputs"].([]any)[0].(map[string]any)["url"] = "untrusted" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var changed map[string]any
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			tc.change(changed)
			invalid, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			err = database.Exec(`
				UPDATE operation.provider_call SET receipt = ?::jsonb, outcome = 'ok',
				  response_summary = '{"state":"completed"}'::jsonb
				WHERE operation_id = ?::uuid
			`, string(invalid), identity.OperationID.String()).Error
			var postgresError *pgconn.PgError
			if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
				t.Fatalf("invalid receipt accepted; PostgreSQL constraint error = %v", err)
			}
		})
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: identity.OperationID, Action: "submit", Attempt: 1,
		Outcome: "ok", State: "completed", Receipt: &receipt,
	}); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
}

func seedDispatchAdministrator(t *testing.T, database *gorm.DB, operationID uuid.UUID) uuid.UUID {
	t.Helper()
	adminID := uuid.New()
	if err := database.Exec(`
		INSERT INTO identity."user"
		  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		SELECT ?::uuid, p.org_id, ?, 'Dispatch Reviewer', 'admin', 'test-hash', false
		FROM operation.operation AS o
		JOIN workspace.project AS p ON p.id = o.project_id WHERE o.id = ?::uuid
	`, adminID.String(), "dispatch-admin-"+adminID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("seed dispatch administrator: %v", err)
	}
	return adminID
}

type dispatchBudgetSnapshot struct {
	Status         string
	ReservedMicros int64
	Settlements    int64
}

func readDispatchBudget(ctx context.Context, database *gorm.DB, operationID uuid.UUID) (dispatchBudgetSnapshot, error) {
	var state dispatchBudgetSnapshot
	err := database.WithContext(ctx).Raw(`
		SELECT r.status, b.reserved_micros,
		       (SELECT count(*) FROM billing.ledger_entry AS e
		        WHERE e.operation_id = o.id AND e.entry_type = 'settle') AS settlements
		FROM operation.operation AS o
		JOIN billing.reservation AS r ON r.id = o.reservation_id
		JOIN billing.budget AS b ON b.project_id = o.project_id
		WHERE o.id = ?::uuid
	`, operationID.String()).Scan(&state).Error
	return state, err
}
