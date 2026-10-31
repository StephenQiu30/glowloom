package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrAssistantTurnActive prevents losing an unresolved or still running turn.
var ErrAssistantTurnActive = errors.New("canvas_assistant_turn_active")

// ErrAssistantEmptySession prevents creating another conversation before sending a message.
var ErrAssistantEmptySession = errors.New("canvas_assistant_empty_session")

// AssistantTurn binds one editor owner; OperationID freezes a closed history turn.
type AssistantTurn struct {
	DraftID     uuid.UUID  `json:"draft_id"`
	OperationID *uuid.UUID `json:"operation_id,omitempty"`
	Skill       string     `json:"skill,omitempty"`
	Execution   string     `json:"execution,omitempty"`
}

// AssistantSession contains private ownership, never model/provider credentials.
type AssistantSession struct {
	ID        uuid.UUID       `json:"id"`
	CanvasID  uuid.UUID       `json:"canvas_id"`
	ProjectID uuid.UUID       `json:"project_id"`
	Title     string          `json:"title"`
	Revision  int64           `json:"revision"`
	Turns     []AssistantTurn `json:"turns"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// AssistantCreate uses a caller-generated durable identity for safe retries.
type AssistantCreate struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// AssistantAppend binds an existing authorized draft in one conditional write.
type AssistantAppend struct {
	ExpectedRevision    int64      `json:"expected_revision"`
	DraftID             uuid.UUID  `json:"draft_id"`
	Skill               string     `json:"skill,omitempty"`
	PreviousOperationID *uuid.UUID `json:"previous_operation_id,omitempty"`
	Execution           string     `json:"execution,omitempty"`
}

// AssistantStore owns authorization, turn serialization and durable identities.
type AssistantStore interface {
	ListAssistant(context.Context, identity.Principal, uuid.UUID) ([]AssistantSession, error)
	ReadAssistant(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (AssistantSession, error)
	CreateAssistant(context.Context, identity.Principal, uuid.UUID, AssistantCreate) (AssistantSession, error)
	AppendAssistant(context.Context, identity.Principal, uuid.UUID, uuid.UUID, AssistantAppend) (AssistantSession, error)
}

// Assistant coordinates the private conversation and the existing operation owner.
type Assistant struct{ store AssistantStore }

// NewAssistant injects the private conversation store.
func NewAssistant(store AssistantStore) *Assistant { return &Assistant{store: store} }

// List returns the caller's bounded history within one authorized canvas.
func (s *Assistant) List(ctx context.Context, a identity.Principal, canvas uuid.UUID) ([]AssistantSession, error) {
	return s.store.ListAssistant(ctx, a, canvas)
}

// Read reauthorizes the conversation and its current execution pointer.
func (s *Assistant) Read(ctx context.Context, a identity.Principal, canvas, id uuid.UUID) (AssistantSession, error) {
	return s.store.ReadAssistant(ctx, a, canvas, id)
}

// Create validates bounded metadata before a durable identity is admitted.
func (s *Assistant) Create(ctx context.Context, a identity.Principal, canvas uuid.UUID, in AssistantCreate) (AssistantSession, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.ID == uuid.Nil || in.Title == "" || !utf8.ValidString(in.Title) || utf8.RuneCountInString(in.Title) > 128 {
		return AssistantSession{}, domain.ErrInvalidCommand
	}
	return s.store.CreateAssistant(ctx, a, canvas, in)
}

// Append validates a draft binding without starting any model invocation.
func (s *Assistant) Append(ctx context.Context, a identity.Principal, canvas, id uuid.UUID, in AssistantAppend) (AssistantSession, error) {
	if in.Execution == "" {
		in.Execution = "discussion"
	}
	if in.DraftID == uuid.Nil || in.ExpectedRevision < 1 || !validAssistantSkill(in.Skill) {
		return AssistantSession{}, domain.ErrInvalidCommand
	}
	if in.Execution != "discussion" && (in.Execution != "image" || (in.Skill != "poster" && in.Skill != "product_images")) {
		return AssistantSession{}, domain.ErrInvalidCommand
	}
	return s.store.AppendAssistant(ctx, a, canvas, id, in)
}
func validAssistantSkill(skill string) bool {
	switch skill {
	case "", "video_analysis", "storyboard", "advertisement", "short_film", "product_images", "poster", "brand":
		return true
	default:
		return false
	}
}
