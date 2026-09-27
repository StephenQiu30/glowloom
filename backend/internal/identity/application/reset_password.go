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

// ErrInvalidResetPassword means the target, revision, or request ID is invalid.
var ErrInvalidResetPassword = errors.New("invalid reset-password command")

// ResetPasswordStore reads the scoped account and commits its new credential
// and both events after rechecking the administrator in PostgreSQL.
type ResetPasswordStore interface {
	FindByID(context.Context, uuid.UUID, uuid.UUID) (domain.User, error)
	ResetPasswordWithEvents(context.Context, uuid.UUID, uuid.UUID, domain.User, string, []OutboxEvent) (domain.User, error)
}

// ResetPasswordInput contains the administrator's requested replacement.
type ResetPasswordInput struct {
	TargetID         uuid.UUID
	ExpectedRevision int64
	NewPassword      string
	RequestID        string
}

// ResetPasswordResult exposes the new revision and first-login requirement.
type ResetPasswordResult struct {
	ID                 uuid.UUID
	Revision           int64
	MustChangePassword bool
}

// ResetPasswordCommand invalidates all target sessions through a new epoch.
type ResetPasswordCommand struct {
	store ResetPasswordStore
	now   func() time.Time
}

// NewResetPasswordCommand injects the transactional store and clock.
func NewResetPasswordCommand(store ResetPasswordStore, now func() time.Time) *ResetPasswordCommand {
	return &ResetPasswordCommand{store: store, now: now}
}

// Execute validates a new secret and commits only its hash and safe events.
func (c *ResetPasswordCommand) Execute(ctx context.Context, actor Principal, input ResetPasswordInput) (ResetPasswordResult, error) {
	if c == nil || c.store == nil || c.now == nil {
		return ResetPasswordResult{}, ErrInvalidResetPassword
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return ResetPasswordResult{}, ErrForbidden
	}
	if input.TargetID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		input.RequestID == "" || len(input.RequestID) > 128 || input.NewPassword == "" {
		return ResetPasswordResult{}, ErrInvalidResetPassword
	}
	current, err := c.store.FindByID(ctx, actor.OrgID, input.TargetID)
	if err != nil {
		return ResetPasswordResult{}, fmt.Errorf("read account for password reset: %w", err)
	}
	if current.ID != input.TargetID || current.OrgID != actor.OrgID {
		return ResetPasswordResult{}, domain.ErrUserNotFound
	}
	if current.Revision != input.ExpectedRevision {
		return ResetPasswordResult{}, domain.ErrRevisionConflict
	}
	previousHash, previousEpoch, previousFlag := current.PasswordHash, current.SessionEpoch, current.MustChangePassword
	if err := current.ResetPassword(input.NewPassword); err != nil {
		return ResetPasswordResult{}, err
	}
	events, err := resetPasswordEvents(actor, input, previousFlag, c.now().UTC())
	if err != nil {
		return ResetPasswordResult{}, fmt.Errorf("build password reset events: %w", err)
	}
	saved, err := c.store.ResetPasswordWithEvents(ctx, actor.ID, actor.OrgID, current, previousHash, events)
	if err != nil {
		return ResetPasswordResult{}, fmt.Errorf("reset account password with events: %w", err)
	}
	if saved.ID != input.TargetID || saved.OrgID != actor.OrgID || saved.Revision != current.Revision ||
		saved.SessionEpoch != previousEpoch+1 || !saved.MustChangePassword {
		return ResetPasswordResult{}, errors.New("reset account result does not match committed command")
	}
	return ResetPasswordResult{ID: saved.ID, Revision: saved.Revision, MustChangePassword: true}, nil
}

func resetPasswordEvents(actor Principal, input ResetPasswordInput, previousFlag bool, now time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "user", "id": input.TargetID, "revision": input.ExpectedRevision + 1},
		"data":      map[string]any{"change": "password_reset"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode password reset change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": now, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.password_reset", "object": map[string]any{"type": "user", "id": input.TargetID},
			"before":     map[string]any{"must_change_password": previousFlag},
			"after":      map[string]any{"must_change_password": true},
			"request_id": input.RequestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode password reset audit: %w", err)
	}
	key := actor.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
