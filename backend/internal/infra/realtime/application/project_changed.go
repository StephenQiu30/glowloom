package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// ProjectChangedTopic carries project invalidations from the workspace Outbox.
const ProjectChangedTopic = "lanverse.workspace.project_changed.v1"

// ProjectChangedHandler projects a workspace event into a safe SSE invalidation.
type ProjectChangedHandler struct {
	processed ProcessedStore
	sink      Sink
}

// NewProjectChangedHandler injects durable deduplication and the realtime sink.
func NewProjectChangedHandler(processed ProcessedStore, sink Sink) *ProjectChangedHandler {
	return &ProjectChangedHandler{processed: processed, sink: sink}
}

// Handle validates routing and writes the projection before acknowledging Kafka.
func (h *ProjectChangedHandler) Handle(ctx context.Context, record inbox.Record) error {
	event, err := projectChanged(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "realtime", event.ID, func(ctx context.Context) error {
		return h.sink.Publish(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish project realtime event %s: %w", event.ID, err)
	}
	return nil
}

func projectChanged(record inbox.Record) (Event, error) {
	if record.Topic != ProjectChangedTopic || len(record.Value) == 0 || len(record.Value) > 32*1024 {
		return Event{}, fmt.Errorf("%w: topic or payload size", ErrInvalidEvent)
	}
	var body struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Actor      struct {
			Kind string    `json:"kind"`
			ID   uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision *int64    `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return Event{}, fmt.Errorf("%w: decode project change: %w", ErrInvalidEvent, err)
	}
	if body.EventID == uuid.Nil || body.OrgID == uuid.Nil || body.ProjectID == uuid.Nil ||
		body.Actor.Kind != "user" || body.Actor.ID == uuid.Nil || body.OccurredAt.IsZero() ||
		body.EventType != record.Topic || string(record.Key) != body.ProjectID.String() ||
		body.Aggregate.Type != "project" || body.Aggregate.ID != body.ProjectID ||
		body.Aggregate.Revision == nil || *body.Aggregate.Revision < 1 ||
		!validProjectChange(body.Data.Change) {
		return Event{}, fmt.Errorf("%w: invalid project change envelope", ErrInvalidEvent)
	}
	return Event{
		ID: body.EventID.String(), ProjectID: body.ProjectID.String(), Type: "project.updated",
		Revision: *body.Aggregate.Revision, Change: body.Data.Change,
	}, nil
}

func validProjectChange(change string) bool {
	switch change {
	case "created", "updated", "archived", "unarchived", "deleted", "restored":
		return true
	default:
		return false
	}
}

var _ inbox.Handler = (*ProjectChangedHandler)(nil)
