package subject_test

import (
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func mediaBoundaryActor(actor identity.Principal) media.Actor {
	return media.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}
