package canvas

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/google/uuid"
	"gorm.io/gorm"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"

	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
)

type assetCreationAccess struct{ tx *gorm.DB }

// AssetCreationAuthority binds the actual workspace creator to the canvas transaction.
func AssetCreationAuthority(tx *gorm.DB) application.AssetCreationAccess {
	return assetCreationAccess{tx: tx}
}

func (a assetCreationAccess) AuthorizeAssetCreation(ctx context.Context, actor identity.Principal) error {
	return assetMutationAccess{tx: a.tx}.AuthorizeAssetCommand(ctx, actor, true)
}

func (a assetCreationAccess) CreateAssetProject(ctx context.Context, actor identity.Principal, key uuid.UUID, name string) (application.AssetProject, error) {
	project, err := workspaceapp.NewCreateProjectCommand(workspacewire.ProvideWorkspaceLifecycleStore(a.tx), time.Now).ExecuteIdempotent(ctx, actor, key, workspacesource.CreateProjectInput{Name: name, AspectRatio: "16:9", StyleType: "realistic", RequestID: key.String()})
	if errors.Is(err, workspacesource.ErrIdempotencyConflict) {
		return application.AssetProject{}, application.ErrIdempotencyConflict
	}
	if err != nil {
		return application.AssetProject{}, assetMutationError(err)
	}
	return application.AssetProject{ProjectID: project.ID, CanvasID: project.CanvasID}, nil
}
