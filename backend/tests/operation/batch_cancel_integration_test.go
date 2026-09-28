package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestBatchCancelQueuedChildReleasesReservationOnce(t *testing.T) {
	database, store, batchID, operationID := confirmedOneItemBatch(t)
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		roleStore := pgoperation.NewStore(tx)
		if err := roleStore.MarkWorkflowBatchCancelRequested(t.Context(), batchID); err != nil {
			return err
		}
		if admitted, err := roleStore.AcquireBatchLaunch(t.Context(), batchID, operationID); err != nil || admitted {
			t.Fatalf("cancelled batch admitted queued child: %v, %v", admitted, err)
		}
		finalizer := app.NewOperationFinalizer(tx, roleStore, pgbilling.NewStore(tx))
		canceler := finalizer
		for range 2 {
			cancelled, err := canceler.CancelQueuedBatchOperation(t.Context(), batchID, operationID)
			if err != nil || !cancelled {
				t.Fatalf("cancel queued child: %v, %v", cancelled, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("batch cancellation with runtime role: %v", err)
	}
	assertCancelledBatchMember(t, database, operationID)
	if err := store.FinishWorkflowBatch(t.Context(), batchID); err != nil {
		t.Fatalf("finish cancelled batch: %v", err)
	}
	if err := store.FinishWorkflowBatch(t.Context(), batchID); err != nil {
		t.Fatalf("replay cancelled batch finish: %v", err)
	}
	var batchStatus string
	if err := database.Raw(`SELECT status FROM operation.batch WHERE id = ?::uuid`, batchID.String()).Scan(&batchStatus).Error; err != nil || batchStatus != "cancelled" {
		t.Fatalf("batch status = %q, %v", batchStatus, err)
	}
	var summaryCount int64
	if err := database.Raw(`SELECT count(*) FROM infra.outbox
		WHERE topic = 'lanverse.batch.finished.v1' AND payload->'data'->>'batch_id' = ?`,
		batchID.String()).Scan(&summaryCount).Error; err != nil || summaryCount != 1 {
		t.Fatalf("cancelled batch summaries = %d, %v", summaryCount, err)
	}
	var eventStatus string
	if err := database.Raw(`SELECT payload->'data'->>'status' FROM infra.outbox
		WHERE topic = 'lanverse.batch.finished.v1' AND payload->'data'->>'batch_id' = ?`,
		batchID.String()).Scan(&eventStatus).Error; err != nil || eventStatus != "cancelled" {
		t.Fatalf("cancelled batch summary status = %q, %v", eventStatus, err)
	}
}

func TestBatchCancelReleasesAdmittedChildBeforeProviderCall(t *testing.T) {
	database, store, batchID, operationID := confirmedOneItemBatch(t)
	if admitted, err := store.AcquireBatchLaunch(t.Context(), batchID, operationID); err != nil || !admitted {
		t.Fatalf("admit batch child: %v, %v", admitted, err)
	}
	if err := store.MarkWorkflowBatchCancelRequested(t.Context(), batchID); err != nil {
		t.Fatal(err)
	}
	canceler := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if cancelled, err := canceler.CancelQueuedBatchOperation(t.Context(), batchID, operationID); err != nil || !cancelled {
		t.Fatalf("cancel admitted child before submit: %v, %v", cancelled, err)
	}
	assertCancelledBatchMember(t, database, operationID)
	var slotState string
	if err := database.Raw(`SELECT state FROM operation.batch_launch WHERE operation_id = ?::uuid`,
		operationID.String()).Scan(&slotState).Error; err != nil || slotState != "done" {
		t.Fatalf("admitted child slot = %q, %v", slotState, err)
	}
}

func TestBatchCancelDoesNotReleaseChildWithProviderAttempt(t *testing.T) {
	database, store, batchID, operationID := confirmedOneItemBatch(t)
	if admitted, err := store.AcquireBatchLaunch(t.Context(), batchID, operationID); err != nil || !admitted {
		t.Fatalf("admit batch child: %v, %v", admitted, err)
	}
	loaded, err := store.LoadWorkflowOperation(t.Context(), operationID)
	if err != nil {
		t.Fatal(err)
	}
	requestKey := loaded.ProviderRequestKey
	if _, err := store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusConfirmed},
		To: domain.StatusSubmitting, Reason: "submit", ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatalf("begin child submission: %v", err)
	}
	if err := store.BeginProviderCall(t.Context(), operationapp.BeginProviderCallInput{
		OperationID: operationID, Action: "submit", Attempt: 1,
	}); err != nil {
		t.Fatalf("record provider attempt: %v", err)
	}
	if err := store.MarkWorkflowBatchCancelRequested(t.Context(), batchID); err != nil {
		t.Fatal(err)
	}
	canceler := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if cancelled, err := canceler.CancelQueuedBatchOperation(t.Context(), batchID, operationID); err != nil || cancelled {
		t.Fatalf("provider attempt wrongly released at zero: %v, %v", cancelled, err)
	}
	var state struct {
		Status      string
		Reservation string
		Launch      string
	}
	if err := database.Raw(`
		SELECT o.status, r.status AS reservation, l.state AS launch
		FROM operation.operation AS o
		JOIN billing.reservation AS r ON r.operation_id = o.id
		JOIN operation.batch_launch AS l ON l.operation_id = o.id
		WHERE o.id = ?::uuid
	`, operationID.String()).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Status != "submitting" || state.Reservation != "held" || state.Launch != "active" {
		t.Fatalf("unsafe provider cancellation: %+v", state)
	}
}

