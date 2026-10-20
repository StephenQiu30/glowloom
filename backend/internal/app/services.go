package app

import (
	"gorm.io/gorm"

	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func init() {
	mediawire.BindProjectWorkGuard(workspacewire.ProvideProjectWorkGuard)
	mediawire.BindProjectNavigation(func(tx *gorm.DB) mediawire.ProjectNavigation {
		return workspacewire.ProvideWorkspaceLifecycleStore(tx)
	})
	scriptwire.BindCharacterReferences(biblewire.ProvideScriptCharacterReferences)
}

// The constructors below stay on the process package so existing callers keep one composition entry.
// Their implementations live in the service packages.

// NewMediaScopeAccess provides request scope access for media callers.
func NewMediaScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess {
	return mediawire.NewMediaScopeAccess(tx)
}

// NewMediaWorkerScopeAccess provides worker scope access for media callers.
func NewMediaWorkerScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess {
	return mediawire.NewMediaWorkerScopeAccess(tx)
}

// NewCanvasProjectAccess provides project authority for canvas callers.
func NewCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return mediawire.NewCanvasProjectAccess(tx)
}

// NewCanvasProjectAccessWithScope provides canvas project authority using explicit scope access.
func NewCanvasProjectAccessWithScope(tx *gorm.DB, access mediaapp.ScopeAccess) canvasapp.ProjectAccess {
	return mediawire.NewCanvasProjectAccessWithScope(tx, access)
}

// NewWorkerCanvasProjectAccess provides worker project authority for canvas callers.
func NewWorkerCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return mediawire.NewWorkerCanvasProjectAccess(tx)
}

// NewMediaLibraryProjectAccess provides project authority for media library callers.
func NewMediaLibraryProjectAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return mediawire.NewMediaLibraryProjectAccess(tx)
}

// NewMediaLibraryWorkerProjectAccess provides worker project authority for media library callers.
func NewMediaLibraryWorkerProjectAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return mediawire.NewMediaLibraryWorkerProjectAccess(tx)
}

// NewMediaPurgeWorker assembles the media purge worker and its store.
func NewMediaPurgeWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.PurgeWorker, *pgmedia.PurgeStore) {
	return mediawire.NewMediaPurgeWorker(database, storage)
}

// NewMediaWorkerReferenceOwners assembles reference owners for media workers.
func NewMediaWorkerReferenceOwners(tx *gorm.DB) mediaapp.LibraryReferences {
	return mediawire.NewMediaWorkerReferenceOwners(tx)
}

// NewOperationFinalizer assembles operation finalization dependencies.
func NewOperationFinalizer(db *gorm.DB, operation *pgoperation.Store, billing *pgbilling.Store) operationflow.OperationFinalizer {
	return operationwire.NewOperationFinalizer(db, operation, billing)
}

// NewCatalogStore assembles the operation-facing catalog store.
func NewCatalogStore(db *gorm.DB) *pgcatalog.Store {
	return operationwire.NewCatalogStore(db)
}
