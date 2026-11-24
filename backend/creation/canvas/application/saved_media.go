package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// SavedMediaSourceStore resolves a saved media node under the canvas lock.
type SavedMediaSourceStore interface {
	FreezeMediaSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error)
}

// SavedMediaSourceReader exposes only an authorized, saved audio or video asset.
type SavedMediaSourceReader struct{ store SavedMediaSourceStore }

// NewSavedMediaSourceReader injects the canvas owner of the source document.
func NewSavedMediaSourceReader(store SavedMediaSourceStore) *SavedMediaSourceReader {
	return &SavedMediaSourceReader{store: store}
}

// FreezeMediaSource requires the exact saved revision without editing it.
func (r *SavedMediaSourceReader) FreezeMediaSource(ctx context.Context, actor identityapp.Principal, project, canvas, node uuid.UUID, revision int64) (uuid.UUID, error) {
	if r == nil || r.store == nil || project == uuid.Nil || canvas == uuid.Nil || node == uuid.Nil || revision < 1 {
		return uuid.Nil, domain.ErrInvalidCommand
	}
	return r.store.FreezeMediaSource(ctx, actor, project, canvas, node, revision)
}
