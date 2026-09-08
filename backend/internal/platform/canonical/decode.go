package canonical

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Decode rejects ambiguous keys before decoding a closed wire contract.
// JSON keeps the same canonical bytes for every previously valid value.
func Decode(raw []byte, target any) error {
	if _, err := uniqueValue(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
func uniqueValue(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := readValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON contains trailing data")
	}
	return value, nil
}
func readValue(d *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		value := map[string]any{}
		for d.More() {
			keyToken, e := d.Token()
			if e != nil {
				return nil, e
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("invalid JSON key")
			}
			if _, exists := value[key]; exists {
				return nil, errors.New("JSON contains duplicate keys")
			}
			item, e := readValue(d, depth+1)
			if e != nil {
				return nil, e
			}
			value[key] = item
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
		return value, nil
	case '[':
		value := []any{}
		for d.More() {
			item, e := readValue(d, depth+1)
			if e != nil {
				return nil, e
			}
			value = append(value, item)
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, errors.New("invalid JSON delimiter")
	}
}
