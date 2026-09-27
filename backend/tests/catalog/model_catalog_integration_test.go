package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func modelCatalogDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_MODEL_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_MODEL_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ctx, conn.DB
}

func insertModelCatalogOrganization(ctx context.Context, t *testing.T, database *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.organization (id, name) VALUES (?::uuid, 'Model Catalog Test')
	`, id.String()).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	return id
}

func insertModelCatalogActor(ctx context.Context, t *testing.T, database *gorm.DB, orgID uuid.UUID) identityapp.Principal {
	t.Helper()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: orgID, Role: identitydomain.RoleProducer}
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO identity."user"
		  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Model Catalog Producer', 'producer', 'test-hash', false)
	`, actor.ID.String(), orgID.String(), "model-catalog-"+actor.ID.String()).Error; err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	return actor
}

func insertModelCatalogProject(ctx context.Context, t *testing.T, database *gorm.DB, orgID uuid.UUID, allowOverseas bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.project
		  (id, org_id, name, aspect_ratio, style_type, allow_overseas_models)
		VALUES (?::uuid, ?::uuid, '模型选择', '16:9', 'realistic', ?)
	`, id.String(), orgID.String(), allowOverseas).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return id
}

func insertModelCatalogProvider(ctx context.Context, t *testing.T, database *gorm.DB, region, status, credentialResult string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.provider
		  (id, key, name, adapter_key, region, status)
		VALUES (?::uuid, ?, 'Catalog Provider', 'catalog-test', ?, ?)
	`, id.String(), "catalog-provider-"+id.String(), region, status).Error; err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if credentialResult != "" {
		if err := database.WithContext(ctx).Exec(`
			INSERT INTO catalog.provider_credential
			  (id, provider_id, label, ciphertext, key_id, last4, status, last_tested_at, last_test_result)
			VALUES (?::uuid, ?::uuid, 'primary', ?::bytea, 'model-catalog-private-key-id', '1234',
			        'active', now(), ?)
		`, uuid.NewString(), id.String(), []byte("model-catalog-private-ciphertext"), credentialResult).Error; err != nil {
			t.Fatalf("insert credential: %v", err)
		}
	}
	return id
}

func insertModelCatalogCapability(ctx context.Context, t *testing.T, database *gorm.DB) string {
	t.Helper()
	key := "video.generate." + uuid.NewString()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, ?, 'video', ARRAY['image2video', 'text2video'], ARRAY['subject'])
	`, uuid.NewString(), key).Error; err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	return key
}

func insertModelCatalogModel(ctx context.Context, t *testing.T, database *gorm.DB, key string, providerID uuid.UUID, capability, status, mode string, deleted bool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	modelID, versionID := uuid.New(), uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.model_profile
		  (id, model_key, provider_id, capability, display_name, status, is_delete)
		VALUES (?::uuid, ?, ?::uuid, ?, 'Catalog Model', 'disabled', ?)
	`, modelID.String(), key, providerID.String(), capability, deleted).Error; err != nil {
		t.Fatalf("insert model: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits, param_schema,
		   supports_query, supports_cancel, supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 1, 'test-model-v1', ARRAY[?::text],
		        '{"roles":{"subject":{"max_count":1,"types":["image"]}},"resolutions":["1080p"],"max_outputs":1}'::jsonb,
		        '[]'::jsonb, true, false, false, 30000)
	`, versionID.String(), modelID.String(), mode).Error; err != nil {
		t.Fatalf("insert model version: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE catalog.model_profile
		SET current_version_id = ?::uuid, status = ?
		WHERE id = ?::uuid
	`, versionID.String(), status, modelID.String()).Error; err != nil {
		t.Fatalf("attach model version: %v", err)
	}
	return modelID, versionID
}

