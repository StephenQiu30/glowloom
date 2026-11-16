// Package prompt wires prompt editor access dependencies.
package prompt

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	promptapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"

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

func (a promptDraftAccess) AuthorizeDraft(ctx context.Context, actor promptapp.Actor, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, legacyActor(actor), project, write)
	if errors.Is(err, workspacesource.ErrProjectNotFound) || errors.Is(err, workspacesource.ErrScopeNotFound) {
		return promptapp.ErrDraftNotFound
	}
	return AccessError(err)
}
