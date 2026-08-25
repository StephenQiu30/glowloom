package openapi

import _ "embed"

// document is the public REST/SSE contract served by lanverse-api.
//
//go:embed lanverse-v1.json
var document []byte

// Document returns a defensive copy of the public API contract.
func Document() []byte {
	return append([]byte(nil), document...)
}
