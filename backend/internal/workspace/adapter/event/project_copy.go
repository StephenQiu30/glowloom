// Package event verifies committed workspace copy events before workflow delivery.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyTopic carries committed copy commands without private manifests.
const ProjectCopyTopic = "lanverse.workspace.project_copy_requested.v1"

// ProjectCopyProcessedStore gives the existing inbox durable external delivery ownership.
type ProjectCopyProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// ProjectCopyDeliveryStore proves exact outbox content before Temporal delivery.
type ProjectCopyDeliveryStore interface {
	VerifyDelivery(context.Context, application.ProjectCopyDelivery) (bool, error)
}

// ProjectCopyStarter starts or wakes the single committed copy job.
type ProjectCopyStarter interface {
	Deliver(context.Context, application.ProjectCopyDelivery) error
}

// ProjectCopyHandler adapts only durable authorized copy outbox events.
type ProjectCopyHandler struct {
	processed ProjectCopyProcessedStore
	store     ProjectCopyDeliveryStore
	starter   ProjectCopyStarter
}

// NewProjectCopyHandler injects durable event proof and the actual workflow starter.
func NewProjectCopyHandler(processed ProjectCopyProcessedStore, store ProjectCopyDeliveryStore, starter ProjectCopyStarter) *ProjectCopyHandler {
	return &ProjectCopyHandler{processed: processed, store: store, starter: starter}
}

// Handle rejects injected fields and acknowledges only successful verified delivery.
func (h *ProjectCopyHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil || record.Topic != ProjectCopyTopic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return domain.ErrInvalidProjectCopy
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
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data application.ProjectCopyDelivery `json:"data"`
	}
	d := json.NewDecoder(bytes.NewReader(record.Value))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF || body.EventID == uuid.Nil || body.OrgID == uuid.Nil || body.EventType != ProjectCopyTopic || body.OccurredAt.IsZero() || body.ProjectID != body.Data.SourceProjectID || body.Actor.Kind != "user" || body.Actor.ID == uuid.Nil || body.Aggregate.Type != "project_copy" || body.Aggregate.ID != body.Data.JobID || body.Aggregate.Revision != body.Data.Revision || string(record.Key) != body.Data.JobID.String() || body.Data.TargetProjectID == uuid.Nil || (body.Data.Action != "requested" && body.Data.Action != "retry" && body.Data.Action != "cancel" && body.Data.Action != "reconcile") {
		return domain.ErrInvalidProjectCopy
	}
	body.Data.EventID, body.Data.OrgID = body.EventID, body.OrgID
	body.Data.ActorID, body.Data.OccurredAt = body.Actor.ID, body.OccurredAt
	verified, err := h.store.VerifyDelivery(ctx, body.Data)
	if err != nil {
		return err
	}
	if !verified {
		return nil
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "project-copy-command", body.EventID.String(), func(ctx context.Context) error {
		valid, err := h.store.VerifyDelivery(ctx, body.Data)
		if err != nil {
			return err
		}
		if !valid {
			return nil
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}
