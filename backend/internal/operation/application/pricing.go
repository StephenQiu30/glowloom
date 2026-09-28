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

// QuoteCostDetail explains the exact inputs to a quote before the final CNY
// ceiling. Unit prices are denominated in Currency, not necessarily CNY.
type QuoteCostDetail struct {
	Unit                  catalogdomain.PriceUnit `json:"unit"`
	Quantity              json.Number             `json:"quantity,omitempty"`
	UnitPriceMicros       *uint64                 `json:"unit_price_micros,omitempty"`
	Outputs               int64                   `json:"outputs"`
	Multipliers           map[string]json.Number  `json:"multipliers,omitempty"`
	Currency              string                  `json:"currency"`
	FXRateToCNY           string                  `json:"fx_rate_to_cny,omitempty"`
	EstimatedInputTokens  *int64                  `json:"estimated_input_tokens,omitempty"`
	MaxOutputTokens       *int64                  `json:"max_output_tokens,omitempty"`
	InputUnitPriceMicros  *uint64                 `json:"input_unit_price_micros,omitempty"`
	OutputUnitPriceMicros *uint64                 `json:"output_unit_price_micros,omitempty"`
}

// BillableUsage is the normalized provider evidence for one logical task.
// Pointer fields distinguish an explicit zero charge from missing evidence.
// BilledDurationMS is the total billed duration across all generated outputs.
type BillableUsage struct {
	OutputCount      *int64 `json:"output_count,omitempty"`
	BilledDurationMS *int64 `json:"billed_duration_ms,omitempty"`
	BillableRequests *int64 `json:"billable_requests,omitempty"`
	CharacterCount   *int64 `json:"character_count,omitempty"`
	InputTokens      *int64 `json:"input_tokens,omitempty"`
	OutputTokens     *int64 `json:"output_tokens,omitempty"`
}

// ParseBillableUsage accepts only exact integer quantities. A missing field is
// not equivalent to an explicit zero, and unknown or repeated keys are unsafe.
func ParseBillableUsage(raw json.RawMessage) (BillableUsage, error) {
	fields, err := pricingObject(raw)
	if err != nil || len(fields) == 0 {
		return BillableUsage{}, ErrInvalidPricingInput
	}
	var usage BillableUsage
	for name, encoded := range fields {
		if string(bytes.TrimSpace(encoded)) == "null" {
			return BillableUsage{}, ErrInvalidPricingInput
		}
		var value int64
		if err := json.Unmarshal(encoded, &value); err != nil || value < 0 {
			return BillableUsage{}, ErrInvalidPricingInput
		}
		switch name {
		case "output_count":
			usage.OutputCount = &value
		case "billed_duration_ms":
			usage.BilledDurationMS = &value
		case "billable_requests":
			usage.BillableRequests = &value
		case "character_count":
			usage.CharacterCount = &value
		case "input_tokens":
			usage.InputTokens = &value
		case "output_tokens":
			usage.OutputTokens = &value
		default:
			return BillableUsage{}, ErrInvalidPricingInput
		}
	}
	return usage, nil
}

// CalculateQuote returns CNY micros from one immutable published price version.
// It keeps quantities, multipliers, and FX exact until one final ceiling.
func CalculateQuote(price catalogdomain.PriceRuleVersion, input PricingInput) (int64, error) {
	amount, _, err := CalculateQuoteDetailed(price, input)
	return amount, err
}

// CalculateQuoteDetailed returns the CNY amount and its immutable pricing
// factors from the same calculation, so the displayed factors cannot drift.
func CalculateQuoteDetailed(price catalogdomain.PriceRuleVersion, input PricingInput) (int64, QuoteCostDetail, error) {
	if len(input.ParameterMultipliers) != 0 {
		return 0, QuoteCostDetail{}, ErrUnsupportedPriceCoefficient
	}
	rule, fx, err := parsePrice(price)
	if err != nil {
		return 0, QuoteCostDetail{}, err
	}
	amount, detail, err := priceAmount(price.Unit, rule, input)
	if err != nil {
		return 0, QuoteCostDetail{}, err
	}
	detail.Currency = price.Currency
	if price.Currency != "CNY" {
		detail.FXRateToCNY = price.FXRateToCNY
	}
	result, err := finishPriceAmount(amount, fx)
	return result, detail, err
}

