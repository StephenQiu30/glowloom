package managed

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// normalizeTokenUsage checks aliases before publishing one normalized quantity.
// Missing token evidence stays missing; it never implies a zero-cost request.
func normalizeTokenUsage(raw json.RawMessage, usage map[string]any) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	normalized := map[string]int64{}
	for _, pair := range [][2]string{{"prompt_tokens", "input_tokens"}, {"input_tokens", "input_tokens"}, {"completion_tokens", "output_tokens"}, {"output_tokens", "output_tokens"}, {"cached_input_tokens", "cached_input_tokens"}} {
		encoded, present := fields[pair[0]]
		if !present {
			continue
		}
		var value *int64
		if json.Unmarshal(encoded, &value) != nil || value == nil || *value < 0 {
			return application.ErrInvalidProviderCall
		}
		if previous, exists := normalized[pair[1]]; exists && previous != *value {
			return application.ErrInvalidProviderCall
		}
		normalized[pair[1]] = *value
	}
	for name, value := range normalized {
		usage[name] = value
	}
	return nil
}
