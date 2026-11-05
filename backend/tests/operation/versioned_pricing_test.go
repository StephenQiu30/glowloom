package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestVersionedPricingUsesFrozenTariffAndExactArithmetic(t *testing.T) {
	price := pricingRule(catalogdomain.PricePerImage, `{"pricing_strategy":"image_flat_v1","base_micros":120000,"markup_multiplier":"1.25","credit_ratio":"100"}`, "CNY", "")
	amount, detail, err := application.CalculateQuoteDetailed(price, application.PricingInput{OutputCount: 3})
	require.NoError(t, err)
	require.Equal(t, int64(450000), amount)
	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	var factors map[string]any
	require.NoError(t, json.Unmarshal(raw, &factors))
	require.Equal(t, "image_flat_v1", factors["pricing_strategy"])
	require.Equal(t, int64(45), *detail.EstimatedCredits)
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"output_count":2}`))
	require.NoError(t, err)
	actual, err := application.CalculateActualCost(price, usage, "text2image", "")
	require.NoError(t, err)
	require.Equal(t, int64(300000), actual)
}

func TestPricingRejectsUnknownAndIncompatibleStrategies(t *testing.T) {
	for _, rule := range []string{
		`{"pricing_strategy":"image_flat_v2","base_micros":120000}`,
		`{"pricing_strategy":"token_io_v1","base_micros":120000}`,
		`{"pricing_strategy":null,"base_micros":120000}`,
	} {
		_, err := application.CalculateQuote(pricingRule(catalogdomain.PricePerImage, rule, "CNY", ""), application.PricingInput{OutputCount: 1})
		require.Error(t, err)
	}
}

func TestSeedanceVersionedTariffUsesOnlyOutputEvidence(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, `{"pricing_strategy":"seedance_video_tokens_v1","input_micros_per_1k":0,"output_micros_per_1k":37000}`, "CNY", "")
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"output_tokens":40176}`))
	require.NoError(t, err)
	actual, err := application.CalculateActualCost(price, usage, "text2video", "480p")
	require.NoError(t, err)
	require.Equal(t, int64(1486512), actual)
}

func TestKnownModelRuntimeRejectsLegacyOverbroadSchema(t *testing.T) {
	var candidate catalogdomain.ModelCandidatePreset
	for _, c := range catalogdomain.BuiltinModelCatalog("volcengine_ark").Models {
		if c.Key == "ark-image-flash-50" {
			candidate = c
		}
	}
	version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	version.ParamSchema = json.RawMessage(`[{"field":"size","label":"Size","component":"select","type":"string","enum":["2K","4K"],"default":"2K"}]`)
	catalog := application.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: version.ID, ProviderModelID: version.ProviderModelID, Region: "domestic", Modes: version.Modes, Limits: version.Limits, ParamSchema: version.ParamSchema, Price: pricingRule(catalogdomain.PricePerImage, `{"base_micros":120000}`, "CNY", "")}
	input := application.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{Capability: "image.generate", Mode: "text2image", Prompt: "云", OutputCount: 1, Params: json.RawMessage(`{"size":"4K"}`)}}
	_, err = application.PrepareFreeQuote(input, catalog, nil, time.Now(), nil)
	require.Error(t, err, "runtime must not trust an older overly broad published schema")
	input.Params = json.RawMessage(`{"size":"2K"}`)
	_, err = application.PrepareFreeQuote(input, catalog, nil, time.Now(), nil)
	require.NoError(t, err)
}

func TestSeedanceStrategyRejectsConflictingUsageDimensions(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, `{"pricing_strategy":"seedance_video_tokens_v1","input_micros_per_1k":0,"output_micros_per_1k":37000}`, "CNY", "")
	for _, raw := range []string{`{"output_tokens":40000,"billable_requests":1}`, `{"input_tokens":0,"output_tokens":40000,"output_count":1}`, `{"input_tokens":1,"output_tokens":40000}`} {
		usage, err := application.ParseBillableUsage(json.RawMessage(raw))
		require.NoError(t, err)
		_, err = application.CalculateActualCost(price, usage, "text2video", "480p")
		require.Error(t, err)
	}
}
