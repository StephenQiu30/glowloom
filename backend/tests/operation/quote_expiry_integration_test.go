package operation_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestExpireQuotedOperationsAndBatchWithoutTouchingConfirmedFunds(t *testing.T) {
	database := quoteExpiryDB(t)
	actor, projectID := operationStoreProject(t, database)
	store := operationStore(database)
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
	database := quoteExpiryDB(t)
	actor, projectID := operationStoreProject(t, database)
	item := oldQuotedSnapshotItem(projectID, 1)
	batchID := uuid.New()
	item.Operation.BatchID = &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed", Scope: json.RawMessage(`{}`),
		Status: domain.BatchStatusQuoted, TotalCount: 1, QuoteTotalMicros: 1,
	}
	store := operationStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		counts, err := operationStore(tx).ExpireQuoted(t.Context(), item.Operation.QuoteExpiresAt.Add(time.Minute), 10)
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

func quoteExpiryDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := operationStoreDB(t)
	tx := database.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin quote expiry isolation: %v", tx.Error)
	}
	t.Cleanup(func() {
		// t.Context is cancelled before cleanup, so database/sql may have
		// already rolled the transaction back.
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("roll back quote expiry isolation: %v", err)
		}
	})

	// Expiry scans all projects, while other integration fixtures are committed
	// to the same database. Hide their quoted work only inside this transaction;
	// repeatable read also excludes fixtures committed after this snapshot.
	// Cleanup restores those rows and discards this test's fixtures and sweeps.
	// All setup and store calls must use the returned transaction; never commit it.
	for _, query := range []string{
		`UPDATE operation.operation SET is_delete = true WHERE status = 'quoted' AND NOT is_delete`,
		`UPDATE operation.batch SET is_delete = true WHERE status = 'quoted' AND NOT is_delete`,
	} {
		if err := tx.Exec(query).Error; err != nil {
			t.Fatalf("isolate existing quote expiry work: %v", err)
		}
	}
	return tx
}

func oldQuotedSnapshotItem(projectID uuid.UUID, amount int64) pgoperation.QuoteItem {
	item := quotedSnapshotItem(projectID, amount)
	created := time.Now().UTC().Add(-time.Hour)
	expires := created.Add(15 * time.Minute)
	item.Operation.CreateTime = created
	item.Operation.QuoteExpiresAt = &expires
	return item
}
