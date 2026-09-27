package operation_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func seedProviderCallOperation(t *testing.T, database *gorm.DB) (uuid.UUID, *pgoperation.Store) {
	t.Helper()
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, _ := operationStoreRows(t, database, actor, projectID)
	var frozen struct {
		ModelProfileVersionID uuid.UUID
		PriceRuleVersionID    uuid.UUID
	}
	if err := database.Raw(`
		SELECT model_profile_version_id, price_rule_version_id
		FROM operation.operation WHERE id = ?::uuid
	`, operationID.String()).Scan(&frozen).Error; err != nil {
		t.Fatalf("read frozen model: %v", err)
	}
	providerID, profileID := uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.provider (id, key, name, adapter_key, region)
		VALUES (?::uuid, ?, 'Mock', 'mock', 'domestic')
	`, providerID.String(), "mock-"+providerID.String()).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, 'image.generate', 'image', ARRAY['text_to_image'], ARRAY['prompt'])
		ON CONFLICT (key) DO NOTHING
	`, uuid.NewString()).Error; err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile
		  (id, model_key, provider_id, capability, display_name, status)
		VALUES (?::uuid, ?, ?::uuid, 'image.generate', 'Mock image', 'active')
	`, profileID.String(), "mock-"+profileID.String(), providerID.String()).Error; err != nil {
		t.Fatalf("seed model: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits,
		   param_schema, supports_query, supports_cancel, supports_callback,
		   expected_max_ms, moderation, queue)
		VALUES (?::uuid, ?::uuid, 1, 'mock-v1', ARRAY['text_to_image'], '{}'::jsonb,
		        '[]'::jsonb, true, true, false, 60000, 'platform', 'agent.mock')
	`, frozen.ModelProfileVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("seed model version: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 1, 'per_image', '{"base_micros":100}'::jsonb,
		        now() - interval '1 minute')
	`, frozen.PriceRuleVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("seed price version: %v", err)
	}
	if err := database.Exec(`UPDATE billing.budget SET reserved_micros = 10 WHERE project_id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("seed held budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, operation_id)
		VALUES (?::uuid, ?::uuid, 'reserve', 10, ?::uuid)
	`, uuid.NewString(), projectID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("seed reserve ledger: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET batch_id = NULL WHERE id = ?::uuid`, operationID.String()).Error; err != nil {
		t.Fatalf("clear batch for single operation: %v", err)
	}
	return operationID, pgoperation.NewStore(database)
}

