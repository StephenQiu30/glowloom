package bible

import (
	"context"
	"errors"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	bibleapp "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"

	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Actor translates the legacy authenticated reference at its composition boundary.
// It copies no platform role or grant and does not authorize any operation.
func Actor(actor identityapp.Principal) bibleapp.Actor {
	return bibleapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

// IdentityActor supplies only the reference needed by existing transaction-bound
// policy implementations while their native owner contracts replace this bridge.
func IdentityActor(actor bibleapp.Actor) identityapp.Principal {
	return identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

// OwningError preserves the original error while exposing Bible's local meaning.
func OwningError(err error) error {
	switch {
	case errors.Is(err, identityapp.ErrSessionInvalid):
		return errors.Join(bibleapp.ErrSessionInvalid, err)
	case errors.Is(err, identityapp.ErrForbidden):
		return errors.Join(bibleapp.ErrForbidden, err)
	case errors.Is(err, workspacesource.ErrProjectNotFound), errors.Is(err, workspacesource.ErrScopeNotFound):
		return errors.Join(bibleapp.ErrNotFound, err)
	case errors.Is(err, workspacedomain.ErrProjectRevisionConflict), errors.Is(err, workspacedomain.ErrProjectStateConflict):
		return errors.Join(bibleapp.ErrConflict, err)
	default:
		return err
	}
}

// ProjectAccess adapts the actual legacy request or worker policy bound to its
// original transaction. Native Bible roots inject their own access implementation.
func ProjectAccess(access *pgworkspace.ProjectContentAccessStore) bibleapp.ProjectAccess {
	return bibleProjectAccess{access: access}
}

type contentAccess interface {
	Authorize(context.Context, identityapp.Principal, uuid.UUID, bool) (workspaceapp.ProjectContentAccess, error)
	TouchContent(context.Context, identityapp.Principal, uuid.UUID, int64) (int64, error)
}

type bibleProjectAccess struct{ access contentAccess }

func (a bibleProjectAccess) Authorize(ctx context.Context, actor bibleapp.Actor, project uuid.UUID, write bool) (bibleapp.ProjectFacts, error) {
	facts, err := a.access.Authorize(ctx, IdentityActor(actor), project, write)
	return bibleapp.ProjectFacts{OwnerScopeID: facts.OwnerScopeID, ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, OwningError(err)
}

func (a bibleProjectAccess) TouchContent(ctx context.Context, actor bibleapp.Actor, project uuid.UUID, revision int64) (int64, error) {
	result, err := a.access.TouchContent(ctx, IdentityActor(actor), project, revision)
	return result, OwningError(err)
}
