package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ErrInvalidUnlockUser means the target, revision, or request ID is invalid.
var ErrInvalidUnlockUser = errors.New("invalid unlock-user command")

// UnlockUserStore commits the account and both events after rechecking the administrator.
type UnlockUserStore interface {
	UnlockWithEvents(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, time.Time, []OutboxEvent) (domain.User, error)
}

// UnlockUserInput identifies the account and expected revision.
type UnlockUserInput struct {
	TargetID         uuid.UUID
	ExpectedRevision int64
	RequestID        string
}

// UnlockedUser is the safe committed result.
type UnlockedUser struct {
	ID       uuid.UUID
	Status   domain.Status
	Revision int64
}

// UnlockUserCommand clears login failure locks without changing credentials or account availability.
type UnlockUserCommand struct {
	store UnlockUserStore
	now   func() time.Time
}

// NewUnlockUserCommand injects the transactional store and clock.
func NewUnlockUserCommand(store UnlockUserStore, now func() time.Time) *UnlockUserCommand {
	return &UnlockUserCommand{store: store, now: now}
}

// Execute requests an audited, organization-scoped unlock.
func (c *UnlockUserCommand) Execute(ctx context.Context, actor Principal, input UnlockUserInput) (UnlockedUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return UnlockedUser{}, ErrInvalidUnlockUser
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword || actor.ID == input.TargetID {
		return UnlockedUser{}, ErrForbidden
	}
	if input.TargetID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		input.RequestID == "" || len(input.RequestID) > 128 {
		return UnlockedUser{}, ErrInvalidUnlockUser
	}
	now := c.now().UTC()
	events, err := unlockedUserEvents(actor, input, now)
	if err != nil {
		return UnlockedUser{}, fmt.Errorf("build account unlock events: %w", err)
	}
	saved, err := c.store.UnlockWithEvents(ctx, actor.ID, actor.OrgID, input.TargetID, input.ExpectedRevision, now, events)
	if err != nil {
		return UnlockedUser{}, fmt.Errorf("unlock account with events: %w", err)
	}
	if saved.ID != input.TargetID || saved.OrgID != actor.OrgID || (saved.Status != domain.StatusActive && saved.Status != domain.StatusDisabled) || !saved.LockedUntil.IsZero() || saved.FailedLoginCount != 0 ||
		saved.Revision != input.ExpectedRevision+1 {
		return UnlockedUser{}, errors.New("unlocked account result does not match committed command")
	}
	return UnlockedUser{ID: saved.ID, Status: saved.Status, Revision: saved.Revision}, nil
}

func unlockedUserEvents(actor Principal, input UnlockUserInput, now time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": input.TargetID, "revision": input.ExpectedRevision + 1},
		"data":      map[string]any{"change": "unlocked"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode unlocked account change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.unlocked", "object": map[string]any{"type": "user", "id": input.TargetID},
			"before":     map[string]any{"login_locked": true},
			"after":      map[string]any{"login_locked": false},
			"request_id": input.RequestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode unlocked account audit: %w", err)
	}
	key := actor.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
