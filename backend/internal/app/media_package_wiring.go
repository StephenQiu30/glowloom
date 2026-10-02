package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func provideMediaPackageStore(database *gorm.DB) *pgmedia.PackageStore {
	// The constructor retains this factory without invoking it. The lifecycle
	// and reference ports below authorize their own rows without calling guards.
	return pgmedia.NewPackageStore(database, provideMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now)
}

func provideMediaPackages(database *gorm.DB, storage *objectstorage.Client, uploads *mediaapp.UploadService) *mediaapp.LibraryPackageService {
	return mediaapp.NewLibraryPackageService(provideMediaPackageStore(database), uploads, mediaobjects.NewProjectCopyObjects(storage), time.Now)
}

type mediaPackageWorkGuard struct{ store *pgmedia.PackageStore }

func (g mediaPackageWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if g.store == nil {
		return false, workspaceapp.ErrProjectDependencyUnavailable
	}
	return g.store.HasInflightPackageWork(ctx, actor, project)
}

type mediaPackageReferences struct{ store *pgmedia.PackageStore }

func (g mediaPackageReferences) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if g.store == nil {
		return false, mediaapp.ErrUnavailable
	}
	return g.store.HasPackageReferences(ctx, actor, project, asset)
}