func insertModelCatalogPrice(ctx context.Context, t *testing.T, database *gorm.DB, modelID uuid.UUID, versionNo int, effective time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, currency, effective_from)
		VALUES (?::uuid, ?::uuid, ?, 'per_second', '{"base_micros":1000}'::jsonb, 'CNY', ?)
	`, id.String(), modelID.String(), versionNo, effective).Error; err != nil {
		t.Fatalf("insert price: %v", err)
	}
	return id
}

func TestModelCatalogReadsProjectPolicyCurrentVersionsAndEffectivePricesOnLocalPostgres(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	orgID := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, orgID)
	domesticOnly := insertModelCatalogProject(ctx, t, database, orgID, false)
	allowOverseas := insertModelCatalogProject(ctx, t, database, orgID, true)
	capability := insertModelCatalogCapability(ctx, t, database)
	otherCapability := insertModelCatalogCapability(ctx, t, database)
	domestic := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "ok")
	withoutCredential := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "")
	disabledProvider := insertModelCatalogProvider(ctx, t, database, "domestic", "disabled", "ok")
	failedCredential := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "auth_failed")
	overseas := insertModelCatalogProvider(ctx, t, database, "overseas", "active", "ok")

	keyPrefix := "model-catalog-" + uuid.NewString()
	activeID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-a", domestic, capability, "active", "image2video", false)
	disabledID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-b", domestic, capability, "disabled", "image2video", false)
	missingCredentialID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-c", withoutCredential, capability, "active", "image2video", false)
	overseasID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-d", overseas, capability, "active", "image2video", false)
	disabledProviderID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-e", disabledProvider, capability, "active", "image2video", false)
	_, _ = insertModelCatalogModel(ctx, t, database, keyPrefix+"-f", domestic, capability, "active", "image2video", true)
	textModeID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-g", domestic, capability, "active", "text2video", false)
	_, _ = insertModelCatalogModel(ctx, t, database, keyPrefix+"-h", domestic, otherCapability, "active", "image2video", false)
	noPriceID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-i", domestic, capability, "disabled", "image2video", false)
	failedCredentialID, _ := insertModelCatalogModel(ctx, t, database, keyPrefix+"-j", failedCredential, capability, "active", "image2video", false)

	newVersionID := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits, param_schema,
		   supports_query, supports_cancel, supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 2, 'test-model-v2', ARRAY['image2video'],
		        '{"roles":{"subject":{"max_count":2,"types":["image"]}},"resolutions":["1080p"],"max_outputs":2}'::jsonb,
		        '[{"field":"seed","label":"随机种子","type":"integer","component":"input"}]'::jsonb,
		        true, true, false, 40000)
	`, newVersionID.String(), activeID.String()).Error; err != nil {
		t.Fatalf("insert newer model version: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE catalog.model_profile SET current_version_id = ?::uuid WHERE id = ?::uuid
	`, newVersionID.String(), activeID.String()).Error; err != nil {
		t.Fatalf("select current model version: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	insertModelCatalogPrice(ctx, t, database, activeID, 1, past)
	chosenPriceID := insertModelCatalogPrice(ctx, t, database, activeID, 2, past)
	insertModelCatalogPrice(ctx, t, database, activeID, 3, future)
	insertModelCatalogPrice(ctx, t, database, disabledID, 1, future)
	insertModelCatalogPrice(ctx, t, database, missingCredentialID, 1, past)
	insertModelCatalogPrice(ctx, t, database, overseasID, 1, past)
	insertModelCatalogPrice(ctx, t, database, disabledProviderID, 1, past)
	insertModelCatalogPrice(ctx, t, database, textModeID, 1, past)
	insertModelCatalogPrice(ctx, t, database, failedCredentialID, 1, past)

	store := pgcatalog.NewStore(database)
	input := catalogapp.ListModelsInput{ProjectID: domesticOnly, Capability: capability, Mode: "image2video", Limit: 2}
	var models []catalogapp.ModelCatalogItem
	var after *catalogapp.ModelCatalogCursor
	for range 4 {
		input.After = after
		page, err := store.ListModelsForProject(ctx, actor, input)
		if err != nil {
			t.Fatalf("list domestic page after %+v: %v", after, err)
		}
		if len(page.Models) > input.Limit {
			t.Fatalf("page has %d models, limit %d", len(page.Models), input.Limit)
		}
		models = append(models, page.Models...)
		if page.Next == nil {
			break
		}
		if after != nil && *page.Next == *after {
			t.Fatalf("pagination did not advance from %+v", after)
		}
		after = page.Next
	}
	wantIDs := []uuid.UUID{activeID, disabledID, missingCredentialID, disabledProviderID, noPriceID, failedCredentialID}
	wantKeys := []string{keyPrefix + "-a", keyPrefix + "-b", keyPrefix + "-c", keyPrefix + "-e", keyPrefix + "-i", keyPrefix + "-j"}
	if len(models) != len(wantIDs) {
		t.Fatalf("domestic models = %d, want %d", len(models), len(wantIDs))
	}
	for index, item := range models {
		if item.ID != wantIDs[index] || item.Key != wantKeys[index] {
			t.Fatalf("domestic model %d = %+v, want ID %s", index, item, wantIDs[index])
		}
		if item.Capability != capability || item.DisplayName != "Catalog Model" {
			t.Fatalf("model lost display metadata: %+v", item)
		}
	}
	first := models[0]
	if first.Status != domain.ModelActive || first.ProviderID != domestic ||
		first.ProviderName != "Catalog Provider" || first.ProviderRegion != domain.RegionDomestic ||
		first.ProviderStatus != domain.ProviderActive || !first.CredentialPresent ||
		first.CredentialTestResult == nil || *first.CredentialTestResult != domain.TestOK {
		t.Fatalf("active model/provider/credential projection = %+v", first)
	}
	if first.CurrentVersion == nil || first.CurrentVersion.ID != newVersionID || first.CurrentVersion.VersionNo != 2 ||
		!slices.Equal(first.CurrentVersion.Modes, []string{"image2video"}) {
		t.Fatalf("current model version = %+v", first.CurrentVersion)
	}
	var limits struct {
		MaxOutputs int `json:"max_outputs"`
	}
	var fields []struct {
		Field string `json:"field"`
	}
	if err := json.Unmarshal(first.CurrentVersion.Limits, &limits); err != nil || limits.MaxOutputs != 2 {
		t.Fatalf("current limits = %s: %v", first.CurrentVersion.Limits, err)
	}
	if err := json.Unmarshal(first.CurrentVersion.ParamSchema, &fields); err != nil || len(fields) != 1 || fields[0].Field != "seed" {
		t.Fatalf("current parameter schema = %s: %v", first.CurrentVersion.ParamSchema, err)
	}
	if first.CurrentPrice == nil || first.CurrentPrice.ID != chosenPriceID || first.CurrentPrice.VersionNo != 2 ||
		first.CurrentPrice.Unit != string(domain.PricePerSecond) || first.CurrentPrice.Currency != "CNY" ||
		!first.CurrentPrice.EffectiveFrom.Equal(past) {
		t.Fatalf("current price = %+v, want version 2 at %s", first.CurrentPrice, past)
	}
	var priceRule struct {
		BaseMicros int `json:"base_micros"`
	}
	if err := json.Unmarshal(first.CurrentPrice.Rule, &priceRule); err != nil || priceRule.BaseMicros != 1000 {
		t.Fatalf("current price rule = %s: %v", first.CurrentPrice.Rule, err)
	}
	if models[1].Status != domain.ModelDisabled || models[1].CurrentPrice != nil {
		t.Fatalf("disabled model with only future price = %+v", models[1])
	}
	if models[2].CredentialPresent || models[2].CredentialTestResult != nil {
		t.Fatalf("missing credential reported present: %+v", models[2])
	}
	if models[3].ProviderStatus != domain.ProviderDisabled {
		t.Fatalf("disabled provider state lost: %+v", models[3])
	}
	if models[4].CurrentPrice != nil {
		t.Fatalf("model without any price has a current price: %+v", models[4])
	}
	if !models[5].CredentialPresent || models[5].CredentialTestResult == nil ||
		*models[5].CredentialTestResult != domain.TestAuthFailed {
		t.Fatalf("failed credential test state lost: %+v", models[5])
	}
	encoded, err := json.Marshal(models)
	if err != nil || strings.Contains(string(encoded), "model-catalog-private-ciphertext") ||
		strings.Contains(string(encoded), "model-catalog-private-key-id") ||
		strings.Contains(string(encoded), "ciphertext") || strings.Contains(string(encoded), "key_id") {
		t.Fatalf("model catalog exposed credential material: %s, %v", encoded, err)
	}

	input.ProjectID, input.Limit, input.After = allowOverseas, 50, nil
	page, err := store.ListModelsForProject(ctx, actor, input)
	if err != nil || len(page.Models) != len(models)+1 || page.Models[3].ID != overseasID {
		t.Fatalf("overseas-allowed project models = %+v: %v", page, err)
	}
	input.Mode = "text2video"
	page, err = store.ListModelsForProject(ctx, actor, input)
	if err != nil || len(page.Models) != 1 || page.Models[0].ID != textModeID {
		t.Fatalf("mode-filtered project models = %+v: %v", page, err)
	}
}

func TestModelCatalogRejectsStaleActorAndInvisibleProjectOnLocalPostgres(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	orgID := insertModelCatalogOrganization(ctx, t, database)
	otherOrgID := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, orgID)
	otherActor := insertModelCatalogActor(ctx, t, database, otherOrgID)
	projectID := insertModelCatalogProject(ctx, t, database, orgID, false)
	store := pgcatalog.NewStore(database)
	input := catalogapp.ListModelsInput{ProjectID: projectID}

	for name, principal := range map[string]identityapp.Principal{
		"other organization":  otherActor,
		"forged organization": {ID: otherActor.ID, OrgID: orgID, Role: identitydomain.RoleProducer},
		"missing account":     {ID: uuid.New(), OrgID: orgID, Role: identitydomain.RoleProducer},
	} {
		t.Run(name, func(t *testing.T) {
			page, err := store.ListModelsForProject(ctx, principal, input)
			if err == nil || len(page.Models) != 0 {
				t.Fatalf("unauthorized actor read models: %+v: %v", page, err)
			}
		})
	}
	if err := database.WithContext(ctx).Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if page, err := store.ListModelsForProject(ctx, actor, input); err == nil || len(page.Models) != 0 {
		t.Fatalf("disabled account read models: %+v: %v", page, err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.project SET is_delete = true WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if page, err := store.ListModelsForProject(ctx, actor, input); err == nil || len(page.Models) != 0 {
		t.Fatalf("deleted project exposed catalog: %+v: %v", page, err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.project SET is_delete = false WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.organization SET status = 'disabled' WHERE id = ?::uuid`, orgID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if page, err := store.ListModelsForProject(ctx, actor, input); err == nil || len(page.Models) != 0 {
		t.Fatalf("disabled organization exposed catalog: %+v: %v", page, err)
	}
}
