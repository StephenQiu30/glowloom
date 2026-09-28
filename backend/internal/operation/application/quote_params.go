package application

import (
	"bytes"
	"encoding/json"
	"math/big"
	"slices"
	"strings"
)

type quoteParamField struct {
	Field    string            `json:"field"`
	Type     string            `json:"type"`
	Required bool              `json:"required"`
	Default  json.RawMessage   `json:"default"`
	Enum     []json.RawMessage `json:"enum"`
	ForModes []string          `json:"for_modes"`
	Min      json.RawMessage   `json:"min"`
	Max      json.RawMessage   `json:"max"`
	Step     json.RawMessage   `json:"step"`
}

// validateQuoteParams repeats the published field constraints at the server
// boundary. A browser form is never the authority for price or provider input.
func validateQuoteParams(raw, schema json.RawMessage, mode string) (map[string]any, error) {
	provided, err := fingerprintObject(raw)
	if err != nil || len(schema) > 64*1024 {
		return nil, ErrFreeQuoteInputNotReady
	}
	var fields []quoteParamField
	if len(bytes.TrimSpace(schema)) == 0 || json.Unmarshal(schema, &fields) != nil {
		return nil, ErrFreeQuoteInputNotReady
	}
	values := make(map[string]any, len(provided))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field.Field == "" || seen[field.Field] {
			return nil, ErrFreeQuoteInputNotReady
		}
		seen[field.Field] = true
		active := len(field.ForModes) == 0 || slices.Contains(field.ForModes, mode)
		value, present := provided[field.Field]
		if !active {
			if present {
				return nil, ErrFreeQuoteInputNotReady
			}
			continue
		}
		if !present {
			if len(field.Default) != 0 {
				value, present = field.Default, true
			} else if field.Required {
				return nil, ErrFreeQuoteInputNotReady
			}
		}
		if !present {
			continue
		}
		canonical, err := normalizeFingerprintValue(value, field.Type)
		if err != nil || field.Required && field.Type == "string" && strings.TrimSpace(canonical.Value) == "" {
			return nil, ErrFreeQuoteInputNotReady
		}
		if len(field.Enum) != 0 {
			matched := false
			for _, option := range field.Enum {
				possible, optionErr := normalizeFingerprintValue(option, field.Type)
				if optionErr == nil && possible == canonical {
					matched = true
					break
				}
			}
			if !matched {
				return nil, ErrFreeQuoteInputNotReady
			}
		}
		if field.Type == "integer" || field.Type == "number" {
			actual, ok := new(big.Rat).SetString(canonical.Value)
			if !ok || !quoteParamWithinBounds(actual, field) {
				return nil, ErrFreeQuoteInputNotReady
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return nil, ErrFreeQuoteInputNotReady
		}
		values[field.Field] = decoded
		delete(provided, field.Field)
	}
	if len(provided) != 0 {
		return nil, ErrFreeQuoteInputNotReady
	}
	return values, nil
}

func quoteParamWithinBounds(value *big.Rat, field quoteParamField) bool {
	var minimum *big.Rat
	for _, bound := range []struct {
		raw json.RawMessage
		min bool
	}{{field.Min, true}, {field.Max, false}} {
		if len(bound.raw) == 0 {
			continue
		}
		parsed, err := normalizeFingerprintValue(bound.raw, "number")
		if err != nil {
			return false
		}
		limit, ok := new(big.Rat).SetString(parsed.Value)
		if !ok || bound.min && value.Cmp(limit) < 0 || !bound.min && value.Cmp(limit) > 0 {
			return false
		}
		if bound.min {
			minimum = limit
		}
	}
	if len(field.Step) == 0 {
		return true
	}
	stepValue, err := normalizeFingerprintValue(field.Step, "number")
	if err != nil {
		return false
	}
	step, ok := new(big.Rat).SetString(stepValue.Value)
	if !ok || step.Sign() <= 0 {
		return false
	}
	if minimum == nil {
		minimum = new(big.Rat)
	}
	difference := new(big.Rat).Sub(value, minimum)
	return new(big.Rat).Quo(difference, step).IsInt()
}
