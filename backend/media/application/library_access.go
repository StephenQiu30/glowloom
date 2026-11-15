package application

import (
	"context"

	"github.com/google/uuid"
)

// LibraryProjectFacts are actual workspace facts held by the content transaction.
type LibraryProjectFacts struct {
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	Revision  int64
}

// LibraryProjectAccess belongs to the consuming media owner. Workspace retains
// its project lock and emits its existing content invalidation in TouchContent.
type LibraryProjectAccess interface {
	Authorize(context.Context, Actor, uuid.UUID, bool) (LibraryProjectFacts, error)
	TouchContent(context.Context, Actor, uuid.UUID, int64) (int64, error)
}
