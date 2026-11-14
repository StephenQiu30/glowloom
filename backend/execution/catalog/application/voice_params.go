package application

import (
	"bytes"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

// A binding may be valid for a published TTS mode without choosing an execution mode.
// Actual generation validates its selected mode again through Operation.
func voiceParamsSupported(params map[string]json.RawMessage, fields []voiceField, modes []string) bool {
	for _, mode := range modes {
		seen, compatible := 0, true
		for _, field := range fields {
			active := len(field.ForModes) == 0 || slices.Contains(field.ForModes, mode)
			if field.Field == "voice_id" {
				compatible = compatible && active
				continue
			}
			raw, provided := params[field.Field]
			if !provided {
				if field.Required && active && len(field.Default) == 0 && voiceParameterField(field.Field) {
					compatible = false
				}
				continue
			}
			seen++
			compatible = compatible && active && validVoiceValue(raw, field)
		}
		if compatible && seen == len(params) {
			return true
		}
	}
	return false
}

func voiceParameterField(name string) bool {
	switch name {
	case "speed", "pitch", "volume", "emotion", "language":
		return true
	default:
		return false
	}
}

func validVoiceValue(raw json.RawMessage, field voiceField) bool {
	if len(raw) > 1024 {
		return false
	}
	if field.Field == "emotion" || field.Field == "language" {
		var value string
		if field.Type != "string" || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
			return false
		}
		if len(field.Enum) == 0 {
			return true
		}
		for _, option := range field.Enum {
			var allowed string
			if json.Unmarshal(option, &allowed) == nil && value == allowed {
				return true
			}
		}
		return false
	}
	if field.Type != "number" && field.Type != "integer" {
		return false
	}
	value, ok := voiceNumber(raw)
	if !ok || field.Type == "integer" && !value.IsInt() {
		return false
	}
	if len(field.Enum) != 0 {
		found := false
		for _, option := range field.Enum {
			allowed, valid := voiceNumber(option)
			found = found || valid && value.Cmp(allowed) == 0
		}
		if !found {
			return false
		}
	}
	base := new(big.Rat)
	if len(field.Min) != 0 {
		minimum, valid := voiceNumber(field.Min)
		if !valid || value.Cmp(minimum) < 0 {
			return false
		}
		base.Set(minimum)
	}
	if len(field.Max) != 0 {
		maximum, valid := voiceNumber(field.Max)
		if !valid || value.Cmp(maximum) > 0 {
			return false
		}
	}
	if len(field.Step) != 0 {
		step, valid := voiceNumber(field.Step)
		if !valid || step.Sign() <= 0 || !new(big.Rat).Quo(new(big.Rat).Sub(value, base), step).IsInt() {
			return false
		}
	}
	return true
}

func voiceNumber(raw json.RawMessage) (*big.Rat, bool) {
	if len(raw) == 0 || len(raw) > 128 || !json.Valid(raw) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	// Bound exponent expansion, including zero and underflow, before rational parsing.
	if i := strings.IndexAny(string(number), "eE"); i >= 0 {
		exponent, err := strconv.Atoi(string(number)[i+1:])
		if err != nil || exponent < -128 || exponent > 128 {
			return nil, false
		}
	}
	parsed, valid := new(big.Rat).SetString(string(number))
	return parsed, valid
}
