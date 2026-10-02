package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgmediatool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	pgscript "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

type mediaReferenceOwners struct{ owners []mediaapp.LibraryReferences }

func (g mediaReferenceOwners) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	used := false
	// These owners share one current-authority transaction. Check every owner
	// sequentially; a true reference cannot turn another unreadable owner idle.
	for _, owner := range g.owners {
		if owner == nil {
			return false, mediaapp.ErrUnavailable
		}
		present, err := owner.HasMediaReferences(ctx, actor, project, asset)
		if err != nil {
			return false, fmt.Errorf("read installed media reference owner: %w", mediaLibraryProjectError(err))
		}
		used = used || present
	}
	return used, nil
}

func provideMediaReferenceOwners(tx *gorm.DB) mediaapp.LibraryReferences {
	return mediaReferenceOwners{owners: []mediaapp.LibraryReferences{
		pgworkspace.NewMediaReferenceGuard(tx),
		pgcanvas.NewMediaReferenceGuard(tx),
		pgoperation.NewMediaReferenceGuard(tx),
		pgbible.NewMediaReferenceGuard(tx, provideBibleProjectAccess(tx)),
		pgscript.NewMediaReferenceGuard(tx, provideScriptProjectAccess(tx)),
		pgmediatool.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx))),
		mediaPackageReferences{store: provideMediaPackageStore(tx)},
	}}
}

func provideMediaPurgeStore(database *gorm.DB) *pgmedia.PurgeStore {
	return pgmedia.NewPurgeStore(database, provideMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, provideMediaReferenceOwners, time.Now)
}

func provideMediaPurgeWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.PurgeWorker, *pgmedia.PurgeStore) {
	store := provideMediaPurgeStore(database)
	return mediaapp.NewPurgeWorker(store, mediaobjects.NewProjectCopyObjects(storage)), store
}
