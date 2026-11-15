package script

import (
	"context"
	"errors"

	"github.com/google/uuid"

	scriptapp "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// Actor projects the original authenticated identity without copying roles.
func Actor(actor identityapp.Principal) scriptapp.Actor {
	return scriptapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

// IdentityActor is used only at the existing legacy policy boundary.
func IdentityActor(actor scriptapp.Actor) identityapp.Principal {
	return identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

// OwningError retains the original policy failure and its Script meaning.
func OwningError(err error) error {
	switch {
	case errors.Is(err, identityapp.ErrSessionInvalid), errors.Is(err, mediaapp.ErrSessionInvalid):
		return errors.Join(scriptapp.ErrSessionInvalid, err)
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, mediaapp.ErrForbidden):
		return errors.Join(scriptapp.ErrForbidden, err)
	case errors.Is(err, workspacesource.ErrScopeNotFound), errors.Is(err, workspacesource.ErrProjectNotFound), errors.Is(err, mediaapp.ErrNotFound):
		return errors.Join(scriptapp.ErrNotFound, err)
	default:
		return err
	}
}

type scriptProjectAccess struct {
	access *pgworkspace.ProjectContentAccessStore
}

// ProjectAccess keeps the original session and project checks in the caller Tx.
func ProjectAccess(access *pgworkspace.ProjectContentAccessStore) scriptapp.ProjectAccess {
	return scriptProjectAccess{access: access}
}

func (a scriptProjectAccess) Authorize(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, write bool) (scriptapp.ProjectFacts, error) {
	if a.access == nil {
		return scriptapp.ProjectFacts{}, scriptapp.ErrUnavailable
	}
	facts, err := a.access.Authorize(ctx, IdentityActor(actor), project, write)
	return scriptapp.ProjectFacts{OwnerScopeID: facts.OwnerScopeID, ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, OwningError(err)
}

func (a scriptProjectAccess) TouchContent(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, revision int64) (int64, error) {
	if a.access == nil {
		return 0, scriptapp.ErrUnavailable
	}
	changed, err := a.access.TouchContent(ctx, IdentityActor(actor), project, revision)
	return changed, OwningError(err)
}
