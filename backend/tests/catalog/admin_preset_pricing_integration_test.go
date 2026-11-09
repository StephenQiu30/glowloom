package catalog_test

import (
	"os"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

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

func TestPresetPricingReviewUsesEffectiveSalesRelease(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role)
		VALUES(?,?,?,'Pricing Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "pricing-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	presets := catalogapp.NewProviderPresets(store)
	document := domain.PresetDocument{SchemaVersion: 1, Name: "Pricing fixture",
		Config: catalogfixture.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
	preset, err := presets.Save(t.Context(), actor, catalogapp.SavePresetInput{
		Slug: "pricing-" + uuid.NewString(), RequestID: uuid.New(), Document: document, PreviewHash: document.Digest(),
	})
	require.NoError(t, err)
	receipt, err := presets.Connect(t.Context(), actor, preset.ID, catalogapp.ConnectPresetInput{VersionID: preset.VersionID, RequestID: uuid.New()})
	require.NoError(t, err)
	require.Len(t, receipt.ModelIDs, 1)
	id := receipt.ModelIDs[0]
	query := catalogapp.NewAdminModelsQuery(store)
	read := func(status string) []catalogapp.CreatedModel {
		page, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{
			ProviderID: receipt.ProviderID, Limit: 10, PricingReview: true, BillingStatus: status,
		})
		require.NoError(t, err)
		return page.Items
	}
	rows := read("price_pending")
	require.Len(t, rows, 1)
	require.Equal(t, id, rows[0].ID)
	require.False(t, rows[0].Billing.SalePublished)
	require.Empty(t, read("billable"))

	// Add a reviewed sales version linked to the procurement version. Merely
	// writing this version cannot make the disabled model appear billable.
	sale := uuid.New()
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.price_rule_version
		(id,model_profile_id,version_no,unit,rule,currency,fx_rate_to_cny,effective_from,source_url,reviewed_at)
		SELECT ?,model_profile_id,2,unit,jsonb_set(rule,'{procurement_price_version_id}',to_jsonb(id::text)),currency,fx_rate_to_cny,
		  effective_from,source_url,reviewed_at FROM catalog.price_rule_version WHERE model_profile_id=? AND version_no=1`, sale, id).Error)
	rows = read("usage_pending")
	require.Len(t, rows, 1)
	require.True(t, rows[0].Billing.ProcurementReady)
	require.True(t, rows[0].Billing.SaleReady)
	require.False(t, rows[0].Billing.SalePublished)
	require.Empty(t, read("price_pending"))
	var releaseID uuid.UUID
	require.NoError(t, conn.DB.Raw(`UPDATE catalog.model_release SET price_version_id=?,status='active',activated_at=now()
		WHERE model_id=? RETURNING id`, sale, id).Row().Scan(&releaseID))
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_profile SET active_release_id=?, status='active' WHERE id=?`, releaseID, id).Error)
	require.Len(t, read("inactive"), 1)
	require.Empty(t, read("billable"))
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider SET status='active' WHERE id=?`, receipt.ProviderID).Error)
	require.NoError(t, store.ReplaceCredential(t.Context(), validCredential(receipt.ProviderID)))
	rows = read("billable")
	require.Len(t, rows, 1)
	require.True(t, rows[0].Billing.SalePublished)

}
