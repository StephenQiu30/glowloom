package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestSQLModelPresetsRetainEveryCloudCandidateAndReplay(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database initialized with schema.sql and data.sql")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var rows []struct {
		Adapter string
		Count   int
	}
	query := `SELECT v.payload->'config'->>'adapter_key' AS adapter,count(*) AS count FROM catalog.model_preset m JOIN catalog.provider_preset_version v ON v.id=m.preset_version_id JOIN catalog.provider_preset p ON p.id=v.preset_id AND p.origin='builtin' WHERE v.version_no=(SELECT max(version_no) FROM catalog.provider_preset_version WHERE preset_id=p.id) GROUP BY adapter ORDER BY adapter`
	require.NoError(t, conn.DB.Raw(query).Scan(&rows).Error)
	expected := map[string]int{"volcengine_ark": 32, "openrouter": 23, "vercel": 23, "byteplus": 4, "deepseek": 2, "openai": 6, "volcengine_speech": 2, "volcengine_music": 1, "volcengine_avatar": 1, "volcengine_motion": 1}
	actual := map[string]int{}
	for _, row := range rows {
		actual[row.Adapter] = row.Count
	}
	require.Equal(t, expected, actual)
	var before string
	digest := `SELECT md5(string_agg(preset_version_id::text||model_key||content_hash,',' ORDER BY preset_version_id,model_key)) FROM catalog.model_preset`
	require.NoError(t, conn.DB.Raw(digest).Scan(&before).Error)
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "data.sql"))
	require.NoError(t, err)
	require.NoError(t, conn.DB.Exec(string(data)).Error)
	var after string
	require.NoError(t, conn.DB.Raw(digest).Scan(&after).Error)
	require.Equal(t, before, after)
}

func TestSQLModelPresetsDriveReadAndSelectionWithoutCompiledFallback(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'SQL Seed Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "seed-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	service := catalogapp.NewProviderPresets(app.NewCatalogStore(conn.DB))
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "SQL catalog", Config: catalogfixture.BuiltinPresets()[3].Config}
	preset, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: "sql-" + uuid.NewString(), RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	const payload = `{"metering_strategy":"token_io_v1","key":"sql-only","adapter_key":"deepseek","display_name":"Only in SQL","provider_model_id":"fixture/sql-only","capability":"text.generate","modes":["text_to_text"],"source_url":"https://example.com/model","reviewed_at":"2026-11-09","lifecycle":"unknown","requires_model_selection":false,"execution":{"limits":{"max_outputs":1},"param_schema":[],"expected_max_ms":120000,"supports_query":false}}`
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.model_preset(preset_version_id,model_key,ordinal,payload,content_hash) SELECT ?,'sql-only',100,?::jsonb,encode(sha256(convert_to((?::jsonb)::text,'UTF8')),'hex')`, preset.VersionID, payload, payload).Error)
	presets, err := service.List(t.Context(), actor)
	require.NoError(t, err)
	var catalog domain.ModelCandidateCatalog
	for _, p := range presets {
		if p.ID == preset.ID {
			catalog = p.ModelCatalog
		}
	}
	found := false
	for _, m := range catalog.Models {
		found = found || m.Key == "sql-only"
	}
	require.True(t, found, "catalog reads the persisted version, not a compiled list")
	receipt, err := service.Connect(t.Context(), actor, preset.ID, catalogapp.ConnectPresetInput{VersionID: preset.VersionID, CatalogVersion: catalog.Version, CandidateKeys: []string{"sql-only"}, SelectedModelsOnly: true, RequestID: uuid.New()})
	require.NoError(t, err)
	require.Len(t, receipt.ModelIDs, 1)
	var releases int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.model_release WHERE model_id=?`, receipt.ModelIDs[0]).Scan(&releases).Error)
	require.Zero(t, releases)

	validator, err := paramvalidation.NewValidator()
	require.NoError(t, err)
	publish := catalogapp.NewPublishModelVersionCommand(app.NewCatalogStore(conn.DB), validator, time.Now)
	_, err = publish.Execute(t.Context(), actor, catalogapp.PublishModelVersionInput{ModelID: receipt.ModelIDs[0], ExpectedRevision: 2, VersionNo: 2, ProviderModelID: "fixture/sql-only", Modes: []string{"text_to_text"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[]`), ExpectedMaxMS: 120000, Moderation: domain.ModerationProvider, Queue: "provider", RequestID: uuid.NewString()})
	require.Error(t, err, "a normal settings save cannot remove the pinned candidate metering contract")
	require.Error(t, conn.DB.Exec(`UPDATE catalog.model_preset SET payload=jsonb_set(payload,'{display_name}','"tampered"'),content_hash=encode(sha256(convert_to(jsonb_set(payload,'{display_name}','"tampered"')::text,'UTF8')),'hex') WHERE preset_version_id=? AND model_key='sql-only'`, preset.VersionID).Error)
}

func TestSQLSeedDoesNotBackfillAnUnrelatedFuturePresetVersion(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	future := uuid.New()
	require.NoError(t, tx.Exec(`INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) SELECT ?,preset_id,9001,payload,content_hash FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000004'`, future).Error)
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "data.sql"))
	require.NoError(t, err)
	require.NoError(t, tx.Exec(string(data)).Error)
	var count int64
	require.NoError(t, tx.Raw(`SELECT count(*) FROM catalog.model_preset WHERE preset_version_id=?`, future).Scan(&count).Error)
	require.Zero(t, count, "an existing seed belongs only to its explicitly declared immutable versions")
}

func TestCustomPresetReenablingCapabilityRestoresItsPinnedCandidates(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Capability Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "cap-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	service := catalogapp.NewProviderPresets(app.NewCatalogStore(conn.DB))
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "Restorable capabilities", Config: catalogfixture.BuiltinPresets()[0].Config}
	slug := "cap-" + uuid.NewString()
	first, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: slug, RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	doc.Config.Capabilities = []string{"text"}
	second, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{ID: first.ID, Slug: slug, ExpectedRevision: first.Revision, RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	for _, m := range second.ModelCatalog.Models {
		require.NotEqual(t, "image.generate", m.Capability)
	}
	doc.Config = catalogfixture.BuiltinPresets()[0].Config
	third, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{ID: first.ID, Slug: slug, ExpectedRevision: second.Revision, RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	found := false
	for _, m := range third.ModelCatalog.Models {
		found = found || m.Capability == "image.generate"
	}
	require.True(t, found, "temporarily hiding a capability must not erase its candidate snapshots")
}
