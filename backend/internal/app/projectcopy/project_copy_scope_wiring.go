package projectcopy

import (
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func provideCopyWorkerScopeAccess(tx *gorm.DB) workspace.WorkerScopeResolver {
	return scopewire.ProvideScopeAccess(tx)
}
func provideCopyAccess(tx *gorm.DB, authority workspace.ProjectCopyAuthority) *pgworkspace.ProjectCopyAccessStore {
	return pgworkspace.NewScopedProjectCopyAccessStore(tx, authority, scopewire.ProvideRequestScopeAccess, provideCopyWorkerScopeAccess)
}
