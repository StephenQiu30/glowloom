// Package pricesync normalizes documented, machine-readable public price sources.
package pricesync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

const (
	// OpenRouterCatalogURL is the public machine-readable model and pricing catalog.
	OpenRouterCatalogURL = "https://openrouter.ai/api/v1/models"
	openRouterParserV1   = "openrouter_models_v1"
)

// ErrInvalidOpenRouterPricing reports a malformed or unsupported source representation.
var ErrInvalidOpenRouterPricing = errors.New("invalid OpenRouter price source")

var decimalRatePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// ParseOpenRouterPriceCandidate converts only the flat token rates implemented by
// Glowloom. Conditional or extra billable dimensions remain review-only candidates.
func ParseOpenRouterPriceCandidate(modelID string, pricing json.RawMessage, fetchedAt time.Time) (domain.PriceCandidate, error) {
	if strings.TrimSpace(modelID) == "" || len(modelID) > 256 || strings.ContainsAny(modelID, "\x00\r\n") || fetchedAt.IsZero() {
		return domain.PriceCandidate{}, ErrInvalidOpenRouterPricing
	}
	canonical, err := canonicalJSON(pricing)
	if err != nil {
		return domain.PriceCandidate{}, fmt.Errorf("canonicalize OpenRouter pricing: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &fields); err != nil || fields == nil {
		return domain.PriceCandidate{}, ErrInvalidOpenRouterPricing
	}

	hash := sha256.Sum256(canonical)
	candidate := domain.PriceCandidate{
		ID:              uuid.NewSHA1(uuid.NameSpaceURL, []byte("glowloom:openrouter:"+openRouterParserV1+":"+modelID+":"+hex.EncodeToString(hash[:]))),
		AdapterKey:      "openrouter",
		ProviderModelID: modelID,
		SourceURL:       OpenRouterCatalogURL,
		SourceHash:      hex.EncodeToString(hash[:]),
		ParserVersion:   openRouterParserV1,
		SourceFetchedAt: fetchedAt.UTC(),
		Currency:        "USD",
		SourcePricing:   canonical,
		Status:          domain.PriceCandidateNeedsReview,
	}

	inputRaw, hasInput := fields["prompt"]
	outputRaw, hasOutput := fields["completion"]
	if !hasInput || !hasOutput {
		return candidate, nil
	}
	input, err := usdPerTokenToMicrosPer1K(inputRaw)
	if err != nil {
		return domain.PriceCandidate{}, fmt.Errorf("parse OpenRouter prompt price: %w", err)
	}
	output, err := usdPerTokenToMicrosPer1K(outputRaw)
	if err != nil {
		return domain.PriceCandidate{}, fmt.Errorf("parse OpenRouter completion price: %w", err)
	}
	if input == 0 && output == 0 {
		return candidate, nil
	}

	var cached *uint64
	if raw, exists := fields["input_cache_read"]; exists {
		value, err := usdPerTokenToMicrosPer1K(raw)
		if err != nil {
			return domain.PriceCandidate{}, fmt.Errorf("parse OpenRouter cache-read price: %w", err)
		}
		cached = &value
	}
	unsupported := false
	for key := range fields {
		switch key {
		case "prompt", "completion", "input_cache_read":
		default:
			// Includes conditional overrides, cache writes, images, requests, audio,
			// tools, and any future dimension not represented by the current rule.
			unsupported = true
		}
	}
	if unsupported || (cached != nil && *cached > input) {
		return candidate, nil
	}

	unit := domain.PricePer1KTokens
	candidate.Unit = &unit
	rule := map[string]json.RawMessage{
		"input_micros_per_1k":  json.RawMessage(strconv.FormatUint(input, 10)),
		"output_micros_per_1k": json.RawMessage(strconv.FormatUint(output, 10)),
	}
	if cached != nil {
		rule["cached_input_micros_per_1k"] = json.RawMessage(strconv.FormatUint(*cached, 10))
	}
	candidate.Rule, err = json.Marshal(rule)
	if err != nil {
		return domain.PriceCandidate{}, fmt.Errorf("encode normalized OpenRouter price: %w", err)
	}
	candidate.Status = domain.PriceCandidatePendingReview
	if err := candidate.Validate(); err != nil {
		return domain.PriceCandidate{}, fmt.Errorf("validate normalized OpenRouter candidate: %w", err)
	}
	return candidate, nil
}

func usdPerTokenToMicrosPer1K(raw json.RawMessage) (uint64, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || !decimalRatePattern.MatchString(value) {
		return 0, ErrInvalidOpenRouterPricing
	}
	rate, ok := new(big.Rat).SetString(value)
	if !ok || rate.Sign() < 0 {
		return 0, ErrInvalidOpenRouterPricing
	}
	micros := new(big.Rat).Mul(rate, new(big.Rat).SetInt64(1_000_000_000))
	if !micros.IsInt() || !micros.Num().IsUint64() || micros.Num().Uint64() > 9_007_199_254_740_991 {
		return 0, ErrInvalidOpenRouterPricing
	}
	return micros.Num().Uint64(), nil
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidOpenRouterPricing
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func readJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		fields := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, ErrInvalidOpenRouterPricing
			}
			if _, duplicate := fields[key]; duplicate {
				return nil, ErrInvalidOpenRouterPricing
			}
			value, err := readJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			fields[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, ErrInvalidOpenRouterPricing
		}
		return fields, nil
	case '[':
		items := make([]any, 0)
		for decoder.More() {
			value, err := readJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, ErrInvalidOpenRouterPricing
		}
		return items, nil
	default:
		return nil, ErrInvalidOpenRouterPricing
	}
}
