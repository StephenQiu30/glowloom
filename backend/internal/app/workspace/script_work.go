package workspace

import (
	"context"

	"github.com/google/uuid"

	pgscript "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
)

type scriptWorkGuard struct{ store *pgscript.SourceStore }

func (g scriptWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	return g.store.HasInflightWork(ctx, scriptwire.Actor(actor), project)
}
