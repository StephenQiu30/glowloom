package media

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

var _ mediaapp.UploadProjectAccess = mediaScopeAccess{}

func (a mediaScopeAccess) AuthorizeUploadProject(ctx context.Context, actor identity.Principal, projectID uuid.UUID) (string, error) {
	if err := a.CheckActor(ctx, actor); err != nil {
		return "", err
	}
	aspect, err := a.project().AuthorizeUploadProject(ctx, actor, projectID)
	return aspect, mediaLibraryProjectError(err)
}

func (a mediaScopeAccess) AwaitUploadProjectCommit(ctx context.Context, projectID uuid.UUID) error {
	return mediaLibraryProjectError(a.project().AwaitUploadProjectCommit(ctx, projectID))
}
