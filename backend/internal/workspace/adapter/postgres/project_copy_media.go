package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func freezeProjectCopyMedia(ctx context.Context, actor identityapp.Principal, owner application.ProjectCopyMediaOwner, binding mediaapp.ProjectCopyBinding, at time.Time, references []uuid.UUID) (mediaapp.ProjectCopySnapshot, error) {
	if len(references) == 0 {
		return owner.Freeze(ctx, actor, binding, at)
	}
	if len(references) > 4096 {
		return mediaapp.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
	}
	seen := make(map[uuid.UUID]bool, len(references))
	for _, id := range references {
		if id == uuid.Nil || seen[id] {
			return mediaapp.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
		}
		seen[id] = true
	}
	historical, ok := owner.(application.ProjectCopyHistoricalMediaOwner)
	if !ok {
		return mediaapp.ProjectCopySnapshot{}, application.ErrProjectDependencyUnavailable
	}
	return historical.FreezeWithReferences(ctx, actor, binding, at, references)
}
