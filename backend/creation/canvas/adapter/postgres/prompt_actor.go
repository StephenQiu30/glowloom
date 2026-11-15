package postgres

import (
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Canvas retains its existing authority port during migration. Prompt receives
// only the content reference and rechecks its own injected owner capabilities.
func canvasPromptReference(actor identity.Principal) prompt.Actor {
	return prompt.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
