package script_test

import (
	"context"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type scriptDocumentSources struct{ sources *mediaapp.DocumentSources }

func (s scriptDocumentSources) Freeze(ctx context.Context, actor app.Actor, project uuid.UUID, ids []uuid.UUID) ([]mediaapp.DocumentSource, error) {
	files, err := s.sources.Freeze(ctx, scriptMediaActor(scriptIdentityActor(actor)), project, ids)
	return files, scriptwire.OwningError(err)
}

func (s scriptDocumentSources) Open(ctx context.Context, actor app.Actor, project uuid.UUID, file mediaapp.DocumentSource) (*mediaapp.Downloaded, error) {
	opened, err := s.sources.Open(ctx, scriptMediaActor(scriptIdentityActor(actor)), project, file)
	return opened, scriptwire.OwningError(err)
}
