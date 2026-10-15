package application

import (
	"context"
	"errors"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

	"github.com/google/uuid"
)

// ErrDraftNotFound hides missing and unauthorized draft identities.
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

// DraftWrite contains complete intent, its owner and a conditional write identity.
type DraftWrite struct {
	ProjectID        uuid.UUID
	DraftID          uuid.UUID
	ExpectedRevision int64
	Key              uuid.UUID
	Draft            domain.Draft
}

// DraftPage is a bounded, caller-private discovery response.
type DraftPage struct {
	Items      []DraftSnapshot `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// DraftAccess is the owning project authorization boundary.
type DraftAccess interface {
	AuthorizeDraft(context.Context, identityapp.Principal, uuid.UUID, bool) error
}

// UploadResolution contains no private upload metadata and is always reauthorized.
type UploadResolution struct {
	Found     bool                    `json:"found"`
	Source    *domain.ReferenceSource `json:"source,omitempty"`
	MediaKind string                  `json:"media_kind,omitempty"`
}

// DraftStore owns persistence, discovery and source validation.
type DraftStore interface {
	ResolveUpload(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (UploadResolution, error)
	ValidateFragment(context.Context, identityapp.Principal, uuid.UUID, domain.Draft) error
	SaveDraft(context.Context, identityapp.Principal, DraftWrite) (DraftSnapshot, error)
	ReadDraft(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (DraftSnapshot, error)
	ListDrafts(context.Context, identityapp.Principal, uuid.UUID, string) (DraftPage, error)
}
