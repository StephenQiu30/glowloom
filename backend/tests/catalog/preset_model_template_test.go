package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func presetTextTemplate() domain.PresetModelTemplate {
	return domain.PresetModelTemplate{Key: "text", Name: "审核过的文本模型", Capability: "text.generate", ProviderModelID: "fixture/text", Modes: []string{"text_to_text"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[]`), ExpectedMaxMS: 60000,
		Price:        domain.PresetModelPrice{Unit: domain.PricePerRequest, Rule: json.RawMessage(`{"base_micros":10}`), Currency: "CNY"},
		UpstreamCost: domain.PresetModelCost{Currency: "CNY", MaxMicros: 20, Source: "fixture reviewed price", ReviewedAt: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)}}
}
func TestPresetModelTemplatesArePortableAndProtocolBound(t *testing.T) {
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "OpenRouter", Config: domain.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
	require.NoError(t, doc.Validate())
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	parsed, err := domain.ParsePreset(encoded)
	require.NoError(t, err)
	require.Len(t, parsed.ModelTemplates, 1)
	require.NotContains(t, string(encoded), "credential_id")
	doc.Config = domain.BuiltinPresets()[0].Config
	require.NoError(t, doc.Validate(), "expanded Ark preset accepts text")
	doc.Config.Capabilities = []string{"video"}
	require.Error(t, doc.Validate(), "historical video-only config must reject text")
	doc.Config = domain.BuiltinPresets()[1].Config
	doc.ModelTemplates = append(doc.ModelTemplates, doc.ModelTemplates[0])
	require.Error(t, doc.Validate(), "duplicate model template keys")
	doc.ModelTemplates = doc.ModelTemplates[:1]
	doc.ModelTemplates[0].Price.Currency = "UNKNOWN"
	require.Error(t, doc.Validate())
}

func TestPresetTemplatesRejectSecretsAndInvalidPriceRules(t *testing.T) {
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "OpenRouter", Config: domain.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{presetTextTemplate()}}
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
