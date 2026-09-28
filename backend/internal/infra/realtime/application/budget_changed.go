package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// BudgetChangedTopic carries available-balance changes from the billing Outbox.
const BudgetChangedTopic = "lanverse.billing.budget_changed.v1"

// BudgetChangedHandler projects one committed budget change to project SSE.
type BudgetChangedHandler struct {
	processed ProcessedStore
	sink      Sink
}

// NewBudgetChangedHandler injects durable deduplication and the realtime sink.
func NewBudgetChangedHandler(processed ProcessedStore, sink Sink) *BudgetChangedHandler {
	return &BudgetChangedHandler{processed: processed, sink: sink}
}

// Handle publishes before marking the event processed so failed effects retry.
func (h *BudgetChangedHandler) Handle(ctx context.Context, record inbox.Record) error {
	event, err := projectBudgetChanged(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "realtime", event.ID, func(ctx context.Context) error {
		return h.sink.Publish(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish budget realtime event %s: %w", event.ID, err)
	}
	return nil
}

func projectBudgetChanged(record inbox.Record) (Event, error) {
	if record.Topic != BudgetChangedTopic || len(record.Value) == 0 || len(record.Value) > 32*1024 {
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
			LimitMicros     *int64 `json:"limit_micros"`
			AvailableMicros *int64 `json:"available_micros"`
			IsOverrun       *bool  `json:"is_overrun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return Event{}, fmt.Errorf("%w: decode budget change: %w", ErrInvalidEvent, err)
	}
	if body.EventID == uuid.Nil || body.OrgID == uuid.Nil || body.ProjectID == uuid.Nil ||
		body.OccurredAt.IsZero() || body.Actor.Kind != "user" || body.Actor.ID == uuid.Nil ||
		body.EventType != record.Topic || string(record.Key) != body.ProjectID.String() ||
		body.Aggregate.Type != "budget" || body.Aggregate.ID == uuid.Nil ||
		body.Aggregate.Revision == nil || *body.Aggregate.Revision < 1 ||
		body.Data.LimitMicros == nil || *body.Data.LimitMicros < 0 ||
		body.Data.AvailableMicros == nil || body.Data.IsOverrun == nil ||
		(*body.Data.AvailableMicros < 0) != *body.Data.IsOverrun ||
		*body.Data.AvailableMicros > *body.Data.LimitMicros {
		return Event{}, fmt.Errorf("%w: invalid budget change envelope", ErrInvalidEvent)
	}
	return Event{
		ID: body.EventID.String(), ProjectID: body.ProjectID.String(), Type: "budget.updated",
		AvailableMicros: *body.Data.AvailableMicros,
	}, nil
}

var _ inbox.Handler = (*BudgetChangedHandler)(nil)
