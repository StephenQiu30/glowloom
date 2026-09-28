package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// BillingSettledTopic carries committed operation settlement facts.
const BillingSettledTopic = "lanverse.billing.settled.v1"

// BillingSettledHandler projects the post-settlement available balance to SSE.
type BillingSettledHandler struct {
	processed ProcessedStore
	sink      Sink
}

// NewBillingSettledHandler injects durable deduplication and the realtime sink.
func NewBillingSettledHandler(processed ProcessedStore, sink Sink) *BillingSettledHandler {
	return &BillingSettledHandler{processed: processed, sink: sink}
}

// Handle commits the consumer marker only after the Redis effect succeeds.
func (h *BillingSettledHandler) Handle(ctx context.Context, record inbox.Record) error {
	event, err := projectBillingSettled(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "realtime", event.ID, func(ctx context.Context) error {
		return h.sink.Publish(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish settlement realtime event %s: %w", event.ID, err)
	}
	return nil
}

func projectBillingSettled(record inbox.Record) (Event, error) {
	if record.Topic != BillingSettledTopic || len(record.Value) == 0 || len(record.Value) > 32*1024 {
		return Event{}, fmt.Errorf("%w: topic or payload size", ErrInvalidEvent)
	}
	var body struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Actor      struct {
			Kind string     `json:"kind"`
			ID   *uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data struct {
			OperationID     uuid.UUID       `json:"operation_id"`
			AvailableMicros json.RawMessage `json:"available_micros"`
			BudgetOverrun   *bool           `json:"budget_overrun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return Event{}, fmt.Errorf("%w: decode billing settlement: %w", ErrInvalidEvent, err)
	}
	if body.EventID == uuid.Nil || body.OrgID == uuid.Nil || body.ProjectID == uuid.Nil ||
		body.OccurredAt.IsZero() || body.Actor.Kind != "system" || body.Actor.ID != nil ||
		body.EventType != record.Topic || string(record.Key) != body.ProjectID.String() ||
		body.Aggregate.Type != "operation" || body.Aggregate.ID == uuid.Nil ||
		body.Data.OperationID != body.Aggregate.ID || body.Data.BudgetOverrun == nil {
		return Event{}, fmt.Errorf("%w: invalid billing settlement envelope", ErrInvalidEvent)
	}
	if len(body.Data.AvailableMicros) == 0 {
		// Events written before the balance field existed still invalidate the project.
		return Event{ID: body.EventID.String(), ProjectID: body.ProjectID.String(), Type: "resync"}, nil
	}
	if bytes.Equal(bytes.TrimSpace(body.Data.AvailableMicros), []byte("null")) {
		return Event{}, fmt.Errorf("%w: null settlement balance", ErrInvalidEvent)
	}
	var availableMicros int64
	if err := json.Unmarshal(body.Data.AvailableMicros, &availableMicros); err != nil {
		return Event{}, fmt.Errorf("%w: decode settlement balance: %w", ErrInvalidEvent, err)
	}
	if (availableMicros < 0) != *body.Data.BudgetOverrun {
		return Event{}, fmt.Errorf("%w: inconsistent settlement balance", ErrInvalidEvent)
	}
	return Event{
		ID: body.EventID.String(), ProjectID: body.ProjectID.String(), Type: "budget.updated",
		AvailableMicros: availableMicros,
	}, nil
}

var _ inbox.Handler = (*BillingSettledHandler)(nil)
