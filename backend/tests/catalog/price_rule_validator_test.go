package catalog_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

func TestPriceRuleValidatorChecksBillableRuleShape(t *testing.T) {
	validator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		unit  domain.PriceUnit
		rule  string
		valid bool
	}{
		{"image base and coefficients", domain.PricePerImage, `{"base_micros":1000,"by_resolution":{"1080p":1.5},"by_mode":{"image2image":0.75}}`, true},
		{"free request", domain.PricePerRequest, `{"base_micros":0}`, true},
		{"second base", domain.PricePerSecond, `{"base_micros":123456}`, true},
		{"character base", domain.PricePer1KChars, `{"base_micros":9007199254740991}`, true},
		{"token input only", domain.PricePer1KTokens, `{"input_micros_per_1k":25,"output_micros_per_1k":0}`, true},
		{"token output only", domain.PricePer1KTokens, `{"input_micros_per_1k":0,"output_micros_per_1k":80}`, true},
		{"missing base", domain.PricePerImage, `{}`, false},
		{"negative base", domain.PricePerImage, `{"base_micros":-1}`, false},
		{"fractional base", domain.PricePerImage, `{"base_micros":1.5}`, false},
		{"unsafe integer", domain.PricePerImage, `{"base_micros":9007199254740992}`, false},
		{"wrong base type", domain.PricePerImage, `{"base_micros":"1000"}`, false},
		{"unknown field", domain.PricePerImage, `{"base_micros":1000,"discount":0.5}`, false},
		{"token field on image", domain.PricePerImage, `{"base_micros":1000,"input_micros_per_1k":10}`, false},
		{"duplicate root field", domain.PricePerImage, `{"base_micros":1000,"base_micros":1}`, false},
		{"empty coefficient key", domain.PricePerImage, `{"base_micros":1000,"by_mode":{" ":1.2}}`, false},
		{"zero coefficient", domain.PricePerImage, `{"base_micros":1000,"by_resolution":{"1080p":0}}`, false},
		{"negative coefficient", domain.PricePerImage, `{"base_micros":1000,"by_mode":{"image2image":-0.5}}`, false},
		{"wrong coefficient type", domain.PricePerImage, `{"base_micros":1000,"by_mode":{"image2image":"1.5"}}`, false},
		{"nonfinite coefficient", domain.PricePerImage, `{"base_micros":1000,"by_mode":{"image2image":1e999}}`, false},
		{"duplicate coefficient key", domain.PricePerImage, `{"base_micros":1000,"by_mode":{"image2image":1,"image2image":2}}`, false},
		{"token missing output", domain.PricePer1KTokens, `{"input_micros_per_1k":25}`, false},
		{"token zero rates", domain.PricePer1KTokens, `{"input_micros_per_1k":0,"output_micros_per_1k":0}`, false},
		{"token unsafe rate", domain.PricePer1KTokens, `{"input_micros_per_1k":9007199254740992,"output_micros_per_1k":1}`, false},
		{"token with base", domain.PricePer1KTokens, `{"base_micros":1000,"input_micros_per_1k":1,"output_micros_per_1k":1}`, false},
		{"token with coefficient", domain.PricePer1KTokens, `{"input_micros_per_1k":1,"output_micros_per_1k":1,"by_mode":{"chat":2}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := validPriceVersion(uuid.New())
			price.Unit = tc.unit
			price.Rule = json.RawMessage(tc.rule)
			err := validator.Validate(price)
			if tc.valid && err != nil {
				t.Fatalf("valid price rule rejected: %v", err)
			}
			if !tc.valid && !errors.Is(err, pricevalidation.ErrInvalidConfiguration) {
				t.Fatalf("invalid price rule accepted: %v", err)
			}
		})
	}
}

func TestPriceRuleValidatorChecksVersionedCurrencyRate(t *testing.T) {
	validator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		currency string
		rate     string
		valid    bool
	}{
		{"CNY implicit parity", "CNY", "", true},
		{"CNY explicit parity", "CNY", "1.000000", true},
		{"CNY wrong parity", "CNY", "1.01", false},
		{"USD versioned rate", "USD", "7.123456", true},
		{"USD missing rate", "USD", "", false},
		{"USD zero rate", "USD", "0", false},
		{"USD negative rate", "USD", "-2", false},
		{"USD imprecise rate", "USD", "7.1234567", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := validPriceVersion(uuid.New())
			price.Currency, price.FXRateToCNY = tc.currency, tc.rate
			err := validator.Validate(price)
			if tc.valid && err != nil {
				t.Fatalf("valid versioned rate rejected: %v", err)
			}
			if !tc.valid && !errors.Is(err, pricevalidation.ErrInvalidConfiguration) {
				t.Fatalf("invalid versioned rate accepted: %v", err)
			}
		})
	}
}
