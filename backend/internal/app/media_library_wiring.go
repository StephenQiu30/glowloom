package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// mediaLibraryProjectAccess retains workspace's project lock in the library
// owner's transaction and converts only facts declared by the media port.
type mediaLibraryProjectAccess struct {
	store *pgworkspace.ProjectContentAccessStore
}

func provideMediaLibraryProjectAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return mediaLibraryProjectAccess{store: pgworkspace.NewProjectContentAccessStore(tx)}
}

func (a mediaLibraryProjectAccess) Authorize(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	facts, err := a.store.Authorize(ctx, actor, projectID, write)
	if err != nil {
		return mediaapp.LibraryProjectFacts{}, mediaLibraryProjectError(err)
	}
	return mediaapp.LibraryProjectFacts{ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, nil
}

func (a mediaLibraryProjectAccess) TouchContent(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, expected int64) (int64, error) {
	revision, err := a.store.TouchContent(ctx, actor, projectID, expected)
	return revision, mediaLibraryProjectError(err)
}

func mediaLibraryProjectError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, workspaceapp.ErrProjectNotFound):
		return errors.Join(mediaapp.ErrNotFound, err)
	case errors.Is(err, workspacedomain.ErrProjectRevisionConflict):
		return errors.Join(mediaapp.ErrLibraryConflict, err)
	case errors.Is(err, workspacedomain.ErrProjectStateConflict):
		return errors.Join(mediadomain.ErrMediaStateConflict, err)
	default:
		return errors.Join(mediaapp.ErrUnavailable, err)
	}
}
