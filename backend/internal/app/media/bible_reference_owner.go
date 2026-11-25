package media

import (
	"context"

	"github.com/google/uuid"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/internal/creation/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/bible/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// These legacy adapters retain the caller's existing request or worker policy
// transaction. Native Media must use owner proofs rather than this local bridge.
type bibleReferenceAccess struct {
	access *pgworkspace.ProjectContentAccessStore
}

func bibleLegacyActor(actor bibleapp.Actor) identityapp.Principal {
	return identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

func (a bibleReferenceAccess) Authorize(ctx context.Context, actor bibleapp.Actor, project uuid.UUID, write bool) (bibleapp.ProjectFacts, error) {
	facts, err := a.access.Authorize(ctx, bibleLegacyActor(actor), project, write)
	return bibleapp.ProjectFacts{OwnerScopeID: facts.OwnerScopeID, ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, err
}

func (a bibleReferenceAccess) TouchContent(ctx context.Context, actor bibleapp.Actor, project uuid.UUID, revision int64) (int64, error) {
	return a.access.TouchContent(ctx, bibleLegacyActor(actor), project, revision)
}

type bibleReferenceOwner struct{ owner *pgbible.MediaReferenceGuard }

func (b bibleReferenceOwner) HasMediaReferences(ctx context.Context, actor mediaapp.Actor, project, asset uuid.UUID) (bool, error) {
	return b.owner.HasMediaReferences(ctx, bibleapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}, project, asset)
}
