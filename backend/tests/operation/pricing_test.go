package operation_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func pricingRule(unit catalogdomain.PriceUnit, rule, currency, fx string) catalogdomain.PriceRuleVersion {
	return catalogdomain.PriceRuleVersion{
		ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1,
		Unit: unit, Rule: json.RawMessage(rule), Currency: currency,
		FXRateToCNY: fx, EffectiveFrom: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
	}
}

func TestCalculateQuoteByPublishedUnit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price catalogdomain.PriceRuleVersion
		input application.PricingInput
		want  int64
	}{
		{"per image outputs", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1000000}`, "CNY", ""), application.PricingInput{OutputCount: 2}, 2_000_000},
		{"per second rounds duration to next allowed value", pricingRule(catalogdomain.PricePerSecond, `{"base_micros":1000000}`, "CNY", ""), application.PricingInput{OutputCount: 2, TargetDurationMS: 6_100, AllowedDurationsMS: []int64{15_000, 5_000, 10_000}}, 20_000_000},
		{"per second retains fractional seconds", pricingRule(catalogdomain.PricePerSecond, `{"base_micros":1000000}`, "CNY", ""), application.PricingInput{OutputCount: 1, TargetDurationMS: 1_250, AllowedDurationsMS: []int64{1_500}}, 1_500_000},
		{"per request ignores output count", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":250000}`, "CNY", ""), application.PricingInput{OutputCount: 3}, 250_000},
		{"per thousand chars rounds character blocks", pricingRule(catalogdomain.PricePer1KChars, `{"base_micros":200000}`, "CNY", ""), application.PricingInput{CharacterCount: 1_001}, 400_000},
		{"per thousand tokens combines input and output before rounding", pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":1,"output_micros_per_1k":1}`, "CNY", ""), application.PricingInput{EstimatedInputTokens: 1, MaxOutputTokens: 1}, 1},
		{"versioned foreign exchange", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":100}`, "USD", "1.234567"), application.PricingInput{}, 124},
		{"mode and resolution multiply without intermediate rounding", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_resolution":{"1080p":1.1},"by_mode":{"image2image":1.1}}`, "USD", "0.8"), application.PricingInput{OutputCount: 1, Mode: "image2image", Resolution: "1080p"}, 1},
		{"unmatched coefficient is identity", pricingRule(catalogdomain.PricePerImage, `{"base_micros":10,"by_resolution":{"1080p":1.5},"by_mode":{"image2image":2}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "text2image", Resolution: "720p"}, 10},
		{"published JSON whitespace", pricingRule(catalogdomain.PricePerImage, `{"base_micros": 10, "by_mode": {"image2image": 1.25}}`, "CNY", "1.000000"), application.PricingInput{OutputCount: 2, Mode: "image2image"}, 25},
		{"free price", pricingRule(catalogdomain.PricePerImage, `{"base_micros":0}`, "CNY", ""), application.PricingInput{OutputCount: 2}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := application.CalculateQuote(tc.price, tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("CalculateQuote() = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestCalculateQuoteRejectsInvalidPricingFacts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price catalogdomain.PriceRuleVersion
		input application.PricingInput
		want  error
	}{
		{"missing output count", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1}`, "CNY", ""), application.PricingInput{}, application.ErrInvalidPricingInput},
		{"negative output count", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1}`, "CNY", ""), application.PricingInput{OutputCount: -1}, application.ErrInvalidPricingInput},
		{"no allowed duration", pricingRule(catalogdomain.PricePerSecond, `{"base_micros":1}`, "CNY", ""), application.PricingInput{OutputCount: 1, TargetDurationMS: 10_001, AllowedDurationsMS: []int64{5_000, 10_000}}, application.ErrInvalidPricingInput},
		{"invalid allowed duration", pricingRule(catalogdomain.PricePerSecond, `{"base_micros":1}`, "CNY", ""), application.PricingInput{OutputCount: 1, TargetDurationMS: 1_000, AllowedDurationsMS: []int64{0, 1_000}}, application.ErrInvalidPricingInput},
		{"negative characters", pricingRule(catalogdomain.PricePer1KChars, `{"base_micros":1}`, "CNY", ""), application.PricingInput{CharacterCount: -1}, application.ErrInvalidPricingInput},
		{"zero output token ceiling", pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":1,"output_micros_per_1k":1}`, "CNY", ""), application.PricingInput{EstimatedInputTokens: 5}, application.ErrInvalidPricingInput},
		{"unsupported parameter coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1}`, "CNY", ""), application.PricingInput{OutputCount: 1, ParameterMultipliers: map[string]string{"quality": "1.2"}}, application.ErrUnsupportedPriceCoefficient},
		{"unsupported rule parameter coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_parameter":{"quality":1.2}}`, "CNY", ""), application.PricingInput{OutputCount: 1}, application.ErrInvalidPricingRule},
		{"missing selected mode", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":2}}`, "CNY", ""), application.PricingInput{OutputCount: 1}, application.ErrInvalidPricingInput},
		{"missing selected resolution", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_resolution":{"1080p":2}}`, "CNY", ""), application.PricingInput{OutputCount: 1}, application.ErrInvalidPricingInput},
		{"duplicate base field", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1,"base_micros":2}`, "CNY", ""), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"duplicate coefficient key", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":1,"chat":2}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "chat"}, application.ErrInvalidPricingRule},
		{"fractional micros", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1.0}`, "CNY", ""), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"unsafe integer", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":9007199254740992}`, "CNY", ""), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"too precise coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":1.0000001}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "chat"}, application.ErrInvalidPricingRule},
		{"zero coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":0}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "chat"}, application.ErrInvalidPricingRule},
		{"excessive coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":1000000.1}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "chat"}, application.ErrInvalidPricingRule},
		{"exponential coefficient", pricingRule(catalogdomain.PricePerImage, `{"base_micros":1,"by_mode":{"chat":1e2}}`, "CNY", ""), application.PricingInput{OutputCount: 1, Mode: "chat"}, application.ErrInvalidPricingRule},
		{"zero token rates", pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":0,"output_micros_per_1k":0}`, "CNY", ""), application.PricingInput{EstimatedInputTokens: 1, MaxOutputTokens: 1}, application.ErrInvalidPricingRule},
		{"token rule with multiplier", pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":1,"output_micros_per_1k":1,"by_mode":{"chat":2}}`, "CNY", ""), application.PricingInput{EstimatedInputTokens: 1, MaxOutputTokens: 1}, application.ErrInvalidPricingRule},
		{"CNY nonparity rate", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1}`, "CNY", "2"), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"missing foreign rate", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1}`, "USD", ""), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"zero foreign rate", pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1}`, "USD", "0"), application.PricingInput{}, application.ErrInvalidPricingRule},
		{"amount exceeds bigint storage", pricingRule(catalogdomain.PricePerImage, `{"base_micros":9007199254740991,"by_mode":{"chat":1000000}}`, "CNY", ""), application.PricingInput{OutputCount: math.MaxInt64, Mode: "chat"}, application.ErrQuoteAmountOverflow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := application.CalculateQuote(tc.price, tc.input)
			if got != 0 || !errors.Is(err, tc.want) {
				t.Fatalf("CalculateQuote() = %d, %v; want %v", got, err, tc.want)
			}
		})
	}
}
