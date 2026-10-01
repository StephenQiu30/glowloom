package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// WorkflowControlTopic transports durable user cancellation and resume intents.
const WorkflowControlTopic = "lanverse.workflow.control_requested.v1"

// ErrInvalidWorkflowControlEvent rejects unbound identities and arbitrary signals.
var ErrInvalidWorkflowControlEvent = errors.New("invalid workflow control event")

// ControlDelivery identifies the persisted control receipt checked before signaling.
type ControlDelivery = application.WorkflowControlDelivery

// ControlDeliveryStore verifies a persisted request and the target's current state.
type ControlDeliveryStore interface {
	CanDeliverWorkflowControl(context.Context, ControlDelivery) (bool, error)
}

// ControlSignaler sends only a fixed control to the target's stable workflow ID.
type ControlSignaler interface {
	SignalControl(context.Context, string, uuid.UUID, string) error
}

// WorkflowControlHandler acknowledges the event only after Temporal accepts it.
type WorkflowControlHandler struct {
	processed ProcessedStore
	store     ControlDeliveryStore
	signaler  ControlSignaler
}

// NewWorkflowControlHandler injects durable deduplication, request checks, and Temporal.
func NewWorkflowControlHandler(processed ProcessedStore, store ControlDeliveryStore, signaler ControlSignaler) *WorkflowControlHandler {
	return &WorkflowControlHandler{processed: processed, store: store, signaler: signaler}
}

// Handle retries failed delivery without changing task state or releasing reservations.
func (h *WorkflowControlHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.signaler == nil {
		return ErrInvalidWorkflowControlEvent
	}
	event, err := parseControl(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "workflow-control", event.EventID.String(), func(ctx context.Context) error {
		eligible, err := h.store.CanDeliverWorkflowControl(ctx, event)
		if err != nil {
			return fmt.Errorf("verify workflow control: %w", err)
		}
		if !eligible {
			return nil
		}
		return h.signaler.SignalControl(ctx, event.TargetType, event.TargetID, event.Action)
	})
	if err != nil {
		return fmt.Errorf("deliver workflow control: %w", err)
	}
	return nil
}

func parseControl(record inbox.Record) (ControlDelivery, error) {
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
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data struct {
			Action    string    `json:"action"`
			RequestID uuid.UUID `json:"request_id"`
		} `json:"data"`
	}
	if len(record.Value) == 0 || len(record.Value) > 8192 {
		return ControlDelivery{}, ErrInvalidWorkflowControlEvent
	}
	decoder := json.NewDecoder(bytes.NewReader(record.Value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return ControlDelivery{}, ErrInvalidWorkflowControlEvent
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ControlDelivery{}, ErrInvalidWorkflowControlEvent
	}
	if record.Topic != WorkflowControlTopic || body.EventType != record.Topic || string(record.Key) != body.ProjectID.String() || body.OccurredAt.IsZero() || body.Actor.Kind != "user" {
		return ControlDelivery{}, ErrInvalidWorkflowControlEvent
	}
	for _, id := range []uuid.UUID{body.EventID, body.OrgID, body.ProjectID, body.Actor.ID, body.Aggregate.ID, body.Data.RequestID} {
		if id == uuid.Nil {
			return ControlDelivery{}, ErrInvalidWorkflowControlEvent
		}
	}
	validControl := body.Aggregate.Type == "operation" && body.Data.Action == "cancel" || body.Aggregate.Type == "batch" && (body.Data.Action == "cancel" || body.Data.Action == "resume")
	if !validControl {
		return ControlDelivery{}, ErrInvalidWorkflowControlEvent
	}
	return ControlDelivery{EventID: body.EventID, ActorID: body.Actor.ID, OrgID: body.OrgID, ProjectID: body.ProjectID, RequestID: body.Data.RequestID, TargetID: body.Aggregate.ID, TargetType: body.Aggregate.Type, Action: body.Data.Action}, nil
}

// WorkflowEventHandler routes confirmation and control topics in one consumer.
type WorkflowEventHandler struct {
	starter *WorkflowStarterHandler
	control *WorkflowControlHandler
}

// NewWorkflowEventHandler binds the two reviewed event families to the relay consumer.
func NewWorkflowEventHandler(starter *WorkflowStarterHandler, control *WorkflowControlHandler) *WorkflowEventHandler {
	return &WorkflowEventHandler{starter: starter, control: control}
}

// Handle dispatches only known workflow event topics.
func (h *WorkflowEventHandler) Handle(ctx context.Context, record inbox.Record) error {
	if record.Topic == WorkflowControlTopic {
		return h.control.Handle(ctx, record)
	}
	if record.Topic == OperationConfirmedTopic || record.Topic == BatchConfirmedTopic {
		return h.starter.Handle(ctx, record)
	}
	return ErrInvalidWorkflowControlEvent
}

var _ inbox.Handler = (*WorkflowControlHandler)(nil)
var _ inbox.Handler = (*WorkflowEventHandler)(nil)
