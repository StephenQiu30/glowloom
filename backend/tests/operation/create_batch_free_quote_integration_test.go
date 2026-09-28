package operation_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestCreateBatchFreeQuoteKeepsValidItemsAndCanConfirmThem(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	good := operationapp.FreeQuoteItemInput{
		ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
		Mode: "text_to_image", Prompt: "第一幅海面", OutputCount: 1,
	}
	second := good
	second.Prompt, second.OutputCount = "第二幅海面", 2
	missingModel := good
	missingModel.ModelKey = "missing-model-" + uuid.NewString()
	missingPrompt := good
	missingPrompt.Prompt = ""
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{good, missingModel, second, missingPrompt},
	}
	store := pgoperation.NewStore(database)
	result, err := operationapp.NewCreateBatchFreeQuoteCommand(store).Execute(t.Context(), actor, input)
	if err != nil || result.BatchID == nil || result.TotalMicros != 3 ||
		result.AvailableMicros != 100 || !result.Confirmable || len(result.Items) != 4 {
		t.Fatalf("batch quote = %+v, %v", result, err)
	}
	if result.Items[0].OperationID == nil || result.Items[0].QuoteMicros == nil ||
		*result.Items[0].QuoteMicros != 1 || result.Items[1].OperationID != nil ||
		result.Items[1].ErrorCode != "model_unavailable" ||
		result.Items[2].OperationID == nil || result.Items[2].QuoteMicros == nil ||
		*result.Items[2].QuoteMicros != 2 || result.Items[3].OperationID != nil ||
		result.Items[3].ErrorCode != "input_not_ready" {
		t.Fatalf("ordered batch items = %+v", result.Items)
	}
	var firstDetail struct {
		Quantity        json.Number `json:"quantity"`
		UnitPriceMicros int64       `json:"unit_price_micros"`
		Outputs         int64       `json:"outputs"`
	}
	if err := json.Unmarshal(result.Items[0].QuoteDetail, &firstDetail); err != nil ||
		firstDetail.Quantity != "1" || firstDetail.UnitPriceMicros != 1 || firstDetail.Outputs != 1 {
		t.Fatalf("batch item quote detail = %+v, %v", firstDetail, err)
	}
	batch, err := store.FindBatch(t.Context(), actor, projectID, *result.BatchID)
	if err != nil || batch.Status != domain.BatchStatusQuoted || batch.TotalCount != 2 || batch.QuoteTotalMicros != 3 {
		t.Fatalf("stored batch = %+v, %v", batch, err)
	}
	for _, index := range []int{0, 2} {
		item, findErr := store.FindOperation(t.Context(), actor, projectID, *result.Items[index].OperationID)
		if findErr != nil || item.BatchID == nil || *item.BatchID != *result.BatchID || item.ReservationID != nil {
			t.Fatalf("stored batch item %d = %+v, %v", index, item, findErr)
		}
	}
	var reservations int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid`, projectID.String()).Scan(&reservations).Error; err != nil || reservations != 0 {
		t.Fatalf("reservations before confirmation = %d, %v", reservations, err)
	}
	confirmed, err := store.ConfirmBatchQuote(t.Context(), actor, operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: *result.BatchID, RequestID: uuid.NewString(),
	})
	if err != nil || confirmed.ConfirmedCount != 2 || confirmed.QuoteTotalMicros != 3 || confirmed.AvailableMicros != 97 {
		t.Fatalf("confirm generated batch quote = %+v, %v", confirmed, err)
	}
}

func TestCreateBatchFreeQuoteReplaysAllInvalidResultAndRejectsChangedRequest(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{
			{ModelKey: "missing-a", Capability: "image.generate", Mode: "text_to_image", Prompt: "A", OutputCount: 1},
			{ModelKey: "missing-b", Capability: "image.generate", Mode: "text_to_image", Prompt: "B", OutputCount: 1},
		},
	}
	store := pgoperation.NewStore(database)
	first, err := store.CreateBatchFreeQuote(t.Context(), actor, input)
	if err != nil || first.BatchID != nil || first.Confirmable || len(first.Items) != 2 ||
		first.Items[0].ErrorCode != "model_unavailable" || first.Items[1].ErrorCode != "model_unavailable" {
		t.Fatalf("all invalid quote = %+v, %v", first, err)
	}
	replay, err := store.CreateBatchFreeQuote(t.Context(), actor, input)
	if err != nil || replay.BatchID != nil || !replay.ExpiresAt.Equal(first.ExpiresAt) || len(replay.Items) != 2 {
		t.Fatalf("all invalid replay = %+v, %v", replay, err)
	}
	input.Items[1].Prompt = "changed"
	if _, err := store.CreateBatchFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrQuoteKeyReused) {
		t.Fatalf("changed batch request = %v", err)
	}
	var batches, operations, requests int64
	for _, query := range []struct {
		sql   string
		count *int64
	}{
		{`SELECT count(*) FROM operation.batch WHERE project_id = ?::uuid`, &batches},
		{`SELECT count(*) FROM operation.operation WHERE project_id = ?::uuid`, &operations},
		{`SELECT count(*) FROM operation.quote_request WHERE actor_id = ?::uuid`, &requests},
	} {
		arg := projectID.String()
		if query.count == &requests {
			arg = actor.ID.String()
		}
		if err := database.Raw(query.sql, arg).Scan(query.count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if batches != 0 || operations != 0 || requests != 1 {
		t.Fatalf("all invalid persisted batches=%d operations=%d requests=%d", batches, operations, requests)
	}
}

func TestCreateBatchFreeQuoteRejectsChangedDuplicateParameterRequest(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{
			{ModelKey: "missing-a", Capability: "image.generate", Mode: "text_to_image", Prompt: "A", Params: json.RawMessage(`{"seed":1,"seed":2}`), OutputCount: 1},
			{ModelKey: "missing-b", Capability: "image.generate", Mode: "text_to_image", Prompt: "B", OutputCount: 1},
		},
	}
	store := pgoperation.NewStore(database)
	first, err := store.CreateBatchFreeQuote(t.Context(), actor, input)
	if err != nil || first.Items[0].ErrorCode != "input_not_ready" {
		t.Fatalf("duplicate parameter quote = %+v, %v", first, err)
	}
	input.Items[0].Params = json.RawMessage(`{"seed":9,"seed":2}`)
	if _, err := store.CreateBatchFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrQuoteKeyReused) {
		t.Fatalf("changed duplicate parameter body = %v", err)
	}
}

func TestCreateBatchFreeQuoteAsRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	item := operationapp.FreeQuoteItemInput{
		ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
		Mode: "text_to_image", Prompt: "运行角色", OutputCount: 1,
	}
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{item, item},
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		result, err := pgoperation.NewStore(tx).CreateBatchFreeQuote(t.Context(), actor, input)
		if err != nil {
			return err
		}
		if result.BatchID == nil || result.TotalMicros != 2 || len(result.Items) != 2 {
			t.Errorf("runtime role batch quote = %+v", result)
		}
		return nil
	}); err != nil {
		t.Fatalf("runtime role batch quote: %v", err)
	}
}

func TestCreateBatchFreeQuoteConcurrentSameKeyCreatesOneBatch(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	item := operationapp.FreeQuoteItemInput{
		ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
		Mode: "text_to_image", Prompt: "并发批次", OutputCount: 1,
	}
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{item, item},
	}
	store := pgoperation.NewStore(database)
	ctx := t.Context()
	var results [2]operationapp.CreateBatchFreeQuoteResult
	var failures [2]error
	var workers sync.WaitGroup
	workers.Add(2)
	for index := range results {
		go func(index int) {
			defer workers.Done()
			results[index], failures[index] = store.CreateBatchFreeQuote(ctx, actor, input)
		}(index)
	}
	workers.Wait()
	if failures[0] != nil || failures[1] != nil || results[0].BatchID == nil ||
		results[1].BatchID == nil || *results[0].BatchID != *results[1].BatchID {
		t.Fatalf("concurrent batch quotes = %+v, %+v; errors %v, %v", results[0], results[1], failures[0], failures[1])
	}
	var batches, operations int64
	if err := database.Raw(`SELECT count(*) FROM operation.batch WHERE project_id = ?::uuid`, projectID.String()).Scan(&batches).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE project_id = ?::uuid`, projectID.String()).Scan(&operations).Error; err != nil {
		t.Fatal(err)
	}
	if batches != 1 || operations != 2 {
		t.Fatalf("concurrent batch writes batches=%d operations=%d", batches, operations)
	}
}

func TestCreateBatchFreeQuoteSupportsMaximumItemCount(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	items := make([]operationapp.FreeQuoteItemInput, 150)
	for index := range items {
		items[index] = operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "批量边界", OutputCount: 1,
		}
	}
	input := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(), Items: items,
	}
	started := time.Now()
	result, err := pgoperation.NewStore(database).CreateBatchFreeQuote(t.Context(), actor, input)
	if err != nil || result.BatchID == nil || len(result.Items) != 150 || result.TotalMicros != 150 || result.Confirmable {
		t.Fatalf("150-item batch = batch %v, items %d, total %d, confirmable %v, err %v",
			result.BatchID, len(result.Items), result.TotalMicros, result.Confirmable, err)
	}
	t.Logf("150-item quote elapsed: %s", time.Since(started))
	input.Items = append(input.Items, items[0])
	if err := input.Validate(); !errors.Is(err, operationapp.ErrInvalidBatchFreeQuote) {
		t.Fatalf("151-item batch validation = %v", err)
	}
}
