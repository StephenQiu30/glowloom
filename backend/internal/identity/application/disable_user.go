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

// ErrInvalidDisableUser means the target, revision, or request ID is invalid.
var ErrInvalidDisableUser = errors.New("invalid disable-user command")

// DisableUserStore rechecks the administrator and commits account plus events.
type DisableUserStore interface {
	DisableWithEvents(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, []OutboxEvent) (domain.User, error)
}

// DisableUserInput identifies the target account and expected revision.
type DisableUserInput struct {
	TargetID         uuid.UUID
	ExpectedRevision int64
	RequestID        string
}

// DisabledUser contains the safe state returned after the transaction commits.
type DisabledUser struct {
	ID       uuid.UUID
	Status   domain.Status
	Revision int64
}

// DisableUserCommand performs an audited, organization-scoped account disable.
type DisableUserCommand struct {
	store DisableUserStore
	now   func() time.Time
}

// NewDisableUserCommand injects the transactional store and clock.
func NewDisableUserCommand(store DisableUserStore, now func() time.Time) *DisableUserCommand {
	return &DisableUserCommand{store: store, now: now}
}

// Execute rejects stale caller claims in the store before it changes the account.
func (c *DisableUserCommand) Execute(ctx context.Context, actor Principal, input DisableUserInput) (DisabledUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return DisabledUser{}, ErrInvalidDisableUser
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return DisabledUser{}, ErrForbidden
	}
	if input.TargetID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision == math.MaxInt64 ||
		input.RequestID == "" || len(input.RequestID) > 128 {
		return DisabledUser{}, ErrInvalidDisableUser
	}
	events, err := disabledUserEvents(actor, input, c.now().UTC())
	if err != nil {
		return DisabledUser{}, fmt.Errorf("build account disable events: %w", err)
	}
	saved, err := c.store.DisableWithEvents(ctx, actor.ID, actor.OrgID, input.TargetID, input.ExpectedRevision, events)
	if err != nil {
		return DisabledUser{}, fmt.Errorf("disable account with events: %w", err)
	}
	if saved.ID != input.TargetID || saved.OrgID != actor.OrgID || saved.Status != domain.StatusDisabled ||
		saved.Revision != input.ExpectedRevision+1 {
		return DisabledUser{}, errors.New("disabled account result does not match committed command")
	}
	return DisabledUser{ID: saved.ID, Status: saved.Status, Revision: saved.Revision}, nil
}

func disabledUserEvents(actor Principal, input DisableUserInput, now time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": input.TargetID, "revision": input.ExpectedRevision + 1},
		"data":      map[string]any{"change": "disabled"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode disabled account change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.disabled", "object": map[string]any{"type": "user", "id": input.TargetID},
			"before":     map[string]any{"status": domain.StatusActive},
			"after":      map[string]any{"status": domain.StatusDisabled},
			"request_id": input.RequestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode disabled account audit: %w", err)
	}
	key := actor.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
