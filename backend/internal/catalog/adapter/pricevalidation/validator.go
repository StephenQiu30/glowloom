// Package pricevalidation checks an immutable price rule before publication.
package pricevalidation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ErrInvalidConfiguration means a price version cannot be published as configured.
var ErrInvalidConfiguration = errors.New("invalid price configuration")

const (
	maxRuleBytes   = 64 * 1024
	maxSafeInteger = uint64(9007199254740991)
	maxCoefficient = uint64(1000000 * 1000000)
)

// Validator checks exact monetary amounts and the currently supported rule fields.
type Validator struct{}

// NewValidator creates a reusable price rule validator.
func NewValidator() (*Validator, error) {
	return &Validator{}, nil
}

// Validate checks the complete price version before its append-only publication.
func (v *Validator) Validate(price domain.PriceRuleVersion) error {
	if v == nil || price.Validate() != nil || len(price.Rule) > maxRuleBytes ||
		!utf8.Valid(price.Rule) || !validCNYRate(price) {
		return ErrInvalidConfiguration
	}
	fields, err := parseObject(price.Rule)
	if err != nil {
		return ErrInvalidConfiguration
	}
	if id, err := domain.ParseProcurementVersion(fields[domain.ProcurementVersionField]); err != nil || id == price.ID {
		return ErrInvalidConfiguration
	}
	delete(fields, domain.ProcurementVersionField)
	var strategy string
	if raw, present := fields["pricing_strategy"]; present {
		if string(raw) == "null" || json.Unmarshal(raw, &strategy) != nil || strategy == "" || !domain.CompatiblePricingStrategy(strategy, price.Unit) {
			return ErrInvalidConfiguration
		}
		if strategy == domain.PricingSeedanceVideoTokensV1 && (string(fields["input_micros_per_1k"]) != "0" || fields["cached_input_micros_per_1k"] != nil || fields["time_pricing"] != nil) {
			return ErrInvalidConfiguration
		}
		delete(fields, "pricing_strategy")
	}
	if strategy == domain.PricingOriginalDurationV1 && fields["unit_quantity"] == nil {
		return ErrInvalidConfiguration
	}
	switch price.Unit {
	case domain.PricePer1KTokens:
		inputRaw, hasInput := fields["input_micros_per_1k"]
		outputRaw, hasOutput := fields["output_micros_per_1k"]
		if !hasInput || !hasOutput {
			return ErrInvalidConfiguration
		}
		input, ok := nonnegativeMicros(inputRaw)
		if !ok {
			return ErrInvalidConfiguration
		}
		output, ok := nonnegativeMicros(outputRaw)
		if !ok || (input == 0 && output == 0) {
			return ErrInvalidConfiguration
		}
		for name, raw := range fields {
			switch name {
			case "input_micros_per_1k", "output_micros_per_1k":
			case "time_pricing":
				var schedule string
				if json.Unmarshal(raw, &schedule) != nil || schedule != domain.DeepSeekCN2026 || price.Currency != "CNY" || fields["cached_input_micros_per_1k"] == nil || fields["by_resolution"] != nil || input == 0 {
					return ErrInvalidConfiguration
				}
			case "cached_input_micros_per_1k":
				cached, valid := nonnegativeMicros(raw)
				if !valid || cached > input {
					return ErrInvalidConfiguration
				}
			case "by_resolution":
				if input != 0 || !validCoefficients(raw) {
					return ErrInvalidConfiguration
				}
			case "markup_multiplier", "credit_ratio":
				if !validMultiplierOrRatio(raw) {
					return ErrInvalidConfiguration
				}
			default:
				return ErrInvalidConfiguration
			}
		}
	default:
		baseRaw, hasBase := fields["base_micros"]
		if !hasBase {
			return ErrInvalidConfiguration
		}
		if _, ok := nonnegativeMicros(baseRaw); !ok {
			return ErrInvalidConfiguration
		}
		for name, raw := range fields {
			switch name {
			case "free_reference_images", "reference_image_micros":
				if price.Unit != domain.PricePerImage && price.Unit != domain.PricePerRequest {
					return ErrInvalidConfiguration
				}
				if _, ok := nonnegativeMicros(raw); !ok {
					return ErrInvalidConfiguration
				}
			case "reference_scope":
				var scope string
				if json.Unmarshal(raw, &scope) != nil || (scope != "per_task" && scope != "per_output") ||
					(price.Unit != domain.PricePerImage && price.Unit != domain.PricePerRequest) ||
					(scope == "per_output" && price.Unit != domain.PricePerImage) {
					return ErrInvalidConfiguration
				}
			case "base_micros":
			case "unit_quantity":
				quantity, ok := nonnegativeMicros(raw)
				if !ok || quantity == 0 || strategy != domain.PricingOriginalDurationV1 {
					return ErrInvalidConfiguration
				}
			case "by_mode", "by_resolution":
				if !validCoefficients(raw) {
					return ErrInvalidConfiguration
				}
			case "markup_multiplier", "credit_ratio":
				if !validMultiplierOrRatio(raw) {
					return ErrInvalidConfiguration
				}
			default:
				return ErrInvalidConfiguration
			}
		}
	}
	_, hasScope := fields["reference_scope"]
	_, hasFree := fields["free_reference_images"]
	_, hasReference := fields["reference_image_micros"]
	if hasScope != hasFree || hasScope != hasReference {
		return ErrInvalidConfiguration
	}
	return nil
}

