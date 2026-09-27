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

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
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
	switch price.Unit {
	case domain.PricePer1KTokens:
		if len(fields) != 2 {
			return ErrInvalidConfiguration
		}
		input, ok := nonnegativeMicros(fields["input_micros_per_1k"])
		if !ok {
			return ErrInvalidConfiguration
		}
		output, ok := nonnegativeMicros(fields["output_micros_per_1k"])
		if !ok || input == 0 && output == 0 {
			return ErrInvalidConfiguration
		}
	default:
		if len(fields) < 1 || len(fields) > 3 {
			return ErrInvalidConfiguration
		}
		if _, ok := nonnegativeMicros(fields["base_micros"]); !ok {
			return ErrInvalidConfiguration
		}
		for name, raw := range fields {
			switch name {
			case "base_micros":
			case "by_mode", "by_resolution":
				if !validCoefficients(raw) {
					return ErrInvalidConfiguration
				}
			default:
				return ErrInvalidConfiguration
			}
		}
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
