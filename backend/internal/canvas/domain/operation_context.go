package domain

import (
	"bytes"
	"encoding/json"
	"strings"
)

func compositeJSON(value []byte) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 && (value[0] == '{' || value[0] == '[')
}

func contextPath(path []string, rule fieldPolicy) []string {
	parts := strings.Split(rule.context, ".")
	for i, part := range parts {
		if part == "@" && i < len(path) {
			parts[i] = path[i]
		}
	}
	return parts
}

func fieldContext(object map[string]any, path []string, rule fieldPolicy) (json.RawMessage, error) {
	parts := contextPath(path, rule)
	container, key, err := fieldContainer(object, parts)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	switch {
	case key == "model" && len(parts) > 1 && parts[1] == "generation":
		keys = []string{"capability", "model_profile_id", "mode"}
	case key == "model" && len(parts) > 1 && parts[1] == "batch_table":
		keys = []string{"operation", "model_profile_id", "mode"}
	case key == "model":
		keys = []string{"modelVersion", "mode"}
	case key == "source" && len(parts) > 1 && parts[1] == "timeline":
		keys = []string{"node_id", "asset_id"}
	case key == "source" && len(parts) > 2 && parts[len(parts)-2] == "panorama":
		keys = []string{"asset_id", "name"}
	case key == "source":
		keys = []string{"primitive", "source_node_id", "asset_id", "builtin_actor"}
	default:
		return nil, operationError("unsupported_field")
	}
	value := make(map[string]any, len(keys))
	for _, name := range keys {
		value[name] = container[name]
	}
	encoded, err := json.Marshal(value)
	return encoded, err
}
