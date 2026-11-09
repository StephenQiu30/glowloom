package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func presetTextTemplate() domain.PresetModelTemplate {
	return domain.PresetModelTemplate{Key: "text", Name: "审核过的文本模型", Capability: "text.generate", ProviderModelID: "fixture/text", Modes: []string{"text_to_text"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[]`), ExpectedMaxMS: 60000,
		Price:        domain.PresetModelPrice{SourceURL: "https://example.invalid/prices", ReviewedAt: priceReviewedFixtureTime(), Unit: domain.PricePerRequest, Rule: json.RawMessage(`{"base_micros":10}`), Currency: "CNY"},
		UpstreamCost: domain.PresetModelCost{Currency: "CNY", MaxMicros: 20, Source: "fixture reviewed price", ReviewedAt: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)}}
}
func TestPresetModelTemplatesArePortableAndProtocolBound(t *testing.T) {
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "OpenRouter", Config: catalogfixture.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
	require.NoError(t, doc.Validate())
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	parsed, err := domain.ParsePreset(encoded)
	require.NoError(t, err)
	require.Len(t, parsed.ModelTemplates, 1)
	require.NotContains(t, string(encoded), "credential_id")
	doc.Config = catalogfixture.BuiltinPresets()[0].Config
	require.NoError(t, doc.Validate(), "expanded Ark preset accepts text")
	doc.Config.Capabilities = []string{"video"}
	require.Error(t, doc.Validate(), "historical video-only config must reject text")
	doc.Config = catalogfixture.BuiltinPresets()[1].Config
	doc.ModelTemplates = append(doc.ModelTemplates, doc.ModelTemplates[0])
	require.Error(t, doc.Validate(), "duplicate model template keys")
	doc.ModelTemplates = doc.ModelTemplates[:1]
	doc.ModelTemplates[0].Price.Currency = "UNKNOWN"
	require.Error(t, doc.Validate())
}

func TestPresetTemplatesRejectSecretsAndInvalidPriceRules(t *testing.T) {
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "OpenRouter", Config: catalogfixture.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(encoded, &raw))
	template := raw["model_templates"].([]any)[0].(map[string]any)
	template["api_key"] = "synthetic-secret"
	secretJSON, err := json.Marshal(raw)
	require.NoError(t, err)
	_, err = domain.ParsePreset(secretJSON)
	require.Error(t, err)
	doc.ModelTemplates[0].Price.Rule = json.RawMessage(`{"api_key":"synthetic-secret"}`)
	require.ErrorIs(t, presetvalidation.Validate(doc), domain.ErrInvalidPreset)
	doc.ModelTemplates[0] = presetTextTemplate()
	doc.ModelTemplates[0].ParamSchema = json.RawMessage(`[{"field":"api_key","label":"Key","component":"input","type":"string","default":"synthetic-secret"}]`)
	require.ErrorIs(t, presetvalidation.Validate(doc), domain.ErrInvalidPreset)
	doc.ModelTemplates[0] = presetTextTemplate()
	doc.ModelTemplates[0].Limits = json.RawMessage(`{"max_outputs":2}`)
	require.ErrorIs(t, doc.Validate(), domain.ErrInvalidPreset)
}

func TestModelCatalogForPresetDynamicallyIncludesAdminTemplates(t *testing.T) {
	doc := domain.PresetDocument{
		SchemaVersion: 1,
		Name:          "Dynamic Custom Preset",
		Config:        catalogfixture.BuiltinPresets()[1].Config, // OpenRouter
		ModelTemplates: []domain.PresetModelTemplate{
			{
				Key:             "admin-custom-model",
				Name:            "Admin Managed Claude 5.5",
				Capability:      "text.generate",
				ProviderModelID: "anthropic/claude-sonnet-5.5",
				Modes:           []string{"text_to_text"},
				Limits:          json.RawMessage(`{"max_outputs":1}`),
				ParamSchema:     json.RawMessage(`[]`),
				ExpectedMaxMS:   60000,
				Price:           domain.PresetModelPrice{Unit: domain.PricePer1KTokens, Rule: json.RawMessage(`{"amount":10}`), Currency: "CNY"},
				UpstreamCost:    domain.PresetModelCost{Currency: "USD", MaxMicros: 20000, Source: "https://openrouter.ai/models", ReviewedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
			},
		},
	}
	catalog := catalogfixture.ModelCatalogForPreset(doc)
	require.NotEmpty(t, catalog.Models)
	var found bool
	for _, m := range catalog.Models {
		if m.Key == "admin-custom-model" {
			found = true
			require.Equal(t, "Admin Managed Claude 5.5", m.DisplayName)
			require.Equal(t, "anthropic/claude-sonnet-5.5", m.ProviderModelID)
			require.Equal(t, "text.generate", m.Capability)
			require.Equal(t, "unknown", m.Lifecycle)
			break
		}
	}
	require.True(t, found, "catalog must dynamically expose administrator-configured model templates")
}

func TestOpenRouterAndVercelCandidatesContainFourCoreFamilies(t *testing.T) {
	for _, adapter := range []string{"openrouter", "vercel"} {
		catalog := catalogfixture.BuiltinModelCatalog(adapter)
		require.NotEmpty(t, catalog.Models)
		var hasChatGPT, hasAnthropic, hasGemini, hasGrok bool
		for _, m := range catalog.Models {
			switch m.Key {
			case adapter + "-gpt-6-1-sol", "or-gpt-6-1-sol", "vercel-gpt-6-1-sol":
				hasChatGPT = true
			case adapter + "-claude-sonnet-5-5", "or-claude-sonnet-5-5", "vercel-claude-sonnet-5-5":
				hasAnthropic = true
			case adapter + "-gemini-4-argon", "or-gemini-4-argon", "vercel-gemini-4-argon":
				hasGemini = true
			case adapter + "-grok-4-7", "or-grok-4-7", "vercel-grok-4-7":
				hasGrok = true
			}
		}
		require.True(t, hasChatGPT, "must contain ChatGPT family models")
		require.True(t, hasAnthropic, "must contain Anthropic family models")
		require.True(t, hasGemini, "must contain Gemini family models")
		require.True(t, hasGrok, "must contain Grok family models")
	}
}
