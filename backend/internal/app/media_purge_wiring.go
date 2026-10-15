package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/internal/bible/adapter/postgres"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgmediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

type avatarReferenceOwner interface {
	HasAvatarReference(context.Context, uuid.UUID) (bool, error)
}
type mediaReferenceOwners struct {
	owners  []mediaapp.LibraryReferences
	avatars []avatarReferenceOwner
}

func (g mediaReferenceOwners) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	used := false
	for _, owner := range g.avatars {
		if owner == nil {
			return false, mediaapp.ErrUnavailable
		}
		present, err := owner.HasAvatarReference(ctx, asset)
		if err != nil {
			return false, err
		}
		used = used || present
	}
	// A nil project is only supplied after the media store authorized a personal library.
	if project == uuid.Nil {
		if len(g.avatars) != 2 {
			return false, mediaapp.ErrUnavailable
		}
		return used, nil
	}
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
	return provideMediaReferenceOwnersWithAccess(tx, provideProjectContentAccess(tx), provideMediaLibraryProjectAccess, provideMediaScopeAccess)
}

// The same installed reference owners use the transaction-bound authority of
// their caller. Workers must not inherit request-session factories indirectly
// through Bible, script, or unpublished package reference readers.
func provideMediaReferenceOwnersWithAccess(tx *gorm.DB, access *pgworkspace.ProjectContentAccessStore, project pgmedia.LibraryProjectAccessFactory, scopes pgmedia.ScopeAccessFactory) mediaapp.LibraryReferences {
	return mediaReferenceOwners{avatars: []avatarReferenceOwner{pgidentity.NewAvatarReferences(tx), pgworkspace.NewTeamAvatarReferences(tx)}, owners: []mediaapp.LibraryReferences{
		pgworkspace.NewScopedMediaReferenceGuard(tx, access),
		pgcanvas.NewScopedMediaReferenceGuard(tx, func(tx *gorm.DB) canvasapp.ProjectAccess { return scopes(tx) }),
		pgoperation.NewScopedMediaReferenceGuard(tx, func(*gorm.DB) operationapp.ProjectAccess { return operationProjectAccess{store: access} }),
		pgbible.NewMediaReferenceGuard(tx, access),
		pgscript.NewMediaReferenceGuard(tx, access),
		pgmediatool.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))),
		mediaPackageReferences{store: pgmedia.NewScopedPackageStore(tx, project, provideMediaLibraryWorkGuards, time.Now, scopes)},
	}}
}

func provideMediaPurgeStore(database *gorm.DB) *pgmedia.PurgeStore {
	return pgmedia.NewScopedPurgeStore(database, provideMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, provideMediaReferenceOwners, time.Now, provideMediaScopeAccess)
}

func provideMediaPurgeWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.PurgeWorker, *pgmedia.PurgeStore) {
	store := provideMediaPurgeWorkerStore(database)
	return mediaapp.NewPurgeWorker(store, mediaobjects.NewProjectCopyObjects(storage)), store
}

func provideMediaWorkerReferenceOwners(tx *gorm.DB) mediaapp.LibraryReferences {
	return provideMediaReferenceOwnersWithAccess(tx, provideWorkerProjectContentAccess(tx), provideMediaLibraryWorkerProjectAccess, provideMediaWorkerScopeAccess)
}

func provideMediaPurgeWorkerStore(database *gorm.DB) *pgmedia.PurgeStore {
	return pgmedia.NewScopedPurgeStore(database, provideMediaLibraryWorkerProjectAccess, provideMediaLibraryWorkGuards, provideMediaWorkerReferenceOwners, time.Now, provideMediaWorkerScopeAccess)
}
