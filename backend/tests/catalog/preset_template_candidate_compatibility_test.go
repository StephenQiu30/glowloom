package catalog_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestLegacyTemplateOwnsItsCandidateIdentityOnSQLKeyCollision(t *testing.T) {
	source := catalogfixture.BuiltinModelCatalog("volcengine_ark")
	selected, err := source.Select(source.Version, []string{"ark-chat"})
	require.NoError(t, err)
	sqlCandidate := selected[0]
	require.NotEmpty(t, sqlCandidate.MeteringStrategy)
	template := presetTextTemplate()
	template.Key, template.ProviderModelID = sqlCandidate.Key, sqlCandidate.ProviderModelID
	document := domain.PresetDocument{SchemaVersion: 1, Name: "Legacy template", Config: catalogfixture.BuiltinPresets()[0].Config, ModelTemplates: []domain.PresetModelTemplate{template}}
	require.NoError(t, presetvalidation.Validate(document))
	catalog := domain.ModelCatalogForPreset(document, source)
	chosen, err := catalog.Select(catalog.Version, []string{template.Key})
	require.NoError(t, err)
	require.NoError(t, chosen[0].ValidatePinnedVersion(template.Version(uuid.New(), uuid.New())), "an explicit legacy template must retain its own version contract")
	require.Equal(t, template.Name, chosen[0].DisplayName)
	require.Equal(t, template.ProviderModelID, chosen[0].ProviderModelID)
	require.Equal(t, template.Modes, chosen[0].Modes)
	require.Equal(t, template.UpstreamCost.Source, chosen[0].SourceURL)
	require.Empty(t, chosen[0].MeteringStrategy)
	require.Nil(t, chosen[0].Execution)
	require.Equal(t, len(source.Models), len(catalog.Models), "a same-key template replaces one projection instead of adding a duplicate")
	for i := range source.Models {
		require.Equal(t, source.Models[i].Key, catalog.Models[i].Key, "overriding a projection must preserve the existing directory order")
	}
	for _, candidate := range catalog.Models {
		if candidate.Key == template.Key {
			continue
		}
		original, err := source.Select(source.Version, []string{candidate.Key})
		require.NoError(t, err)
		require.Equal(t, original[0], candidate, "unselected SQL contracts keep their exact execution and meter")
		if candidate.MeteringStrategy == "" || candidate.RequiresModelSelection {
			continue
		}
		version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
		require.NoError(t, err)
		version.Limits = json.RawMessage(`{"max_outputs":1}`)
		require.Error(t, candidate.ValidatePinnedVersion(version), "SQL candidates still reject dropping their meter")
	}
}

func TestSelectedLegacyTemplateKeepsItsOwnSQLSnapshotAndVersion(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Template Compatibility','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "template-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	template := presetTextTemplate()
	template.Key, template.ProviderModelID = "ark-chat", "doubao-seed-2-1-pro-260915"
	document := domain.PresetDocument{SchemaVersion: 1, Name: "Legacy collision", Config: catalogfixture.BuiltinPresets()[0].Config, ModelTemplates: []domain.PresetModelTemplate{template}}
	service := catalogapp.NewProviderPresets(app.NewCatalogStore(conn.DB))
	preset, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: "template-" + uuid.NewString(), RequestID: uuid.New(), Document: document, PreviewHash: document.Digest()})
	require.NoError(t, err)
	created, err := service.Connect(t.Context(), actor, preset.ID, catalogapp.ConnectPresetInput{SelectedModelsOnly: true, VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{template.Key}, RequestID: uuid.New()})
	require.NoError(t, err)
	require.Len(t, created.ModelIDs, 1)
	var row struct {
		ProviderModelID                        string
		Limits, ParamSchema, CandidateSnapshot []byte
	}
	require.NoError(t, conn.DB.Raw(`SELECT v.provider_model_id,v.limits,v.param_schema,m.candidate_snapshot FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id WHERE m.id=?`, created.ModelIDs[0]).Scan(&row).Error)
	require.Equal(t, template.ProviderModelID, row.ProviderModelID)
	require.JSONEq(t, string(template.Limits), string(row.Limits))
	require.JSONEq(t, string(template.ParamSchema), string(row.ParamSchema))
	var snapshot domain.ModelCandidateSelection
	require.NoError(t, json.Unmarshal(row.CandidateSnapshot, &snapshot))
	require.Equal(t, template.Name, snapshot.Model.DisplayName)
	require.Empty(t, snapshot.Model.MeteringStrategy)
	require.Nil(t, snapshot.Model.Execution)
	var prices, releases int64
	require.NoError(t, conn.DB.Table("catalog.price_rule_version").Where("model_profile_id = ?", created.ModelIDs[0]).Count(&prices).Error)
	require.NoError(t, conn.DB.Table("catalog.model_release").Where("model_id = ?", created.ModelIDs[0]).Count(&releases).Error)
	require.Zero(t, prices)
	require.Zero(t, releases)
}
