package media

import (
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// ProvideAssetDownloads retains current scope authorization and explicit object routes.
// Legacy is used only for rows with no persisted ObjectLocation.
func ProvideAssetDownloads(database *gorm.DB, objects application.ObjectReader, legacy *objectstorage.Client) *application.AssetDownloads {
	store := pgmedia.NewScopedStore(database, NewMediaScopeAccess)
	if legacy == nil {
		return application.NewAssetDownloads(store, objects)
	}
	return application.NewAssetDownloads(store, objects, legacy)
}
