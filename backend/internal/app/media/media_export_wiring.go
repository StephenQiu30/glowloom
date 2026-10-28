package media

import (
	"gorm.io/gorm"

	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

// ProvideMediaExportStore binds source authorization and media publication to
// the export command transaction through their owning application ports.
func ProvideMediaExportStore(database *gorm.DB) *pgtool.Store {
	return ProvideMediaExportStoreWithAccess(database, NewMediaScopeAccess)
}

// ProvideMediaExportStoreWithAccess assembles the export store with explicit scope access.
func ProvideMediaExportStoreWithAccess(database *gorm.DB, scopes pgmedia.ScopeAccessFactory) *pgtool.Store {
	return pgtool.NewStore(database,
		func(tx *gorm.DB) toolapp.TimelineReader {
			return canvasapp.NewTimelineReader(pgcanvas.NewScopedStore(tx,
				func(mediaTx *gorm.DB) canvasapp.MediaReader {
					return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(mediaTx, scopes), nil)
				}, func(tx *gorm.DB) canvasapp.ProjectAccess { return canvasScopedAccess{ScopeAccess: scopes(tx), tx: tx} }))
		},
		func(tx *gorm.DB) toolapp.DerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
		})
}

// ProvideRoutedMediaExportStore freezes the output destination at admission.
func ProvideRoutedMediaExportStore(database *gorm.DB, scopes pgmedia.ScopeAccessFactory) *pgtool.Store {
	return ProvideMediaExportStoreWithAccess(database, scopes).WithStorageAdmission(func(tx *gorm.DB) toolapp.ToolStorageLedger { return pgmedia.NewStorageRoutes(tx) })
}

// ToolObjects provides each tool execution its own frozen object resolver.
func ToolObjects(database *gorm.DB, registry *objectstorage.Registry, legacy *objectstorage.Client) toolapp.WorkObjectsFactory {
	return toolflow.NewRoutedObjects(registry, pgmedia.NewStorageRoutes(database), legacy)
}
