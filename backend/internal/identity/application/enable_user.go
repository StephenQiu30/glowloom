package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidEnableUser means the target, revision, or request ID is invalid.
var ErrInvalidEnableUser = errors.New("invalid enable-user command")

// EnableUserStore commits the account and both events after rechecking the administrator.
type EnableUserStore interface {
	EnableWithEvents(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, []OutboxEvent) (domain.User, error)
}

// EnableUserInput identifies the account and expected revision.
type EnableUserInput struct {
	TargetID         uuid.UUID
	ExpectedRevision int64
	RequestID        string
}

// EnabledUser is the safe committed result.
type EnabledUser struct {
	ID       uuid.UUID
	Status   domain.Status
	Revision int64
}

// EnableUserCommand restores an account with a fresh session epoch.
type EnableUserCommand struct {
	store EnableUserStore
	now   func() time.Time
}

// NewEnableUserCommand injects the transactional store and clock.
func NewEnableUserCommand(store EnableUserStore, now func() time.Time) *EnableUserCommand {
	return &EnableUserCommand{store: store, now: now}
}

// Execute requests an audited, organization-scoped enable.
func (c *EnableUserCommand) Execute(ctx context.Context, actor Principal, input EnableUserInput) (EnabledUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return EnabledUser{}, ErrInvalidEnableUser
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return EnabledUser{}, ErrForbidden
	}
	if input.TargetID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		input.RequestID == "" || len(input.RequestID) > 128 {
		return EnabledUser{}, ErrInvalidEnableUser
	}
	events, err := enabledUserEvents(actor, input, c.now().UTC())
	if err != nil {
		return EnabledUser{}, fmt.Errorf("build account enable events: %w", err)
	}
	saved, err := c.store.EnableWithEvents(ctx, actor.ID, actor.OrgID, input.TargetID, input.ExpectedRevision, events)
	if err != nil {
		return EnabledUser{}, fmt.Errorf("enable account with events: %w", err)
	}
	if saved.ID != input.TargetID || saved.OrgID != actor.OrgID || saved.Status != domain.StatusActive ||
		saved.Revision != input.ExpectedRevision+1 {
		return EnabledUser{}, errors.New("enabled account result does not match committed command")
	}
	return EnabledUser{ID: saved.ID, Status: saved.Status, Revision: saved.Revision}, nil
}

func enabledUserEvents(actor Principal, input EnableUserInput, now time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": input.TargetID, "revision": input.ExpectedRevision + 1},
		"data":      map[string]any{"change": "enabled"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode enabled account change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.enabled", "object": map[string]any{"type": "user", "id": input.TargetID},
			"before":     map[string]any{"status": domain.StatusDisabled},
			"after":      map[string]any{"status": domain.StatusActive},
			"request_id": input.RequestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode enabled account audit: %w", err)
	}
	key := actor.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
