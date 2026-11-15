package subject

import (
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func sourceMediaActor(actor identity.Principal) media.Actor {
	return media.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}
func sourceIdentityActor(actor media.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}
