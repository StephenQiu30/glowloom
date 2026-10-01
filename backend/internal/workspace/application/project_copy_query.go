package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyCursor is the last immutable creation position in a bounded job page.
type ProjectCopyCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ProjectCopyListInput restricts recovery to one authorized source project.
type ProjectCopyListInput struct {
	SourceProjectID uuid.UUID
	Limit           int
	After           *ProjectCopyCursor
}

// ProjectCopyPage returns safe jobs and a keyset cursor, never request bodies or keys.
type ProjectCopyPage struct {
	Jobs []domain.ProjectCopyJob
	Next *ProjectCopyCursor
}

// ProjectCopyListStore reads current jobs behind source and account authorization.
type ProjectCopyListStore interface {
	List(context.Context, identityapp.Principal, ProjectCopyListInput) (ProjectCopyPage, error)
}

// List restores copy jobs after unknown creation or a page refresh.
func (s *ProjectCopyService) List(ctx context.Context, actor identityapp.Principal, input ProjectCopyListInput) (ProjectCopyPage, error) {
	if !canManageModelDefaults(actor) {
		return ProjectCopyPage{}, identityapp.ErrForbidden
	}
	if input.SourceProjectID == uuid.Nil || input.Limit < 0 || input.Limit > 100 || input.After != nil && (input.After.ID == uuid.Nil || input.After.CreatedAt.IsZero()) {
		return ProjectCopyPage{}, domain.ErrInvalidProjectCopy
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	if s == nil || s.store == nil {
		return ProjectCopyPage{}, ErrProjectDependencyUnavailable
	}
	store, ok := s.store.(ProjectCopyListStore)
	if !ok {
		return ProjectCopyPage{}, ErrProjectDependencyUnavailable
	}
	return store.List(ctx, actor, input)
}
