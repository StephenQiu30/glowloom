package workspace_test

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// This fixture preserves Workspace's original caller contract at the explicit
// Media boundary, using the same transaction and unchanged owning guard.
type mediaWorkBoundary struct {
	owner interface {
		HasInflightWork(context.Context, media.Actor, uuid.UUID) (bool, error)
	}
}

func (g mediaWorkBoundary) HasInflightWork(ctx context.Context, actor identity.Principal, project uuid.UUID) (bool, error) {
	return g.owner.HasInflightWork(ctx, mediaBoundaryActor(actor), project)
}

func mediaBoundaryActor(actor identity.Principal) media.Actor {
	return media.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}
