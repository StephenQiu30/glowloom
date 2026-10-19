package media_test

import (
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func mediaTestStore(db *gorm.DB) *pgmedia.Store {
	return pgmedia.NewScopedStore(db, app.NewMediaScopeAccess)
}

func mediaWorkerStore(db *gorm.DB) *pgmedia.Store {
	return pgmedia.NewScopedStore(db, app.NewMediaWorkerScopeAccess)
}

func mediaCanvasStore(db *gorm.DB, media pgcanvas.MediaReaderFactory) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(db, media, app.NewCanvasProjectAccess)
}

func mediaCanvasWorkerStore(db *gorm.DB, media pgcanvas.MediaReaderFactory) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(db, media, app.NewWorkerCanvasProjectAccess)
}

func mediaLibraryStore(db *gorm.DB, project pgmedia.LibraryProjectAccessFactory, clock func() time.Time) *pgmedia.LibraryStore {
	return pgmedia.NewScopedLibraryStore(db, project, clock, app.NewMediaScopeAccess)
}

func mediaPackageStore(db *gorm.DB, project pgmedia.LibraryProjectAccessFactory, guards pgmedia.LibraryWorkGuardFactory, clock func() time.Time) *pgmedia.PackageStore {
	return pgmedia.NewScopedPackageStore(db, project, guards, clock, app.NewMediaScopeAccess)
}

// Request and worker consumers retain separate concrete repositories and real
// production authority factories. Call sites select Worker explicitly.
type mediaTransferStores struct {
	*pgmedia.TransferStore
	Worker *pgmedia.TransferStore
}

func mediaTransferStore(db *gorm.DB, project pgmedia.LibraryProjectAccessFactory, guards pgmedia.LibraryWorkGuardFactory, clock func() time.Time) *mediaTransferStores {
	return &mediaTransferStores{
		TransferStore: pgmedia.NewScopedTransferStore(db, project, guards, clock, app.NewMediaScopeAccess),
		Worker:        pgmedia.NewScopedTransferStore(db, app.NewMediaLibraryWorkerProjectAccess, guards, clock, app.NewMediaWorkerScopeAccess),
	}
}

type mediaPurgeStores struct {
	*pgmedia.PurgeStore
	Worker *pgmedia.PurgeStore
}

func mediaPurgeStore(db *gorm.DB, project pgmedia.LibraryProjectAccessFactory, guards pgmedia.LibraryWorkGuardFactory, references pgmedia.LibraryReferenceFactory, clock func() time.Time) *mediaPurgeStores {
	return &mediaPurgeStores{
		PurgeStore: pgmedia.NewScopedPurgeStore(db, project, guards, references, clock, app.NewMediaScopeAccess),
		Worker:     pgmedia.NewScopedPurgeStore(db, app.NewMediaLibraryWorkerProjectAccess, guards, references, clock, app.NewMediaWorkerScopeAccess),
	}
}

func mediaCopyStore(db *gorm.DB) *copyMediaTransactions {
	return &copyMediaTransactions{ProjectCopyStore: pgmedia.NewScopedProjectCopyStore(db, app.NewMediaWorkerScopeAccess), db: db}
}

func mediaDocumentStore(db *gorm.DB) *pgmedia.DocumentSourceStore {
	return pgmedia.NewScopedDocumentSourceStore(db, app.NewMediaScopeAccess)
}

func mediaWorkspaceRequestScope(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(db *gorm.DB) workspaceapp.SessionActorAccess { return pgidentity.NewStore(db) }), time.Now)
}

func mediaStorageUsageStore(db *gorm.DB, project pgmedia.LibraryProjectAccessFactory, intents pgmedia.StorageUsageIntentFactory) *pgmedia.StorageUsageStore {
	return pgmedia.NewScopedStorageUsageStore(db, project, intents, app.NewMediaScopeAccess)
}

func mediaCanvasStoreWithScope(db *gorm.DB, media pgcanvas.MediaReaderFactory, scopes pgmedia.ScopeAccessFactory) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(db, media, func(tx *gorm.DB) canvasapp.ProjectAccess { return app.NewCanvasProjectAccessWithScope(tx, scopes(tx)) })
}

func mediaScopedIngestActivities(db *gorm.DB, objects *objectstorage.Client, policy mediaflow.DownloadPolicy) (*mediaflow.Activities, error) {
	return mediaflow.NewScopedActivities(db, objects, policy, app.NewMediaWorkerScopeAccess)
}
