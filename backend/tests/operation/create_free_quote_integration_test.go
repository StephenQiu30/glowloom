package operation_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestCreateFreeQuoteUsesCurrentCatalogAndCanBeConfirmed(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`
		UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb
		WHERE id = ?::uuid
	`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	command := operationapp.NewCreateFreeQuoteCommand(store)
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "有风的海面", Params: json.RawMessage(`{}`), OutputCount: 2,
		},
	}
	result, err := command.Execute(t.Context(), actor, input)
	if err != nil || result.OperationID == uuid.Nil || result.QuoteMicros != 2 ||
		result.AvailableMicros != 100 || !result.Confirmable || result.ReusedFromID != nil ||
		!result.ExpiresAt.After(time.Now()) {
		t.Fatalf("free quote = %+v, %v", result, err)
	}
	var resultDetail struct {
		Quantity        json.Number `json:"quantity"`
		UnitPriceMicros int64       `json:"unit_price_micros"`
		Outputs         int64       `json:"outputs"`
	}
	if err := json.Unmarshal(result.QuoteDetail, &resultDetail); err != nil ||
		resultDetail.Quantity != "1" || resultDetail.UnitPriceMicros != 1 || resultDetail.Outputs != 2 {
		t.Fatalf("single quote detail = %+v, %v", resultDetail, err)
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, result.OperationID)
	if err != nil || got.Status != domain.StatusQuoted || got.QuoteMicros == nil ||
		*got.QuoteMicros != 2 || got.ModelProfileVersionID == nil ||
		*got.ModelProfileVersionID != *quoted.ModelProfileVersionID ||
		got.PriceRuleVersionID == nil || *got.PriceRuleVersionID != *quoted.PriceRuleVersionID {
		t.Fatalf("persisted free quote = %+v, %v", got, err)
	}
	var storedDetail struct {
		Quantity        json.Number `json:"quantity"`
		UnitPriceMicros int64       `json:"unit_price_micros"`
		Outputs         int64       `json:"outputs"`
	}
	if err := json.Unmarshal(got.QuoteDetail, &storedDetail); err != nil ||
		storedDetail != resultDetail {
		t.Fatalf("stored free quote detail = %+v, %v; result %+v", storedDetail, err, resultDetail)
	}
	inputs, err := store.FindOperationInputs(t.Context(), actor, projectID, got.ID)
	if err != nil || len(inputs) != 1 || inputs[0].TextValue == nil || *inputs[0].TextValue != input.Prompt {
		t.Fatalf("frozen free quote inputs = %+v, %v", inputs, err)
	}
	confirmed, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: got.ID, RequestID: uuid.NewString(),
	})
	if err != nil || confirmed.ReservationID == uuid.Nil || confirmed.AvailableMicros != 98 {
		t.Fatalf("confirm generated free quote = %+v, %v", confirmed, err)
	}
}

