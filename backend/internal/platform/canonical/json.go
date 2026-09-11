package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

const maximumSafeInteger int64 = 9007199254740991

// JSON implements the production content contract shared across Backend
// domains and Agent wires. It NFC-normalizes strings before applying RFC 8785
// and only permits integers that are exactly representable across runtimes.
func JSON(raw json.RawMessage) ([]byte, error) {
	if !utf8.Valid(raw) || !validUnicodeEscapes(raw) {
		return nil, canonicalError(errorInvalidUnicode, "Production Canonical JSON contains invalid Unicode")
	}
	value, err := uniqueValue(raw)
	if err != nil {
		return nil, wrapCanonicalError(errorInvalidJSON, err)
	}
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, wrapCanonicalError(errorUnsupportedValue, err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return nil, wrapCanonicalError(errorInvalidJSON, err)
	}
	return canonical, nil
}

func Hash(raw json.RawMessage) (string, error) {
	canonical, err := JSON(raw)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), nil
}

func normalize(value any) (any, error) {
	switch typed := value.(type) {
	case nil, bool, json.Number:
		if number, ok := typed.(json.Number); ok {
			integer, err := number.Int64()
			if err != nil {
				return nil, canonicalError(errorIntegerRequired, "Production Canonical JSON only permits decimal integers")
			}
			if integer < -maximumSafeInteger || integer > maximumSafeInteger {
				return nil, canonicalError(errorIntegerOutOfRange, "Production Canonical JSON integer exceeds the safe range")
			}
		}
		return typed, nil
	case string:
		return norm.NFC.String(typed), nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			normalized, err := normalize(item)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			normalizedKey := norm.NFC.String(key)
			if _, exists := result[normalizedKey]; exists {
				return nil, canonicalError(errorDuplicateNormalizedKey, "Production Canonical JSON contains duplicate normalized keys")
			}
			normalized, err := normalize(item)
			if err != nil {
				return nil, err
			}
			result[normalizedKey] = normalized
		}
		return result, nil
	default:
		return nil, canonicalError(errorUnsupportedValue, "Production Canonical JSON contains an unsupported value")
	}
}

func validUnicodeEscapes(raw []byte) bool {
	inString := false
	for index := 0; index < len(raw); index++ {
		switch raw[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(raw) {
				continue
			}
			index++
			if raw[index] != 'u' || index+4 >= len(raw) {
				continue
			}
			first, err := strconv.ParseUint(string(raw[index+1:index+5]), 16, 16)
			if err != nil {
				continue
			}
			index += 4
			switch {
			case first >= 0xd800 && first <= 0xdbff:
				if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
					return false
				}
				second, parseErr := strconv.ParseUint(string(raw[index+3:index+7]), 16, 16)
				if parseErr != nil || second < 0xdc00 || second > 0xdfff {
					return false
				}
				index += 6
			case first >= 0xdc00 && first <= 0xdfff:
				return false
			}
		}
	}
	return true
}
