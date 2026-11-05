package media

import (
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ProvideAssetDownloads retains scope authorization and explicit object routes.
func ProvideAssetDownloads(database *gorm.DB, objects application.ObjectReader) *application.AssetDownloads {
	store := pgmedia.NewScopedStore(database, NewMediaScopeAccess)
	return application.NewAssetDownloads(store, objects)
}
