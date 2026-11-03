package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// seedReviewedOnboardingPrices explicitly authorizes synthetic zero sales prices
// for legacy onboarding protocol fixtures, never live supplier or platform policy.
func seedReviewedOnboardingPrices(t *testing.T, db *gorm.DB, actor identityapp.Principal, connection catalogapp.PresetConnection, paid bool) {
	t.Helper()
	validator, err := pricevalidation.NewValidator()
	require.NoError(t, err)
	command := catalogapp.NewPublishPriceRuleCommand(app.NewCatalogStore(db), validator, time.Now)
	for _, id := range connection.ModelIDs {
		var version struct {
			ID              uuid.UUID
			ProviderModelID string
		}
		require.NoError(t, db.Raw(`SELECT v.id,v.provider_model_id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id WHERE m.id=?`, id).Scan(&version).Error)
		price, err := command.Execute(t.Context(), actor, catalogapp.PublishPriceRuleInput{ModelID: id, ExpectedRevision: 2, VersionNo: 1, Unit: domain.PricePerRequest, Rule: json.RawMessage(`{"base_micros":0}`), Currency: "CNY", EffectiveFrom: time.Now().Add(-time.Minute), RequestID: uuid.NewString(), SourceURL: "https://example.invalid/explicit-zero-sales-fixture", ReviewedAt: priceReviewedFixtureTime()})
		require.NoError(t, err)
		cost := "{}"
		if paid || version.ProviderModelID == "seed-audio-1.0" {
			cost = `{"currency":"CNY","max_micros":100000,"source":"explicit synthetic fixture bound","reviewed_at":"2026-09-26T00:00:00Z"}`
		}
		require.NoError(t, db.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost) VALUES(?,?,?,?,?,?::jsonb)`, uuid.New(), id, version.ID, price.ID, connection.Config.ID, cost).Error)
	}
}
