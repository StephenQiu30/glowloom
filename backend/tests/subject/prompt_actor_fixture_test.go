package subject_test

import (
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

func subjectPromptActor(actor identity.Principal) prompt.Actor {
	return prompt.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
