package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type promptDraftAccess struct {
	store interface {
		Authorize(context.Context, identityapp.Principal, uuid.UUID, bool) (workspaceapp.ProjectContentAccess, error)
	}
}

func (a promptDraftAccess) AuthorizeDraft(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, actor, project, write)
	if errors.Is(err, workspaceapp.ErrProjectNotFound) {
		return promptapp.ErrDraftNotFound
	}
	return err
}
