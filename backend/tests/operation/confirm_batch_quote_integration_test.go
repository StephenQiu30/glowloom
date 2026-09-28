package operation_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestConfirmBatchQuoteCommitsSelectedReservationsAndOneBatchEvent(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 20, 30)
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
		ExcludeOperationIDs: []uuid.UUID{items[1].Operation.ID},
	})
	if err != nil || result.ConfirmedCount != 1 || result.QuoteTotalMicros != 20 ||
		result.AvailableMicros != 80 || len(result.Items) != 2 {
		t.Fatalf("confirm selected batch = %+v, %v", result, err)
	}
	var batch struct {
		Status           string
		TotalCount       int32
		QuoteTotalMicros int64
		WorkflowID       *string
	}
	if err := database.Raw(`
		SELECT status, total_count, quote_total_micros, workflow_id
		FROM operation.batch WHERE id = ?::uuid
	`, batchID.String()).Scan(&batch).Error; err != nil || batch.Status != "confirmed" ||
		batch.TotalCount != 1 || batch.QuoteTotalMicros != 20 || batch.WorkflowID == nil ||
		*batch.WorkflowID != "batch/"+batchID.String() {
		t.Fatalf("confirmed batch = %+v, %v", batch, err)
	}
	var selected struct {
		Status        string
		ReservationID *uuid.UUID
		WorkflowID    *string
	}
	if err := database.Raw(`
		SELECT status, reservation_id, workflow_id FROM operation.operation WHERE id = ?::uuid
	`, items[0].Operation.ID.String()).Scan(&selected).Error; err != nil ||
		selected.Status != "confirmed" || selected.ReservationID == nil ||
		selected.WorkflowID == nil || *selected.WorkflowID != "operation/"+items[0].Operation.ID.String() {
		t.Fatalf("selected operation = %+v, %v", selected, err)
	}
	var excluded struct {
		Status      string
		FailureCode *string
	}
	if err := database.Raw(`
		SELECT status, failure_code FROM operation.operation WHERE id = ?::uuid
	`, items[1].Operation.ID.String()).Scan(&excluded).Error; err != nil ||
		excluded.Status != "expired" || excluded.FailureCode == nil || *excluded.FailureCode != "excluded_by_user" {
		t.Fatalf("excluded operation = %+v, %v", excluded, err)
	}
	var reservations, ledger, batchEvents, singleEvents, auditEvents int64
	for _, check := range []struct {
		query string
		args  []any
		count *int64
	}{
		{`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid`, []any{items[0].Operation.ID.String()}, &reservations},
		{`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'reserve'`, []any{items[0].Operation.ID.String()}, &ledger},
		{`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.batch.confirmed.v1' AND payload->'data'->>'batch_id' = ?`, []any{batchID.String()}, &batchEvents},
		{`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.operation.confirmed.v1' AND partition_key = ?`, []any{projectID.String()}, &singleEvents},
		{`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.audit.recorded.v1' AND payload->'data'->>'action' = 'batch.confirmed' AND partition_key = ?`, []any{projectID.String()}, &auditEvents},
	} {
		if err := database.Raw(check.query, check.args...).Scan(check.count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if reservations != 1 || ledger != 1 || batchEvents != 1 || singleEvents != 0 || auditEvents != 1 {
		t.Fatalf("batch confirmation facts: reservations=%d ledger=%d batch=%d single=%d audit=%d",
			reservations, ledger, batchEvents, singleEvents, auditEvents)
	}
}

func TestConfirmBatchQuoteKeepsAllSelectedQuotedWhenBudgetIsInsufficient(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 60, 50)
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor,
		operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
			RequestID: uuid.NewString()})
	if !errors.Is(err, billingdomain.ErrBudgetInsufficient) || result.ConfirmedCount != 0 || len(result.Items) != 0 {
		t.Fatalf("insufficient batch budget = %+v, %v", result, err)
	}
	for _, item := range items {
		var state struct {
			Status        string
			ReservationID *uuid.UUID
		}
		if err := database.Raw(`SELECT status, reservation_id FROM operation.operation WHERE id = ?::uuid`,
			item.Operation.ID.String()).Scan(&state).Error; err != nil ||
			state.Status != "quoted" || state.ReservationID != nil {
			t.Fatalf("selected quote after budget rejection = %+v, %v", state, err)
		}
	}
	var batchStatus string
	if err := database.Raw(`SELECT status FROM operation.batch WHERE id = ?::uuid`,
		batchID.String()).Scan(&batchStatus).Error; err != nil || batchStatus != "quoted" {
		t.Fatalf("batch after budget rejection = %s, %v", batchStatus, err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid`,
		projectID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("reservations after budget rejection = %d, %v", count, err)
	}
}

func TestConfirmBatchQuotePreservesExclusionWhenReservationRollsBack(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 110, 20)
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor,
		operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
			RequestID: uuid.NewString(), ExcludeOperationIDs: []uuid.UUID{items[1].Operation.ID}})
	if !errors.Is(err, billingdomain.ErrBudgetInsufficient) || result.ConfirmedCount != 0 ||
		len(result.Items) != 1 || result.Items[0].OperationID != items[1].Operation.ID ||
		result.Items[0].Status != domain.StatusExpired {
		t.Fatalf("exclusion with insufficient budget = %+v, %v", result, err)
	}
	for index, wantStatus := range []string{"quoted", "expired"} {
		var status string
		if err := database.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`,
			items[index].Operation.ID.String()).Scan(&status).Error; err != nil || status != wantStatus {
			t.Fatalf("item %d after budget rollback = %s, %v", index, status, err)
		}
	}
}