func TestBatchCancelSubmittedChildRequiresProviderEvidence(t *testing.T) {
	database, store, batchID, operationID := confirmedOneItemBatch(t)
	if err := database.Exec(`
		UPDATE catalog.provider AS p SET adapter_key = 'mock'
		FROM catalog.model_profile AS m
		JOIN catalog.model_profile_version AS v ON v.model_profile_id = m.id
		JOIN operation.operation AS o ON o.model_profile_version_id = v.id
		WHERE p.id = m.provider_id AND o.id = ?::uuid
	`, operationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		UPDATE catalog.model_profile_version AS v SET supports_cancel = true
		FROM operation.operation AS o
		WHERE o.model_profile_version_id = v.id AND o.id = ?::uuid
	`, operationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if admitted, err := store.AcquireBatchLaunch(t.Context(), batchID, operationID); err != nil || !admitted {
		t.Fatalf("admit batch child: %v, %v", admitted, err)
	}
	loaded, err := store.LoadWorkflowOperation(t.Context(), operationID)
	if err != nil {
		t.Fatal(err)
	}
	requestKey, taskID := loaded.ProviderRequestKey, "mock-cancel-"+operationID.String()
	if _, err := store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusConfirmed},
		To: domain.StatusSubmitting, Reason: "submit", ProviderRequestKey: &requestKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), operationapp.BeginProviderCallInput{
		OperationID: operationID, Action: "submit", Attempt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteProviderCall(t.Context(), operationapp.CompleteProviderCallInput{
		OperationID: operationID, Action: "submit", Attempt: 1, Outcome: "ok",
		State: "accepted", ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusSubmitting},
		To: domain.StatusSubmitted, Reason: "accepted", ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkWorkflowBatchCancelRequested(t.Context(), batchID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusSubmitted},
		To: domain.StatusCancelling, Reason: "cancel_requested",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginProviderCall(t.Context(), operationapp.BeginProviderCallInput{
		OperationID: operationID, Action: "cancel", Attempt: 1, ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatalf("persist cancel intent before dispatch: %v", err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: operationID.String(), From: domain.StatusCancelling,
		To: domain.StatusCancelled, ActualCostMicros: 0,
	}); err == nil {
		t.Fatal("unconfirmed cancellation settled before provider result")
	}
	if err := store.CompleteProviderCall(t.Context(), operationapp.CompleteProviderCallInput{
		OperationID: operationID, Action: "cancel", Attempt: 1, Outcome: "ok",
		State: "cancelled", ProviderTaskID: &taskID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: operationID.String(), From: domain.StatusCancelling,
		To: domain.StatusCancelled, ActualCostMicros: 0,
	}); err != nil {
		t.Fatalf("settle confirmed cancellation: %v", err)
	}
	assertCancelledBatchMember(t, database, operationID)
	var launchState string
	if err := database.Raw(`SELECT state FROM operation.batch_launch WHERE operation_id = ?::uuid`,
		operationID.String()).Scan(&launchState).Error; err != nil || launchState != "done" {
		t.Fatalf("cancelled child slot = %q, %v", launchState, err)
	}
}

func confirmedOneItemBatch(t *testing.T) (*gorm.DB, *pgoperation.Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	return confirmedOneItemBatchOnDB(t, operationStoreDB(t))
}

func confirmedOneItemBatchOnDB(t *testing.T, database *gorm.DB) (*gorm.DB, *pgoperation.Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	batchID := uuid.New()
	if err := database.Exec(`
		INSERT INTO operation.batch (id, project_id, kind, scope, status, total_count, quote_total_micros, workflow_id)
		VALUES (?::uuid, ?::uuid, 'mixed', '{}'::jsonb, 'running', 1, 1, ?)
	`, batchID.String(), projectID.String(), "batch/"+batchID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE operation.operation SET batch_id = ?::uuid WHERE id = ?::uuid`,
		batchID.String(), quoted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	return database, store, batchID, quoted.ID
}

func assertCancelledBatchMember(t *testing.T, database *gorm.DB, operationID uuid.UUID) {
	t.Helper()
	var state struct {
		Status         string
		SettledMicros  *int64
		Reservation    string
		ReservedMicros int64
		BudgetRevision int64
	}
	if err := database.Raw(`
		SELECT o.status, o.settled_micros, r.status AS reservation,
		       b.reserved_micros, b.revision AS budget_revision
		FROM operation.operation AS o
		JOIN billing.reservation AS r ON r.operation_id = o.id
		JOIN billing.budget AS b ON b.project_id = o.project_id
		WHERE o.id = ?::uuid
	`, operationID.String()).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Status != "cancelled" || state.SettledMicros == nil || *state.SettledMicros != 0 ||
		state.Reservation != "released" || state.ReservedMicros != 0 {
		t.Fatalf("queued cancellation state: %+v", state)
	}
	var settlements int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry
		WHERE operation_id = ?::uuid AND entry_type = 'settle' AND NOT is_delete`,
		operationID.String()).Scan(&settlements).Error; err != nil || settlements != 1 {
		t.Fatalf("queued cancellation settlements = %d, %v", settlements, err)
	}
}