func validCNYRate(price domain.PriceRuleVersion) bool {
	if price.Currency != "CNY" || price.FXRateToCNY == "" {
		return true
	}
	rate, ok := new(big.Rat).SetString(price.FXRateToCNY)
	return ok && rate.Cmp(big.NewRat(1, 1)) == 0
}

// parseObject retains raw values so numbers never pass through float64. It
// checks duplicate property names after JSON escape decoding.
func parseObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidConfiguration
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidConfiguration
		}
		name, ok := token.(string)
		if !ok {
			return nil, ErrInvalidConfiguration
		}
		if _, duplicated := fields[name]; duplicated {
			return nil, ErrInvalidConfiguration
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidConfiguration
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, ErrInvalidConfiguration
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidConfiguration
	}
	return fields, nil
}

func validCoefficients(raw json.RawMessage) bool {
	entries, err := parseObject(raw)
	if err != nil {
		return false
	}
	for name, value := range entries {
		if strings.TrimSpace(name) == "" || !positiveCoefficient(value) {
			return false
		}
	}
	return true
}

func nonnegativeMicros(raw json.RawMessage) (uint64, bool) {
	text, ok := numberText(raw)
	if !ok || text == "" {
		return 0, false
	}
	// Require an integer JSON token. Decimal and exponent spellings can expand
	// far beyond PostgreSQL jsonb's numeric range even when their value is 0 or 1.
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	amount, err := strconv.ParseUint(text, 10, 64)
	return amount, err == nil && amount <= maxSafeInteger
}

func positiveCoefficient(raw json.RawMessage) bool {
	text, ok := numberText(raw)
	if !ok || strings.ContainsAny(text, "-+eE") {
		return false
	}
	whole, fraction, dotted := strings.Cut(text, ".")
	if len(whole) > 7 || dotted && (len(fraction) == 0 || len(fraction) > 6) {
		return false
	}
	wholeValue, err := strconv.ParseUint(whole, 10, 64)
	if err != nil || wholeValue > 1000000 {
		return false
	}
	fractionValue := uint64(0)
	if dotted {
		fractionValue, err = strconv.ParseUint(fraction, 10, 64)
		if err != nil {
			return false
		}
		for len(fraction) < 6 {
			fractionValue *= 10
			fraction += "0"
		}
	}
	scaled := wholeValue*1000000 + fractionValue
	return scaled > 0 && scaled <= maxCoefficient
}

func numberText(raw json.RawMessage) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	number, ok := value.(json.Number)
	return number.String(), ok
}

func validMultiplierOrRatio(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case string:
		text = v
	default:
		return false
	}
	if text == "" || strings.ContainsAny(text, "+-eE") {
		return false
	}
	whole, fraction, dotted := strings.Cut(text, ".")
	if len(whole) == 0 || len(whole) > 7 || (dotted && (len(fraction) == 0 || len(fraction) > 6)) {
		return false
	}
	wholeValue, err := strconv.ParseUint(whole, 10, 64)
	if err != nil || wholeValue > 1000000 {
		return false
	}
	fractionValue := uint64(0)
	if dotted {
		fractionValue, err = strconv.ParseUint(fraction, 10, 64)
		if err != nil {
			return false
		}
		for len(fraction) < 6 {
			fractionValue *= 10
			fraction += "0"
		}
	}
	scaled := wholeValue*1000000 + fractionValue
	return scaled > 0 && scaled <= maxCoefficient
}
