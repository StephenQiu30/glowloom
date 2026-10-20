package workspace

import (
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func ProvideProjectFolderStore(database *gorm.DB) *pgworkspace.FolderStore {
	return pgworkspace.NewScopedFolderStore(database, scopewire.ProvideRequestScopeAccess, ProvideProjectWorkGuard,
		func(tx *gorm.DB) workspaceapp.FolderMediaReader {
			return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
		})
}
