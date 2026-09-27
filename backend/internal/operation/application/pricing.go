// Package application contains operation use-case rules that do not depend on adapters.
package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	catalogdomain "github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

var (
	// ErrInvalidPricingRule means the price snapshot is not publishable.
	ErrInvalidPricingRule = errors.New("invalid pricing rule")
	// ErrInvalidPricingInput means the validated target cannot be priced with this unit.
	ErrInvalidPricingInput = errors.New("invalid pricing input")
	// ErrUnsupportedPriceCoefficient means a caller requested an unpublished coefficient type.
	ErrUnsupportedPriceCoefficient = errors.New("unsupported price coefficient")
	// ErrQuoteAmountOverflow means a calculated quote cannot fit the persisted bigint amount.
	ErrQuoteAmountOverflow = errors.New("quote amount exceeds bigint")
)

const (
	maxPriceRuleBytes = 64 * 1024
	maxSafeInteger    = uint64(9_007_199_254_740_991)
	coefficientScale  = uint64(1_000_000)
)

// PricingInput contains validated, frozen target quantities. CharacterCount is
// a character count supplied by the input builder, not a UTF-8 byte count.
// ParameterMultipliers is rejected until its publication shape is specified.
type PricingInput struct {
	OutputCount          int64
	TargetDurationMS     int64
	AllowedDurationsMS   []int64
	CharacterCount       int64
	EstimatedInputTokens int64
	MaxOutputTokens      int64
	Mode                 string
	Resolution           string
	ParameterMultipliers map[string]string
}

type parsedPriceRule struct {
	baseMicros   uint64
	inputMicros  uint64
	outputMicros uint64
	byMode       map[string]*big.Rat
	byResolution map[string]*big.Rat
}

// CalculateQuote returns CNY micros from one immutable published price version.
// It keeps quantities, multipliers, and FX exact until one final ceiling.
func CalculateQuote(price catalogdomain.PriceRuleVersion, input PricingInput) (int64, error) {
	if len(input.ParameterMultipliers) != 0 {
		return 0, ErrUnsupportedPriceCoefficient
	}
	rule, fx, err := parsePrice(price)
	if err != nil {
		return 0, err
	}
	amount, err := priceAmount(price.Unit, rule, input)
	if err != nil {
		return 0, err
	}
	amount.Mul(amount, fx)
	whole, remainder := new(big.Int).QuoRem(amount.Num(), amount.Denom(), new(big.Int))
	if remainder.Sign() > 0 {
		whole.Add(whole, big.NewInt(1))
	}
	if !whole.IsInt64() {
		return 0, ErrQuoteAmountOverflow
	}
	return whole.Int64(), nil
}

