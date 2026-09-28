package operation_test

import (
	"encoding/json"
	"errors"
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

func TestBatchLaunchProviderSlotReleasesWithTerminalSettlement(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	first, _, providerID := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.provider SET concurrency_limit = 1 WHERE id = ?::uuid`,
		providerID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	firstItem := quotedSnapshotItem(projectID, 1)
	firstItem.Operation = first
	firstItem.Inputs[0].OperationID = first.ID
	secondActor, secondProjectID := operationStoreProject(t, database)
	secondItem := quotedSnapshotItem(secondProjectID, 1)
	secondItem.Operation.Capability = first.Capability
	secondItem.Operation.ModelProfileVersionID = first.ModelProfileVersionID
	secondItem.Operation.PriceRuleVersionID = first.PriceRuleVersionID
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{firstItem}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: first.ID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateQuoteSnapshot(t.Context(), secondActor, nil, []pgoperation.QuoteItem{secondItem}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), secondActor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: secondProjectID, OperationID: secondItem.Operation.ID,
		RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	batchByOperation := make(map[uuid.UUID]uuid.UUID, 2)
	for _, item := range []pgoperation.QuoteItem{firstItem, secondItem} {
		batchID := uuid.New()
		if err := database.Exec(`
		INSERT INTO operation.batch (id, project_id, kind, scope, status, total_count,
		  quote_total_micros, workflow_id)
		VALUES (?::uuid, ?::uuid, 'mixed', '{}'::jsonb, 'running', 1, 1, ?)
	`, batchID.String(), item.Operation.ProjectID.String(), "batch/"+batchID.String()).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`
		UPDATE operation.operation SET batch_id = ?::uuid
		WHERE id = ?::uuid
	`, batchID.String(), item.Operation.ID.String()).Error; err != nil {
			t.Fatal(err)
		}
		batchByOperation[item.Operation.ID] = batchID
	}
	type admission struct {
		id       uuid.UUID
		acquired bool
		err      error
	}
	results := make(chan admission, 2)
	for _, id := range []uuid.UUID{first.ID, secondItem.Operation.ID} {
		go func(operationID uuid.UUID) {
			acquired, err := store.AcquireBatchLaunch(t.Context(), batchByOperation[operationID], operationID)
			results <- admission{id: operationID, acquired: acquired, err: err}
		}(id)
	}
	var winner, waiting uuid.UUID
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent batch admission: %v", result.err)
		}
		if result.acquired {
			if winner != uuid.Nil {
				t.Fatal("provider limit admitted two children")
			}
			winner = result.id
		} else {
			waiting = result.id
		}
	}
	if winner == uuid.Nil || waiting == uuid.Nil {
		t.Fatalf("admission results: winner=%s waiting=%s", winner, waiting)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return store.ReleaseBatchLaunchInTransaction(t.Context(), tx, winner)
	}); err == nil {
		t.Fatal("released batch slot before child settled")
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{
		OperationID: winner.String(), From: domain.StatusConfirmed, To: domain.StatusFailed,
		FailureCode: "batch_test_failure", ActualCostMicros: 1,
	}); err == nil {
		t.Fatal("settlement without provider cost evidence succeeded")
	}
	var stateBefore struct{ State string }
	if err := database.Raw(`SELECT state FROM operation.batch_launch WHERE operation_id = ?::uuid`,
		winner.String()).Scan(&stateBefore).Error; err != nil || stateBefore.State != "active" {
		t.Fatalf("failed settlement released slot: %q, %v", stateBefore.State, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		roleStore := pgoperation.NewStore(tx)
		return app.NewOperationFinalizer(tx, roleStore, pgbilling.NewStore(tx)).FinalizeOperation(t.Context(),
			operationflow.SettlementInput{
				OperationID: winner.String(), From: domain.StatusConfirmed, To: domain.StatusFailed,
				FailureCode: "batch_test_failure", ActualCostMicros: 0,
			})
	}); err != nil {
		t.Fatalf("terminal settlement: %v", err)
	}
	var released struct {
		State string
	}
	if err := database.Raw(`SELECT state FROM operation.batch_launch WHERE operation_id = ?::uuid`,
		winner.String()).Scan(&released).Error; err != nil || released.State != "done" {
		t.Fatalf("terminal slot state = %q, %v", released.State, err)
	}
	waitingBatchID := batchByOperation[waiting]
	roleBatchChange := func(change func(*pgoperation.Store) error) error {
		return database.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
				return err
			}
			return change(pgoperation.NewStore(tx))
		})
	}
	if err := roleBatchChange(func(roleStore *pgoperation.Store) error {
		return roleStore.PauseWorkflowBatch(t.Context(), waitingBatchID, "provider:rate_limited")
	}); err != nil {
		t.Fatalf("pause batch: %v", err)
	}
	if err := store.PauseWorkflowBatch(t.Context(), waitingBatchID, "provider:rate_limited"); err != nil {
		t.Fatalf("replay batch pause: %v", err)
	}
	if acquired, err := store.AcquireBatchLaunch(t.Context(), waitingBatchID, waiting); err != nil || acquired {
		t.Fatalf("paused batch admission = %v, %v", acquired, err)
	}
	if err := roleBatchChange(func(roleStore *pgoperation.Store) error {
		return roleStore.ResumeWorkflowBatch(t.Context(), waitingBatchID)
	}); err != nil {
		t.Fatalf("resume batch: %v", err)
	}
	if err := store.ResumeWorkflowBatch(t.Context(), waitingBatchID); err != nil {
		t.Fatalf("replay batch resume: %v", err)
	}
	acquired, err := store.AcquireBatchLaunch(t.Context(), batchByOperation[waiting], waiting)
	if err != nil || !acquired {
		t.Fatalf("admission after terminal settlement = %v, %v", acquired, err)
	}
	if err := store.FinishWorkflowBatch(t.Context(), batchByOperation[waiting]); !errors.Is(err, operationapp.ErrWorkflowBatchNotReady) {
		t.Fatalf("unfinished child allowed batch finish: %v", err)
	}
	completedBatchID := batchByOperation[winner]
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		return pgoperation.NewStore(tx).FinishWorkflowBatch(t.Context(), completedBatchID)
	}); err != nil {
		t.Fatalf("finish completed batch: %v", err)
	}
	if err := store.FinishWorkflowBatch(t.Context(), completedBatchID); err != nil {
		t.Fatalf("replay batch finish: %v", err)
	}
	var finishedEvents int64
	if err := database.Raw(`
		SELECT count(*) FROM infra.outbox
		WHERE topic = 'lanverse.batch.finished.v1'
		  AND payload->'data'->>'batch_id' = ?
	`, completedBatchID.String()).Scan(&finishedEvents).Error; err != nil || finishedEvents != 1 {
		t.Fatalf("batch finished events = %d, %v", finishedEvents, err)
	}
	var eventRow struct{ Payload []byte }
	if err := database.Raw(`
		SELECT payload FROM infra.outbox
		WHERE topic = 'lanverse.batch.finished.v1'
		  AND payload->'data'->>'batch_id' = ?
	`, completedBatchID.String()).Scan(&eventRow).Error; err != nil {
		t.Fatal(err)
	}
	var event struct {
		OrgID     uuid.UUID `json:"org_id"`
		ProjectID uuid.UUID `json:"project_id"`
		Aggregate struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data struct {
			BatchID        uuid.UUID `json:"batch_id"`
			Status         string    `json:"status"`
			TotalCount     int32     `json:"total_count"`
			FailedCount    int32     `json:"failed_count"`
			CancelledCount int32     `json:"cancelled_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(eventRow.Payload, &event); err != nil ||
		event.Aggregate.Type != "batch" || event.Aggregate.ID != completedBatchID ||
		event.Data.BatchID != completedBatchID || event.Data.Status != "finished" || event.Data.TotalCount != 1 ||
		event.Data.FailedCount != 1 || event.Data.CancelledCount != 0 ||
		event.OrgID == uuid.Nil || event.ProjectID == uuid.Nil {
		t.Fatalf("batch finished payload: %+v, %v", event, err)
	}
}
