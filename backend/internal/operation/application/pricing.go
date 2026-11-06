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
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
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
	QuotedAt             time.Time
	ReferenceImageCount  int64
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
	strategy          string
	timePricing       string
	freeReferences    uint64
	referenceMicros   uint64
	referenceScope    string
	baseMicros        uint64
	cachedInputMicros *uint64
	inputMicros       uint64
	outputMicros      uint64
	markupMultiplier  *big.Rat
	creditRatio       *big.Rat
	byMode            map[string]*big.Rat
	byResolution      map[string]*big.Rat
}

// QuoteCostDetail explains the exact inputs to a quote before the final CNY
// ceiling. Unit prices are denominated in Currency, not necessarily CNY.
type QuoteCostDetail struct {
	PricingStrategy       string                  `json:"pricing_strategy,omitempty"`
	TimePricing           string                  `json:"time_pricing,omitempty"`
	ReservationPeriod     string                  `json:"reservation_period,omitempty"`
	SourceURL             string                  `json:"source_url,omitempty"`
	ReviewedAt            *time.Time              `json:"reviewed_at,omitempty"`
	ReferenceImageCount   *int64                  `json:"reference_image_count,omitempty"`
	FreeReferenceImages   *uint64                 `json:"free_reference_images,omitempty"`
	ReferenceImageMicros  *uint64                 `json:"reference_image_micros,omitempty"`
	ReferenceScope        string                  `json:"reference_scope,omitempty"`
	PriceRuleVersionID    uuid.UUID               `json:"price_rule_version_id"`
	PriceVersionNo        int                     `json:"price_version_no"`
	AmountKind            string                  `json:"amount_kind"`
	Unit                  catalogdomain.PriceUnit `json:"unit"`
	Quantity              json.Number             `json:"quantity,omitempty"`
	UnitPriceMicros       *uint64                 `json:"unit_price_micros,omitempty"`
	Outputs               int64                   `json:"outputs"`
	Multipliers           map[string]json.Number  `json:"multipliers,omitempty"`
	MarkupMultiplier      json.Number             `json:"markup_multiplier,omitempty"`
	WholeCredits          bool                    `json:"whole_credits,omitempty"`
	ExactQuoteMicros      string                  `json:"exact_quote_micros,omitempty"`
	MaxChargeCredits      string                  `json:"max_charge_credits,omitempty"`
	ExactCredits          string                  `json:"exact_credits,omitempty"`
	EstimatedCredits      *int64                  `json:"estimated_credits,omitempty"`
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
	ReferenceImageCount *int64 `json:"reference_image_count,omitempty"`
	OutputCount         *int64 `json:"output_count,omitempty"`
	BilledDurationMS    *int64 `json:"billed_duration_ms,omitempty"`
	BillableRequests    *int64 `json:"billable_requests,omitempty"`
	CharacterCount      *int64 `json:"character_count,omitempty"`
	CachedInputTokens   *int64 `json:"cached_input_tokens,omitempty"`
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
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
		case "reference_image_count":
			usage.ReferenceImageCount = &value
		case "output_count":
			usage.OutputCount = &value
		case "billed_duration_ms":
			usage.BilledDurationMS = &value
		case "billable_requests":
			usage.BillableRequests = &value
		case "character_count":
			usage.CharacterCount = &value
		case "cached_input_tokens":
			usage.CachedInputTokens = &value
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
	if rule.timePricing != "" {
		if _, err := catalogdomain.DeepSeekOffPeak2026(input.QuotedAt); err != nil {
			return 0, QuoteCostDetail{}, ErrInvalidPricingInput
		}
	}
	amount, detail, err := priceAmount(price.Unit, rule, input)
	if err != nil {
		return 0, QuoteCostDetail{}, err
	}
	if rule.referenceScope != "" {
		surcharge, err := referencePrice(rule, input.ReferenceImageCount, input.OutputCount)
		if err != nil {
			return 0, QuoteCostDetail{}, err
		}
		amount.Add(amount, surcharge)
		refs := input.ReferenceImageCount
		detail.ReferenceImageCount, detail.FreeReferenceImages, detail.ReferenceImageMicros = &refs, &rule.freeReferences, &rule.referenceMicros
		detail.ReferenceScope = rule.referenceScope
	}
	if rule.timePricing != "" {
		detail.TimePricing, detail.ReservationPeriod = rule.timePricing, "peak"
	}
	detail.PricingStrategy = rule.strategy
	detail.SourceURL, detail.ReviewedAt = price.SourceURL, price.ReviewedAt
	detail.PriceRuleVersionID, detail.PriceVersionNo, detail.AmountKind = price.ID, price.VersionNo, "reservation"
	amount.Mul(amount, rule.markupMultiplier)
	detail.MarkupMultiplier = decimalNumber(rule.markupMultiplier, 4)
	detail.Currency = price.Currency
	if price.Currency != "CNY" {
		detail.FXRateToCNY = price.FXRateToCNY
	}
	result, err := finishPriceAmount(amount, fx)
	if err != nil {
		return 0, QuoteCostDetail{}, err
	}
	credits := calculateCredits(result, rule.creditRatio)
	detail.EstimatedCredits = &credits
	detail.ExactQuoteMicros = strconv.FormatInt(result, 10)
	detail.ExactCredits = billingdomain.Credits(result)
	detail.MaxChargeCredits = detail.ExactCredits
	return result, detail, nil
}

