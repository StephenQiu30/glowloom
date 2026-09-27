// Package application projects domain events into short realtime invalidation messages.
package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// OperationStatusTopic is the event topic accepted by the realtime projection.
const OperationStatusTopic = "lanverse.operation.status_changed.v1"

// ErrInvalidEvent means the record cannot be safely projected.
var ErrInvalidEvent = errors.New("invalid realtime event")

// Event is a project-scoped SSE event. ID stays equal to the Kafka event ID.
type Event struct {
	ID          string
	ProjectID   string
	Type        string
	OperationID string
	BatchID     string
	TargetType  string
	TargetID    string
	Status      string
	Progress    json.RawMessage
	Revision    int64
	Change      string
}

// ProcessedStore commits a consumer marker after its external effect succeeds.
type ProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// Sink stores and publishes one realtime event.
type Sink interface {
	Publish(context.Context, Event) error
}

// OperationStatusHandler handles operation state changes for the realtime consumer.
type OperationStatusHandler struct {
	processed ProcessedStore
	sink      Sink
}

// NewOperationStatusHandler injects the processed-event store and realtime sink.
func NewOperationStatusHandler(processed ProcessedStore, sink Sink) *OperationStatusHandler {
	return &OperationStatusHandler{processed: processed, sink: sink}
}

// Handle validates the Kafka route and sends one project invalidation event.
func (h *OperationStatusHandler) Handle(ctx context.Context, record inbox.Record) error {
	event, err := projectOperationStatus(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "realtime", event.ID, func(ctx context.Context) error {
		return h.sink.Publish(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish realtime event %s: %w", event.ID, err)
	}
	return nil
}

func projectOperationStatus(record inbox.Record) (Event, error) {
	var body struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
		ProjectID string `json:"project_id"`
		Aggregate struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"aggregate"`
		Data struct {
			BatchID    string          `json:"batch_id"`
			TargetType string          `json:"target_type"`
			TargetID   string          `json:"target_id"`
			Status     string          `json:"status"`
			Progress   json.RawMessage `json:"progress"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return Event{}, fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	if record.Topic != OperationStatusTopic || body.EventType != record.Topic ||
		body.Aggregate.Type != "operation" || string(record.Key) != body.ProjectID ||
		body.Data.TargetType == "" || body.Data.Status == "" {
		return Event{}, fmt.Errorf("%w: routing or required field mismatch", ErrInvalidEvent)
	}
	for _, id := range []string{body.EventID, body.ProjectID, body.Aggregate.ID} {
		if _, err := uuid.Parse(id); err != nil {
			return Event{}, fmt.Errorf("%w: invalid UUID: %w", ErrInvalidEvent, err)
		}
	}
	for _, id := range []string{body.Data.BatchID, body.Data.TargetID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return Event{}, fmt.Errorf("%w: invalid optional UUID: %w", ErrInvalidEvent, err)
			}
		}
	}
	if err := domain.Status(body.Data.Status).CanTransitionTo(domain.Status(body.Data.Status)); err != nil {
		return Event{}, fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	if len(body.Data.Progress) > 0 && bytes.Equal(body.Data.Progress, []byte("null")) {
		body.Data.Progress = nil
	}
	return Event{
		ID: body.EventID, ProjectID: body.ProjectID, Type: "operation.updated",
		OperationID: body.Aggregate.ID, BatchID: body.Data.BatchID,
		TargetType: body.Data.TargetType, TargetID: body.Data.TargetID,
		Status: body.Data.Status, Progress: body.Data.Progress,
	}, nil
}

var _ inbox.Handler = (*OperationStatusHandler)(nil)
