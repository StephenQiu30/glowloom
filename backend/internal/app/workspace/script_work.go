package workspace

import (
	"context"

	"github.com/google/uuid"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type scriptWorkGuard struct{ store *pgscript.SourceStore }

func (g scriptWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	return g.store.HasInflightWork(ctx, scriptwire.Actor(actor), project)
}
