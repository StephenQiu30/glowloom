package app

import (
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func provideProjectFolderStore(database *gorm.DB) *pgworkspace.FolderStore {
	return pgworkspace.NewFolderStore(database, provideProjectWorkGuard,
		func(tx *gorm.DB) workspaceapp.FolderMediaReader {
			return mediaapp.NewAssetQuery(pgmedia.NewStore(tx), nil)
		})
}
