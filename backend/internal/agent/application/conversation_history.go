package application

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ConversationSourceReader checks current read authority for the draft's referenced sources.
type ConversationSourceReader interface {
	CheckConversationSources(context.Context, identity.Principal, uuid.UUID, prompt.Draft) error
}

// ConversationHistoryReader filters historical input through current source authority.
type ConversationHistoryReader interface {
	ReadConversationInput(context.Context, identity.Principal, uuid.UUID, json.RawMessage) (json.RawMessage, error)
}

// UnresolvedConversationReader lists attempts that still require conversation recovery.
type UnresolvedConversationReader interface {
	UnresolvedConversationRuns(context.Context, identity.Principal, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
}

// ConversationHistory applies current source authority without changing accepted audit input.
type ConversationHistory struct{ sources ConversationSourceReader }

// NewConversationHistory injects the owning source reader for historical input checks.
func NewConversationHistory(sources ConversationSourceReader) *ConversationHistory {
	return &ConversationHistory{sources: sources}
}

// ReadConversationInput retains readable input and removes references whose authority is unavailable.
func (h *ConversationHistory) ReadConversationInput(ctx context.Context, a identity.Principal, pid uuid.UUID, raw json.RawMessage) (json.RawMessage, error) {
	var input AuthorizedConversationInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if len(input.Draft.References) == 0 {
		return raw, nil
	}
	if h.sources != nil && h.sources.CheckConversationSources(ctx, a, pid, input.Draft) == nil {
		return raw, nil
	}
	// Fail closed for stale sources or unavailable authority; keep the user's own text.
	input.Prompt = input.PlainText
	input.MediaPins = []prompt.SourceMedia{}
	input.Draft = prompt.Draft{SchemaVersion: 1, Document: prompt.Document{Type: "doc", Content: []prompt.Paragraph{{Type: "paragraph", Content: []prompt.Inline{{Type: "text", Text: input.PlainText}}}}}, References: []prompt.Reference{}, Params: map[string]any{}, LabelState: prompt.LabelState{HighWater: map[string]int64{}, Allocations: map[string]prompt.LabelAllocation{}}}
	out := struct {
		AuthorizedConversationInput
		ReferencesUnavailable bool `json:"references_unavailable"`
	}{input, true}
	return json.Marshal(out)
}
