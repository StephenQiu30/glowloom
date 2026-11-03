package catalog_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestAdapterDeclarationImmutableVersionsAndAtomicFailure(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: org, PlatformRole: "platform_admin"}
	require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Declaration Admin','producer','synthetic',false,'platform_admin')`, actor.ID, org, "declaration-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, database, actor)
	service := application.NewDeclaredProviderPresets(app.NewCatalogStore(database), installedProtocol)
	doc := declarationPreset(t)
	doc.ModelTemplates = append(doc.ModelTemplates, presetTextTemplate())
	slug := "declaration-" + uuid.NewString()
	first, err := service.Save(ctx, actor, application.SavePresetInput{Slug: slug, RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	connection, err := service.Connect(ctx, actor, first.ID, application.ConnectPresetInput{VersionID: first.VersionID, RequestID: uuid.New()})
	require.NoError(t, err)
	originalConfig := connection.Config
	secondDoc := doc
	secondDoc.Name = "Updated declaration"
	secondDoc.Config.TimeoutSeconds = 180
	second, err := service.Save(ctx, actor, application.SavePresetInput{ID: first.ID, Slug: slug, ExpectedRevision: first.Revision, RequestID: uuid.New(), Document: secondDoc, PreviewHash: secondDoc.Digest()})
	require.NoError(t, err)
	require.NotEqual(t, first.VersionID, second.VersionID)
	var snapshot struct{ Payload []byte }
	require.NoError(t, database.Raw(`SELECT payload FROM catalog.provider_preset_version WHERE id=?`, first.VersionID).Scan(&snapshot).Error)
	var frozen map[string]any
	require.NoError(t, json.Unmarshal(snapshot.Payload, &frozen))
	require.Equal(t, doc.Name, frozen["name"])
	var frozenConfig struct{ PresetVersionID uuid.UUID }
	require.NoError(t, database.Raw(`SELECT preset_version_id FROM catalog.provider_config_version WHERE id=?`, originalConfig.ID).Scan(&frozenConfig).Error)
	require.Equal(t, first.VersionID, frozenConfig.PresetVersionID)
	var verified int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM catalog.model_validation v JOIN catalog.model_release r ON r.id=v.release_id JOIN catalog.model_profile m ON m.id=r.model_id WHERE m.provider_id=?`, connection.ProviderID).Scan(&verified).Error)
	require.Zero(t, verified)
	// Stale writes leave neither a new version nor a changed document.
	_, err = service.Save(ctx, actor, application.SavePresetInput{ID: first.ID, Slug: slug, ExpectedRevision: first.Revision, RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.ErrorIs(t, err, application.ErrPresetConflict)
	var count int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM catalog.provider_preset_version WHERE preset_id=?`, first.ID).Scan(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestAdapterMetadataRechecksRevokedPlatformRole(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: org, PlatformRole: "platform_admin"}
	require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Metadata Admin','producer','synthetic',false,'platform_admin')`, actor.ID, org, "metadata-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, database, actor)
	query := application.NewAdapterMetadataQuery(app.NewCatalogStore(database), credentialschema.NewRegistry(), installedProtocol)
	items, err := query.List(ctx, actor)
	require.NoError(t, err)
	require.NotEmpty(t, items)
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	_, err = query.List(ctx, actor)
	require.Error(t, err)
}

func TestAdapterMetadataHTTPCurrentSessionEpochAndOrganizationMatrix(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	create := func(role string) identityapp.Principal {
		org := insertModelCatalogOrganization(ctx, t, database)
		actor := identityapp.Principal{ID: uuid.New(), OrgID: org, PlatformRole: role}
		require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Metadata matrix','producer','synthetic',false,?)`, actor.ID, org, "matrix-"+actor.ID.String(), role).Error)
		return identityfixture.Session(t, database, actor)
	}
	request := func(actor identityapp.Principal) int {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("principal", actor) })
		cataloghttp.NewAdminHandler(cataloghttp.AdminDependencies{Adapters: application.NewAdapterMetadataQuery(app.NewCatalogStore(database), credentialschema.NewRegistry(), installedProtocol)}).Register(router.Group("/api"))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequestWithContext(ctx, "GET", "/api/admin/provider-adapters", nil))
		return res.Code
	}
	admin := create("platform_admin")
	require.Equal(t, 200, request(admin))
	otherAdmin := create("platform_admin")
	require.Equal(t, 200, request(otherAdmin), "global metadata remains available across organizations")
	creator := create("creator")
	require.Equal(t, 403, request(creator))
	mixed := admin
	mixed.SessionID = otherAdmin.SessionID
	require.Equal(t, 401, request(mixed))
	require.NoError(t, database.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, admin.SessionID).Error)
	require.Equal(t, 401, request(admin))
	admin = create("platform_admin")
	require.NoError(t, database.Exec(`UPDATE identity."user" SET session_epoch=session_epoch+1 WHERE id=?`, admin.ID).Error)
	require.Equal(t, 401, request(admin))
	admin = create("platform_admin")
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, admin.ID).Error)
	require.Equal(t, 403, request(admin))
}

