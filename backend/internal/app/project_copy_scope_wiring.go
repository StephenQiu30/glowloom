package app

import (
	"gorm.io/gorm"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func provideCopyWorkerScopeAccess(tx *gorm.DB) workspace.WorkerScopeResolver {
	return provideScopeAccess(tx)
}
func provideCopyAccess(tx *gorm.DB, authority workspace.ProjectCopyAuthority) *pgworkspace.ProjectCopyAccessStore {
	return pgworkspace.NewScopedProjectCopyAccessStore(tx, authority, provideRequestScopeAccess, provideCopyWorkerScopeAccess)
}
