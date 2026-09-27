package catalog_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
)

func TestCredentialSchemaRequiresOnlyDeclaredStringFields(t *testing.T) {
	registry := credentialschema.NewRegistry()
	for _, tc := range []struct {
		adapter string
		secret  string
		last4   string
	}{
		{"volcengine_ark", `{"api_key":"ark-test-A9F2"}`, "A9F2"},
		{"minimax", `{"api_key":"minimax-test-B8C3","group_id":"group-1"}`, "B8C3"},
		{"openrouter", `{"api_key":"openrouter-test-C7D4"}`, "C7D4"},
	} {
		last4, err := registry.Validate(tc.adapter, json.RawMessage(tc.secret))
		if err != nil || last4 != tc.last4 {
			t.Fatalf("%s: last4 %q, error %v", tc.adapter, last4, err)
		}
	}
	for _, tc := range []struct {
		adapter string
		secret  string
	}{
		{"unknown", `{"api_key":"test1234"}`},
		{"minimax", `{"api_key":"test1234"}`},
		{"openrouter", `{"api_key":"test1234","extra":"value"}`},
		{"openrouter", `{"api_key":1234}`},
		{"openrouter", `{"api_key":"123"}`},
		{"openrouter", `{"api_key":" test1234"}`},
		{"openrouter", `{"api_key":"test1234\n"}`},
		{"openrouter", `{"api_key":null}`},
		{"openrouter", `{"api_key":"first1234","api_key":"other5678"}`},
		{"openrouter", `{"api_key":"test1234"} true`},
		{"openrouter", `{}`},
		{"openrouter", `[]`},
	} {
		if _, err := registry.Validate(tc.adapter, json.RawMessage(tc.secret)); !errors.Is(err, credentialschema.ErrInvalidSecret) {
			t.Fatalf("%s accepted invalid secret %s: %v", tc.adapter, tc.secret, err)
		}
	}
}
