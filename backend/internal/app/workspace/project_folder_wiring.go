// Package workspace wires project lifecycle and folder dependencies.
package workspace

import (
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// ProvideProjectFolderStore assembles project folder persistence.
func ProvideProjectFolderStore(database *gorm.DB) *pgworkspace.FolderStore {
	return pgworkspace.NewScopedFolderStore(database, scopewire.ProvideRequestScopeAccess, ProvideProjectWorkGuard,
		func(tx *gorm.DB) workspaceapp.FolderMediaReader {
			return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess))
		})
}