func TestMockProviderCallEvidenceSettlesOnce(t *testing.T) {
	database := operationStoreDB(t)
	opID, store := seedProviderCallOperation(t, database)
	requestKey, taskID := "operation/"+opID.String(), "mock-task-1"
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting,
		ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatalf("begin submission state: %v", err)
	}
	begin := application.BeginProviderCallInput{OperationID: opID, Action: "submit", Attempt: 1}
	for range 2 {
		if err := store.BeginProviderCall(t.Context(), begin); err != nil {
			t.Fatalf("begin submit replay: %v", err)
		}
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id = ?::uuid`, opID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("begin wrote %d rows: %v", count, err)
	}
	if _, err := store.LoadProviderCost(t.Context(), opID); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("unknown submit had conclusive cost: %v", err)
	}
	accepted := application.CompleteProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1, Outcome: "ok", State: "accepted",
		ProviderTaskID: &taskID,
	}
	for attempt := range 2 {
		if err := store.CompleteProviderCall(t.Context(), accepted); err != nil {
			t.Fatalf("complete submit replay %d: %v", attempt, err)
		}
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: opID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted,
		ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatalf("record accepted state: %v", err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: opID, Action: "query", Attempt: 1, ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatalf("begin query: %v", err)
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: opID, Action: "query", Attempt: 1, Outcome: "ok", State: "succeeded",
		ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatalf("complete query: %v", err)
	}
	cost, err := store.LoadProviderCost(t.Context(), opID)
	if err != nil || cost.ActualCostMicros != 0 || !cost.HasSubmission {
		t.Fatalf("established mock charge = %+v: %v", cost, err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusSubmitted, To: domain.StatusFailed,
		ActualCostMicros: 1, FailureCode: "test_finalization", Retryable: false,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("unverified nonzero charge was accepted: %v", err)
	}
	for range 2 {
		if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
			OperationID: opID.String(), From: domain.StatusSubmitted, To: domain.StatusFailed,
			ActualCostMicros: 0, FailureCode: "test_finalization", Retryable: false,
		}); err != nil {
			t.Fatalf("settle mock evidence replay: %v", err)
		}
	}
	var settles int64
	if err := database.Raw(`
		SELECT count(*) FROM billing.ledger_entry
		WHERE operation_id = ?::uuid AND entry_type = 'settle'
	`, opID.String()).Scan(&settles).Error; err != nil || settles != 1 {
		t.Fatalf("settlement count %d: %v", settles, err)
	}
	// The call partition has a shorter retention period than the ledger. A
	// terminal replay must verify the immutable billing fact without reopening
	// already established provider evidence.
	if err := database.Exec(`DELETE FROM operation.provider_call WHERE operation_id = ?::uuid`, opID.String()).Error; err != nil {
		t.Fatalf("simulate archived call partition: %v", err)
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusSubmitted, To: domain.StatusFailed,
		ActualCostMicros: 0, FailureCode: "test_finalization", Retryable: false,
	}); err != nil {
		t.Fatalf("terminal replay after call retention: %v", err)
	}
}

func TestRealProviderUnknownChargeRemainsHeld(t *testing.T) {
	database := operationStoreDB(t)
	opID, store := seedProviderCallOperation(t, database)
	if err := database.Exec(`
		UPDATE catalog.provider AS p SET adapter_key = 'real'
		FROM catalog.model_profile AS m
		JOIN catalog.model_profile_version AS v ON v.model_profile_id = m.id
		JOIN operation.operation AS o ON o.model_profile_version_id = v.id
		WHERE p.id = m.provider_id AND o.id = ?::uuid
	`, opID.String()).Error; err != nil {
		t.Fatal(err)
	}
	requestKey := "operation/" + opID.String()
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting,
		ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1,
		Outcome: "error", State: "rejected",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadProviderCost(t.Context(), opID); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("real provider without a pricing adapter was treated as free: %v", err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusSubmitting, To: domain.StatusFailed,
		ActualCostMicros: 0, FailureCode: "provider_rejected",
	}); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("real provider settled without pricing evidence: %v", err)
	}
}

func TestProviderCostRefusesUnprovenZeroCharge(t *testing.T) {
	database := operationStoreDB(t)
	opID, store := seedProviderCallOperation(t, database)
	requestKey, taskID := "operation/"+opID.String(), "mock-task-2"
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting,
		ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusSubmitting, To: domain.StatusFailed,
		FailureCode: "unknown", ActualCostMicros: 0,
	}); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("unknown submit settled at zero: %v", err)
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1, Outcome: "ok",
		State: "accepted", ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusSubmitting, To: domain.StatusFailed,
		FailureCode: "unknown", ActualCostMicros: 0,
	}); !errors.Is(err, application.ErrProviderCostUnknown) {
		t.Fatalf("accepted submit without final charge settled at zero: %v", err)
	}
	var status string
	if err := database.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`, opID.String()).Scan(&status).Error; err != nil || status != "submitting" {
		t.Fatalf("failed settlement changed state to %q: %v", status, err)
	}
}

