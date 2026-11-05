package domain

import (
	"encoding/json"
	"strings"
)

// ConversationInput is immutable creative intent after the owning source check.
// URLs, file bytes and client-supplied readiness never belong in this record.
type ConversationInput struct {
	SchemaVersion int           `json:"schema_version"`
	Draft         Draft         `json:"draft"`
	InputHash     string        `json:"input_hash"`
	PlainText     string        `json:"plain_text"`
	Prompt        string        `json:"prompt"`
	MediaPins     []SourceMedia `json:"media_pins"`
}

// NewConversationInput validates and freezes a draft and its compiled creative intent.
func NewConversationInput(d Draft) (ConversationInput, error) {
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > 1<<20 {
		return ConversationInput{}, ErrInvalidDraft
	}
	// Clone caller-owned slices/maps so subsequent edits cannot alter a snapshot.
	frozen, err := DecodeDraft(raw)
	if err != nil {
		return ConversationInput{}, err
	}
	compiled, err := frozen.Compile()
	if err != nil {
		return ConversationInput{}, err
	}
	var text strings.Builder
	for i, p := range frozen.Document.Content {
		if i > 0 {
			text.WriteByte('\n')
		}
		for _, n := range p.Content {
			switch n.Type {
			case "text":
				text.WriteString(n.Text)
			case "hardBreak":
				text.WriteByte('\n')
			}
		}
	}
	meaningful := strings.TrimSpace(text.String()) != "" || len(compiled.Media) > 0
	for _, ref := range frozen.References {
		if (ref.Source.Kind == "subject" || ref.Source.Kind == "canvas_subject") && strings.TrimSpace(ref.Source.Description) != "" {
			meaningful = true
		}
	}
	if !meaningful {
		return ConversationInput{}, ErrInvalidDraft
	}
	_, hash := frozen.Hashes()
	pins := make([]SourceMedia, 0, len(compiled.Media))
	for _, m := range compiled.Media {
		pins = append(pins, m.SourceMedia)
	}
	return ConversationInput{SchemaVersion: 1, Draft: frozen, InputHash: hash, PlainText: text.String(), Prompt: compiled.Prompt, MediaPins: pins}, nil
}
