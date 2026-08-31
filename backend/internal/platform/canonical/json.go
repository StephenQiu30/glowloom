package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"golang.org/x/text/unicode/norm"
)

// JSON implements the production content contract shared across Backend
// domains and Agent wires. Object keys and string values are NFC normalized;
// numbers are restricted to integers so cross-language hashes remain stable.
func JSON(raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("production canonical JSON contains multiple values")
		}
		return nil, err
	}
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err = write(&buffer, normalized); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
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
			if _, err := number.Int64(); err != nil {
				return nil, errors.New("production canonical JSON only permits integers")
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
				return nil, errors.New("production canonical JSON contains duplicate normalized keys")
			}
			normalized, err := normalize(item)
			if err != nil {
				return nil, err
			}
			result[normalizedKey] = normalized
		}
		return result, nil
	default:
		return nil, errors.New("production canonical JSON contains an unsupported value")
	}
}

func write(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case bool:
		if typed {
			buffer.WriteString("true")
		} else {
			buffer.WriteString("false")
		}
	case json.Number:
		buffer.WriteString(typed.String())
	case string:
		if err := writeString(buffer, typed); err != nil {
			return err
		}
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := write(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeString(buffer, key); err != nil {
				return err
			}
			buffer.WriteByte(':')
			if err := write(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		return errors.New("production canonical JSON contains an unsupported value")
	}
	return nil
}

func writeString(buffer *bytes.Buffer, value string) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	buffer.Write(bytes.TrimSpace(encoded.Bytes()))
	return nil
}