// CalculateActualCost prices conclusive provider usage against the same
// published rule and exchange rate frozen when the operation was quoted.
func CalculateActualCost(price catalogdomain.PriceRuleVersion, usage BillableUsage, mode, resolution string) (int64, error) {
	rule, fx, err := parsePrice(price)
	if err != nil {
		return 0, err
	}
	amount, err := actualPriceAmount(price.Unit, rule, usage)
	if err != nil {
		return 0, err
	}
	if price.Unit != catalogdomain.PricePer1KTokens {
		if err := applyPriceFactors(amount, rule, mode, resolution, nil); err != nil {
			return 0, err
		}
	}
	return finishPriceAmount(amount, fx)
}

func finishPriceAmount(amount, fx *big.Rat) (int64, error) {
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

func actualPriceAmount(unit catalogdomain.PriceUnit, rule parsedPriceRule, usage BillableUsage) (*big.Rat, error) {
	fields := 0
	for _, value := range []*int64{usage.OutputCount, usage.BilledDurationMS, usage.BillableRequests,
		usage.CharacterCount, usage.InputTokens, usage.OutputTokens} {
		if value != nil {
			fields++
			if *value < 0 {
				return nil, ErrInvalidPricingInput
			}
		}
	}
	if unit == catalogdomain.PricePer1KTokens {
		if fields != 2 || usage.InputTokens == nil || usage.OutputTokens == nil {
			return nil, ErrInvalidPricingInput
		}
		inputCost := new(big.Int).Mul(big.NewInt(*usage.InputTokens), new(big.Int).SetUint64(rule.inputMicros))
		outputCost := new(big.Int).Mul(big.NewInt(*usage.OutputTokens), new(big.Int).SetUint64(rule.outputMicros))
		return new(big.Rat).SetFrac(inputCost.Add(inputCost, outputCost), big.NewInt(1_000)), nil
	}
	if fields != 1 {
		return nil, ErrInvalidPricingInput
	}
	quantity := new(big.Rat)
	switch unit {
	case catalogdomain.PricePerImage:
		if usage.OutputCount == nil {
			return nil, ErrInvalidPricingInput
		}
		quantity.SetInt64(*usage.OutputCount)
	case catalogdomain.PricePerSecond:
		if usage.BilledDurationMS == nil {
			return nil, ErrInvalidPricingInput
		}
		quantity.SetFrac(big.NewInt(*usage.BilledDurationMS), big.NewInt(1_000))
	case catalogdomain.PricePerRequest:
		if usage.BillableRequests == nil || *usage.BillableRequests > 1 {
			return nil, ErrInvalidPricingInput
		}
		quantity.SetInt64(*usage.BillableRequests)
	case catalogdomain.PricePer1KChars:
		if usage.CharacterCount == nil {
			return nil, ErrInvalidPricingInput
		}
		blocks := *usage.CharacterCount / 1_000
		if *usage.CharacterCount%1_000 != 0 {
			blocks++
		}
		quantity.SetInt64(blocks)
	default:
		return nil, ErrInvalidPricingRule
	}
	return quantity.Mul(quantity, new(big.Rat).SetInt(new(big.Int).SetUint64(rule.baseMicros))), nil
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

func priceAmount(unit catalogdomain.PriceUnit, rule parsedPriceRule, input PricingInput) (*big.Rat, QuoteCostDetail, error) {
	detail := QuoteCostDetail{Unit: unit, Outputs: input.OutputCount}
	if unit == catalogdomain.PricePer1KTokens {
		if input.EstimatedInputTokens < 0 || input.MaxOutputTokens <= 0 {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		inputTokens, outputTokens := input.EstimatedInputTokens, input.MaxOutputTokens
		inputRate, outputRate := rule.inputMicros, rule.outputMicros
		detail.EstimatedInputTokens, detail.MaxOutputTokens = &inputTokens, &outputTokens
		detail.InputUnitPriceMicros, detail.OutputUnitPriceMicros = &inputRate, &outputRate
		inputCost := new(big.Int).Mul(big.NewInt(input.EstimatedInputTokens), new(big.Int).SetUint64(rule.inputMicros))
		outputCost := new(big.Int).Mul(big.NewInt(input.MaxOutputTokens), new(big.Int).SetUint64(rule.outputMicros))
		return new(big.Rat).SetFrac(inputCost.Add(inputCost, outputCost), big.NewInt(1_000)), detail, nil
	}
	quantity := new(big.Rat)
	displayQuantity := new(big.Rat)
	switch unit {
	case catalogdomain.PricePerImage:
		if input.OutputCount <= 0 {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		quantity.SetInt64(input.OutputCount)
		displayQuantity.SetInt64(1)
	case catalogdomain.PricePerSecond:
		if input.OutputCount <= 0 || input.TargetDurationMS <= 0 || len(input.AllowedDurationsMS) == 0 {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		selected := int64(0)
		for _, duration := range input.AllowedDurationsMS {
			if duration <= 0 {
				return nil, QuoteCostDetail{}, ErrInvalidPricingInput
			}
			if duration >= input.TargetDurationMS && (selected == 0 || duration < selected) {
				selected = duration
			}
		}
		if selected == 0 {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		quantity.SetFrac(new(big.Int).Mul(big.NewInt(selected), big.NewInt(input.OutputCount)), big.NewInt(1_000))
		displayQuantity.SetFrac(big.NewInt(selected), big.NewInt(1_000))
	case catalogdomain.PricePerRequest:
		quantity.SetInt64(1)
		displayQuantity.SetInt64(1)
	case catalogdomain.PricePer1KChars:
		if input.CharacterCount < 0 {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		blocks := input.CharacterCount / 1_000
		if input.CharacterCount%1_000 != 0 {
			blocks++
		}
		quantity.SetInt64(blocks)
		displayQuantity.SetInt64(blocks)
	default:
		return nil, QuoteCostDetail{}, ErrInvalidPricingRule
	}
	unitPrice := rule.baseMicros
	detail.UnitPriceMicros = &unitPrice
	detail.Quantity = decimalNumber(displayQuantity, 3)
	amount := quantity.Mul(quantity, new(big.Rat).SetInt(new(big.Int).SetUint64(rule.baseMicros)))
	if err := applyPriceFactors(amount, rule, input.Mode, input.Resolution, &detail); err != nil {
		return nil, QuoteCostDetail{}, err
	}
	return amount, detail, nil
}

func applyPriceFactors(amount *big.Rat, rule parsedPriceRule, mode, resolution string, detail *QuoteCostDetail) error {
	if (len(rule.byMode) != 0 && mode == "") || (len(rule.byResolution) != 0 && resolution == "") {
		return ErrInvalidPricingInput
	}
	if factor, ok := rule.byMode[mode]; ok {
		amount.Mul(amount, factor)
		if detail != nil {
			detail.Multipliers = map[string]json.Number{"mode_" + mode: decimalNumber(factor, 6)}
		}
	}
	if factor, ok := rule.byResolution[resolution]; ok {
		amount.Mul(amount, factor)
		if detail != nil {
			if detail.Multipliers == nil {
				detail.Multipliers = make(map[string]json.Number)
			}
			detail.Multipliers["resolution_"+resolution] = decimalNumber(factor, 6)
		}
	}
	return nil
}

func decimalNumber(value *big.Rat, places int) json.Number {
	formatted := value.FloatString(places)
	if places > 0 {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return json.Number(formatted)
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
