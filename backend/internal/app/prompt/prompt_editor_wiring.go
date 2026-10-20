package prompt

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

// DraftAccess authorizes a prompt draft through the current project content scope.
func DraftAccess(tx *gorm.DB) promptapp.DraftAccess {
	return promptDraftAccess{scopewire.ProvideProjectContentAccess(tx)}
}

type promptDraftAccess struct {
	store interface {
		Authorize(context.Context, identityapp.Principal, uuid.UUID, bool) (workspaceapp.ProjectContentAccess, error)
	}
}

func (a promptDraftAccess) AuthorizeDraft(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, actor, project, write)
	if errors.Is(err, workspaceapp.ErrProjectNotFound) || errors.Is(err, workspaceapp.ErrScopeNotFound) {
		return promptapp.ErrDraftNotFound
	}
	return err
}
