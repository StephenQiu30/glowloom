package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const (
	userChangedTopic = "lanverse.identity.user_changed.v1"
	auditTopic       = "lanverse.audit.recorded.v1"
)

var (
	// ErrForbidden means the caller may not administer accounts.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidCreateUser means an account command is missing required fields.
	ErrInvalidCreateUser = errors.New("invalid create-user command")
)

// OutboxEvent is a complete event to persist with an account change.
type OutboxEvent struct {
	ID           uuid.UUID
	Topic        string
	PartitionKey string
	Payload      json.RawMessage
}

// CreateUserStore rechecks the administrator and commits account plus events atomically.
type CreateUserStore interface {
	CreateWithEvents(context.Context, uuid.UUID, domain.User, []OutboxEvent) (domain.User, error)
}

// CreateUserInput contains the fields accepted from an account administrator.
type CreateUserInput struct {
	LoginName       string
	DisplayName     string
	Role            domain.Role
	InitialPassword string
	RequestID       string
}

// CreatedUser is safe to return across the application boundary.
type CreatedUser struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               domain.Role
	Status             domain.Status
	MustChangePassword bool
	Revision           int64
	CreateTime         time.Time
}

// CreateUserCommand creates an account and both required durable events.
type CreateUserCommand struct {
	store CreateUserStore
	now   func() time.Time
}

// NewCreateUserCommand injects the transactional account store and clock.
func NewCreateUserCommand(store CreateUserStore, now func() time.Time) *CreateUserCommand {
	return &CreateUserCommand{store: store, now: now}
}

// Execute hashes the initial password before the transaction and never returns it.
func (c *CreateUserCommand) Execute(ctx context.Context, actor Principal, input CreateUserInput) (CreatedUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return CreatedUser{}, ErrInvalidCreateUser
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return CreatedUser{}, ErrForbidden
	}
	loginName := strings.TrimSpace(input.LoginName)
	displayName := strings.TrimSpace(input.DisplayName)
	if loginName == "" || displayName == "" ||
		(input.Role != domain.RoleAdmin && input.Role != domain.RoleProducer) ||
		input.RequestID == "" || len(input.RequestID) > 128 {
		return CreatedUser{}, ErrInvalidCreateUser
	}
	hash, err := domain.HashPassword(input.InitialPassword, "")
	if err != nil {
		return CreatedUser{}, err
	}
	user := domain.User{
		ID: uuid.New(), OrgID: actor.OrgID, LoginName: loginName,
		DisplayName: displayName, Role: input.Role, Status: domain.StatusActive,
		PasswordHash: hash, MustChangePassword: true, SessionEpoch: 1, Revision: 1,
	}
	events, err := createdUserEvents(actor, user, input.RequestID, c.now().UTC())
	if err != nil {
		return CreatedUser{}, fmt.Errorf("build account events: %w", err)
	}
	saved, err := c.store.CreateWithEvents(ctx, actor.ID, user, events)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("create account with events: %w", err)
	}
	return CreatedUser{
		ID: saved.ID, OrgID: saved.OrgID, LoginName: saved.LoginName,
		DisplayName: saved.DisplayName, Role: saved.Role, Status: saved.Status,
		MustChangePassword: saved.MustChangePassword, Revision: saved.Revision,
		CreateTime: saved.CreateTime,
	}, nil
}

func createdUserEvents(actor Principal, user domain.User, requestID string, occurredAt time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedBody := map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": occurredAt, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": user.ID, "revision": user.Revision},
		"data":      map[string]any{"change": "created"},
	}
	auditBody := map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.created", "object": map[string]any{"type": "user", "id": user.ID},
			"after": map[string]any{
				"role": user.Role, "status": user.Status,
				"must_change_password": user.MustChangePassword,
			},
			"request_id": requestID,
		},
	}
	changedPayload, err := json.Marshal(changedBody)
	if err != nil {
		return nil, fmt.Errorf("encode account change: %w", err)
	}
	auditPayload, err := json.Marshal(auditBody)
	if err != nil {
		return nil, fmt.Errorf("encode account audit: %w", err)
	}
	key := user.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
