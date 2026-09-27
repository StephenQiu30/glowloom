package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func quotedSnapshotItem(projectID uuid.UUID, amount int64) pgoperation.QuoteItem {
	opID := uuid.New()
	created := time.Now().UTC().Add(-time.Second)
	expires := created.Add(15 * time.Minute)
	modelID, priceID := uuid.New(), uuid.New()
	region := "domestic"
	prompt := "冻结的提示词"
	return pgoperation.QuoteItem{
		Operation: domain.Operation{
			ID: opID, ProjectID: projectID, TargetType: "free",
			Capability: "image.generate", Mode: "text_to_image",
			ModelProfileVersionID: &modelID, PriceRuleVersionID: &priceID,
			Params: json.RawMessage(`{}`), OutputCount: 1,
			InputHash: uuid.NewString(), Origin: "canvas", Status: domain.StatusQuoted,
			QuoteMicros: &amount, QuoteDetail: json.RawMessage(`{"unit":"per_image"}`),
			QuoteExpiresAt: &expires, Region: &region, CreateTime: created,
		},
		Inputs: []domain.OperationInput{{
			ID: uuid.New(), OperationID: opID, SeqNo: 0,
			Role: "prompt", RefType: "text", TextValue: &prompt,
		}},
	}
}

func TestCreateQuoteSnapshotAtomicallyPersistsBatchAndInputs(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	first, second := quotedSnapshotItem(projectID, 20), quotedSnapshotItem(projectID, 30)
	batchID := uuid.New()
	first.Operation.BatchID, second.Operation.BatchID = &batchID, &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed",
		Scope: json.RawMessage(`{}`), Status: domain.BatchStatusQuoted,
		TotalCount: 2, QuoteTotalMicros: 50,
	}
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{first, second}); err != nil {
		t.Fatalf("create quoted batch: %v", err)
	}
	gotBatch, err := store.FindBatch(t.Context(), actor, projectID, batchID)
	if err != nil || gotBatch.TotalCount != 2 || gotBatch.QuoteTotalMicros != 50 {
		t.Fatalf("persisted batch = %+v: %v", gotBatch, err)
	}
	for _, want := range []pgoperation.QuoteItem{first, second} {
		got, err := store.FindOperation(t.Context(), actor, projectID, want.Operation.ID)
		if err != nil || got.Status != domain.StatusQuoted || got.ReservationID != nil {
			t.Fatalf("persisted quote = %+v: %v", got, err)
		}
		inputs, err := store.FindOperationInputs(t.Context(), actor, projectID, want.Operation.ID)
		if err != nil || len(inputs) != 1 || inputs[0].ID != want.Inputs[0].ID {
			t.Fatalf("persisted inputs = %+v: %v", inputs, err)
		}
	}
	var reservations, ledgerEntries int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid`, projectID.String()).Scan(&reservations).Error; err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&ledgerEntries).Error; err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if reservations != 0 || ledgerEntries != 0 {
		t.Fatalf("unconfirmed quote produced reservation=%d ledger=%d", reservations, ledgerEntries)
	}
}

func TestCreateQuoteSnapshotRollsBackOnFrozenInputConflict(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	existing := quotedSnapshotItem(projectID, 1)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{existing}); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	first, second := quotedSnapshotItem(projectID, 2), quotedSnapshotItem(projectID, 3)
	second.Inputs[0].ID = existing.Inputs[0].ID
	batchID := uuid.New()
	first.Operation.BatchID, second.Operation.BatchID = &batchID, &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed",
		Scope: json.RawMessage(`{}`), Status: domain.BatchStatusQuoted,
		TotalCount: 2, QuoteTotalMicros: 5,
	}
	if err := store.CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{first, second}); err == nil {
		t.Fatal("duplicate frozen input ID was accepted")
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.batch WHERE id = ?::uuid`, batchID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed batch survived rollback: count=%d err=%v", count, err)
	}
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE id IN (?::uuid, ?::uuid)`,
		first.Operation.ID.String(), second.Operation.ID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed operations survived rollback: count=%d err=%v", count, err)
	}
}

func TestCreateQuoteSnapshotRejectsForeignActorAndArchivedProject(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	foreign, _ := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	item := quotedSnapshotItem(projectID, 5)
	if err := store.CreateQuoteSnapshot(t.Context(), foreign, nil, []pgoperation.QuoteItem{item}); err == nil {
		t.Fatal("foreign organization created a project quote")
	}
	if err := database.Exec(`UPDATE workspace.project SET status = 'archived' WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err == nil {
		t.Fatal("archived project accepted a new quote")
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE id = ?::uuid`, item.Operation.ID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected quote was persisted: count=%d err=%v", count, err)
	}
}