func TestConfirmBatchQuoteExpiresAllExcludedItemsWithoutReservation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 20, 30)
	input := operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
		RequestID: uuid.NewString(), ExcludeOperationIDs: []uuid.UUID{items[0].Operation.ID, items[1].Operation.ID}}
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, input)
	if !errors.Is(err, operationapp.ErrQuoteStale) || len(result.Items) != 2 || result.ConfirmedCount != 0 {
		t.Fatalf("empty selected set = %+v, %v", result, err)
	}
	replay, replayErr := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor, input)
	if !errors.Is(replayErr, operationapp.ErrQuoteStale) || !reflect.DeepEqual(replay, result) {
		t.Fatalf("empty selected set replay = %+v, %v", replay, replayErr)
	}
	var status string
	if err := database.Raw(`SELECT status FROM operation.batch WHERE id = ?::uuid`,
		batchID.String()).Scan(&status).Error; err != nil || status != "expired" {
		t.Fatalf("empty batch status = %s, %v", status, err)
	}
	for _, item := range result.Items {
		if item.Status != domain.StatusExpired || len(item.Reasons) != 1 || item.Reasons[0] != "excluded_by_user" {
			t.Fatalf("excluded result = %+v", item)
		}
	}
}

func TestConfirmBatchQuoteRunsAsRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, _ := seedConfirmableBatch(t, database, 20)
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := pgoperation.NewStore(tx).ConfirmBatchQuote(t.Context(), actor,
			operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
				RequestID: uuid.NewString()})
		return err
	})
	if err != nil {
		t.Fatalf("confirm batch as runtime role: %v", err)
	}
}

func TestConfirmBatchQuoteKeepsFreshSiblingWhenOneQuoteExpires(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 20, 30)
	if err := database.Exec(`UPDATE operation.operation
		SET create_time = now() - interval '2 hours',
		    quote_expires_at = now() - interval '1 hour'
		WHERE id = ?::uuid`, items[1].Operation.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor,
		operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
			RequestID: uuid.NewString()})
	if err != nil || result.ConfirmedCount != 1 || result.QuoteTotalMicros != 20 || len(result.Items) != 2 {
		t.Fatalf("partly stale batch = %+v, %v", result, err)
	}
	var expired struct {
		Status      string
		FailureCode *string
	}
	if err := database.Raw(`SELECT status, failure_code FROM operation.operation WHERE id = ?::uuid`,
		items[1].Operation.ID.String()).Scan(&expired).Error; err != nil ||
		expired.Status != "expired" || expired.FailureCode == nil || *expired.FailureCode != "quote_expired" {
		t.Fatalf("expired batch item = %+v, %v", expired, err)
	}
}

func TestConfirmBatchQuoteIncludesPreviouslyExpiredSibling(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 20, 30)
	if err := database.Exec(`UPDATE operation.operation
		SET status = 'expired', failure_code = 'quote_expired'
		WHERE id = ?::uuid`, items[1].Operation.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	result, err := pgoperation.NewStore(database).ConfirmBatchQuote(t.Context(), actor,
		operationapp.ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID,
			RequestID: uuid.NewString()})
	if err != nil || result.ConfirmedCount != 1 || len(result.Items) != 2 {
		t.Fatalf("previously expired sibling = %+v, %v", result, err)
	}
}

func seedConfirmableBatch(t *testing.T, database *gorm.DB, amounts ...int64) (identityapp.Principal, uuid.UUID, uuid.UUID, []pgoperation.QuoteItem) {
	t.Helper()
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	batchID := uuid.New()
	items := make([]pgoperation.QuoteItem, 0, len(amounts))
	var total int64
	for _, amount := range amounts {
		item := quotedSnapshotItem(projectID, amount)
		item.Operation.Capability = quoted.Capability
		item.Operation.ModelProfileVersionID = quoted.ModelProfileVersionID
		item.Operation.PriceRuleVersionID = quoted.PriceRuleVersionID
		item.Operation.BatchID = &batchID
		items = append(items, item)
		total += amount
	}
	batch := domain.Batch{ID: batchID, ProjectID: projectID, Kind: "mixed",
		Scope: json.RawMessage(`{}`), Status: domain.BatchStatusQuoted,
		TotalCount: int32(len(items)), QuoteTotalMicros: total}
	if err := pgoperation.NewStore(database).CreateQuoteSnapshot(t.Context(), actor, &batch, items); err != nil {
		t.Fatal(err)
	}
	return actor, projectID, batchID, items
}
