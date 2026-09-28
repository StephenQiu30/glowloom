// Package event consumes durable operation events after quote confirmation.
package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

const (
	// OperationConfirmedTopic starts one confirmed operation outside a batch.
	OperationConfirmedTopic = "lanverse.operation.confirmed.v1"
	// BatchConfirmedTopic starts the parent workflow for confirmed batch members.
	BatchConfirmedTopic     = "lanverse.batch.confirmed.v1"
	workflowStarterConsumer = "workflow-starter"
)

// ErrInvalidConfirmedEvent means a Kafka record cannot identify one confirmation.
var ErrInvalidConfirmedEvent = errors.New("invalid operation confirmation event")

// ProcessedStore records an event only after Temporal accepts its start.
type ProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// ConfirmedStore verifies the committed operation and lists stale confirmations.
type ConfirmedStore interface {
	ConfirmedSingle(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	StaleConfirmedSingles(context.Context, time.Time, int, int) ([]uuid.UUID, error)
	ConfirmedBatch(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	StaleConfirmedBatches(context.Context, time.Time, int, int) ([]uuid.UUID, error)
}

// OperationStarter starts a workflow with a stable operation ID.
type OperationStarter interface {
	StartOperation(context.Context, uuid.UUID) error
	StartBatch(context.Context, uuid.UUID) error
}

// WorkflowStarterHandler handles Kafka delivery and the periodic missed-event sweep.
type WorkflowStarterHandler struct {
	processed ProcessedStore
	store     ConfirmedStore
	starter   OperationStarter
}

// NewWorkflowStarterHandler injects durable deduplication, operation reads, and Temporal.
func NewWorkflowStarterHandler(processed ProcessedStore, store ConfirmedStore, starter OperationStarter) *WorkflowStarterHandler {
	return &WorkflowStarterHandler{processed: processed, store: store, starter: starter}
}

// Handle validates the event, checks the database, then starts before acknowledging.
func (h *WorkflowStarterHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil {
		return ErrInvalidConfirmedEvent
	}
	if record.Topic == BatchConfirmedTopic {
		return h.handleBatch(ctx, record)
	}
	event, err := parseConfirmedOperation(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, workflowStarterConsumer, event.id.String(), func(ctx context.Context) error {
		eligible, err := h.store.ConfirmedSingle(ctx, event.orgID, event.projectID, event.operationID, event.reservationID)
		if err != nil {
			return fmt.Errorf("check confirmed operation %s: %w", event.operationID, err)
		}
		if !eligible {
			return nil
		}
		return h.starter.StartOperation(ctx, event.operationID)
	})
	if err != nil {
		return fmt.Errorf("start confirmed operation %s: %w", event.operationID, err)
	}
	return nil
}

func (h *WorkflowStarterHandler) handleBatch(ctx context.Context, record inbox.Record) error {
	event, err := parseConfirmedBatch(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, workflowStarterConsumer, event.id.String(), func(ctx context.Context) error {
		eligible, err := h.store.ConfirmedBatch(ctx, event.orgID, event.projectID, event.batchID)
		if err != nil {
			return fmt.Errorf("check confirmed batch %s: %w", event.batchID, err)
		}
		if !eligible {
			return nil
		}
		return h.starter.StartBatch(ctx, event.batchID)
	})
	if err != nil {
		return fmt.Errorf("start confirmed batch %s: %w", event.batchID, err)
	}
	return nil
}

// SweepOnce retries old confirmed singles when event delivery failed.
func (h *WorkflowStarterHandler) SweepOnce(ctx context.Context, now time.Time) error {
	if h == nil || h.store == nil || h.starter == nil || now.IsZero() {
		return ErrInvalidConfirmedEvent
	}
	var operations []uuid.UUID
	for offset := 0; ; offset += 100 {
		ids, err := h.store.StaleConfirmedSingles(ctx, now.UTC().Add(-2*time.Minute), 100, offset)
		if err != nil {
			return fmt.Errorf("list stale confirmed operations: %w", err)
		}
		operations = append(operations, ids...)
		if len(ids) < 100 {
			break
		}
	}
	for _, id := range operations {
		if err := h.starter.StartOperation(ctx, id); err != nil {
			return fmt.Errorf("start stale confirmed operation %s: %w", id, err)
		}
	}
	return nil
}

// SweepBatchesOnce retries old parents. The relay enables this after
// BatchWorkflow is registered in the flow worker.
func (h *WorkflowStarterHandler) SweepBatchesOnce(ctx context.Context, now time.Time) error {
	if h == nil || h.store == nil || h.starter == nil || now.IsZero() {
		return ErrInvalidConfirmedEvent
	}
	var batches []uuid.UUID
	for offset := 0; ; offset += 100 {
		ids, err := h.store.StaleConfirmedBatches(ctx, now.UTC().Add(-2*time.Minute), 100, offset)
		if err != nil {
			return fmt.Errorf("list stale confirmed batches: %w", err)
		}
		batches = append(batches, ids...)
		if len(ids) < 100 {
			break
		}
	}
	for _, id := range batches {
		if err := h.starter.StartBatch(ctx, id); err != nil {
			return fmt.Errorf("start stale confirmed batch %s: %w", id, err)
		}
	}
	return nil
}

type confirmedOperationEvent struct {
	id            uuid.UUID
	orgID         uuid.UUID
	projectID     uuid.UUID
	operationID   uuid.UUID
	reservationID uuid.UUID
}

type confirmedBatchEvent struct {
	id        uuid.UUID
	orgID     uuid.UUID
	projectID uuid.UUID
	batchID   uuid.UUID
}

type confirmedEnvelope struct {
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
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"aggregate"`
	Data struct {
		OperationID      uuid.UUID `json:"operation_id"`
		ReservationID    uuid.UUID `json:"reservation_id"`
		BatchID          uuid.UUID `json:"batch_id"`
		TotalCount       *int32    `json:"total_count"`
		QuoteTotalMicros *int64    `json:"quote_total_micros"`
		QuoteMicros      *int64    `json:"quote_micros"`
	} `json:"data"`
}

func parseConfirmedEnvelope(record inbox.Record) (confirmedEnvelope, error) {
	if len(record.Value) == 0 || len(record.Value) > 32*1024 {
		return confirmedEnvelope{}, ErrInvalidConfirmedEvent
	}
	var body confirmedEnvelope
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return confirmedEnvelope{}, fmt.Errorf("%w: %w", ErrInvalidConfirmedEvent, err)
	}
	if body.EventID == uuid.Nil || body.ProjectID == uuid.Nil || body.OrgID == uuid.Nil ||
		body.OccurredAt.IsZero() || body.Actor.Kind != "user" || body.Actor.ID == uuid.Nil ||
		body.EventType != record.Topic || string(record.Key) != body.ProjectID.String() ||
		body.Aggregate.ID == uuid.Nil {
		return confirmedEnvelope{}, ErrInvalidConfirmedEvent
	}
	return body, nil
}

func parseConfirmedOperation(record inbox.Record) (confirmedOperationEvent, error) {
	if record.Topic != OperationConfirmedTopic {
		return confirmedOperationEvent{}, ErrInvalidConfirmedEvent
	}
	body, err := parseConfirmedEnvelope(record)
	if err != nil {
		return confirmedOperationEvent{}, err
	}
	if body.Aggregate.Type != "operation" ||
		body.Aggregate.ID != body.Data.OperationID || body.Data.ReservationID == uuid.Nil ||
		body.Data.QuoteMicros == nil || *body.Data.QuoteMicros < 0 {
		return confirmedOperationEvent{}, ErrInvalidConfirmedEvent
	}
	return confirmedOperationEvent{
		id: body.EventID, orgID: body.OrgID, projectID: body.ProjectID,
		operationID: body.Data.OperationID, reservationID: body.Data.ReservationID,
	}, nil
}

func parseConfirmedBatch(record inbox.Record) (confirmedBatchEvent, error) {
	if record.Topic != BatchConfirmedTopic {
		return confirmedBatchEvent{}, ErrInvalidConfirmedEvent
	}
	body, err := parseConfirmedEnvelope(record)
	if err != nil {
		return confirmedBatchEvent{}, err
	}
	if body.Aggregate.Type != "batch" || body.Aggregate.ID != body.Data.BatchID ||
		body.Data.TotalCount == nil || *body.Data.TotalCount < 1 || *body.Data.TotalCount > 300 ||
		body.Data.QuoteTotalMicros == nil || *body.Data.QuoteTotalMicros < 0 {
		return confirmedBatchEvent{}, ErrInvalidConfirmedEvent
	}
	return confirmedBatchEvent{id: body.EventID, orgID: body.OrgID, projectID: body.ProjectID,
		batchID: body.Data.BatchID}, nil
}

var _ inbox.Handler = (*WorkflowStarterHandler)(nil)
