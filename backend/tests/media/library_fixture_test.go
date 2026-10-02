package media_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func libraryRuntimeDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("LV_TEST_LIBRARY_OWNER_DB_DSN") == "" {
		t.Skip("set the isolated library fixture scope before running library integration tests")
	}
	db := mediaStoreDB(t)
	var name, role string
	if err := db.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "lanverse_library" || role != "lanverse_app" {
		t.Fatal("library tests require isolated database and actual nonowner role", name, role, err)
	}
	return db
}

type libraryTestProjectAccess struct {
	store *pgworkspace.ProjectContentAccessStore
}

func libraryTestAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return libraryTestProjectAccess{pgworkspace.NewProjectContentAccessStore(tx)}
}

func (a libraryTestProjectAccess) Authorize(ctx context.Context, actor identityapp.Principal, id uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	facts, err := a.store.Authorize(ctx, actor, id, write)
	if errors.Is(err, workspaceapp.ErrProjectNotFound) {
		err = errors.Join(mediaapp.ErrNotFound, err)
	}
	return mediaapp.LibraryProjectFacts{ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, err
}

func (a libraryTestProjectAccess) TouchContent(ctx context.Context, actor identityapp.Principal, id uuid.UUID, revision int64) (int64, error) {
	next, err := a.store.TouchContent(ctx, actor, id, revision)
	if errors.Is(err, workspacedomain.ErrProjectRevisionConflict) {
		err = errors.Join(mediaapp.ErrLibraryConflict, err)
	}
	return next, err
}
