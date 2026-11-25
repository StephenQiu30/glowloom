package canvas_test

import (
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func promptActor(actor identityapp.Principal) promptapp.Actor {
	return promptapp.Actor{
		ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID,
		SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch,
	}
}
