package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// UnmarshalJSON preserves unrecognized persisted fields, including nested fields.
// Such a config can be returned for inspection but cannot enter a graph mutation.
func (c *NodeConfig) UnmarshalJSON(data []byte) error {
	type knownConfig NodeConfig
	var decoded knownConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		// A future nested custom decoder may reject fields before a permissive
		// decode can finish. Retain that complete object rather than its prefix.
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil || object == nil {
			return fmt.Errorf("%w: canvas config must be an object", ErrInvalidCommand)
		}
		decoded.unknown = bytes.Clone(data)
	}
	*c = NodeConfig(decoded)
	return nil
}

// MarshalJSON retains the original config when this version cannot interpret it.
func (c NodeConfig) MarshalJSON() ([]byte, error) {
	if len(c.unknown) > 0 {
		return bytes.Clone(c.unknown), nil
	}
	type knownConfig NodeConfig
	return json.Marshal(knownConfig(c))
}

// IsOpaque reports a config that cannot be interpreted without losing data.
// Reference/copy consumers must fail closed instead of proving its absence.
func (c NodeConfig) IsOpaque() bool { return len(c.unknown) != 0 }
