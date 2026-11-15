package agentbusiness_test

import (
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
)

func agentScriptActor(actor identityapp.Principal) scriptapp.Actor { return scriptwire.Actor(actor) }
func agentScriptAccess(access *workspacepg.ProjectContentAccessStore) scriptapp.ProjectAccess {
	return scriptwire.ProjectAccess(access)
}
