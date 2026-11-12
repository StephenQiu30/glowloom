package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func modelCatalogDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_MODEL_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_MODEL_CATALOG_DB_DSN to an isolated migrated database")
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

	actor = identityfixture.Session(t, database, actor)
	return actor
}

func insertModelCatalogProject(ctx context.Context, t *testing.T, database *gorm.DB, actor identityapp.Principal, allowOverseas bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.project
		  (id, org_id, owner_id, owner_scope_id, name, aspect_ratio, style_type, allow_overseas_models)
		VALUES (?::uuid, ?::uuid, ?::uuid, ?::uuid, '模型选择', '16:9', 'realistic', ?)
	`, id.String(), actor.OrgID.String(), actor.ID.String(), actor.ScopeID.String(), allowOverseas).Error; err != nil {
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

// This projection includes only published, reviewed and validated models.
// The receipts below are synthetic SQL fixtures, not supplier acceptance.
func TestModelCatalogReadsProjectPolicyCurrentVersionsAndEffectivePricesOnLocalPostgres(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	actor := insertModelCatalogActor(ctx, t, database, insertModelCatalogOrganization(ctx, t, database))
	domesticProject := insertModelCatalogProject(ctx, t, database, actor, false)
	overseasProject := insertModelCatalogProject(ctx, t, database, actor, true)
	prefix := "model-catalog-" + uuid.NewString()
	domestic := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "ok")
	overseas := insertModelCatalogProvider(ctx, t, database, "overseas", "active", "ok")
	active, firstVersion := insertModelCatalogModel(ctx, t, database, prefix+"-a", domestic, "video.generate", "active", "image2video", false)
	foreign, _ := insertModelCatalogModel(ctx, t, database, prefix+"-b", overseas, "video.generate", "active", "image2video", false)
	_, _ = insertModelCatalogModel(ctx, t, database, prefix+"-c", domestic, "video.generate", "disabled", "image2video", false)
	_, _ = insertModelCatalogModel(ctx, t, database, prefix+"-d", domestic, "video.generate", "active", "image2video", false)
	past := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	price := insertModelCatalogPrice(ctx, t, database, active, 1, past)
	newer := insertModelCatalogPrice(ctx, t, database, active, 2, past)
	insertModelCatalogPrice(ctx, t, database, active, 3, time.Now().Add(time.Hour))
	foreignPrice := insertModelCatalogPrice(ctx, t, database, foreign, 1, past)
	// A newer installed version supplies public limits and parameters only after
	// the published release and validation point to that exact version.
	current := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms,queue)
 VALUES(?,?,2,'test-model-v2',ARRAY['image2video'],'{"max_outputs":1,"roles":{"subject":{"max_count":2,"types":["image"]}}}','[{"field":"seed","label":"随机种子","type":"integer","component":"input"}]',true,false,false,40000,'provider')`, current, active).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, current, active).Error)
	release, credential := seedCatalogProjectionRelease(t, database, domesticProject, active, newer)
	seedCatalogProjectionRelease(t, database, overseasProject, foreign, foreignPrice)
	store := app.NewCatalogStore(database)
	query := catalogapp.ListModelsInput{ProjectID: domesticProject, ModelKey: prefix + "-a", Mode: "image2video", Limit: 1}
	page, err := store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Len(t, page.Models, 1)
	require.Nil(t, page.Next)
	item := page.Models[0]
	require.Equal(t, active, item.ID)
	require.Equal(t, domestic, item.ProviderID)
	require.Equal(t, current, item.CurrentVersion.ID)
	require.Equal(t, 2, item.CurrentVersion.VersionNo)
	require.True(t, slices.Equal([]string{"image2video"}, item.CurrentVersion.Modes))
	require.Equal(t, newer, item.CurrentPrice.ID)
	require.True(t, past.Equal(item.CurrentPrice.EffectiveFrom))
	var limits struct {
		MaxOutputs int `json:"max_outputs"`
	}
	require.NoError(t, json.Unmarshal(item.CurrentVersion.Limits, &limits))
	require.Equal(t, 1, limits.MaxOutputs)
	require.Contains(t, string(item.CurrentVersion.ParamSchema), `"seed"`)
	require.NotEqual(t, price, item.CurrentPrice.ID)
	encoded, err := json.Marshal(page.Models)
	require.NoError(t, err)
	for _, secret := range []string{"ciphertext", "key_id", "model-catalog-private"} {
		require.NotContains(t, string(encoded), secret)
	}
	// Disabled, unpriced, unvalidated and foreign models never become selectable.
	for _, key := range []string{prefix + "-b", prefix + "-c", prefix + "-d"} {
		query.ModelKey = key
		page, err = store.ListModelsForProject(ctx, actor, query)
		require.NoError(t, err)
		require.Empty(t, page.Models)
	}
	query.ProjectID = overseasProject
	query.ModelKey = prefix + "-b"
	page, err = store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Len(t, page.Models, 1)
	require.Equal(t, foreign, page.Models[0].ID)
	query.ModelKey = prefix + "-a"
	query.Mode = "text2video"
	page, err = store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Empty(t, page.Models)
	query.Mode = "image2video"
	require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET last_test_result='auth_failed' WHERE id=?`, credential).Error)
	page, err = store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Empty(t, page.Models)
	require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET last_test_result='ok' WHERE id=?`, credential).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, firstVersion, active).Error)
	page, err = store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Empty(t, page.Models)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, current, active).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='retired' WHERE id=?`, release).Error)
	page, err = store.ListModelsForProject(ctx, actor, query)
	require.NoError(t, err)
	require.Empty(t, page.Models)
}

func seedCatalogProjectionRelease(t *testing.T, db *gorm.DB, project, model, price uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var row struct {
		Provider, Version, Credential uuid.UUID
		Modes                         []byte
	}
	require.NoError(t, db.Raw(`SELECT m.provider_id AS provider,m.current_version_id AS version,c.id AS credential,to_jsonb(v.modes) AS modes FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id JOIN catalog.provider_credential c ON c.provider_id=m.provider_id WHERE m.id=?`, model).Scan(&row).Error)
	config, release, purchase := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) SELECT ?,?,id,1,payload->'config',content_hash FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000002'`, config, row.Provider).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='openrouter',current_config_version_id=? WHERE id=?`, config, row.Provider).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET queue='provider',supports_cancel=false WHERE id=?`, row.Version).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) SELECT ?,model_profile_id,100,unit,rule,currency,effective_from-interval '1 hour','https://example.invalid/catalog-procurement',now()-interval '1 hour' FROM catalog.price_rule_version WHERE id=?`, purchase, price).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET rule=jsonb_set(rule,'{procurement_price_version_id}',to_jsonb(?::text)),source_url='https://example.invalid/catalog-retail',reviewed_at=now()-interval '1 hour' WHERE id=?`, purchase, price).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,status,upstream_cost) VALUES(?,?,?,?,?,'active','{}')`, release, model, row.Version, price, config).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET active_release_id=? WHERE id=?`, release, model).Error)
	var modes []string
	require.NoError(t, json.Unmarshal(row.Modes, &modes))
	catalogfixture.SeedValidationEvidence(t, db, project, release, row.Credential, modes...)
	return release, row.Credential
}

func TestModelCatalogRejectsStaleActorAndInvisibleProjectOnLocalPostgres(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	orgID := insertModelCatalogOrganization(ctx, t, database)
	otherOrgID := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, orgID)
	otherActor := insertModelCatalogActor(ctx, t, database, otherOrgID)
	team, scope := uuid.New(), uuid.New()
	for _, row := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Catalog fixture team',?)`, []any{team, actor.ID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, []any{team, actor.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, actor.OrgID}},
	} {
		if err := database.WithContext(ctx).Exec(row.sql, row.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor.ScopeID = scope
	projectID := insertModelCatalogProject(ctx, t, database, actor, false)
	store := app.NewCatalogStore(database)
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
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.team SET status = 'suspended', resume_status = 'active' WHERE id = ?::uuid`, team.String()).Error; err != nil {
		t.Fatal(err)
	}
	if page, err := store.ListModelsForProject(ctx, actor, input); err == nil || len(page.Models) != 0 {
		t.Fatalf("suspended team exposed catalog: %+v: %v", page, err)
	}
}
