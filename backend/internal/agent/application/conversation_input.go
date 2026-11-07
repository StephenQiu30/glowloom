package application

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// QuoteRef identifies a public message in an existing run.
type QuoteRef struct {
	RunID     uuid.UUID `json:"run_id"`
	MessageID string    `json:"message_id"`
}

// ConversationQuote freezes an authorized message's text and content hash.
type ConversationQuote struct {
	QuoteRef
	Text        string `json:"text"`
	ContentHash string `json:"content_hash"`
}

// AuthorizedConversationInput combines frozen creative intent with an optional authorized quote.
type AuthorizedConversationInput struct {
	prompt.ConversationInput
	Quote *ConversationQuote `json:"quote,omitempty"`
}

// ConversationModel pins the selected catalog release, version and context window.
type ConversationModel struct {
	ReleaseID     uuid.UUID `json:"release_id"`
	VersionID     uuid.UUID `json:"version_id"`
	ContextWindow int       `json:"context_window"`
}

// DraftInputSource freezes a draft after checking its owning source authority.
type DraftInputSource interface {
	FreezeConversationInput(context.Context, identity.Principal, uuid.UUID, prompt.Draft) (prompt.ConversationInput, error)
}

// QuoteReader reads an authorized message from the selected private conversation.
type QuoteReader interface {
	ReadConversationQuote(context.Context, identity.Principal, uuid.UUID, uuid.UUID, QuoteRef) (ConversationQuote, error)
}

// ConversationInputFreezer prepares frozen creative intent and a currently usable model.
type ConversationInputFreezer interface {
	Prepare(context.Context, identity.Principal, uuid.UUID, uuid.UUID, json.RawMessage, *QuoteRef, string) (AuthorizedConversationInput, ConversationModel, error)
}

// ConversationInputService combines draft, model and quote owners for input admission.
type ConversationInputService struct {
	drafts DraftInputSource
	models catalog.ListModelsStore
	quotes QuoteReader
}

// NewConversationInputService injects the draft, model and quote owners.
func NewConversationInputService(drafts DraftInputSource, models catalog.ListModelsStore, quotes QuoteReader) *ConversationInputService {
	return &ConversationInputService{drafts: drafts, models: models, quotes: quotes}
}

// Prepare authorizes and freezes the draft, quote and selected model.
func (s *ConversationInputService) Prepare(ctx context.Context, a identity.Principal, pid, sid uuid.UUID, raw json.RawMessage, quote *QuoteRef, modelKey string) (AuthorizedConversationInput, ConversationModel, error) {
	var out AuthorizedConversationInput
	var selected ConversationModel
	if s == nil || s.drafts == nil || s.models == nil || sid == uuid.Nil || strings.TrimSpace(modelKey) == "" {
		return out, selected, domain.ErrUnavailable
	}
	draft, err := prompt.DecodeDraft(raw)
	if err != nil {
		return out, selected, err
	}
	frozen, err := s.drafts.FreezeConversationInput(ctx, a, pid, draft)
	if err != nil {
		return out, selected, err
	}
	page, err := catalog.NewListModelsQuery(s.models).Execute(ctx, a, catalog.ListModelsInput{ProjectID: pid, ModelKey: modelKey, Capability: "text.generate", Limit: 2})
	if err != nil {
		return out, selected, err
	}
	if len(page.Models) != 1 {
		return out, selected, domain.ErrUnavailable
	}
	model := page.Models[0]
	if model.Key != modelKey || model.Status != "active" || model.ProviderStatus != "active" || !model.CredentialPresent || model.CurrentVersion == nil || model.CurrentPrice == nil || model.ReleaseID == uuid.Nil {
		return out, selected, domain.ErrUnavailable
	}
	if len(frozen.MediaPins) > 90 {
		return out, selected, prompt.ErrReferenceUnavailable
	}
	var limits struct {
		Roles map[string]struct {
			MaxCount int      `json:"max_count"`
			Types    []string `json:"types"`
		} `json:"roles"`
	}
	if json.Unmarshal(model.CurrentVersion.Limits, &limits) != nil {
		return out, selected, domain.ErrInvalid
	}
	counts := map[string]int{}
	visuals := 0
	for _, pin := range frozen.MediaPins {
		counts[pin.MediaKind]++
		if counts[pin.MediaKind] > 30 {
			return out, selected, prompt.ErrReferenceUnavailable
		}
		switch pin.MediaKind {
		case "image":
			visuals++
		case "video":
			visuals += 3
		case "document": // Native TXT/DOCX extraction supplies actual text.
		default:
			return out, selected, prompt.ErrReferenceUnavailable
		}
	}
	if visuals > 0 {
		// Videos are sampled to real images, never passed as fictitious native
		// video support. The selected model must support the complete projection.
		limit, ok := limits.Roles["reference_image"]
		if !ok || !slices.Contains(model.InputRoles, "reference_image") || !slices.Contains(limit.Types, "image") || visuals > limit.MaxCount {
			return out, selected, prompt.ErrReferenceUnavailable
		}
	}
	out.ConversationInput = frozen
	if quote != nil {
		if s.quotes == nil || quote.RunID == uuid.Nil || quote.MessageID == "" || len(quote.MessageID) > 256 {
			return out, selected, domain.ErrInvalid
		}
		value, err := s.quotes.ReadConversationQuote(ctx, a, pid, sid, *quote)
		if err != nil {
			return out, selected, err
		}
		if value.RunID != quote.RunID || value.MessageID != quote.MessageID || len(value.Text) > 65536 || value.ContentHash != domain.Hash([]byte(value.Text)) {
			return out, selected, domain.ErrInvalid
		}
		out.Quote = &value
		out.InputHash = domain.ContentHash(struct {
			Input string
			Quote ConversationQuote
		}{frozen.InputHash, value})
	}
	selected = ConversationModel{ReleaseID: model.ReleaseID, VersionID: model.CurrentVersion.ID, ContextWindow: model.CurrentVersion.ContextWindow}
	return out, selected, nil
}