func TestCreateFreeQuoteLocksReadyMediaAndFreezesAssetIdentity(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.capability SET input_roles = ARRAY['prompt','subject'] WHERE key = ?`, quoted.Capability).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		UPDATE catalog.model_profile_version
		SET limits = '{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["image"],"max_bytes":100},"_total_images":1}}'::jsonb
		WHERE id = ?::uuid
	`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO media.media_asset
		  (id, project_id, kind, origin, status, object_key, mime_type, byte_size, sha256, moderation_status)
		VALUES (?::uuid, ?::uuid, 'image', 'upload', 'ready', ?, 'image/png', 50, ?, 'passed')
	`, assetID.String(), projectID.String(), "quote-input/"+assetID.String(),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Error; err != nil {
		t.Fatal(err)
	}
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "一片海面", OutputCount: 1,
			MediaInputs: []operationapp.FreeQuoteMediaInput{{Role: "subject", MediaAssetID: assetID}},
		},
	}
	store := pgoperation.NewStore(database)
	result, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || result.OperationID == uuid.Nil {
		t.Fatalf("ready media quote = %+v, %v", result, err)
	}
	inputs, err := store.FindOperationInputs(t.Context(), actor, projectID, result.OperationID)
	if err != nil || len(inputs) != 2 || inputs[1].MediaAssetID == nil || *inputs[1].MediaAssetID != assetID ||
		inputs[1].RefID == nil || *inputs[1].RefID != assetID || inputs[1].RefVersion == nil ||
		*inputs[1].RefVersion != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("frozen media inputs = %+v, %v", inputs, err)
	}
	input.MediaInputs[0].MediaAssetID = uuid.New()
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrQuoteKeyReused) {
		t.Fatalf("changed media with same key = %v", err)
	}
	input.MediaInputs[0].MediaAssetID = assetID
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'pending' WHERE id = ?::uuid`, assetID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input.RequestID = uuid.NewString()
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("unreviewed media quote = %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'passed', contains_real_person = true WHERE id = ?::uuid`, assetID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("unverifiable consent quote = %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET contains_real_person = false WHERE id = ?::uuid`, assetID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		runtimeInput := input
		runtimeInput.RequestID = uuid.NewString()
		_, err := pgoperation.NewStore(tx).CreateFreeQuote(t.Context(), actor, runtimeInput)
		return err
	}); err != nil {
		t.Fatalf("runtime role media quote: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET sha256 = ? WHERE id = ?::uuid`,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", assetID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: result.OperationID, RequestID: uuid.NewString(),
	}); !errors.Is(err, pgoperation.ErrWorkflowInputNotReady) {
		t.Fatalf("changed media content confirmed: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET sha256 = ? WHERE id = ?::uuid`,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", assetID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: result.OperationID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatalf("restored media quote confirmation: %v", err)
	}
	_, foreignProjectID := operationStoreProject(t, database)
	foreignAssetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO media.media_asset
		  (id, project_id, kind, origin, status, object_key, mime_type, byte_size, sha256, moderation_status)
		VALUES (?::uuid, ?::uuid, 'image', 'upload', 'ready', ?, 'image/png', 50, ?, 'passed')
	`, foreignAssetID.String(), foreignProjectID.String(), "quote-input/"+foreignAssetID.String(),
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc").Error; err != nil {
		t.Fatal(err)
	}
	good := input.FreeQuoteItemInput
	bad := good
	bad.MediaInputs = []operationapp.FreeQuoteMediaInput{{Role: "subject", MediaAssetID: foreignAssetID}}
	batch, err := store.CreateBatchFreeQuote(t.Context(), actor, operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(), Items: []operationapp.FreeQuoteItemInput{good, bad},
	})
	if err != nil || batch.BatchID == nil || len(batch.Items) != 2 || batch.Items[0].OperationID == nil ||
		batch.Items[1].OperationID != nil || batch.Items[1].ErrorCode != "input_not_ready" {
		t.Fatalf("project-scoped media batch = %+v, %v", batch, err)
	}
}

func TestCreateFreeQuoteUsesProjectDefaultModelAndReplaysResolvedQuote(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	modelKey := "quote-model-" + modelID.String()
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":2}'::jsonb WHERE id = ?::uuid`,
		quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE workspace.project SET default_models = jsonb_build_object(?, ?) WHERE id = ?::uuid`,
		quoted.Capability, modelKey, projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			Capability: quoted.Capability, Mode: "text_to_image", Prompt: "默认模型的海面", OutputCount: 1,
		},
	}
	first, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || first.OperationID == uuid.Nil || first.QuoteMicros != 1 {
		t.Fatalf("project default quote = %+v, %v", first, err)
	}
	batchInput := operationapp.CreateBatchFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		Items: []operationapp.FreeQuoteItemInput{input.FreeQuoteItemInput, input.FreeQuoteItemInput},
	}
	batch, err := store.CreateBatchFreeQuote(t.Context(), actor, batchInput)
	if err != nil || batch.BatchID == nil || len(batch.Items) != 2 ||
		batch.Items[0].ModelKey != modelKey || batch.Items[1].ModelKey != modelKey {
		t.Fatalf("batch effective model keys = %+v, %v", batch, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		runtimeInput := input
		runtimeInput.RequestID = uuid.NewString()
		_, err := pgoperation.NewStore(tx).CreateFreeQuote(t.Context(), actor, runtimeInput)
		return err
	}); err != nil {
		t.Fatalf("runtime role project default quote: %v", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET default_models = jsonb_build_object(?, ?) WHERE id = ?::uuid`,
		quoted.Capability, "missing-model", projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || replay.OperationID != first.OperationID {
		t.Fatalf("default changed quote replay = %+v, %v", replay, err)
	}
	batchReplay, err := store.CreateBatchFreeQuote(t.Context(), actor, batchInput)
	if err != nil || batchReplay.BatchID == nil || *batchReplay.BatchID != *batch.BatchID ||
		batchReplay.Items[0].ModelKey != modelKey {
		t.Fatalf("default changed batch replay = %+v, %v", batchReplay, err)
	}
	input.RequestID = uuid.NewString()
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteModelMissing) {
		t.Fatalf("missing new default model = %v", err)
	}
	batchInput.RequestID = uuid.NewString()
	missingBatch, err := store.CreateBatchFreeQuote(t.Context(), actor, batchInput)
	if err != nil || missingBatch.BatchID != nil || len(missingBatch.Items) != 2 ||
		missingBatch.Items[0].ErrorCode != "model_unavailable" || missingBatch.Items[1].ErrorCode != "model_unavailable" {
		t.Fatalf("missing default batch quote = %+v, %v", missingBatch, err)
	}
	input.ModelKey = modelKey
	input.RequestID = uuid.NewString()
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); err != nil {
		t.Fatalf("explicit model overrides missing default: %v", err)
	}
}

