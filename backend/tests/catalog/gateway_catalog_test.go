package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"
)

func TestGatewayCannotInheritOfficialCatalogOrPricedTemplates(t *testing.T) {
	spec, ok := domain.GetAdapterSpec("openai")
	require.True(t, ok)
	document := domain.PresetDocument{
		SchemaVersion: 1, Name: "Gateway",
		Config:         domain.ProviderConfig{AdapterKey: "openai", BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 60},
		ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()},
	}
	require.NoError(t, document.Validate())
	source := catalogfixture.BuiltinModelCatalog("openai")
	require.NotEmpty(t, domain.ModelCatalogForPreset(document, source).Models)
	document.Config.BaseURL = "https://gateway.example.com/v1"
	require.ErrorIs(t, document.Validate(), domain.ErrInvalidPreset, "priced portable templates are not gateway drafts")
	require.Empty(t, domain.ModelCatalogForPreset(document, source).Models)
	document.ModelTemplates = nil
	require.NoError(t, document.Validate())
	require.Empty(t, domain.ModelCatalogForPreset(document, source).Models)
	require.NotEmpty(t, source.Models, "the shared official source must remain intact")
}
