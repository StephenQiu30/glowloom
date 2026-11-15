package app

import (
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/postgres"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/settlement"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/task"
	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func init() {
	mediawire.BindProjectWorkGuard(workspacewire.ProvideProjectWorkGuard)
	mediawire.BindProjectNavigation(func(tx *gorm.DB) mediawire.ProjectNavigation {
		return workspacewire.ProvideWorkspaceLifecycleStore(tx)
	})
	scriptwire.BindCharacterReferences(biblewire.ProvideScriptCharacterReferences)
}

// Process-level constructors compose dependencies from the owning service packages.

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
func NewMediaPurgeWorker(database *gorm.DB, registry *objectstorage.Registry) (*mediaapp.PurgeWorker, *pgmedia.PurgeStore) {
	return mediawire.NewRoutedMediaPurgeWorker(database, registry)
}

// NewMediaWorkerReferenceOwners assembles reference owners for media workers.
func NewMediaWorkerReferenceOwners(tx *gorm.DB) mediaapp.LibraryReferences {
	return mediawire.NewMediaWorkerReferenceOwners(tx)
}

// NewOperationFinalizer assembles operation finalization dependencies.
func NewOperationFinalizer(db *gorm.DB, operation *pgoperation.Store, billing *pgbilling.Store) operationflow.OperationFinalizer {
	return settlement.NewOperationFinalizer(db, operation, billing)
}

// NewCatalogStore assembles the operation-facing catalog store.
func NewCatalogStore(db *gorm.DB) *pgcatalog.Store {
	return operationwire.NewCatalogStore(db)
}
