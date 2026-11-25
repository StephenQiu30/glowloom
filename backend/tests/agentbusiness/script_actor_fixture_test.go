package agentbusiness_test

import (
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func agentScriptActor(actor identityapp.Principal) scriptapp.Actor { return scriptwire.Actor(actor) }
func agentScriptAccess(access *workspacepg.ProjectContentAccessStore) scriptapp.ProjectAccess {
	return scriptwire.ProjectAccess(access)
}
