package domain

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func equalJSON(a, b []byte) bool {
	var x, y any
	left := json.NewDecoder(bytes.NewReader(a))
	left.UseNumber()
	right := json.NewDecoder(bytes.NewReader(b))
	right.UseNumber()
	return left.Decode(&x) == nil && right.Decode(&y) == nil && equalJSONValue(x, y)
}

func equalJSONValue(x, y any) bool {
	switch left := x.(type) {
	case json.Number:
		right, ok := y.(json.Number)
		if !ok {
			return false
		}
		if left == right {
			return true
		}
		ld, le, lok := normalizedJSONNumber(string(left))
		rd, re, rok := normalizedJSONNumber(string(right))
		return lok && rok && ld == rd && le == re
	case []any:
		right, ok := y.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i, value := range left {
			if !equalJSONValue(value, right[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := y.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !equalJSONValue(value, other) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(x, y)
	}
}

// JSONB expands exponent notation and may retain decimal zeroes. Compare exact
// signed digits and a decimal exponent without float64 rounding or allocating
// 10^exponent digits. Nonidentical literals with exponents outside int64 fail
// closed without expansion.
func normalizedJSONNumber(text string) (string, int64, bool) {
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	var exponent int64
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		var err error
		exponent, err = strconv.ParseInt(text[index+1:], 10, 64)
		if err != nil {
			return "", 0, false
		}
		text = text[:index]
	}
	fraction := 0
	if index := strings.IndexByte(text, '.'); index >= 0 {
		fraction = len(text) - index - 1
		text = text[:index] + text[index+1:]
	}
	text = strings.TrimLeft(text, "0")
	if text == "" {
		return "0", 0, true
	}
	digits := strings.TrimRight(text, "0")
	shift := int64(len(text) - len(digits) - fraction)
	if shift > 0 && exponent > math.MaxInt64-shift || shift < 0 && exponent < math.MinInt64-shift {
		return "", 0, false
	}
	if negative {
		digits = "-" + digits
	}
	return digits, exponent + shift, true
}