func TestManualNotExecutedRequiresAdministratorEvidence(t *testing.T) {
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
			t.Fatalf("manual transition %s: %v", transition.To, err)
		}
	}
	if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
		OperationID: opID, Action: "submit", Attempt: 1,
		Outcome: "unknown", State: "unknown",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckManualNotExecuted(t.Context(), opID); !errors.Is(err, application.ErrManualResolutionUnverified) {
		t.Fatalf("missing administrator evidence was accepted: %v", err)
	}
	if _, err := store.LoadProviderCost(t.Context(), opID); !errors.Is(err, application.ErrManualResolutionUnverified) {
		t.Fatalf("uncertain manual operation was treated as free: %v", err)
	}
	var organization struct{ OrgID uuid.UUID }
	if err := database.Raw(`
		SELECT p.org_id FROM operation.operation AS o
		JOIN workspace.project AS p ON p.id = o.project_id
		WHERE o.id = ?::uuid
	`, opID.String()).Scan(&organization).Error; err != nil {
		t.Fatal(err)
	}
	adminID, producerID := uuid.New(), uuid.New()
	for _, user := range []struct {
		id   uuid.UUID
		role string
	}{{adminID, "admin"}, {producerID, "producer"}} {
		if err := database.Exec(`
			INSERT INTO identity."user"
			  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
			VALUES (?::uuid, ?::uuid, ?, 'Reviewer', ?, 'hash', false)
		`, user.id.String(), organization.OrgID.String(), "manual-"+user.id.String(), user.role).Error; err != nil {
			t.Fatalf("seed reviewer: %v", err)
		}
	}
	decision := application.ManualNotExecutedInput{
		OperationID: opID, AdminID: producerID, Evidence: "mock task absent in provider console",
	}
	if err := store.RecordManualNotExecuted(t.Context(), decision); !errors.Is(err, application.ErrManualResolutionUnverified) {
		t.Fatalf("producer was allowed to resolve: %v", err)
	}
	decision.AdminID = adminID
	for range 2 {
		if err := store.RecordManualNotExecuted(t.Context(), decision); err != nil {
			t.Fatalf("administrator decision/replay: %v", err)
		}
	}
	decision.Evidence = "different evidence"
	if err := store.RecordManualNotExecuted(t.Context(), decision); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("conflicting manual decision accepted: %v", err)
	}
	if err := store.CheckManualNotExecuted(t.Context(), opID); err != nil {
		t.Fatalf("recorded resolution was not visible: %v", err)
	}
	cost, err := store.LoadProviderCost(t.Context(), opID)
	if err != nil || cost.ActualCostMicros != 0 || !cost.ManualNotExecuted {
		t.Fatalf("manual zero charge = %+v: %v", cost, err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusManual, To: domain.StatusFailed,
		FailureCode: "other_failure", ActualCostMicros: 0,
	}); !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatalf("wrong manual failure code accepted: %v", err)
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: opID.String(), From: domain.StatusManual, To: domain.StatusFailed,
		FailureCode: "provider_not_executed", ActualCostMicros: 0,
	}); err != nil {
		t.Fatalf("settle administrator decision: %v", err)
	}
}

func TestProviderCallSettlementUnderRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	opID, _ := seedProviderCallOperation(t, database)
	requestKey := "operation/" + opID.String()
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return fmt.Errorf("assume runtime role: %w", err)
		}
		store := pgoperation.NewStore(tx)
		if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
			OperationID: opID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting,
			ProviderRequestKey: &requestKey,
		}); err != nil {
			return fmt.Errorf("transition under runtime role: %w", err)
		}
		if err := store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{
			OperationID: opID, Action: "submit", Attempt: 1,
		}); err != nil {
			return fmt.Errorf("begin under runtime role: %w", err)
		}
		if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{
			OperationID: opID, Action: "submit", Attempt: 1, Outcome: "ok", State: "rejected",
		}); err != nil {
			return fmt.Errorf("complete under runtime role: %w", err)
		}
		cost, err := store.LoadProviderCost(t.Context(), opID)
		if err != nil || cost.ActualCostMicros != 0 || !cost.HasSubmission {
			return fmt.Errorf("load runtime charge %+v: %w", cost, err)
		}
		finalizer := app.NewOperationFinalizer(tx, store, pgbilling.NewStore(tx))
		if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
			OperationID: opID.String(), From: domain.StatusSubmitting, To: domain.StatusFailed,
			FailureCode: "provider_rejected", ActualCostMicros: 0,
		}); err != nil {
			return fmt.Errorf("finalize under runtime role: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
