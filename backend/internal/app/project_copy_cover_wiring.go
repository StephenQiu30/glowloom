package app

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

type projectCopyCoverOwner struct{ tx *gorm.DB }

func (o projectCopyCoverOwner) Freeze(ctx context.Context, actor identityapp.Principal, project uuid.UUID, asset *uuid.UUID) error {
	return workspaceapp.FreezeProjectCover(ctx, mediaapp.NewAssetQuery(pgmedia.NewStore(o.tx), nil), actor, project, asset)
}

func (o projectCopyCoverOwner) Verify(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, expected, actual *uuid.UUID) error {
	reader := copiedCanvasMedia{store: pgmedia.NewProjectCopyStore(o.tx), binding: binding}
	return workspaceapp.VerifyProjectCover(ctx, reader, actor, binding.TargetProjectID, expected, actual)
}

var _ workspaceapp.ProjectCopyCoverOwner = projectCopyCoverOwner{}
