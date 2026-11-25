package media

import (
	"errors"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// Actor projects only authenticated reference facts at the legacy composition boundary.
// It carries neither platform role nor resource grants.
func Actor(actor identityapp.Principal) mediaapp.Actor {
	return mediaapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}

// IdentityActor adapts a reference to legacy live identity policy; it grants nothing.
func IdentityActor(actor mediaapp.Actor) identityapp.Principal {
	return identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}

// OwningError preserves original evidence while exposing Media's local error meaning.
func OwningError(err error) error {
	switch {
	case errors.Is(err, identityapp.ErrSessionInvalid):
		return errors.Join(mediaapp.ErrSessionInvalid, err)
	case errors.Is(err, identityapp.ErrForbidden):
		return errors.Join(mediaapp.ErrForbidden, err)
	case errors.Is(err, workspacesource.ErrScopeNotFound), errors.Is(err, workspacesource.ErrProjectNotFound):
		return errors.Join(mediaapp.ErrNotFound, err)
	default:
		return err
	}
}
