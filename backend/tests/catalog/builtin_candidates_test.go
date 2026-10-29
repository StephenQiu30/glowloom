package catalog_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestCommonCandidatesAreNotPublishableTemplates(t *testing.T) {
	for _, adapter := range []string{"volcengine_ark", "openrouter", "byteplus", "deepseek", "codex", "volcengine_speech"} {
		t.Run(adapter, func(t *testing.T) {
			catalog := domain.BuiltinModelCatalog(adapter)
			require.NotEmpty(t, catalog.Version)
			require.NotEmpty(t, catalog.Models)
			for _, model := range catalog.Models {
				require.NotEmpty(t, model.Key)
				require.NotEmpty(t, model.SourceURL)
				require.NotEmpty(t, model.ReviewedAt)
				require.Equal(t, adapter, model.AdapterKey)
				require.Equal(t, model.ProviderModelID == "", model.RequiresModelSelection)
				data, err := json.Marshal(model)
				require.NoError(t, err)
				require.NotContains(t, string(data), `"price":`)
				require.NotContains(t, string(data), "api_key")
				_, err = domain.ParsePreset(data)
				require.Error(t, err, "a public candidate must not be accepted as a complete preset")
			}
			catalog.Models[0].Modes[0] = "tampered"
			require.NotEqual(t, "tampered", domain.BuiltinModelCatalog(adapter).Models[0].Modes[0])
		})
	}
	require.Empty(t, domain.BuiltinModelCatalog("unregistered").Models)
}

func TestCloudRegistryRejectsCrossProviderHostsAndCapabilities(t *testing.T) {
	for _, key := range []string{"byteplus", "deepseek"} {
		var config domain.ProviderConfig
		for _, preset := range domain.BuiltinPresets() {
			if preset.Config.AdapterKey == key {
				config = preset.Config
			}
		}
		require.Equal(t, key, config.AdapterKey)
		require.NoError(t, config.Validate())
		config.BaseURL = "https://openrouter.ai/api/v1"
		require.Error(t, config.Validate())
	}
}

func TestCandidateSelectionRequiresExactCatalogAndProvider(t *testing.T) {
	catalog := domain.BuiltinModelCatalog("deepseek")
	selected, err := catalog.Select(catalog.Version, []string{"ds-pro", "ds-flash"})
	require.NoError(t, err)
	require.Equal(t, "ds-pro", selected[0].Key)
	for _, input := range []struct {
		version string
		keys    []string
	}{
		{"stale", []string{"ds-pro"}}, {catalog.Version, []string{"or-gpt-chat"}},
		{catalog.Version, []string{"ds-pro", "ds-pro"}},
	} {
		_, err := catalog.Select(input.version, input.keys)
		require.ErrorIs(t, err, domain.ErrInvalidPreset)
	}
	_, err = catalog.Select("", nil)
	require.NoError(t, err)
}

func TestHistoricalArkConfigDoesNotAcquireTextCandidates(t *testing.T) {
	config := domain.BuiltinPresets()[0].Config
	config.Capabilities = []string{"video"}
	catalog := domain.ModelCatalogForConfig(config)
	require.NotEmpty(t, catalog.Models)
	for _, model := range catalog.Models {
		require.Equal(t, "video.generate", model.Capability)
	}
	_, err := catalog.Select(catalog.Version, []string{"ark-chat"})
	require.ErrorIs(t, err, domain.ErrInvalidPreset)
}

func TestEachCloudPresetAcceptsAnExplicitPricedTextTemplate(t *testing.T) {
	for _, preset := range domain.BuiltinPresets() {
		doc := domain.PresetDocument{SchemaVersion: 1, Name: preset.Name, Config: preset.Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
		if slices.Contains(preset.Config.Capabilities, "text") {
			require.NoError(t, doc.Validate(), preset.Slug)
		} else {
			require.ErrorIs(t, doc.Validate(), domain.ErrInvalidPreset, preset.Slug)
		}
	}
}
