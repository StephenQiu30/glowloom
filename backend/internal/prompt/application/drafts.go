package application

import (
	"context"
	"errors"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

	"github.com/google/uuid"
)

var ErrDraftNotFound = errors.New("prompt_draft_not_found")

// DraftSnapshot is an atomic receipt, including metadata which is absent from InputHash.
type DraftSnapshot struct {
	DraftID   uuid.UUID    `json:"draft_id"`
	Revision  int64        `json:"revision"`
	DraftHash string       `json:"draft_hash"`
	InputHash string       `json:"input_hash"`
	Draft     domain.Draft `json:"draft"`
	UpdatedAt time.Time    `json:"updated_at"`
}
type DraftWrite struct {
	ProjectID        uuid.UUID
	DraftID          uuid.UUID
	ExpectedRevision int64
	Key              uuid.UUID
	Draft            domain.Draft
}
type DraftPage struct {
	Items      []DraftSnapshot `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
type DraftAccess interface {
	AuthorizeDraft(context.Context, identityapp.Principal, uuid.UUID, bool) error
}
type DraftStore interface {
	SaveDraft(context.Context, identityapp.Principal, DraftWrite) (DraftSnapshot, error)
	ReadDraft(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (DraftSnapshot, error)
	ListDrafts(context.Context, identityapp.Principal, uuid.UUID, string) (DraftPage, error)
}
