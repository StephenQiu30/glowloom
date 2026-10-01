package domain

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// GenerationConfig is the shared form/canvas draft, not an operation result.
type GenerationConfig struct {
	Version        int                   `json:"version"`
	Capability     string                `json:"capability"`
	ModelProfileID *uuid.UUID            `json:"model_profile_id" extensions:"x-nullable"`
	Mode           string                `json:"mode"`
	Prompt         string                `json:"prompt"`
	Params         json.RawMessage       `json:"params" swaggertype:"object"`
	OutputCount    int                   `json:"output_count"`
	Inputs         []GenerationReference `json:"inputs"`
}

// GenerationReference preserves the published model role and exact asset order.
type GenerationReference struct {
	Role         string    `json:"role"`
	MediaAssetID uuid.UUID `json:"media_asset_id"`
}

func validGeneration(c GenerationConfig) bool {
	if c.Version != 1 || strings.TrimSpace(c.Capability) == "" || !utf8.ValidString(c.Capability) || utf8.RuneCountInString(c.Capability) > 128 || c.ModelProfileID != nil && *c.ModelProfileID == uuid.Nil || !utf8.ValidString(c.Mode) || utf8.RuneCountInString(c.Mode) > 128 || !validPrompt(c.Prompt) || !validToolParams(c.Params) || c.OutputCount < 1 || c.OutputCount > 8 || c.Inputs == nil || len(c.Inputs) > 255 {
		return false
	}
	for _, input := range c.Inputs {
		if input.MediaAssetID == uuid.Nil || strings.TrimSpace(input.Role) == "" || input.Role == "prompt" || !utf8.ValidString(input.Role) || utf8.RuneCountInString(input.Role) > 128 {
			return false
		}
	}
	raw, err := json.Marshal(c)
	return err == nil && len(raw) <= 512<<10
}
