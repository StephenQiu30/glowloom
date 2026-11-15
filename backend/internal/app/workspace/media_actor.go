package workspace

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// Legacy composition retains Workspace's caller contract and Media's own Actor.
type workspaceMediaQuery struct{ query *media.AssetQuery }

func (q workspaceMediaQuery) Reference(ctx context.Context, actor identity.Principal, project, id uuid.UUID) (media.AssetSummary, error) {
	return q.query.Reference(ctx, mediawire.Actor(actor), project, id)
}

type mediaProjectWorkReader interface {
	HasInflightWork(context.Context, media.Actor, uuid.UUID) (bool, error)
}
type mediaProjectWorkGuard struct{ owner mediaProjectWorkReader }

func (g mediaProjectWorkGuard) HasInflightWork(ctx context.Context, actor identity.Principal, project uuid.UUID) (bool, error) {
	return g.owner.HasInflightWork(ctx, mediawire.Actor(actor), project)
}
