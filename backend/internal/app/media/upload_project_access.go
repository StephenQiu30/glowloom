package media

import (
	"context"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

var _ mediaapp.UploadProjectAccess = mediaScopeAccess{}

func (a mediaScopeAccess) AuthorizeUploadProject(ctx context.Context, actor mediaapp.Actor, projectID uuid.UUID) (string, error) {
	if err := a.CheckActor(ctx, actor); err != nil {
		return "", err
	}
	aspect, err := a.project().AuthorizeUploadProject(ctx, IdentityActor(actor), projectID)
	return aspect, mediaLibraryProjectError(err)
}

func (a mediaScopeAccess) AwaitUploadProjectCommit(ctx context.Context, projectID uuid.UUID) error {
	return mediaLibraryProjectError(a.project().AwaitUploadProjectCommit(ctx, projectID))
}
