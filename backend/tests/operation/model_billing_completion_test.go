package operation_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestCachedTokenSettlementChargesEachInputTokenOnce(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":1000,"cached_input_micros_per_1k":100,"output_micros_per_1k":2000}`, "CNY", "")
	quote, err := application.CalculateQuote(price, application.PricingInput{EstimatedInputTokens: 1000, MaxOutputTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, int64(3000), quote)
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":1000}`))
	require.NoError(t, err)
	actual, err := application.CalculateActualCost(price, usage, "text_to_text", "")
	require.NoError(t, err)
	require.Equal(t, int64(2280), actual)
	for _, raw := range []string{`{"input_tokens":1000,"output_tokens":1000}`, `{"input_tokens":1000,"cached_input_tokens":1001,"output_tokens":1000}`} {
		usage, err := application.ParseBillableUsage(json.RawMessage(raw))
		if err == nil {
			_, err = application.CalculateActualCost(price, usage, "text_to_text", "")
		}
		require.Error(t, err, "missing or contradictory cache evidence must not become full-price input")
	}
}

func TestSeedanceQuoteUsesOutputTokensAndActualUsage(t *testing.T) {
	for _, tc := range []struct {
		key    string
		factor int64
		tokens int64
	}{
		{"ark-video-25", 70, 38430}, {"ark-video", 46, 40176}, {"ark-video-fast", 37, 40176}, {"ark-video-mini", 23, 40176},
	} {
		t.Run(tc.key, func(t *testing.T) {
			var candidate catalogdomain.ModelCandidatePreset
			for _, item := range catalogfixture.LegacyBuiltinModelCatalog("volcengine_ark").Models {
				if item.Key == tc.key {
					candidate = item
				}
			}
			version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
			require.NoError(t, err)
			price := pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":0,"output_micros_per_1k":1000,"by_resolution":{"480p":`+jsonNumber(tc.factor)+`}}`, "CNY", "")
			now := time.Now().UTC()
			catalog := application.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: version.ID, ProviderModelID: version.ProviderModelID, Region: "domestic", Modes: version.Modes, Limits: version.Limits, ParamSchema: version.ParamSchema, Price: price}
			input := application.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{Capability: "video.generate", Mode: "text2video", Prompt: "云朵缓慢移动", OutputCount: 1, Params: json.RawMessage(`{"duration_ms":4000,"resolution":"480p","aspect_ratio":"16:9"}`)}}
			prepared, err := application.PrepareFreeQuote(input, catalog, nil, now, nil)
			require.NoError(t, err)
			require.Equal(t, tc.tokens*tc.factor, *prepared.Operation.QuoteMicros)
			var detail application.QuoteCostDetail
			require.NoError(t, json.Unmarshal(prepared.Operation.QuoteDetail, &detail))
			require.Equal(t, tc.tokens, *detail.MaxOutputTokens)
			usage, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":0,"output_tokens":40000}`))
			require.NoError(t, err)
			actual, err := application.CalculateActualCost(price, usage, "text2video", "480p")
			require.NoError(t, err)
			require.Equal(t, int64(40000)*tc.factor, actual)
			input.Params = json.RawMessage(`{"duration_ms":4000,"resolution":"480p","aspect_ratio":"adaptive"}`)
			_, err = application.PrepareFreeQuote(input, catalog, nil, now, nil)
			require.Error(t, err)
		})
	}
}

func jsonNumber(n int64) string { data, _ := json.Marshal(n); return string(data) }

func TestProImageQuoteCountsFrozenReferencesAndSizeTier(t *testing.T) {
	var candidate catalogdomain.ModelCandidatePreset
	for _, item := range catalogfixture.LegacyBuiltinModelCatalog("volcengine_ark").Models {
		if item.Key == "ark-image-pro-50" {
			candidate = item
		}
	}
	version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	now := time.Now().UTC()
	price := pricingRule(catalogdomain.PricePerImage, `{"base_micros":300000,"by_resolution":{"2K":2},"free_reference_images":1,"reference_image_micros":20000,"reference_scope":"per_task"}`, "CNY", "")
	catalog := application.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: version.ID, ProviderModelID: version.ProviderModelID, Region: "domestic", Modes: version.Modes, Limits: version.Limits, ParamSchema: version.ParamSchema, InputRoles: []string{"reference_image"}, Price: price}
	input := application.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{Capability: "image.generate", Mode: "image2image", Prompt: "保持构图", OutputCount: 1, Params: json.RawMessage(`{"size":"2K"}`)}}
	var media []application.FreeQuoteMediaFact
	for range 2 {
		id := uuid.New()
		input.MediaInputs = append(input.MediaInputs, application.FreeQuoteMediaInput{Role: "reference_image", MediaAssetID: id})
		media = append(media, application.FreeQuoteMediaFact{ID: id, Kind: "image", ByteSize: 100, SHA256: strings.Repeat("a", 64)})
	}
	prepared, err := application.PrepareFreeQuote(input, catalog, media, now, nil)
	require.NoError(t, err)
	require.Equal(t, int64(620000), *prepared.Operation.QuoteMicros)
	var detail application.QuoteCostDetail
	require.NoError(t, json.Unmarshal(prepared.Operation.QuoteDetail, &detail))
	require.Equal(t, int64(2), *detail.ReferenceImageCount)
}
