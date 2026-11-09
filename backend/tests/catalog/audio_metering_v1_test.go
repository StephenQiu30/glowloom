package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestOriginalDurationUnitQuantityMustBeExplicitAndCompatible(t *testing.T) {
	validator, err := pricevalidation.NewValidator()
	require.NoError(t, err)
	for _, rule := range []string{
		`{"pricing_strategy":"original_duration_v1","base_micros":1000000}`,
		`{"pricing_strategy":"original_duration_v1","base_micros":1000000,"unit_quantity":0}`,
		`{"pricing_strategy":"original_duration_v1","base_micros":1000000,"unit_quantity":1.5}`,
		`{"base_micros":1000000,"unit_quantity":60}`,
	} {
		price := domain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: domain.PricePerSecond, Rule: json.RawMessage(rule), Currency: "CNY", EffectiveFrom: time.Now()}
		require.Error(t, validator.Validate(price), rule)
	}
}

func TestAudioModelMeteringContractMatchesItsFrozenRule(t *testing.T) {
	config := domain.ProviderConfig{AdapterKey: "volcengine_speech"}
	version := domain.ModelVersion{ProviderModelID: "seed-audio-1.0", Modes: []string{"omni_audio"}, Limits: json.RawMessage(`{"metering_strategy":"original_duration_v1"}`), ParamSchema: json.RawMessage(`[]`)}
	price := domain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: domain.PricePerSecond, Rule: json.RawMessage(`{"pricing_strategy":"original_duration_v1","base_micros":1000000,"unit_quantity":60}`), Currency: "CNY", EffectiveFrom: time.Now()}
	require.NoError(t, pricevalidation.ValidateModel(config, "audio.generate", version, price))
	for _, mutate := range []func(){func() { version.ProviderModelID = "seed-tts-2.0" }, func() { version.Limits = json.RawMessage(`{"metering_strategy":"character_count_v1"}`) }, func() { version.Limits = json.RawMessage(`{"metering_strategy":"provider_invoice_v1"}`) }} {
		original := version
		mutate()
		require.Error(t, pricevalidation.ValidateModel(config, "audio.generate", version, price))
		version = original
	}
}

func TestAudioProcurementAndSaleShareTheQuantityAlgorithm(t *testing.T) {
	now := time.Now()
	supplier := domain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), Unit: domain.PricePer1KChars, Rule: json.RawMessage(`{"pricing_strategy":"character_count_v1","base_micros":300000}`), EffectiveFrom: now, SourceURL: "https://example.invalid/pricing", ReviewedAt: &now}
	sale := supplier
	sale.ID = uuid.New()
	require.NoError(t, pricevalidation.ValidateProcurement(sale, supplier))
	sale.Rule = json.RawMessage(`{"base_micros":300000}`)
	require.Error(t, pricevalidation.ValidateProcurement(sale, supplier))
	sale, supplier = supplier, sale
	require.Error(t, pricevalidation.ValidateProcurement(sale, supplier))
}
