package application

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Location contains only the authorized routing identity, never document content.
type Location struct {
	CanvasID  uuid.UUID `json:"canvas_id"`
	ProjectID uuid.UUID `json:"project_id"`
	ScopeID   uuid.UUID `json:"scope_id"`
}

// Locate resolves an existing link independently of the page's selected scope.
func (s *Service) Locate(ctx context.Context, actor identity.Principal, id uuid.UUID) (Location, error) {
	return s.store.Locate(ctx, actor, id)
}
