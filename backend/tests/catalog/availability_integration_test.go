package catalog_test

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

// This test seeds completed evidence only in its explicitly isolated database.
// It never invokes an adapter, generates media, or charges a supplier.
func TestAvailabilityFiltersBeforePaginationAndInvalidatesEvidence(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Availability admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "availability-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	connection, err := catalogapp.NewProviderPresets(store).Connect(t.Context(), actor, uuid.MustParse("6da07a10-0000-4000-8000-000000000002"), catalogapp.ConnectPresetInput{VersionID: uuid.MustParse("6da07a11-0000-4000-8000-000000000002"), Key: "availability-" + uuid.NewString(), Name: "Availability", RequestID: uuid.New()})
	require.NoError(t, err)
	credential, err := newCredentialCommand(t, store).Execute(t.Context(), actor, catalogapp.SetCredentialInput{ProviderID: connection.ProviderID, Label: "synthetic", Secret: json.RawMessage(`{"api_key":"fixture-only-availability-key"}`), RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider SET status='active' WHERE id=?`, connection.ProviderID).Error)
	org := insertModelCatalogOrganization(t.Context(), t, conn.DB)
	creator := insertModelCatalogActor(t.Context(), t, conn.DB, org)
	project := insertModelCatalogProject(t.Context(), t, conn.DB, creator, true)
	prefix := "availability-" + uuid.NewString()
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status)
 SELECT gen_random_uuid(),? || '-a-' || lpad(n::text,3,'0'),?,'video.generate','Unavailable fixture','disabled' FROM generate_series(1,205) n`, prefix, connection.ProviderID).Error)
	modelIDs := []uuid.UUID{}
	releaseIDs := []uuid.UUID{}
	for _, suffix := range []string{"z1", "z2"} {
		modelID, versionID := insertModelCatalogModel(t.Context(), t, conn.DB, prefix+"-"+suffix, connection.ProviderID, "video.generate", "active", "text2video", false)
		require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video','image2video'],queue='provider' WHERE id=?`, versionID).Error)
		purchase, sale, release := uuid.New(), uuid.New(), uuid.New()
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) VALUES(?,?,1,'per_request','{"base_micros":1000}','CNY',now()-interval '1 hour','https://example.invalid/prices',now()-interval '2 hour')`, purchase, modelID).Error)
		rule := `{"base_micros":2000,"procurement_price_version_id":"` + purchase.String() + `"}`
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) VALUES(?,?,2,'per_request',?::jsonb,'CNY',now()-interval '1 hour','https://example.invalid/prices',now()-interval '2 hour')`, sale, modelID, rule).Error)
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,status,upstream_cost) VALUES(?,?,?,?,?,'active','{}')`, release, modelID, versionID, sale, connection.Config.ID).Error)
		require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_profile SET active_release_id=? WHERE id=?`, release, modelID).Error)
		seedOnboardingModeEvidence(t, conn.DB, project, credential.ID, catalogapp.OnboardingReleaseReview{ModelRelease: catalogapp.ModelRelease{ID: release}}, "text2video")
		modelIDs = append(modelIDs, modelID)
		releaseIDs = append(releaseIDs, release)
	}
	query := catalogapp.NewAdminModelsQuery(store)
	first, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{Limit: 1, ProviderID: connection.ProviderID, Status: "available"})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Next)
	require.Equal(t, modelIDs[0], first.Items[0].ID)
	require.True(t, first.Items[0].Available)
	second, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{Limit: 1, ProviderID: connection.ProviderID, Status: "available", After: first.Next})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, modelIDs[1], second.Items[0].ID)
	require.Nil(t, second.Next)
	exact, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{Limit: 1, Name: modelIDs[0].String(), Status: "available"})
	require.NoError(t, err)
	require.Len(t, exact.Items, 1)
	require.Equal(t, modelIDs[0], exact.Items[0].ID)
	detail, err := query.Detail(t.Context(), actor, modelIDs[0])
	require.NoError(t, err)
	require.True(t, detail.Model.Available)
	require.False(t, detail.Model.AvailableForMode("image2video"))
	public, err := store.ListModelsForProject(t.Context(), creator, catalogapp.ListModelsInput{ProjectID: project, ModelKey: prefix + "-z1", Mode: "image2video", Limit: 1})
	require.NoError(t, err)
	require.Empty(t, public.Models)
	public, err = store.ListModelsForProject(t.Context(), creator, catalogapp.ListModelsInput{ProjectID: project, ModelKey: prefix + "-z1", Mode: "text2video", Limit: 1})
	require.NoError(t, err)
	require.Len(t, public.Models, 1)
	providers, err := catalogapp.NewListProvidersQuery(store).Execute(t.Context(), actor, catalogapp.ListProvidersInput{Name: "Availability", Status: "available", Limit: 1})
	require.NoError(t, err)
	require.Len(t, providers.Providers, 1)
	require.Equal(t, 2, providers.Providers[0].AvailableModelCount)
	require.NoError(t, conn.DB.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,1000000)`, uuid.New(), project).Error)
	operations := operationwire.ProvideOperationStore(conn.DB)
	quoteInput := operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: prefix + "-z1", Capability: "video.generate", Mode: "text2video", Prompt: "Synthetic availability quote", Params: json.RawMessage(`{}`), OutputCount: 1}}
	newQuote := func() operationapp.CreateFreeQuoteResult {
		t.Helper()
		quoteInput.RequestID = uuid.NewString()
		quote, err := operations.CreateFreeQuote(t.Context(), creator, quoteInput)
		require.NoError(t, err)
		return quote
	}
	assertClosed := func(quote operationapp.CreateFreeQuoteResult) {
		t.Helper()
		quoteInput.RequestID = uuid.NewString()
		_, err := operations.CreateFreeQuote(t.Context(), creator, quoteInput)
		require.ErrorIs(t, err, operationapp.ErrFreeQuoteModelMissing)
		_, err = operations.ConfirmSingleQuote(t.Context(), creator, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: uuid.NewString()})
		require.True(t, errors.Is(err, operationapp.ErrQuoteStale), "confirmation should reject stale availability, got %v", err)
	}
	invalidMode := quoteInput
	invalidMode.RequestID = uuid.NewString()
	invalidMode.Mode = "image2video"
	_, err = operations.CreateFreeQuote(t.Context(), creator, invalidMode)
	require.ErrorIs(t, err, operationapp.ErrFreeQuoteModelMissing)
	for _, change := range []struct {
		name, sql, undo string
		args            []any
	}{
		{"retired", `UPDATE catalog.model_release SET status='retired' WHERE id IN ?`, `UPDATE catalog.model_release SET status='active' WHERE id IN ?`, []any{releaseIDs}},
		{"price", `UPDATE catalog.price_rule_version SET is_delete=true WHERE model_profile_id IN ?`, `UPDATE catalog.price_rule_version SET is_delete=false WHERE model_profile_id IN ?`, []any{modelIDs}},
		{"configuration", `UPDATE catalog.provider SET current_config_version_id=NULL WHERE id IN ?`, `UPDATE catalog.provider SET current_config_version_id='` + connection.Config.ID.String() + `' WHERE id IN ?`, []any{[]uuid.UUID{connection.ProviderID}}},
		{"emergency", `UPDATE catalog.provider_preset SET emergency_disabled=true WHERE id IN (SELECT pv.preset_id FROM catalog.provider_config_version cfg JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id WHERE cfg.id IN ?)`, `UPDATE catalog.provider_preset SET emergency_disabled=false WHERE id IN (SELECT pv.preset_id FROM catalog.provider_config_version cfg JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id WHERE cfg.id IN ?)`, []any{[]uuid.UUID{connection.Config.ID}}},
		{"driver_contract", `UPDATE catalog.model_profile_version SET supports_cancel=true WHERE model_profile_id IN ?`, `UPDATE catalog.model_profile_version SET supports_cancel=false WHERE model_profile_id IN ?`, []any{modelIDs}},
		{"credential", `UPDATE catalog.provider_credential SET last_test_result='auth_failed' WHERE id IN ?`, `UPDATE catalog.provider_credential SET last_test_result='ok' WHERE id IN ?`, []any{[]uuid.UUID{credential.ID}}},
	} {
		t.Run(change.name, func(t *testing.T) {
			quote := newQuote()
			require.NoError(t, conn.DB.Exec(change.sql, change.args...).Error)
			assertClosed(quote)
			page, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{Limit: 1, ProviderID: connection.ProviderID, Status: "available"})
			require.NoError(t, err)
			require.Empty(t, page.Items)
			require.NoError(t, conn.DB.Exec(change.undo, change.args...).Error)
		})
	}
	rotationQuote := newQuote()
	rotated, err := newCredentialCommand(t, store).Execute(t.Context(), actor, catalogapp.SetCredentialInput{ProviderID: connection.ProviderID, Label: "rotated", Secret: json.RawMessage(`{"api_key":"fixture-only-rotated-key"}`), RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NotEqual(t, credential.ID, rotated.ID)
	assertClosed(rotationQuote)
	afterRotation, err := query.List(t.Context(), actor, catalogapp.AdminModelListInput{Limit: 1, ProviderID: connection.ProviderID, Status: "available"})
	require.NoError(t, err)
	require.Empty(t, afterRotation.Items)
}
