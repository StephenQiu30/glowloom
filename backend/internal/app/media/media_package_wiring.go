package media

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func provideMediaPackageStore(database *gorm.DB) *pgmedia.PackageStore {
	// The constructor retains this factory without invoking it. The lifecycle
	// and reference ports below authorize their own rows without calling guards.
	return pgmedia.NewScopedPackageStore(database, NewMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaScopeAccess)
}

// PackageWorkGuard reports in-flight package work for the workspace lifecycle owner list.
func PackageWorkGuard(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
	return mediaPackageWorkGuard{store: provideMediaPackageStore(tx)}
}

type mediaPackageWorkGuard struct{ store *pgmedia.PackageStore }

func (g mediaPackageWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if g.store == nil {
		return false, workspacesource.ErrProjectDependencyUnavailable
	}
	return g.store.HasInflightProjectLifecycleWork(ctx, Actor(actor), project)
}

type mediaPackageReferences struct{ store *pgmedia.PackageStore }

func (g mediaPackageReferences) HasMediaReferences(ctx context.Context, actor mediaapp.Actor, project, asset uuid.UUID) (bool, error) {
	if g.store == nil {
		return false, mediaapp.ErrUnavailable
	}
	return g.store.HasPackageReferences(ctx, actor, project, asset)
}

// ProvideRoutedMediaPackages binds imported ZIPs and all assets to one frozen work profile.
func ProvideRoutedMediaPackages(database *gorm.DB, registry *objectstorage.Registry, uploads *mediaapp.UploadService) *mediaapp.LibraryPackageService {
	store := provideMediaPackageStore(database).WithObjectLocations()
	return mediaapp.NewLibraryPackageService(store, uploads, mediaobjects.NewRoutedPackageObjects(registry, pgmedia.NewStorageRoutes(database)), time.Now)
}
