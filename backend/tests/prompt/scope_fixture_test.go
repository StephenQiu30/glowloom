package prompt_test

import (
	"time"

	"gorm.io/gorm"

	promptpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	identitypg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
)

func preferenceStore(db *gorm.DB) *promptpg.Store {
	return promptpg.NewScopedStore(db, promptwire.SessionAccess)
}

func preferenceRequestScope(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(workspacepg.NewRequestScopeStore(tx, func(db *gorm.DB) workspaceapp.SessionActorAccess {
		return identitypg.NewStore(db)
	}), time.Now)
}

func preferenceWorkspaceStore(db *gorm.DB) *workspacepg.Store {
	return workspacepg.NewScopedStore(db, preferenceRequestScope, nil, nil)
}

func preferenceProjectAccess(tx *gorm.DB) *workspacepg.ProjectContentAccessStore {
	return workspacepg.NewScopedProjectContentAccessStore(tx, preferenceRequestScope(tx))
}
