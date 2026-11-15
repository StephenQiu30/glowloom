package bible_test

import (
	"context"

	"github.com/google/uuid"

	scriptapp "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type bibleDocumentSources struct{ sources *mediaapp.DocumentSources }

func (s bibleDocumentSources) Freeze(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, ids []uuid.UUID) ([]mediaapp.DocumentSource, error) {
	files, err := s.sources.Freeze(ctx, mediawire.Actor(scriptwire.IdentityActor(actor)), project, ids)
	return files, scriptwire.OwningError(err)
}

func (s bibleDocumentSources) Open(ctx context.Context, actor scriptapp.Actor, project uuid.UUID, file mediaapp.DocumentSource) (*mediaapp.Downloaded, error) {
	opened, err := s.sources.Open(ctx, mediawire.Actor(scriptwire.IdentityActor(actor)), project, file)
	return opened, scriptwire.OwningError(err)
}
