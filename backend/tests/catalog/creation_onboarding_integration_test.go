package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestCreationCandidatesPersistWithCostAndStayDisabled(t *testing.T) {
	for _, slug := range []string{"volcengine-music", "volcengine-avatar", "volcengine-motion", "volcengine-speech"} {
		t.Run(slug, func(t *testing.T) {
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
				if p.Slug == slug {
					preset = p
				}
			}
			require.NotEqual(t, uuid.Nil, preset.ID)
			var storedHash string
			require.NoError(t, database.Raw(`SELECT content_hash FROM catalog.provider_preset_version WHERE id=?`, preset.VersionID).Scan(&storedHash).Error)
			require.Equal(t, preset.Document.Digest(), storedHash)
			candidateIndex := 0
			if slug == "volcengine-speech" {
				require.Len(t, preset.ModelCatalog.Models, 2)
				candidateIndex = 1
				require.Equal(t, 300, preset.Document.Config.TimeoutSeconds)
			} else {
				require.Len(t, preset.ModelCatalog.Models, 1)
			}
			input := catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{preset.ModelCatalog.Models[candidateIndex].Key}, DirectUse: true}
			connected, err := presets.Connect(ctx, actor, preset.ID, input)
			require.NoError(t, err)
			again, err := presets.Connect(ctx, actor, preset.ID, input)
			require.NoError(t, err)
			require.Equal(t, connected, again)
			state, err := catalogapp.NewProviderOnboarding(store).Read(ctx, actor, connected.ProviderID)
			require.NoError(t, err)
			require.Equal(t, "disabled", state.Status)
			require.Empty(t, state.Models, "paid releases must not be eligible for automatic onboarding activation")
			var row struct {
				Status string
				Cost   []byte
			}
			require.NoError(t, database.Raw(`SELECT m.status,p.upstream_cost AS cost FROM catalog.model_profile m JOIN catalog.model_release p ON p.model_id=m.id WHERE m.id=?`, connected.ModelIDs[0]).Scan(&row).Error)
			require.Equal(t, "disabled", row.Status)
			var cost map[string]any
			require.NoError(t, json.Unmarshal(row.Cost, &cost))
			require.NotEmpty(t, cost, "paid supplier must not inherit zero-cost onboarding")
		})
	}
}
