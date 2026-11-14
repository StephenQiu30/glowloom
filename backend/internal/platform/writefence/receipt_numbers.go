package writefence

import (
	"encoding/json"
	"strconv"
	"strings"
)

// normalizeReceiptValue bounds the total expanded numeric representation before
// any JSONB cast. A tiny exponent must not produce an unbounded or unrecoverable
// database receipt. Use decimal strings throughout; float64 loses command facts.
func normalizeReceiptValue(value any, remaining *int, depth int) (any, error) {
	if depth > 256 {
		return nil, ErrInvalid
	}
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.IndexByte(key, 0) >= 0 {
				return nil, ErrInvalid
			}
			normalized, err := normalizeReceiptValue(child, remaining, depth+1)
			if err != nil {
				return nil, err
			}
			value[key] = normalized
		}
		return value, nil
	case []any:
		for i, child := range value {
			normalized, err := normalizeReceiptValue(child, remaining, depth+1)
			if err != nil {
				return nil, err
			}
			value[i] = normalized
		}
		return value, nil
	case string:
		if strings.IndexByte(value, 0) >= 0 {
			return nil, ErrInvalid
		}
		return value, nil
	case json.Number:
		normalized, err := decimalReceiptNumber(value.String(), *remaining)
		if err != nil {
			return nil, err
		}
		*remaining -= len(normalized)
		return json.Number(normalized), nil
	default:
		return value, nil
	}
}

func decimalReceiptNumber(raw string, budget int) (string, error) {
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	mantissa, exponent := raw, "0"
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		mantissa, exponent = raw[:index], raw[index+1:]
	}
	integerDigits := len(mantissa)
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		integerDigits = index
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	trimmed := strings.TrimLeft(digits, "0")
	if trimmed == "" {
		if budget < 1 {
			return "", ErrInvalid
		}
		return "0", nil
	}
	power, err := strconv.ParseInt(exponent, 10, 64)
	if err != nil || power > MaxReceiptBytes || power < -MaxReceiptBytes {
		return "", ErrInvalid
	}
	position := int64(integerDigits) + power - int64(len(digits)-len(trimmed))
	digits = strings.TrimRight(trimmed, "0")
	// PostgreSQL numeric supports these exact pre/post decimal digit bounds.
	// Refuse unsupported values before opening a database cast/transaction error.
	if position > 131072 || int64(len(digits))-position > 16383 {
		return "", ErrInvalid
	}
	length := int64(len(digits) + 1)
	switch {
	case position <= 0:
		length = 2 - position + int64(len(digits))
	case position >= int64(len(digits)):
		length = position
	}
	if negative {
		length++
	}
	if length > int64(budget) {
		return "", ErrInvalid
	}
	var result strings.Builder
	result.Grow(int(length))
	if negative {
		result.WriteByte('-')
	}
	switch {
	case position <= 0:
		result.WriteString("0.")
		result.WriteString(strings.Repeat("0", int(-position)))
		result.WriteString(digits)
	case position >= int64(len(digits)):
		result.WriteString(digits)
		result.WriteString(strings.Repeat("0", int(position)-len(digits)))
	default:
		result.WriteString(digits[:int(position)])
		result.WriteByte('.')
		result.WriteString(digits[int(position):])
	}
	return result.String(), nil
}
