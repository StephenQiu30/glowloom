package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidSetModelStatus means the requested status change is incomplete.
var ErrInvalidSetModelStatus = errors.New("invalid set-model-status command")

// SetModelStatusStore commits the state transition and audit atomically.
type SetModelStatusStore interface {
	SetModelStatusWithAudit(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, domain.ModelStatus, int64, identityapp.OutboxEvent) (domain.ModelProfile, error)
}

// SetModelStatusInput identifies the requested administrator transition.
type SetModelStatusInput struct {
	ModelID          uuid.UUID
	ExpectedRevision int64
	Status           domain.ModelStatus
	RequestID        string
}

// ChangedModelStatus is the safe result of an accepted transition.
type ChangedModelStatus struct {
	ID         uuid.UUID          `json:"id"`
	Status     domain.ModelStatus `json:"status"`
	Revision   int64              `json:"revision"`
	UpdateTime time.Time          `json:"update_time"`
}

// SetModelStatusCommand changes availability after generating a safe audit.
type SetModelStatusCommand struct {
	store SetModelStatusStore
	now   func() time.Time
}

// NewSetModelStatusCommand injects persistence and the audit clock.
func NewSetModelStatusCommand(store SetModelStatusStore, now func() time.Time) *SetModelStatusCommand {
	return &SetModelStatusCommand{store: store, now: now}
}

// Execute validates the request and requests one audited state transition.
func (c *SetModelStatusCommand) Execute(ctx context.Context, actor identityapp.Principal, input SetModelStatusInput) (ChangedModelStatus, error) {
	if c == nil || c.store == nil || c.now == nil {
		return ChangedModelStatus{}, ErrInvalidSetModelStatus
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return ChangedModelStatus{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		input.ModelID == uuid.Nil || input.ExpectedRevision < 1 ||
		input.ExpectedRevision >= math.MaxInt32 ||
		(input.Status != domain.ModelActive && input.Status != domain.ModelDisabled) {
		return ChangedModelStatus{}, ErrInvalidSetModelStatus
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return ChangedModelStatus{}, ErrInvalidSetModelStatus
	}
	event, err := modelStatusAudit(actor, input, occurredAt)
	if err != nil {
		return ChangedModelStatus{}, fmt.Errorf("build model status audit: %w", err)
	}
	saved, err := c.store.SetModelStatusWithAudit(ctx, actor.ID, actor.OrgID, input.ModelID, input.Status, input.ExpectedRevision, event)
	if err != nil {
		return ChangedModelStatus{}, fmt.Errorf("set model status with audit: %w", err)
	}
	if saved.Validate() != nil || saved.ID != input.ModelID ||
		saved.Status != input.Status || saved.Revision != input.ExpectedRevision+1 ||
		saved.UpdateTime.IsZero() {
		return ChangedModelStatus{}, ErrInvalidSetModelStatus
	}
	return ChangedModelStatus{
		ID: saved.ID, Status: saved.Status, Revision: saved.Revision,
		UpdateTime: saved.UpdateTime,
	}, nil
}

func modelStatusAudit(actor identityapp.Principal, input SetModelStatusInput, occurredAt time.Time) (identityapp.OutboxEvent, error) {
	previous := domain.ModelActive
	action := "model.disabled"
	if input.Status == domain.ModelActive {
		previous = domain.ModelDisabled
		action = "model.enabled"
	}
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action":     action,
			"object":     map[string]any{"type": "model_profile", "id": input.ModelID},
			"request_id": input.RequestID,
			"before":     map[string]any{"status": previous, "revision": input.ExpectedRevision},
			"after":      map[string]any{"status": input.Status, "revision": input.ExpectedRevision + 1},
		},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, fmt.Errorf("encode model status audit: %w", err)
	}
	return identityapp.OutboxEvent{
		ID: eventID, Topic: auditTopic, PartitionKey: actor.OrgID.String(), Payload: payload,
	}, nil
}
