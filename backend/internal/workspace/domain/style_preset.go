package domain

import "github.com/google/uuid"

// StylePreset is an organization or project scoped prompt preset.
// A nil ProjectID denotes an organization preset.
type StylePreset struct {
	ID                uuid.UUID
	OrgID             uuid.UUID
	ProjectID         uuid.UUID
	Name              string
	StyleType         string
	StyleSubtype      string
	PromptFragment    string
	NegativePrompt    string
	ReferenceAssetIDs []uuid.UUID
}
