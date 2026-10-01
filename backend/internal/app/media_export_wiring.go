package app

import (
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// provideMediaExportStore binds source authorization and media publication to
// the export command transaction through their owning application ports.
func provideMediaExportStore(database *gorm.DB) *pgtool.Store {
	return pgtool.NewStore(database,
		func(tx *gorm.DB) toolapp.TimelineReader {
			return canvasapp.NewTimelineReader(pgcanvas.NewStore(tx,
				func(mediaTx *gorm.DB) canvasapp.MediaReader {
					return mediaapp.NewAssetQuery(pgmedia.NewStore(mediaTx), nil)
				}))
		},
		func(tx *gorm.DB) toolapp.DerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewStore(tx))
		})
}
