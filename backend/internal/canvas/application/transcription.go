package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// TranscriptionSourceStore resolves a saved media node under the canvas lock.
type TranscriptionSourceStore interface {
	FreezeTranscriptionSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error)
}

// TranscriptionSourceReader exposes only an authorized, saved audio or video asset.
type TranscriptionSourceReader struct{ store TranscriptionSourceStore }

// NewTranscriptionSourceReader injects the canvas owner of the source document.
func NewTranscriptionSourceReader(store TranscriptionSourceStore) *TranscriptionSourceReader {
	return &TranscriptionSourceReader{store: store}
}

// FreezeTranscriptionSource requires the exact saved revision without editing it.
func (r *TranscriptionSourceReader) FreezeTranscriptionSource(ctx context.Context, actor identityapp.Principal, project, canvas, node uuid.UUID, revision int64) (uuid.UUID, error) {
	if r == nil || r.store == nil || project == uuid.Nil || canvas == uuid.Nil || node == uuid.Nil || revision < 1 {
		return uuid.Nil, domain.ErrInvalidCommand
	}
	return r.store.FreezeTranscriptionSource(ctx, actor, project, canvas, node, revision)
}
