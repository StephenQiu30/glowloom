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

func NewMediaScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess {
	return mediawire.NewMediaScopeAccess(tx)
}

func NewMediaWorkerScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess {
	return mediawire.NewMediaWorkerScopeAccess(tx)
}

func NewCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return mediawire.NewCanvasProjectAccess(tx)
}

func NewCanvasProjectAccessWithScope(tx *gorm.DB, access mediaapp.ScopeAccess) canvasapp.ProjectAccess {
	return mediawire.NewCanvasProjectAccessWithScope(tx, access)
}

func NewWorkerCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return mediawire.NewWorkerCanvasProjectAccess(tx)
}

func NewMediaLibraryProjectAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return mediawire.NewMediaLibraryProjectAccess(tx)
}

func NewMediaLibraryWorkerProjectAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return mediawire.NewMediaLibraryWorkerProjectAccess(tx)
}

func NewMediaPurgeWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.PurgeWorker, *pgmedia.PurgeStore) {
	return mediawire.NewMediaPurgeWorker(database, storage)
}

func NewMediaWorkerReferenceOwners(tx *gorm.DB) mediaapp.LibraryReferences {
	return mediawire.NewMediaWorkerReferenceOwners(tx)
}

func NewOperationFinalizer(db *gorm.DB, operation *pgoperation.Store, billing *pgbilling.Store) operationflow.OperationFinalizer {
	return operationwire.NewOperationFinalizer(db, operation, billing)
}

func NewCatalogStore(db *gorm.DB) *pgcatalog.Store {
	return operationwire.NewCatalogStore(db)
}
