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

func TestOpenAITextAndImageCanCreateReleases(t *testing.T) {
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
		if p.Slug == "openai" {
			preset = p
		}
	}
	require.NotEqual(t, uuid.Nil, preset.ID)
	connection, err := presets.Connect(ctx, actor, preset.ID, catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{"openai-sol", "openai-image-sunburst"}, DirectUse: true})
	require.NoError(t, err)
	seedReviewedOnboardingPrices(t, database, actor, connection, false)
	state, err := catalogapp.NewProviderOnboarding(store).Read(ctx, actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, state.Models, 2)
	for _, model := range state.Models {
		t.Run(model.Capability, func(t *testing.T) {
			var price struct{ ID uuid.UUID }
			require.NoError(t, database.Raw(`SELECT id FROM catalog.price_rule_version WHERE model_profile_id=?`, model.ID).Scan(&price).Error)
			release, err := catalogapp.NewModelReleases(store).Create(ctx, actor, model.ID, catalogapp.CreateReleaseInput{ModelVersionID: model.VersionID, PriceVersionID: price.ID, ConfigVersionID: connection.Config.ID, RequestID: uuid.New(), UpstreamCost: catalogapp.UpstreamCost{Currency: "USD", MaxMicros: 100000, Source: "fixture only, not supplier pricing", ReviewedAt: time.Now()}})
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, release.ID)
			require.Empty(t, release.ValidatedModes)
		})
	}
}
