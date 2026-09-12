package domain

import "encoding/json"

func referenceObservationNodeConfig(call bool) json.RawMessage {
	hash := map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"}
	properties := map[string]any{
		"execution_ref": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "revision": map[string]any{"type": "integer", "const": 1}, "content_hash": hash}, "required": []string{"id", "revision", "content_hash"}, "additionalProperties": false},
		"job_hash":      hash, "previous_call_key": hash,
	}
	required := []string{"execution_ref", "job_hash", "previous_call_key"}
	if call {
		properties["call_key"] = hash
		properties["previous_call_key"] = map[string]any{"type": "string", "pattern": "^([0-9a-f]{64})?$"}
		required = append(required, "call_key")
	}
	raw, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
	return raw
}