// CalculateActualCost prices conclusive provider usage against the same
// published rule and exchange rate frozen when the operation was quoted.
func CalculateActualCost(price catalogdomain.PriceRuleVersion, usage BillableUsage, mode, resolution string) (int64, error) {
	return CalculateActualCostDuring(price, usage, mode, resolution, time.Time{}, time.Time{})
}

// CalculateActualCostDuring uses the original dispatch and response observations,
// never a recovery clock. A boundary-crossing request needs supplier review:
// completion creation time is not documented as its billing timestamp.
func CalculateActualCostDuring(price catalogdomain.PriceRuleVersion, usage BillableUsage, mode, resolution string, startedAt, receivedAt time.Time) (int64, error) {
	rule, fx, err := parsePrice(price)
	if err != nil {
		return 0, err
	}
	offPeak := false
	if rule.timePricing != "" {
		// HTTP submissions have a five-minute bound. Within this interval no two
		// tariff transitions can occur, so matching endpoints prove one rate.
		if startedAt.IsZero() || receivedAt.IsZero() || receivedAt.Before(startedAt) || receivedAt.Sub(startedAt) > 5*time.Minute {
			return 0, ErrInvalidPricingInput
		}
		var err error
		offPeak, err = catalogdomain.DeepSeekOffPeak2026(startedAt)
		if err != nil {
			return 0, ErrInvalidPricingInput
		}
		endOffPeak, err := catalogdomain.DeepSeekOffPeak2026(receivedAt)
		if err != nil || offPeak != endOffPeak {
			return 0, ErrInvalidPricingInput
		}
	}
	amount, err := actualPriceAmount(price.Unit, rule, usage)
	if err != nil {
		return 0, err
	}
	if err := applyPriceFactors(amount, rule, mode, resolution, nil); err != nil {
		return 0, err
	}
	if rule.referenceScope != "" {
		if usage.ReferenceImageCount == nil {
			return 0, ErrInvalidPricingInput
		}
		outputs := int64(1)
		if rule.referenceScope == "per_output" {
			if usage.OutputCount == nil {
				return 0, ErrInvalidPricingInput
			}
			outputs = *usage.OutputCount
		}
		surcharge, err := referencePrice(rule, *usage.ReferenceImageCount, outputs)
		if err != nil {
			return 0, err
		}
		amount.Add(amount, surcharge)
	} else if usage.ReferenceImageCount != nil {
		return 0, ErrInvalidPricingInput
	}
	if offPeak {
		amount.Mul(amount, big.NewRat(1, 2))
	}
	amount.Mul(amount, rule.markupMultiplier)
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
		if rule.strategy == catalogdomain.PricingSeedanceVideoTokensV1 {
			expectedFields := 1
			if usage.InputTokens != nil {
				expectedFields++
			}
			if usage.OutputTokens == nil || usage.CachedInputTokens != nil || (usage.InputTokens != nil && *usage.InputTokens != 0) || fields != expectedFields {
				return nil, ErrInvalidPricingInput
			}
			return new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(*usage.OutputTokens), new(big.Int).SetUint64(rule.outputMicros)), big.NewInt(1000)), nil
		}
		if fields != 2 || usage.InputTokens == nil || usage.OutputTokens == nil {
			return nil, ErrInvalidPricingInput
		}
		uncached := *usage.InputTokens
		cachedCost := new(big.Int)
		if rule.cachedInputMicros != nil {
			if usage.CachedInputTokens == nil || *usage.CachedInputTokens < 0 || *usage.CachedInputTokens > uncached {
				return nil, ErrInvalidPricingInput
			}
			uncached -= *usage.CachedInputTokens
			cachedCost.Mul(big.NewInt(*usage.CachedInputTokens), new(big.Int).SetUint64(*rule.cachedInputMicros))
		} else if usage.CachedInputTokens != nil {
			return nil, ErrInvalidPricingInput
		}
		inputCost := new(big.Int).Mul(big.NewInt(uncached), new(big.Int).SetUint64(rule.inputMicros))
		inputCost.Add(inputCost, cachedCost)
		outputCost := new(big.Int).Mul(big.NewInt(*usage.OutputTokens), new(big.Int).SetUint64(rule.outputMicros))
		return new(big.Rat).SetFrac(inputCost.Add(inputCost, outputCost), big.NewInt(1_000)), nil
	}
	if usage.CachedInputTokens != nil || fields != 1 {
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
	rule := parsedPriceRule{
		markupMultiplier: big.NewRat(1, 1),
		creditRatio:      big.NewRat(10, 1),
	}
	if id, err := catalogdomain.ParseProcurementVersion(fields[catalogdomain.ProcurementVersionField]); err != nil || id == price.ID {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	delete(fields, catalogdomain.ProcurementVersionField)
	if raw, present := fields["pricing_strategy"]; present {
		if string(bytes.TrimSpace(raw)) == "null" || json.Unmarshal(raw, &rule.strategy) != nil || rule.strategy == "" || !catalogdomain.CompatiblePricingStrategy(rule.strategy, price.Unit) {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		if rule.strategy == catalogdomain.PricingSeedanceVideoTokensV1 && (string(fields["input_micros_per_1k"]) != "0" || fields["cached_input_micros_per_1k"] != nil || fields["time_pricing"] != nil) {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		delete(fields, "pricing_strategy")
	}
	if price.Unit == catalogdomain.PricePer1KTokens {
		inputRaw, hasInput := fields["input_micros_per_1k"]
		outputRaw, hasOutput := fields["output_micros_per_1k"]
		if !hasInput || !hasOutput {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		var ok bool
		rule.inputMicros, ok = priceMicros(inputRaw)
		if !ok {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		rule.outputMicros, ok = priceMicros(outputRaw)
		if !ok || (rule.inputMicros == 0 && rule.outputMicros == 0) {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		for name, raw := range fields {
			switch name {
			case "input_micros_per_1k", "output_micros_per_1k":
			case "time_pricing":
				if json.Unmarshal(raw, &rule.timePricing) != nil || rule.timePricing != catalogdomain.DeepSeekCN2026 || price.Currency != "CNY" || fields["cached_input_micros_per_1k"] == nil || fields["by_resolution"] != nil || rule.inputMicros == 0 {
					return parsedPriceRule{}, nil, ErrInvalidPricingRule
				}
			case "cached_input_micros_per_1k":
				value, valid := priceMicros(raw)
				if !valid || value > rule.inputMicros {
					return parsedPriceRule{}, nil, ErrInvalidPricingRule
				}
				rule.cachedInputMicros = &value
			case "by_resolution":
				if rule.inputMicros != 0 {
					return parsedPriceRule{}, nil, ErrInvalidPricingRule
				}
				rule.byResolution, err = pricingCoefficients(raw)
				if err != nil {
					return parsedPriceRule{}, nil, err
				}
			case "markup_multiplier":
				rule.markupMultiplier, ok = parseMultiplierOrRatio(raw)
				if !ok {
					return parsedPriceRule{}, nil, ErrInvalidPricingRule
				}
			case "credit_ratio":
				rule.creditRatio, ok = parseMultiplierOrRatio(raw)
				if !ok {
					return parsedPriceRule{}, nil, ErrInvalidPricingRule
				}
			default:
				return parsedPriceRule{}, nil, ErrInvalidPricingRule
			}
		}
		return rule, fx, nil
	}
	if raw, present := fields["reference_scope"]; present {
		if price.Unit != catalogdomain.PricePerImage && price.Unit != catalogdomain.PricePerRequest {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		if json.Unmarshal(raw, &rule.referenceScope) != nil || (rule.referenceScope != "per_task" && rule.referenceScope != "per_output") {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		var ok bool
		rule.freeReferences, ok = priceMicros(fields["free_reference_images"])
		if !ok {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		rule.referenceMicros, ok = priceMicros(fields["reference_image_micros"])
		if !ok {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
		if rule.referenceScope == "per_output" && price.Unit != catalogdomain.PricePerImage {
			return parsedPriceRule{}, nil, ErrInvalidPricingRule
		}
	} else if fields["free_reference_images"] != nil || fields["reference_image_micros"] != nil {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	baseRaw, hasBase := fields["base_micros"]
	if !hasBase {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	var ok bool
	rule.baseMicros, ok = priceMicros(baseRaw)
	if !ok {
		return parsedPriceRule{}, nil, ErrInvalidPricingRule
	}
	for name, raw := range fields {
		switch name {
		case "base_micros", "free_reference_images", "reference_image_micros", "reference_scope":
		case "by_mode":
			rule.byMode, err = pricingCoefficients(raw)
		case "by_resolution":
			rule.byResolution, err = pricingCoefficients(raw)
		case "markup_multiplier":
			rule.markupMultiplier, ok = parseMultiplierOrRatio(raw)
			if !ok {
				return parsedPriceRule{}, nil, ErrInvalidPricingRule
			}
		case "credit_ratio":
			rule.creditRatio, ok = parseMultiplierOrRatio(raw)
			if !ok {
				return parsedPriceRule{}, nil, ErrInvalidPricingRule
			}
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
		if input.EstimatedInputTokens < 0 || input.MaxOutputTokens <= 0 || (rule.strategy == catalogdomain.PricingSeedanceVideoTokensV1 && input.EstimatedInputTokens != 0) {
			return nil, QuoteCostDetail{}, ErrInvalidPricingInput
		}
		inputTokens, outputTokens := input.EstimatedInputTokens, input.MaxOutputTokens
		inputRate, outputRate := rule.inputMicros, rule.outputMicros
		detail.EstimatedInputTokens, detail.MaxOutputTokens = &inputTokens, &outputTokens
		detail.InputUnitPriceMicros, detail.OutputUnitPriceMicros = &inputRate, &outputRate
		inputCost := new(big.Int).Mul(big.NewInt(input.EstimatedInputTokens), new(big.Int).SetUint64(rule.inputMicros))
		outputCost := new(big.Int).Mul(big.NewInt(input.MaxOutputTokens), new(big.Int).SetUint64(rule.outputMicros))
		amount := new(big.Rat).SetFrac(inputCost.Add(inputCost, outputCost), big.NewInt(1_000))
		if err := applyPriceFactors(amount, rule, input.Mode, input.Resolution, &detail); err != nil {
			return nil, QuoteCostDetail{}, err
		}
		return amount, detail, nil
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
	if rule.strategy != "" && ((len(rule.byMode) != 0 && rule.byMode[mode] == nil) || (len(rule.byResolution) != 0 && rule.byResolution[resolution] == nil)) {
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

func calculateCredits(micros int64, ratio *big.Rat) int64 {
	if micros <= 0 || ratio == nil || ratio.Sign() <= 0 {
		return 0
	}
	r := new(big.Rat).SetInt64(micros)
	r.Mul(r, ratio)
	r.Quo(r, big.NewRat(1_000_000, 1))
	whole, remainder := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if remainder.Sign() > 0 {
		whole.Add(whole, big.NewInt(1))
	}
	return whole.Int64()
}

func parseMultiplierOrRatio(raw json.RawMessage) (*big.Rat, bool) {
	text := strings.Trim(string(raw), "\"")
	return priceCoefficient(json.RawMessage(text))
}

// referencePrice is a math-only contract. Publication remains disabled until a
// reviewed per-model counting contract is accepted; fixtures are not price evidence.
func referencePrice(rule parsedPriceRule, references, outputs int64) (*big.Rat, error) {
	if references < 0 || outputs < 0 {
		return nil, ErrInvalidPricingInput
	}
	quantity := new(big.Int).SetInt64(references)
	quantity.Sub(quantity, new(big.Int).SetUint64(rule.freeReferences))
	if quantity.Sign() < 0 {
		quantity.SetInt64(0)
	}
	if rule.referenceScope == "per_output" {
		quantity.Mul(quantity, big.NewInt(outputs))
	}
	quantity.Mul(quantity, new(big.Int).SetUint64(rule.referenceMicros))
	return new(big.Rat).SetInt(quantity), nil
}
