package prompt_test

import (
	"time"

	"gorm.io/gorm"

	identitypg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func preferenceStore(db *gorm.DB) *promptpg.Store {
	return promptpg.NewScopedStore(db, func(tx *gorm.DB) promptapp.ActorAccess {
		return identitypg.NewStore(tx)
	})
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
