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

func TestCharacterCountV1UsesExactCharactersAndPreservesHistoricalBlocks(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KChars, `{"pricing_strategy":"character_count_v1","base_micros":300000}`, "CNY", "")
	for _, count := range []int64{1, 100, 1001, 10000} {
		quote, err := application.CalculateQuote(price, application.PricingInput{CharacterCount: count})
		require.NoError(t, err)
		require.Equal(t, count*300, quote)
		actual, err := application.CalculateActualCost(price, application.BillableUsage{CharacterCount: &count}, "text2audio", "")
		require.NoError(t, err)
		require.Equal(t, quote, actual)
	}
	count := int64(1001)
	price.Rule = json.RawMessage(`{"base_micros":300000}`)
	actual, err := application.CalculateActualCost(price, application.BillableUsage{CharacterCount: &count}, "text2audio", "")
	require.NoError(t, err)
	require.Equal(t, int64(600000), actual)
}

func TestAudioQuoteBindsMeteringAndKeepsOnlyMaximumForSeedAudio(t *testing.T) {
	now := time.Now().UTC()
	input := application.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{ModelKey: "audio", Capability: "audio.generate", Mode: "omni_audio", Prompt: "海浪", Params: json.RawMessage(`{}`), OutputCount: 1}}
	price := pricingRule(catalogdomain.PricePerSecond, `{"pricing_strategy":"original_duration_v1","base_micros":1000000,"unit_quantity":60,"procurement_price_version_id":"`+uuid.NewString()+`"}`, "CNY", "")
	catalog := application.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: uuid.New(), ProviderModelID: "seed-audio-1.0", Region: "domestic", Modes: []string{"omni_audio"}, Limits: json.RawMessage(`{"max_outputs":1,"durations_ms":[120000],"metering_strategy":"original_duration_v1"}`), ParamSchema: json.RawMessage(`[]`), Price: price}
	quoted, err := application.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.NoError(t, err)
	var detail application.QuoteCostDetail
	require.NoError(t, json.Unmarshal(quoted.Operation.QuoteDetail, &detail))
	require.Nil(t, detail.EstimatedCredits)
	require.Equal(t, "20", detail.MaxChargeCredits)
	catalog.Limits = json.RawMessage(`{"max_outputs":1,"durations_ms":[120000],"metering_strategy":"character_count_v1"}`)
	_, err = application.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.Error(t, err)
}

func TestTTSQuoteCountsSpacesAndNewlinesWithoutUTF8BytePricing(t *testing.T) {
	now := time.Now().UTC()
	input := application.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{ModelKey: "tts", Capability: "audio.generate", Mode: "text2audio", Prompt: "你 好\n", Params: json.RawMessage(`{}`), OutputCount: 1}}
	catalog := application.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: uuid.New(), ProviderModelID: "seed-tts-2.0", Region: "domestic", Modes: []string{"text2audio"}, Limits: json.RawMessage(`{"max_outputs":1,"metering_strategy":"character_count_v1"}`), ParamSchema: json.RawMessage(`[]`), Price: pricingRule(catalogdomain.PricePer1KChars, `{"pricing_strategy":"character_count_v1","base_micros":300000}`, "CNY", "")}
	quoted, err := application.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1200), *quoted.Operation.QuoteMicros, "four characters, including the space and newline")
	require.Equal(t, input.Prompt, *quoted.Inputs[0].TextValue)
}

func TestOriginalDurationV1ExactMinuteRateAndSingleCeiling(t *testing.T) {
	price := pricingRule(catalogdomain.PricePerSecond, `{"pricing_strategy":"original_duration_v1","base_micros":1000000,"unit_quantity":60,"markup_multiplier":"1.2"}`, "CNY", "")
	for _, tc := range []struct {
		seconds string
		want    int64
	}{{"60", 1200000}, {"6e1", 1200000}, {"1", 20000}, {"0.0000000000000000001", 1}, {"1e-19", 1}, {"1.0000000000000000001", 20001}, {"119.9999999999999999999", 2400000}} {
		usage, err := application.ParseBillableUsage(json.RawMessage(`{"original_duration_seconds":` + tc.seconds + `}`))
		require.NoError(t, err)
		actual, err := application.CalculateActualCost(price, usage, "omni_audio", "")
		require.NoError(t, err)
		require.Equal(t, tc.want, actual)
	}
	quote, detail, err := application.CalculateQuoteDetailed(price, application.PricingInput{OutputCount: 1, Mode: "omni_audio", TargetDurationMS: 1000, AllowedDurationsMS: []int64{1000}})
	require.NoError(t, err)
	require.Equal(t, int64(2400000), quote, "unconstrained Seed-Audio reserves all 120 original seconds")
	require.Nil(t, detail.EstimatedCredits, "a maximum reservation must not pretend to estimate actual duration")
	for _, raw := range []string{`{"billed_duration_ms":60000}`, `{"original_duration_seconds":null}`, `{"original_duration_seconds":"60"}`, `{"original_duration_seconds":120.00000000000000001}`, `{"original_duration_seconds":-1}`, `{"original_duration_seconds":60,"original_duration_seconds":30}`} {
		usage, err := application.ParseBillableUsage(json.RawMessage(raw))
		if err == nil {
			_, err = application.CalculateActualCost(price, usage, "omni_audio", "")
		}
		require.Error(t, err, raw)
	}
}
