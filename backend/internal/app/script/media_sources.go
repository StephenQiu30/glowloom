package script

import (
	"context"

	"github.com/google/uuid"

	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type documentSources struct{ sources *mediaapp.DocumentSources }

func mediaActor(actor scriptapp.Actor) mediaapp.Actor {
	return mediaapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

func (d documentSources) Freeze(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, ids []uuid.UUID) ([]mediaapp.DocumentSource, error) {
	if d.sources == nil {
		return nil, scriptapp.ErrUnavailable
	}
	files, err := d.sources.Freeze(ctx, mediaActor(actor), project, ids)
	return files, OwningError(err)
}

func (d documentSources) Open(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, file mediaapp.DocumentSource) (*mediaapp.Downloaded, error) {
	if d.sources == nil {
		return nil, scriptapp.ErrUnavailable
	}
	opened, err := d.sources.Open(ctx, mediaActor(actor), project, file)
	return opened, OwningError(err)
}
