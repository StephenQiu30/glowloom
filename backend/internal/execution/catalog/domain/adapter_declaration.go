package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
)

// AdapterDeclaration is a data-only reference to an installed adapter protocol.
// Endpoints follow ProviderConfig validation; regions stay pinned to the host
// specification. Credentials, custom headers and executable expressions are
// unrepresentable.
type AdapterDeclaration struct {
	SchemaVersion int      `json:"schema_version"`
	AdapterKey    string   `json:"adapter_key"`
	ProtocolID    string   `json:"protocol_id"`
	BaseURL       string   `json:"base_url"`
	Region        Region   `json:"region"`
	Capabilities  []string `json:"capabilities"`
}

// Validate keeps the declaration consistent with the immutable execution config.
func (d AdapterDeclaration) Validate(config ProviderConfig) error {
	spec, ok := GetAdapterSpec(d.AdapterKey)
	if !ok || !spec.IsManagedCloud || d.SchemaVersion != 1 || config.Validate() != nil || d.AdapterKey != config.AdapterKey || !slices.Contains(spec.ProtocolIDs, d.ProtocolID) || d.BaseURL != config.BaseURL || d.Region != config.Region || len(d.Capabilities) != len(config.Capabilities) {
		return ErrInvalidPreset
	}
	seen := map[string]bool{}
	for _, capability := range d.Capabilities {
		if seen[capability] || !slices.Contains(config.Capabilities, capability) {
			return ErrInvalidPreset
		}
		seen[capability] = true
	}
	return nil
}

// UnmarshalJSON applies the import restrictions to direct save requests too.
func (d *AdapterDeclaration) UnmarshalJSON(data []byte) error {
	if len(data) > 262144 || rejectDuplicateFields(json.NewDecoder(bytes.NewReader(data))) != nil {
		return ErrInvalidPreset
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return ErrInvalidPreset
	}
	for name := range fields {
		switch name {
		case "schema_version", "adapter_key", "protocol_id", "base_url", "region", "capabilities":
		default:
			return ErrInvalidPreset
		}
	}
	type declaration AdapterDeclaration
	var value declaration
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalidPreset
	}
	*d = AdapterDeclaration(value)
	return nil
}
