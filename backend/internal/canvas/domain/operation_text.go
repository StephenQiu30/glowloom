package domain

import (
	"encoding/json"
	"slices"
)

func negativePromptPath(path []string) bool {
	return slices.Equal(path, []string{"config", "draft", "composer", "params", "negative_prompt"})
}
func textValue(path []string, value json.RawMessage, exists bool) (json.RawMessage, bool) {
	if !exists && negativePromptPath(path) {
		return json.RawMessage(`""`), true
	}
	return value, exists
}
