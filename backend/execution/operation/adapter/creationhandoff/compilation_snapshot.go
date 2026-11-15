package creationhandoff

import (
	"time"

	"github.com/google/uuid"
)

// These closed wire documents serve only full canonical-content binding. They
// provide no template editor, defaults, policy, lookup or compilation behavior.
type snapshotCompilationInputs struct {
	CompilerVersion string                  `json:"compiler_version"`
	ProjectName     string                  `json:"project_name"`
	StyleType       string                  `json:"style_type"`
	Template        *snapshotFrozenTemplate `json:"template,omitempty"`
}
type snapshotFrozenTemplate struct {
	Definition    snapshotFrozenDefinition     `json:"definition"`
	Customization *snapshotFrozenCustomization `json:"customization,omitempty"`
	RequestSHA256 string                       `json:"request_sha256"`
	Outline       *snapshotOutline             `json:"outline,omitempty"`
}
type snapshotFrozenDefinition struct {
	Operation       string             `json:"operation"`
	Label           string             `json:"label"`
	Category        string             `json:"category"`
	Description     string             `json:"description"`
	OutputType      string             `json:"output_type"`
	SchemaKey       string             `json:"schema_key"`
	Variables       []snapshotVariable `json:"variables"`
	OutputContract  string             `json:"output_contract"`
	Content         string             `json:"content"`
	TemplateID      uuid.UUID          `json:"template_id"`
	TemplateVersion int                `json:"template_version"`
}
type snapshotVariable struct {
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}
type snapshotFrozenCustomization struct {
	ID             uuid.UUID `json:"id"`
	Operation      string    `json:"operation"`
	Mode           string    `json:"mode"`
	Content        string    `json:"content"`
	BaseTemplateID uuid.UUID `json:"base_template_id"`
	Revision       int64     `json:"revision"`
	UpdateTime     time.Time `json:"update_time"`
}
type snapshotOutline struct {
	ChapterCount   string `json:"chapter_count"`
	Structure      string `json:"structure"`
	WordCount      string `json:"word_count"`
	Perspective    string `json:"perspective"`
	Tone           string `json:"tone"`
	CharacterScale string `json:"character_scale"`
	ChapterLength  string `json:"chapter_length"`
}
