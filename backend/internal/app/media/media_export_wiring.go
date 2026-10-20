package media

import (
	"gorm.io/gorm"

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
