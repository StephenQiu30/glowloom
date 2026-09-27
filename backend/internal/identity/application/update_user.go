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

// ErrInvalidUpdateUser means the target, patch, revision, or request ID is invalid.
var ErrInvalidUpdateUser = errors.New("invalid update-user command")

// UpdateUserStore reads a target and atomically revalidates the change with both events.
type UpdateUserStore interface {
	FindByID(context.Context, uuid.UUID, uuid.UUID) (domain.User, error)
	UpdateProfileWithEvents(context.Context, uuid.UUID, domain.User, domain.User, []OutboxEvent) (domain.User, error)
}

// UpdateUserInput permits only display name and role changes.
type UpdateUserInput struct {
	TargetID         uuid.UUID
	ExpectedRevision int64
	DisplayName      *string
	Role             *domain.Role
	RequestID        string
}

// UpdatedUser is safe to return across the application boundary.
type UpdatedUser struct {
	ID          uuid.UUID
	DisplayName string
	Role        domain.Role
	Revision    int64
}

// UpdateUserCommand coordinates an audited account profile change.
type UpdateUserCommand struct {
	store UpdateUserStore
	now   func() time.Time
}

// NewUpdateUserCommand injects the account store and clock.
func NewUpdateUserCommand(store UpdateUserStore, now func() time.Time) *UpdateUserCommand {
	return &UpdateUserCommand{store: store, now: now}
}

// Execute validates the proposed change; PostgreSQL rechecks all mutable state.
func (c *UpdateUserCommand) Execute(ctx context.Context, actor Principal, input UpdateUserInput) (UpdatedUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return UpdatedUser{}, ErrInvalidUpdateUser
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return UpdatedUser{}, ErrForbidden
	}
	if input.TargetID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		(input.DisplayName == nil && input.Role == nil) || input.RequestID == "" || len(input.RequestID) > 128 {
		return UpdatedUser{}, ErrInvalidUpdateUser
	}
	before, err := c.store.FindByID(ctx, actor.OrgID, input.TargetID)
	if err != nil {
		return UpdatedUser{}, fmt.Errorf("read account for update: %w", err)
	}
	if before.OrgID != actor.OrgID || before.ID != input.TargetID {
		return UpdatedUser{}, domain.ErrUserNotFound
	}
	if before.Revision != input.ExpectedRevision {
		return UpdatedUser{}, domain.ErrRevisionConflict
	}
	after := before
	// The transaction counts active administrators under an organization lock.
	if err := after.UpdateProfile(input.DisplayName, input.Role, math.MaxInt); err != nil {
		return UpdatedUser{}, err
	}
	events, err := updatedUserEvents(actor, before, after, input.RequestID, c.now().UTC())
	if err != nil {
		return UpdatedUser{}, fmt.Errorf("build account update events: %w", err)
	}
	saved, err := c.store.UpdateProfileWithEvents(ctx, actor.ID, before, after, events)
	if err != nil {
		return UpdatedUser{}, fmt.Errorf("update account with events: %w", err)
	}
	if saved.ID != input.TargetID || saved.OrgID != actor.OrgID || saved.DisplayName != after.DisplayName ||
		saved.Role != after.Role || saved.Revision != after.Revision {
		return UpdatedUser{}, errors.New("updated account result does not match committed command")
	}
	return UpdatedUser{ID: saved.ID, DisplayName: saved.DisplayName, Role: saved.Role, Revision: saved.Revision}, nil
}

func updatedUserEvents(actor Principal, before, after domain.User, requestID string, occurredAt time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": occurredAt, "org_id": before.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": before.ID, "revision": after.Revision},
		"data":      map[string]any{"change": "updated"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode updated account change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": before.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.updated", "object": map[string]any{"type": "user", "id": before.ID},
			"before":     map[string]any{"display_name": before.DisplayName, "role": before.Role},
			"after":      map[string]any{"display_name": after.DisplayName, "role": after.Role},
			"request_id": requestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode updated account audit: %w", err)
	}
	key := before.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
