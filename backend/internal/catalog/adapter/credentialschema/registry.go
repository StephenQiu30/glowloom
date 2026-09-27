// Package credentialschema validates the fields declared by provider adapters.
package credentialschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ErrInvalidSecret means an adapter or its credential fields are unsupported.
var ErrInvalidSecret = errors.New("invalid provider credential fields")

// Registry holds the credential contracts for the initial provider adapters.
type Registry struct{}

// NewRegistry constructs the set of provider credential contracts.
func NewRegistry() *Registry { return &Registry{} }

// Supports reports whether the current Agent has a declared credential adapter.
func (r *Registry) Supports(adapterKey string) bool {
	if r == nil {
		return false
	}
	switch adapterKey {
	case "volcengine_ark", "minimax", "openrouter":
		return true
	default:
		return false
	}
}

// Validate returns the last four API-key characters without retaining secret data.
func (r *Registry) Validate(adapterKey string, secret json.RawMessage) (string, error) {
	if !r.Supports(adapterKey) || len(secret) == 0 || len(secret) > 16*1024 {
		return "", ErrInvalidSecret
	}
	requireGroup := adapterKey == "minimax"
	decoder := json.NewDecoder(bytes.NewReader(secret))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return "", ErrInvalidSecret
	}
	fields := make(map[string]string, 2)
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return "", ErrInvalidSecret
		}
		name, ok := nameToken.(string)
		if !ok {
			return "", ErrInvalidSecret
		}
		switch name {
		case "api_key":
		case "group_id":
			if !requireGroup {
				return "", ErrInvalidSecret
			}
		default:
			return "", ErrInvalidSecret
		}
		if _, duplicate := fields[name]; duplicate {
			return "", ErrInvalidSecret
		}
		var value *string
		if err := decoder.Decode(&value); err != nil || value == nil || !printableASCII(*value) {
			return "", ErrInvalidSecret
		}
		fields[name] = *value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return "", ErrInvalidSecret
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return "", ErrInvalidSecret
	}
	apiKey := fields["api_key"]
	if len(apiKey) < 4 || len(apiKey) > 4096 || (requireGroup && (len(fields["group_id"]) == 0 || len(fields["group_id"]) > 128)) {
		return "", ErrInvalidSecret
	}
	return apiKey[len(apiKey)-4:], nil
}

func printableASCII(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		if value[index] < '!' || value[index] > '~' {
			return false
		}
	}
	return true
}
