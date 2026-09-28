package operation_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestConfirmSingleQuoteRejectsReusedKeyForDifferentOperation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	store := pgoperation.NewStore(database)
	ids := make([]uuid.UUID, 2)
	for index := range ids {
		item := quotedSnapshotItem(projectID, 20)
		item.Operation.Capability = quoted.Capability
		item.Operation.ModelProfileVersionID = quoted.ModelProfileVersionID
		item.Operation.PriceRuleVersionID = quoted.PriceRuleVersionID
		if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
			t.Fatal(err)
		}
		ids[index] = item.Operation.ID
	}
	key := uuid.NewString()
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: ids[0], RequestID: key,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: ids[1], RequestID: key,
	}); !errors.Is(err, operationapp.ErrConfirmationKeyReused) {
		t.Fatalf("changed target with same key = %v", err)
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, ids[1])
	if err != nil || got.Status != domain.StatusQuoted || got.ReservationID != nil {
		t.Fatalf("second quote after reused key = %+v, %v", got, err)
	}
}

func TestConfirmBatchQuoteReplaysFirstResultAndRejectsChangedExclusions(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 20, 30)
	input := operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
		ExcludeOperationIDs: []uuid.UUID{items[1].Operation.ID},
	}
	first, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, input)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("durable batch replay = %+v, %v; first %+v", replay, err, first)
	}
	changed := input
	changed.ExcludeOperationIDs = nil
	if _, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, changed); !errors.Is(err, operationapp.ErrConfirmationKeyReused) {
		t.Fatalf("changed exclusion with same key = %v", err)
	}
	var reservations, events int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid`,
		projectID.String()).Scan(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox
		WHERE topic = 'lanverse.batch.confirmed.v1' AND payload->'data'->>'batch_id' = ?`,
		batchID.String()).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 1 || events != 1 {
		t.Fatalf("batch replay wrote reservations=%d events=%d", reservations, events)
	}
}

func TestConfirmBatchQuoteConcurrentSameKeyCreatesOneReservation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, _ := seedConfirmableBatch(t, database, 20)
	input := operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
	}
	type outcome struct {
		result operationapp.ConfirmBatchQuoteResult
		err    error
	}
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, input)
			results <- outcome{result: result, err: err}
		}()
	}
	wait.Wait()
	close(results)
	var first *operationapp.ConfirmBatchQuoteResult
	for value := range results {
		if value.err != nil {
			t.Fatalf("concurrent confirmation: %v", value.err)
		}
		if first == nil {
			first = &value.result
		} else if !reflect.DeepEqual(*first, value.result) {
			t.Fatalf("concurrent replays differ: %+v / %+v", *first, value.result)
		}
	}
	var reservations int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid`,
		projectID.String()).Scan(&reservations).Error; err != nil || reservations != 1 {
		t.Fatalf("concurrent reservations=%d err=%v", reservations, err)
	}
}

func TestConfirmBatchQuoteReplaysBudgetRejectionAfterBudgetChanges(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, _ := seedConfirmableBatch(t, database, 110)
	input := operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
	}
	store := pgoperation.NewStore(database)
	first, err := store.ConfirmBatchQuote(t.Context(), actor, input)
	if !errors.Is(err, billingdomain.ErrBudgetInsufficient) {
		t.Fatalf("first budget rejection = %+v, %v", first, err)
	}
	if err := database.Exec(`UPDATE billing.budget SET limit_micros = 200 WHERE project_id = ?::uuid`,
		projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := store.ConfirmBatchQuote(t.Context(), actor, input)
	if !errors.Is(err, billingdomain.ErrBudgetInsufficient) || !reflect.DeepEqual(replay, first) {
		t.Fatalf("budget rejection replay = %+v, %v; first %+v", replay, err, first)
	}
	input.RequestID = uuid.NewString()
	confirmed, err := store.ConfirmBatchQuote(t.Context(), actor, input)
	if err != nil || confirmed.ConfirmedCount != 1 {
		t.Fatalf("new request after budget change = %+v, %v", confirmed, err)
	}
}