func TestPresetConnectionRejectsMissingPriceProvenance(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: org, PlatformRole: "platform_admin"}
	require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Preset provenance','producer','synthetic',false,'platform_admin')`, actor.ID, org, "provenance-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, database, actor)
	service := application.NewDeclaredProviderPresets(app.NewCatalogStore(database), installedProtocol)
	doc := declarationPreset(t)
	doc.ModelTemplates = []domain.PresetModelTemplate{presetTextTemplate()}
	// Historical portable documents can be read; instantiating a new price must fail closed.
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	price := fields["model_templates"].([]any)[0].(map[string]any)["price"].(map[string]any)
	delete(price, "source_url")
	delete(price, "reviewed_at")
	encoded, err = json.Marshal(fields)
	require.NoError(t, err)
	doc = domain.PresetDocument{}
	require.NoError(t, json.Unmarshal(encoded, &doc))
	saved, err := service.Save(ctx, actor, application.SavePresetInput{Slug: "provenance-" + uuid.NewString(), RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.NoError(t, err)
	_, err = service.Connect(ctx, actor, saved.ID, application.ConnectPresetInput{VersionID: saved.VersionID, RequestID: uuid.New()})
	require.Error(t, err, "preset bypassed price source publication gate")
}

func TestDirectUseCandidateDoesNotPublishUnknownZeroPrice(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, org)
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, database, actor)
	service := application.NewProviderPresets(app.NewCatalogStore(database))
	presets, err := service.List(ctx, actor)
	require.NoError(t, err)
	var preset domain.Preset
	for _, p := range presets {
		if p.Slug == "volcengine-ark" {
			preset = p
		}
	}
	connected, err := service.Connect(ctx, actor, preset.ID, application.ConnectPresetInput{VersionID: preset.VersionID, RequestID: uuid.New(), DirectUse: true, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{"ark-chat"}})
	require.NoError(t, err)
	var prices, releases int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id IN ?`, connected.ModelIDs).Scan(&prices).Error)
	require.Zero(t, prices, "candidate documentation is not a reviewed price source")
	require.NoError(t, database.Raw(`SELECT count(*) FROM catalog.model_release WHERE model_id IN ?`, connected.ModelIDs).Scan(&releases).Error)
	require.Zero(t, releases)
	state, err := application.NewProviderOnboarding(app.NewCatalogStore(database)).Read(ctx, actor, connected.ProviderID)
	require.NoError(t, err)
	require.Len(t, state.Models, 1)
	require.Equal(t, "disabled", state.Status)
	detail, err := app.NewCatalogStore(database).ReadAdminModel(ctx, actor, state.Models[0].ID)
	require.NoError(t, err)
	require.Empty(t, detail.Prices)
	require.Len(t, detail.Versions, 1)
}
