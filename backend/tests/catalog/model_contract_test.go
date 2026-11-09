package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestAdminCandidateDoesNotInventReviewFacts(t *testing.T) {
	template := presetTextTemplate()
	template.UpstreamCost.Source = ""
	template.UpstreamCost.ReviewedAt = time.Time{}
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "custom", Config: catalogfixture.BuiltinPresets()[1].Config, ModelTemplates: []domain.PresetModelTemplate{template}}
	for _, candidate := range catalogfixture.ModelCatalogForPreset(doc).Models {
		if candidate.Key == template.Key {
			require.Empty(t, candidate.SourceURL)
			require.Empty(t, candidate.ReviewedAt)
			require.Equal(t, "unknown", candidate.Lifecycle)
			return
		}
	}
	t.Fatal("administrator template missing")
}

func TestReviewedSeedanceTokenTemplateIsPortable(t *testing.T) {
	config := catalogfixture.BuiltinPresets()[0].Config
	candidate := catalogfixture.LegacyBuiltinModelCatalog("volcengine_ark").Models
	var selected domain.ModelCandidatePreset
	for _, c := range candidate {
		if c.Key == "ark-video-fast" {
			selected = c
		}
	}
	v, err := selected.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	template := presetTextTemplate()
	template.Key = "video"
	template.Name = "Seedance Fast"
	template.Capability = "video.generate"
	template.ProviderModelID = selected.ProviderModelID
	template.Modes = v.Modes
	template.Limits = v.Limits
	template.ParamSchema = v.ParamSchema
	template.Price.Unit = domain.PricePer1KTokens
	template.Price.Rule = json.RawMessage(`{"input_micros_per_1k":0,"output_micros_per_1k":37000}`)
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "Ark", Config: config, ModelTemplates: []domain.PresetModelTemplate{template}}
	require.NoError(t, presetvalidation.Validate(doc))
	doc.ModelTemplates[0].Price.Rule = json.RawMessage(`{"input_micros_per_1k":1,"output_micros_per_1k":37000}`)
	require.Error(t, presetvalidation.Validate(doc), "video cannot charge text prompt tokens")
}

func TestKnownModelsRejectUnsupportedParameterContracts(t *testing.T) {
	validator, err := paramvalidation.NewValidator()
	require.NoError(t, err)
	for _, tc := range []struct {
		adapter, key, field string
		schema              string
	}{
		{"volcengine_ark", "ark-video-fast", "resolution", `[{"field":"resolution","label":"R","component":"select","type":"string","enum":["720p","1080p"],"default":"720p"}]`},
		{"volcengine_ark", "ark-video", "duration_ms", `[{"field":"duration_ms","label":"D","component":"input","type":"integer","min":4000,"max":16000,"step":1000,"default":5000}]`},
		{"volcengine_ark", "ark-image-flash-50", "size", `[{"field":"size","label":"S","component":"select","type":"string","enum":["2K","4K"],"default":"2K"}]`},
	} {
		t.Run(tc.key+"/"+tc.field, func(t *testing.T) {
			var candidate domain.ModelCandidatePreset
			for _, c := range catalogfixture.LegacyBuiltinModelCatalog(tc.adapter).Models {
				if c.Key == tc.key {
					candidate = c
				}
			}
			v, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
			require.NoError(t, err)
			require.NoError(t, validator.Validate(v), "current safe preset must remain valid")
			v.ParamSchema = json.RawMessage(tc.schema)
			require.Error(t, validator.Validate(v), "invalid model-specific domain must not be published")
		})
	}
}

func TestPresetTariffsCoverEveryPublishedDimension(t *testing.T) {
	config := catalogfixture.BuiltinPresets()[0].Config
	var selected domain.ModelCandidatePreset
	for _, c := range catalogfixture.LegacyBuiltinModelCatalog(config.AdapterKey).Models {
		if c.Key == "ark-video-fast" {
			selected = c
		}
	}
	v, err := selected.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	template := presetTextTemplate()
	template.Key, template.Name, template.Capability, template.ProviderModelID = "fast", "Fast", "video.generate", selected.ProviderModelID
	template.Modes, template.Limits, template.ParamSchema = v.Modes, v.Limits, v.ParamSchema
	template.Price.Unit = domain.PricePer1KTokens
	for _, rule := range []string{
		`{"input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":37}}`,
		`{"input_micros_per_1k":0,"output_micros_per_1k":1000,"cached_input_micros_per_1k":0}`,
	} {
		template.Price.Rule = json.RawMessage(rule)
		require.Error(t, presetvalidation.Validate(domain.PresetDocument{SchemaVersion: 1, Name: "Ark", Config: config, ModelTemplates: []domain.PresetModelTemplate{template}}))
	}
	template.Price.Rule = json.RawMessage(`{"input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":37,"720p":37}}`)
	require.NoError(t, presetvalidation.Validate(domain.PresetDocument{SchemaVersion: 1, Name: "Ark", Config: config, ModelTemplates: []domain.PresetModelTemplate{template}}))
}

func TestBytePlusAPProvidesExactRegionalImageCandidates(t *testing.T) {
	catalog := catalogfixture.LegacyBuiltinModelCatalog("byteplus")
	count := 0
	for _, c := range catalog.Models {
		if c.RequiresModelSelection {
			continue
		}
		count++
		require.Contains(t, []string{"dola-seedream-5-0-pro-260628", "dola-seedream-5-0-flash-260915"}, c.ProviderModelID)
		require.Equal(t, "2026-11-05", c.ReviewedAt)
		require.Equal(t, "unknown", c.Lifecycle, "document review does not prove regional account access")
		v, err := c.OnboardingVersion(uuid.New(), uuid.New())
		require.NoError(t, err)
		validator, err := paramvalidation.NewValidator()
		require.NoError(t, err)
		require.NoError(t, validator.Validate(v))
		require.NotContains(t, string(v.ParamSchema), "4K")
	}
	require.Equal(t, 2, count)
}

func TestPricePublicationSupportsOnlyInstalledStrategyVersions(t *testing.T) {
	validator, err := pricevalidation.NewValidator()
	require.NoError(t, err)
	template := presetTextTemplate()
	template.Price.Unit = domain.PricePerImage
	for _, tc := range []struct {
		rule  string
		valid bool
	}{
		{`{"pricing_strategy":"image_flat_v1","base_micros":120000}`, true},
		{`{"pricing_strategy":"image_flat_v2","base_micros":120000}`, false},
		{`{"pricing_strategy":"token_io_v1","base_micros":120000}`, false},
		{`{"pricing_strategy":null,"base_micros":120000}`, false},
	} {
		template.Price.Rule = json.RawMessage(tc.rule)
		err := validator.Validate(template.PriceVersion(uuid.New(), uuid.New(), time.Now()))
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