func parsePrice(price catalogdomain.PriceRuleVersion) (parsedPriceRule, *big.Rat, error) {
	if price.Validate() != nil || len(price.Rule) > maxPriceRuleBytes || !utf8.Valid(price.Rule) {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	fx := big.NewRat(1, 1)
	if price.FXRateToCNY != "" {
		var ok bool
		fx, ok = new(big.Rat).SetString(price.FXRateToCNY)
		if !ok || fx.Sign() <= 0 {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
	}
	if price.Currency == "CNY" && fx.Cmp(big.NewRat(1, 1)) != 0 {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	fields, err := pricingObject(price.Rule)
	if err != nil {
		return parsedPriceRule{}, nil, err
	}
	rule := parsedPriceRule{}
	if price.Unit == catalogdomain.PricePer1KTokens {
		if len(fields) != 2 {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		var ok bool
		rule.inputMicros, ok = priceMicros(fields["input_micros_per_1k"])
		if !ok {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		rule.outputMicros, ok = priceMicros(fields["output_micros_per_1k"])
		if !ok || rule.inputMicros == 0 && rule.outputMicros == 0 {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		return rule, fx, nil
	}
	if len(fields) < 1 || len(fields) > 3 {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	var ok bool
	rule.baseMicros, ok = priceMicros(fields["base_micros"])
	if !ok {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	for name, raw := range fields {
		switch name {
		case "base_micros":
		case "by_mode":
			rule.byMode, err = pricingCoefficients(raw)
		case "by_resolution":
			rule.byResolution, err = pricingCoefficients(raw)
		default:
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		if err != nil {
			return parsedPriceRule{}, nil, err
		}
	}
	return rule, fx, nil
}

func priceAmount(unit catalogdomain.PriceUnit, rule parsedPriceRule, input PricingInput) (*big.Rat, error) {
	if unit == catalogdomain.PricePer1KTokens {
		if input.EstimatedInputTokens < 0 || input.MaxOutputTokens <= 0 {
			return nil, ErrInvalidPricingInput
		}
		inputCost := new(big.Int).Mul(big.NewInt(input.EstimatedInputTokens), new(big.Int).SetUint64(rule.inputMicros))
		outputCost := new(big.Int).Mul(big.NewInt(input.MaxOutputTokens), new(big.Int).SetUint64(rule.outputMicros))
		return new(big.Rat).SetFrac(inputCost.Add(inputCost, outputCost), big.NewInt(1_000)), nil
	}
	quantity := new(big.Rat)
	switch unit {
	case catalogdomain.PricePerImage:
		if input.OutputCount <= 0 {
			return nil, ErrInvalidPricingInput
		}
		quantity.SetInt64(input.OutputCount)
	case catalogdomain.PricePerSecond:
		if input.OutputCount <= 0 || input.TargetDurationMS <= 0 || len(input.AllowedDurationsMS) == 0 {
			return nil, ErrInvalidPricingInput
		}
		selected := int64(0)
		for _, duration := range input.AllowedDurationsMS {
			if duration <= 0 {
				return nil, ErrInvalidPricingInput
			}
			if duration >= input.TargetDurationMS && (selected == 0 || duration < selected) {
				selected = duration
			}
		}
		if selected == 0 {
			return nil, ErrInvalidPricingInput
		}
		quantity.SetFrac(new(big.Int).Mul(big.NewInt(selected), big.NewInt(input.OutputCount)), big.NewInt(1_000))
	case catalogdomain.PricePerRequest:
		quantity.SetInt64(1)
	case catalogdomain.PricePer1KChars:
		if input.CharacterCount < 0 {
			return nil, ErrInvalidPricingInput
		}
		blocks := input.CharacterCount / 1_000
		if input.CharacterCount%1_000 != 0 {
			blocks++
		}
		quantity.SetInt64(blocks)
	default:
		return nil, ErrInvalidPricingRule
	}
	amount := quantity.Mul(quantity, new(big.Rat).SetInt(new(big.Int).SetUint64(rule.baseMicros)))
	if (len(rule.byMode) != 0 && input.Mode == "") ||
		(len(rule.byResolution) != 0 && input.Resolution == "") {
		return nil, ErrInvalidPricingInput
	}
	if factor, ok := rule.byMode[input.Mode]; ok {
		amount.Mul(amount, factor)
	}
	if factor, ok := rule.byResolution[input.Resolution]; ok {
		amount.Mul(amount, factor)
	}
	return amount, nil
}

func pricingObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidPricingRule
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidPricingRule
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, ErrInvalidPricingRule
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidPricingRule
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidPricingRule
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, ErrInvalidPricingRule
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidPricingRule
	}
	return fields, nil
}

func pricingCoefficients(raw json.RawMessage) (map[string]*big.Rat, error) {
	fields, err := pricingObject(raw)
	if err != nil {
		return nil, err
	}
	factors := make(map[string]*big.Rat, len(fields))
	for name, value := range fields {
		if strings.TrimSpace(name) == "" {
			return nil, ErrInvalidPricingRule
		}
		factor, ok := priceCoefficient(value)
		if !ok {
			return nil, ErrInvalidPricingRule
		}
		factors[name] = factor
	}
	return factors, nil
}

func priceMicros(raw json.RawMessage) (uint64, bool) {
	text := string(raw)
	if text == "" {
		return 0, false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	amount, err := strconv.ParseUint(text, 10, 64)
	return amount, err == nil && amount <= maxSafeInteger
}

func priceCoefficient(raw json.RawMessage) (*big.Rat, bool) {
	text := string(raw)
	if text == "" || strings.ContainsAny(text, "+-eE") {
		return nil, false
	}
	whole, fraction, dotted := strings.Cut(text, ".")
	if len(whole) == 0 || len(whole) > 7 || dotted && (len(fraction) == 0 || len(fraction) > 6) {
		return nil, false
	}
	wholeValue, err := strconv.ParseUint(whole, 10, 64)
	if err != nil || wholeValue > 1_000_000 {
		return nil, false
	}
	fractionValue := uint64(0)
	if dotted {
		fractionValue, err = strconv.ParseUint(fraction, 10, 64)
		if err != nil {
			return nil, false
		}
		for len(fraction) < 6 {
			fractionValue *= 10
			fraction += "0"
		}
	}
	scaled := wholeValue*coefficientScale + fractionValue
	if scaled == 0 || scaled > 1_000_000*coefficientScale {
		return nil, false
	}
	return new(big.Rat).SetFrac(new(big.Int).SetUint64(scaled), big.NewInt(int64(coefficientScale))), true
}
