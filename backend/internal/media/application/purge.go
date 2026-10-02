package application

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// LibraryReferences reads actual current and immutable historical references
// from the installed owners. Missing or unreadable owners must return an error.
type LibraryReferences interface {
	HasMediaReferences(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (bool, error)
}

// PurgeInput binds an explicitly confirmed permanent deletion to visible CAS.
// Personal scopes use project revision zero, never a synthetic project UUID.
type PurgeInput struct {
	Scope                    domain.LibraryScope   `json:"scope"`
	Items                    []LibraryItemRevision `json:"items"`
	ExpectedRevision         int64                 `json:"expected_revision"`
	ExpectedProjectRevision  int64                 `json:"expected_project_revision"`
	PermanentDeleteConfirmed bool                  `json:"permanent_delete_confirmed"`
	Key                      uuid.UUID             `json:"-"`
}

// Validate closes the scope, explicit confirmation and bounded full batch CAS.
func (in PurgeInput) Validate() error {
	if in.Scope.Validate() != nil || !in.PermanentDeleteConfirmed || in.Key == uuid.Nil || len(in.Items) < 1 || len(in.Items) > 200 || in.ExpectedRevision < 0 || in.ExpectedRevision > math.MaxInt32 ||
		in.Scope.Kind == domain.LibraryPersonal && in.ExpectedProjectRevision != 0 || in.Scope.Kind == domain.LibraryProject && (in.ExpectedProjectRevision < 1 || in.ExpectedProjectRevision > math.MaxInt32) {
		return domain.ErrInvalidLibrary
	}
	seen := make(map[uuid.UUID]bool, len(in.Items))
	for _, item := range in.Items {
		if item.ID == uuid.Nil || item.Revision < 1 || item.Revision > math.MaxInt32 || seen[item.ID] {
			return domain.ErrInvalidLibrary
		}
		seen[item.ID] = true
	}
	return nil
}

// FrozenPurgeItem retains ownership facts without freezing editable text that
// this permanent command will scrub. Paths remain internal to the media owner.
type FrozenPurgeItem struct {
	ItemID          uuid.UUID          `json:"item_id"`
	CatalogRevision int64              `json:"catalog_revision"`
	Asset           *domain.MediaAsset `json:"asset,omitempty"`
	Renditions      []domain.Rendition `json:"renditions"`
}

// PurgeRepository owns public permanent commands and current authorized reads.
type PurgeRepository interface {
	CreatePurge(context.Context, identityapp.Principal, PurgeInput) (domain.PurgeJob, error)
	GetPurge(context.Context, identityapp.Principal, uuid.UUID) (domain.PurgeJob, error)
	ListPurges(context.Context, identityapp.Principal, domain.LibraryScope, int, int) ([]domain.PurgeJob, error)
	ControlPurge(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, int64, string) (domain.PurgeJob, error)
}

// PurgeWorkID is the immutable dispatcher identity of one physical attempt.
type PurgeWorkID struct {
	JobID       uuid.UUID `json:"job_id"`
	Attempt     int       `json:"attempt"`
	ExecutionID string    `json:"execution_id,omitempty"`
}

// PurgeDelivery proves a permanent command and the exact committed outbox data.
type PurgeDelivery struct {
	PurgeWorkID
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	Action    string    `json:"action"`
}

// PurgeLease binds private operations to a real creator and current SQL fence.
type PurgeLease struct {
	Work      PurgeWorkID
	Actor     identityapp.Principal
	Scope     domain.LibraryScope
	Fence     uuid.UUID
	ItemCount int
	Done      bool
	Job       domain.PurgeJob
	StartedAt time.Time
}
