package projectcopy

import (
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/bible/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func projectCopyBibleActor(actor identityapp.Principal) bibleapp.Actor {
	return bibleapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

func projectCopyIdentityActor(actor bibleapp.Actor) identityapp.Principal {
	return identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
