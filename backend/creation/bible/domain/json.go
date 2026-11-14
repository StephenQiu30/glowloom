package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// DecodeClosedJSON rejects duplicate names, deep input and fields outside a typed contract.
func DecodeClosedJSON(data []byte, out any) error {
	return decodeClosedJSON(data, out, MaxVersionBytes, true)
}

// DecodeClosedJSONDocument checks a complete retained document under its caller's
// byte contract. Complete Copy history has no per-version depth or node budget;
// each contained version is validated separately by its owning reader.
func DecodeClosedJSONDocument(data []byte, out any, maxBytes int) error {
	return decodeClosedJSON(data, out, maxBytes, false)
}

func decodeClosedJSON(data []byte, out any, maxBytes int, versionBudget bool) error {
	if len(data) == 0 || len(data) > maxBytes || !utf8.Valid(data) {
		return ErrInvalidContent
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	if err := jsonValue(decoder, 0, &nodes, versionBudget); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalidContent
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalidContent
	}
	return nil
}

func jsonValue(d *json.Decoder, depth int, nodes *int, versionBudget bool) error {
	*nodes++
	if versionBudget && (depth > 64 || *nodes > 200000) {
		return ErrInvalidContent
	}
	token, err := d.Token()
	if err != nil {
		return ErrInvalidContent
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]bool)
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return ErrInvalidContent
			}
			key, ok := token.(string)
			if !ok || keys[key] {
				return ErrInvalidContent
			}
			keys[key] = true
			if err := jsonValue(d, depth+1, nodes, versionBudget); err != nil {
				return err
			}
		}
		ending, err := d.Token()
		if err != nil || ending != json.Delim('}') {
			return ErrInvalidContent
		}
	case '[':
		for d.More() {
			if err := jsonValue(d, depth+1, nodes, versionBudget); err != nil {
				return err
			}
		}
		ending, err := d.Token()
		if err != nil || ending != json.Delim(']') {
			return ErrInvalidContent
		}
	default:
		return ErrInvalidContent
	}
	return nil
}
