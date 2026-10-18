package catalog_test

import (
	"os"

	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestPresetModelsInstantiateAtomicallyWithoutCopyingPublication(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Preset Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "preset-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	service := catalogapp.NewProviderPresets(app.NewCatalogStore(conn.DB))
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "Complete OpenRouter", Config: domain.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
	preset, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: "models-" + uuid.NewString(), RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	input := catalogapp.ConnectPresetInput{VersionID: preset.VersionID, RequestID: uuid.New()}
	receipt, err := service.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Len(t, receipt.ModelIDs, 1)
	again, err := service.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Equal(t, receipt, again)
	var snapshot struct {
		ProviderStatus, Name, Key, ModelStatus, ReleaseStatus string
		CredentialCount, ValidationCount, PriceCount          int64
	}
	require.NoError(t, conn.DB.Raw(`SELECT p.status AS provider_status,p.name,p.key,m.status AS model_status,r.status AS release_status,
 (SELECT count(*) FROM catalog.provider_credential WHERE provider_id=p.id) AS credential_count,
 (SELECT count(*) FROM catalog.model_validation WHERE release_id=r.id) AS validation_count,
 (SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id=m.id) AS price_count
 FROM catalog.provider p JOIN catalog.model_profile m ON m.provider_id=p.id JOIN catalog.model_release r ON r.model_id=m.id WHERE p.id=?`, receipt.ProviderID).Scan(&snapshot).Error)
	require.Equal(t, "disabled", snapshot.ProviderStatus)
	require.Equal(t, "disabled", snapshot.ModelStatus)
	require.Equal(t, "draft", snapshot.ReleaseStatus)
	require.Contains(t, snapshot.Name, doc.Name)
	require.Equal(t, "connection-"+input.RequestID.String(), snapshot.Key)
	require.Zero(t, snapshot.CredentialCount)
	require.Zero(t, snapshot.ValidationCount)
	require.EqualValues(t, 1, snapshot.PriceCount)
	// A failure after the first model insert must roll back the whole connection.
	broken := doc
	broken.ModelTemplates = append([]domain.PresetModelTemplate{}, doc.ModelTemplates...)
	second := presetTextTemplate()
	second.Key = "reject"
	second.Name = "preset-reject-" + uuid.NewString()
	broken.ModelTemplates = append(broken.ModelTemplates, second)
	failedPreset, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: "failed-" + uuid.NewString(), RequestID: uuid.New(), Document: broken, PreviewHash: broken.Digest()})
	require.NoError(t, err)
	trigger := "preset_fail_" + uuid.New().String()[:8]
	require.NoError(t, conn.DB.Exec(`CREATE FUNCTION catalog.`+trigger+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.display_name='`+second.Name+`' THEN RAISE EXCEPTION 'fixture fault'; END IF; RETURN NEW; END $$`).Error)
	require.NoError(t, conn.DB.Exec(`CREATE TRIGGER `+trigger+` BEFORE INSERT ON catalog.model_profile FOR EACH ROW EXECUTE FUNCTION catalog.`+trigger+`()`).Error)
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS ` + trigger + ` ON catalog.model_profile`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS catalog.` + trigger + `()`).Error
	})
	before := int64(0)
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.provider`).Scan(&before).Error)
	_, err = service.Connect(t.Context(), actor, failedPreset.ID, catalogapp.ConnectPresetInput{VersionID: failedPreset.VersionID, RequestID: uuid.New()})
	require.Error(t, err)
	var after int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.provider`).Scan(&after).Error)
	require.Equal(t, before, after)
}
