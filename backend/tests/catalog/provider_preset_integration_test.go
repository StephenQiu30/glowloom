package catalog_test

import (
	"encoding/json"
	"os"
	"sync"
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

func TestPresetManagementTransactions(t *testing.T) {
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
	presets, err := service.List(t.Context(), actor)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(presets), 2)
	var base domain.Preset
	for _, p := range presets {
		if p.Slug == "openrouter" {
			base = p
		}
	}
	input := catalogapp.SavePresetInput{Slug: "custom-" + uuid.NewString(), ParentID: &base.ID, RequestID: uuid.New(), Document: base.Document, PreviewHash: base.Document.Digest()}
	var wg sync.WaitGroup
	results := make([]domain.Preset, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], errs[i] = service.Save(t.Context(), actor, input) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, results[0].ID, results[1].ID)
	custom := results[0]
	input.Document.Name = "changed"
	input.PreviewHash = input.Document.Digest()
	_, err = service.Save(t.Context(), actor, input)
	require.ErrorIs(t, err, catalogapp.ErrPresetConflict)
	connect := catalogapp.ConnectPresetInput{VersionID: custom.VersionID, Key: "connection-" + uuid.NewString(), Name: "Test", RequestID: uuid.New()}
	receipt, err := service.Connect(t.Context(), actor, custom.ID, connect)
	require.NoError(t, err)
	again, err := service.Connect(t.Context(), actor, custom.ID, connect)
	require.NoError(t, err)
	require.Equal(t, receipt, again)
	var status string
	require.NoError(t, conn.DB.Raw(`SELECT status FROM catalog.provider WHERE id=?`, receipt.ProviderID).Scan(&status).Error)
	require.Equal(t, "disabled", status)
	updated := custom.Document
	updated.Config.TimeoutSeconds = 60
	custom, err = service.Save(t.Context(), actor, catalogapp.SavePresetInput{ID: custom.ID, Slug: custom.Slug, ExpectedRevision: custom.Revision, RequestID: uuid.New(), Document: updated, PreviewHash: updated.Digest()})
	require.NoError(t, err)
	configs := catalogapp.NewProviderConfigs(app.NewCatalogStore(conn.DB))
	upgrade := catalogapp.ConfigUpgradeInput{ExpectedRevision: 1, ExpectedConfigID: receipt.Config.ID, PresetVersionID: custom.VersionID}
	preview, err := configs.Preview(t.Context(), actor, receipt.ProviderID, upgrade)
	require.NoError(t, err)
	require.Equal(t, 60, preview.Proposed.TimeoutSeconds)
	apply := catalogapp.ApplyConfigInput{Upgrade: upgrade, RequestID: uuid.New(), PreviewHash: preview.Hash, Resolution: preview.Proposed}
	upgraded, err := configs.Apply(t.Context(), actor, receipt.ProviderID, apply)
	require.NoError(t, err)
	require.NotEqual(t, receipt.Config.ID, upgraded.ID)
	replay, err := configs.Apply(t.Context(), actor, receipt.ProviderID, apply)
	require.NoError(t, err)
	require.Equal(t, upgraded, replay)
	apply.RequestID = uuid.New()
	_, err = configs.Apply(t.Context(), actor, receipt.ProviderID, apply)
	require.ErrorIs(t, err, catalogapp.ErrPresetConflict)

	_, err = service.SetState(t.Context(), actor, custom.ID, catalogapp.PresetStateInput{ExpectedRevision: custom.Revision, RequestID: uuid.New(), Archived: true})
	require.NoError(t, err)
	connect.RequestID = uuid.New()
	_, err = service.Connect(t.Context(), actor, custom.ID, connect)
	require.ErrorIs(t, err, catalogapp.ErrPresetUnavailable)
	var count int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.provider_config_version WHERE id=?`, receipt.Config.ID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	encoded, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "ciphertext")
	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	_, err = service.List(t.Context(), actor)
	require.ErrorIs(t, err, identityapp.ErrForbidden)
}
