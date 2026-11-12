package catalog_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestSeedanceTokenPriceCanCreateDraftRelease(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, org)
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, database, actor)
	store := app.NewCatalogStore(database)
	presets := catalogapp.NewProviderPresets(store)
	list, err := presets.List(ctx, actor)
	require.NoError(t, err)
	var preset domain.Preset
	for _, p := range list {
		if p.Slug == "volcengine-ark" {
			preset = p
		}
	}
	connection, err := presets.Connect(ctx, actor, preset.ID, catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{"ark-video"}, DirectUse: true})
	require.NoError(t, err)
	seedReviewedOnboardingPrices(t, database, actor, connection, false)
	state, err := catalogapp.NewProviderOnboarding(store).Read(ctx, actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, state.Models, 1)
	model := state.Models[0]
	var price struct{ ID uuid.UUID }
	require.NoError(t, database.Raw(`SELECT id FROM catalog.price_rule_version WHERE model_profile_id=? ORDER BY version_no DESC LIMIT 1`, model.ID).Scan(&price).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":70,"720p":70,"1080p":77}}' WHERE id=?`, price.ID).Error)
	input := catalogapp.CreateReleaseInput{ModelVersionID: model.VersionID, PriceVersionID: price.ID, ConfigVersionID: connection.Config.ID, RequestID: uuid.New(), UpstreamCost: catalogapp.UpstreamCost{Currency: "CNY", MaxMicros: 5000000, Source: "fixture only", ReviewedAt: time.Now()}}
	release, err := catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, input)
	require.NoError(t, err)
	require.Equal(t, "draft", release.Status)
	require.Empty(t, release.ValidatedModes)
	// Video tariffs cannot silently charge prompt tokens like a text model.
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET rule=jsonb_set(rule,'{input_micros_per_1k}','1') WHERE id=?`, price.ID).Error)
	input.RequestID = uuid.New()
	_, err = catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, input)
	require.Error(t, err)
}

func TestSeedanceReleaseRejectsIncompletePriceMatrix(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, org)
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, database, actor)
	store := app.NewCatalogStore(database)
	presets := catalogapp.NewProviderPresets(store)
	list, err := presets.List(ctx, actor)
	require.NoError(t, err)
	var preset domain.Preset
	for _, p := range list {
		if p.Slug == "volcengine-ark" {
			preset = p
		}
	}
	connection, err := presets.Connect(ctx, actor, preset.ID, catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{"ark-video-fast"}, DirectUse: true})
	require.NoError(t, err)
	seedReviewedOnboardingPrices(t, database, actor, connection, false)
	state, err := catalogapp.NewProviderOnboarding(store).Read(ctx, actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, state.Models, 1)
	model := state.Models[0]
	var price struct{ ID uuid.UUID }
	require.NoError(t, database.Raw(`SELECT id FROM catalog.price_rule_version WHERE model_profile_id=? ORDER BY version_no DESC LIMIT 1`, model.ID).Scan(&price).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"pricing_strategy":"seedance_video_tokens_v1","input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":37}}' WHERE id=?`, price.ID).Error)
	input := catalogapp.CreateReleaseInput{ModelVersionID: model.VersionID, PriceVersionID: price.ID, ConfigVersionID: connection.Config.ID, RequestID: uuid.New(), UpstreamCost: catalogapp.UpstreamCost{Currency: "CNY", MaxMicros: 5000000, Source: "fixture only", ReviewedAt: time.Now()}}
	_, err = catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, input)
	require.Error(t, err, "720p is published but has no tariff")
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET rule=jsonb_set(rule,'{by_resolution,720p}','37',true) WHERE id=?`, price.ID).Error)
	procurement := uuid.New()
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET
		rule=jsonb_set(rule,'{procurement_price_version_id}',to_jsonb(?::text)),source_url='https://example.invalid/retail',reviewed_at=now()
		WHERE id=?`, procurement, price.ID).Error)
	input.RequestID = uuid.New()
	_, err = catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, input)
	require.Error(t, err, "unpublished procurement reference")
	require.NoError(t, database.Exec(`INSERT INTO catalog.price_rule_version
		(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at)
		SELECT ?,model_profile_id,version_no+1,unit,'{"input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":30,"720p":30}}',
		'CNY',effective_from,'https://example.invalid/procurement',now()
		FROM catalog.price_rule_version WHERE id=?`, procurement, price.ID).Error)
	input.RequestID = uuid.New()
	release, err := catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, input)
	require.NoError(t, err)
	require.Equal(t, "draft", release.Status)
	require.Empty(t, release.ValidatedModes)
}