func TestCreateFreeQuoteReusesCompletedInputUnlessForced(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "可复用的海面", OutputCount: 1,
		},
	}
	first, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || first.QuoteMicros != 1 {
		t.Fatalf("first quote = %+v, %v", first, err)
	}
	firstOperation, err := store.FindOperation(t.Context(), actor, projectID, first.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := seedCompletedOutput(t, database, projectID, firstOperation.InputHash, "passed", "ready", "passed")
	input.RequestID = uuid.NewString()
	reused, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || reused.QuoteMicros != 0 || reused.ReusedFromID == nil || *reused.ReusedFromID != sourceID {
		t.Fatalf("reused quote = %+v, %v", reused, err)
	}
	input.ForceRegenerate = true
	input.RequestID = uuid.NewString()
	forced, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || forced.QuoteMicros != 1 || forced.ReusedFromID != nil {
		t.Fatalf("forced quote = %+v, %v", forced, err)
	}
}

func TestCreateFreeQuotePersistsRequestKeyAcrossRetries(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "重试的海面", OutputCount: 1,
		},
	}
	first, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'disabled' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || replayed.OperationID != first.OperationID || replayed.QuoteMicros != first.QuoteMicros || !replayed.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("quote retry = %+v, %v; first %+v", replayed, err, first)
	}
	input.Prompt = "另一片海面"
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrQuoteKeyReused) {
		t.Fatalf("same key changed body = %v", err)
	}
	var operations, requests int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE project_id = ?::uuid`, projectID.String()).Scan(&operations).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM operation.quote_request WHERE actor_id = ?::uuid`, actor.ID.String()).Scan(&requests).Error; err != nil {
		t.Fatal(err)
	}
	if operations != 1 || requests != 1 {
		t.Fatalf("quote retry wrote operations=%d requests=%d", operations, requests)
	}
	if err := database.Exec(`
		UPDATE workspace.project
		SET is_delete = true, delete_time = now(), purge_after = now() + interval '30 days'
		WHERE id = ?::uuid
	`, projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input.Prompt = "重试的海面"
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("deleted project quote replay = %v", err)
	}
}

func TestCreateFreeQuoteConcurrentSameKeyCreatesOneSnapshot(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "同时重试", OutputCount: 1,
		},
	}
	store := pgoperation.NewStore(database)
	var results [2]operationapp.CreateFreeQuoteResult
	var failures [2]error
	var workers sync.WaitGroup
	workers.Add(2)
	for index := range results {
		go func(index int) {
			defer workers.Done()
			results[index], failures[index] = store.CreateFreeQuote(t.Context(), actor, input)
		}(index)
	}
	workers.Wait()
	if failures[0] != nil || failures[1] != nil || results[0].OperationID == uuid.Nil || results[0].OperationID != results[1].OperationID {
		t.Fatalf("concurrent quote results = %+v, %+v; errors %v, %v", results[0], results[1], failures[0], failures[1])
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE project_id = ?::uuid`, projectID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("concurrent quote snapshots = %d, %v", count, err)
	}
}

func TestCreateFreeQuoteRejectsUnavailableModelAndForeignProjectWithoutWrites(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	foreign, _ := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`
		UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb
		WHERE id = ?::uuid
	`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "测试权限", Params: json.RawMessage(`{}`), OutputCount: 1,
		},
	}
	store := pgoperation.NewStore(database)
	if _, err := store.CreateFreeQuote(t.Context(), foreign, input); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("foreign project quote = %v", err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'disabled' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteModelMissing) {
		t.Fatalf("disabled model quote = %v", err)
	}
	var count int64
	if err := database.Model(&struct{}{}).Table("operation.operation").Where("project_id = ?", projectID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected quote rows = %d, %v", count, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		if _, err := pgoperation.NewStore(tx).CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteModelMissing) {
			t.Errorf("runtime role disabled model = %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("runtime role free quote read: %v", err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'active' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		created, err := pgoperation.NewStore(tx).CreateFreeQuote(t.Context(), actor, input)
		if err != nil {
			return err
		}
		if created.OperationID == uuid.Nil || created.QuoteMicros != 1 {
			t.Errorf("runtime role quote = %+v", created)
		}
		return nil
	}); err != nil {
		t.Fatalf("runtime role free quote write: %v", err)
	}
}

func TestCreateFreeQuoteRejectsRevokedActor(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits = '{"max_outputs":4}'::jsonb WHERE id = ?::uuid`, quoted.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: projectID, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "quote-model-" + modelID.String(), Capability: quoted.Capability,
			Mode: "text_to_image", Prompt: "测试撤权", Params: json.RawMessage(`{}`), OutputCount: 1,
		},
	}
	if _, err := pgoperation.NewStore(database).CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor quote = %v", err)
	}
}
