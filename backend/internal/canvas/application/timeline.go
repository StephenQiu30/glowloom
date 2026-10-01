package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// TimelineStore rechecks ownership and freezes references under the document lock.
type TimelineStore interface {
	FreezeTimeline(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (domain.TimelineConfig, error)
}

// TimelineReader exposes immutable timeline inputs to local media processing.
type TimelineReader struct{ store TimelineStore }

// NewTimelineReader injects the canvas owner of the source document.
func NewTimelineReader(store TimelineStore) *TimelineReader { return &TimelineReader{store: store} }

// FreezeTimeline requires an exact saved revision and never changes the canvas.
func (r *TimelineReader) FreezeTimeline(ctx context.Context, actor identityapp.Principal, project, canvas, node uuid.UUID, revision int64) (domain.TimelineConfig, error) {
	if project == uuid.Nil || canvas == uuid.Nil || node == uuid.Nil || revision < 1 {
		return domain.TimelineConfig{}, domain.ErrInvalidCommand
	}
	return r.store.FreezeTimeline(ctx, actor, project, canvas, node, revision)
}
