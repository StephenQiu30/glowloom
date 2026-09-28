package operation_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestExpireQuotedOperationsAndBatchWithoutTouchingConfirmedFunds(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	first, second := oldQuotedSnapshotItem(projectID, 1), oldQuotedSnapshotItem(projectID, 1)
	batchID := uuid.New()
	first.Operation.BatchID, second.Operation.BatchID = &batchID, &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed", Scope: json.RawMessage(`{}`),
		Status: domain.BatchStatusQuoted, TotalCount: 2, QuoteTotalMicros: 2,
	}
	if err := store.CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{first, second}); err != nil {
		t.Fatal(err)
	}
	confirmedQuote, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	confirmed := quotedSnapshotItem(projectID, 1)
	confirmed.Operation = confirmedQuote
	confirmed.Inputs[0].OperationID = confirmedQuote.ID
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{confirmed}); err != nil {
		t.Fatal(err)
	}
	reservation, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: confirmedQuote.ID, RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	before := first.Operation.QuoteExpiresAt.Add(-time.Second)
	counts, err := store.ExpireQuoted(t.Context(), before, 1)
	if err != nil || counts.Operations != 0 || counts.Batches != 0 {
		t.Fatalf("early expiry = %+v, %v", counts, err)
	}
	deadline := first.Operation.QuoteExpiresAt.Add(time.Minute)
	var total operationapp.ExpiredQuotes
	for i := 0; i < 5; i++ {
		counts, err = store.ExpireQuoted(t.Context(), deadline, 1)
		if err != nil {
			t.Fatal(err)
		}
		total.Operations += counts.Operations
		total.Batches += counts.Batches
		if counts.Operations == 0 && counts.Batches == 0 {
			break
		}
	}
	if total.Operations != 2 || total.Batches != 1 {
		t.Fatalf("expiry counts = %+v", total)
	}
	for _, id := range []uuid.UUID{first.Operation.ID, second.Operation.ID} {
		var row struct{ Status, FailureCode string }
		if err := database.Raw(`SELECT status, failure_code FROM operation.operation WHERE id = ?::uuid`,
			id.String()).Scan(&row).Error; err != nil || row.Status != "expired" || row.FailureCode != "quote_expired" {
			t.Fatalf("expired quote %s = %+v, %v", id, row, err)
		}
	}
	gotBatch, err := store.FindBatch(t.Context(), actor, projectID, batchID)
	if err != nil || gotBatch.Status != domain.BatchStatusExpired {
		t.Fatalf("expired batch = %+v, %v", gotBatch, err)
	}
	gotConfirmed, err := store.FindOperation(t.Context(), actor, projectID, confirmedQuote.ID)
	if err != nil || gotConfirmed.Status != domain.StatusConfirmed ||
		gotConfirmed.ReservationID == nil || *gotConfirmed.ReservationID != reservation.ReservationID {
		t.Fatalf("confirmed operation after sweep = %+v, %v", gotConfirmed, err)
	}
	var held int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid AND status = 'held'`,
		confirmedQuote.ID.String()).Scan(&held).Error; err != nil || held != 1 {
		t.Fatalf("confirmed held reservation = %d, %v", held, err)
	}
}

func TestExpireQuotedUnderRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	item := oldQuotedSnapshotItem(projectID, 1)
	batchID := uuid.New()
	item.Operation.BatchID = &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed", Scope: json.RawMessage(`{}`),
		Status: domain.BatchStatusQuoted, TotalCount: 1, QuoteTotalMicros: 1,
	}
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		counts, err := pgoperation.NewStore(tx).ExpireQuoted(t.Context(), item.Operation.QuoteExpiresAt.Add(time.Minute), 10)
		if err != nil {
			return err
		}
		if counts.Operations != 1 || counts.Batches != 1 {
			return fmt.Errorf("runtime role expiry = %+v", counts)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expire under runtime role: %v", err)
	}
}

func oldQuotedSnapshotItem(projectID uuid.UUID, amount int64) pgoperation.QuoteItem {
	item := quotedSnapshotItem(projectID, amount)
	created := time.Now().UTC().Add(-time.Hour)
	expires := created.Add(15 * time.Minute)
	item.Operation.CreateTime = created
	item.Operation.QuoteExpiresAt = &expires
	return item
}
