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
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestCompleteFromReuseCopiesOutputsAndSettlesZeroOnce(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	hash := uuid.NewString()
	sourceID := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	if err := database.Exec(`UPDATE operation.operation SET output_count = 2 WHERE id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_output
		  (id, project_id, operation_id, seq_no, kind, json_payload, moderation_status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 1, 'json', '{"result":"approved"}'::jsonb, 'passed')
	`, uuid.NewString(), projectID.String(), sourceID.String()).Error; err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	quoted.InputHash, quoted.ReusedFromID = hash, &sourceID
	quoted.QuoteMicros, quoted.OutputCount = &zero, 2
	item := quotedSnapshotItem(projectID, 0)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatalf("seed reuse quote: %v", err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatalf("confirm reuse quote: %v", err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.CompleteFromReuse(t.Context(), quoted.ID.String()); err != nil {
		t.Fatalf("complete reuse: %v", err)
	}
	if err := finalizer.CompleteFromReuse(t.Context(), quoted.ID.String()); err != nil {
		t.Fatalf("replay reuse completion: %v", err)
	}
	var operation struct {
		Status        string
		SettledMicros *int64
	}
	if err := database.Raw(`SELECT status, settled_micros FROM operation.operation WHERE id = ?::uuid`, quoted.ID.String()).Scan(&operation).Error; err != nil ||
		operation.Status != string(domain.StatusCompleted) || operation.SettledMicros == nil || *operation.SettledMicros != 0 {
		t.Fatalf("reuse operation = %+v, %v", operation, err)
	}
	var reservation struct{ Status string }
	if err := database.Raw(`SELECT status FROM billing.reservation WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&reservation).Error; err != nil || reservation.Status != "released" {
		t.Fatalf("reuse reservation = %+v, %v", reservation, err)
	}
	var outputs []struct {
		ID           uuid.UUID
		SeqNo        int32
		MediaAssetID *uuid.UUID
		JSONPayload  json.RawMessage
	}
	if err := database.Raw(`SELECT id, seq_no, media_asset_id, json_payload FROM operation.operation_output WHERE operation_id = ?::uuid ORDER BY seq_no`, quoted.ID.String()).Scan(&outputs).Error; err != nil || len(outputs) != 2 || outputs[0].MediaAssetID == nil ||
		outputs[1].JSONPayload == nil {
		t.Fatalf("reused outputs = %+v, %v", outputs, err)
	}
	var sourceAsset struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT media_asset_id AS id FROM operation.operation_output WHERE operation_id = ?::uuid AND seq_no = 0`, sourceID.String()).Scan(&sourceAsset).Error; err != nil || *outputs[0].MediaAssetID != sourceAsset.ID {
		t.Fatalf("reused media asset = %+v, source=%+v, %v", outputs[0], sourceAsset, err)
	}
	var sourceOutputs []struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT id FROM operation.operation_output WHERE operation_id = ?::uuid ORDER BY seq_no`, sourceID.String()).Scan(&sourceOutputs).Error; err != nil || len(sourceOutputs) != len(outputs) {
		t.Fatalf("source outputs = %+v, %v", sourceOutputs, err)
	}
	for index := range outputs {
		if outputs[index].ID == sourceOutputs[index].ID {
			t.Fatalf("reused output %d kept source row identity", index)
		}
	}
	var calls, settledEvents, completedEvents int64
	if err := database.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&calls).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.billing.settled.v1' AND payload -> 'data' ->> 'operation_id' = ?`, quoted.ID.String()).Scan(&settledEvents).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.operation.completed.v1' AND payload -> 'aggregate' ->> 'id' = ?`, quoted.ID.String()).Scan(&completedEvents).Error; err != nil {
		t.Fatal(err)
	}
	if calls != 0 || settledEvents != 1 || completedEvents != 1 {
		t.Fatalf("reuse events and calls: calls=%d settled=%d completed=%d", calls, settledEvents, completedEvents)
	}
}

func TestCompleteFromReuseRejectsSourceRevokedAfterConfirmation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	hash := uuid.NewString()
	sourceID := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	zero := int64(0)
	quoted.InputHash, quoted.ReusedFromID, quoted.QuoteMicros = hash, &sourceID, &zero
	item := quotedSnapshotItem(projectID, 0)
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
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'rejected' WHERE source_operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatal(err)
	}
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.CompleteFromReuse(t.Context(), quoted.ID.String()); !errors.Is(err, operationapp.ErrReuseSourceUnavailable) {
		t.Fatalf("revoked source completion = %v", err)
	}
	var operation struct{ Status string }
	if err := database.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`, quoted.ID.String()).Scan(&operation).Error; err != nil || operation.Status != "confirmed" {
		t.Fatalf("revoked reuse state = %+v, %v", operation, err)
	}
	var outputs, settlements int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&outputs).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'settle'`, quoted.ID.String()).Scan(&settlements).Error; err != nil {
		t.Fatal(err)
	}
	if outputs != 0 || settlements != 0 {
		t.Fatalf("revoked reuse left outputs=%d settlements=%d", outputs, settlements)
	}
}

func TestCompleteFromReuseRunsAsRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	hash := uuid.NewString()
	sourceID := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	zero := int64(0)
	quoted.InputHash, quoted.ReusedFromID, quoted.QuoteMicros = hash, &sourceID, &zero
	item := quotedSnapshotItem(projectID, 0)
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
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		finalizer := app.NewOperationFinalizer(tx, pgoperation.NewStore(tx), pgbilling.NewStore(tx))
		return finalizer.CompleteFromReuse(t.Context(), quoted.ID.String())
	}); err != nil {
		t.Fatalf("complete reuse as runtime role: %v", err)
	}
}
