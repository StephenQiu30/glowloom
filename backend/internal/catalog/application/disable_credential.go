package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidDisableCredential means a disable request or result is incomplete.
var ErrInvalidDisableCredential = errors.New("invalid disable-credential command")

// CredentialToDisable exposes only the metadata needed for safe events.
type CredentialToDisable struct {
	ID         uuid.UUID
	ProviderID uuid.UUID
	Last4      string
}

// DisableCredentialStore checks current access and commits status plus events.
type DisableCredentialStore interface {
	FindCredentialForAdmin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (CredentialToDisable, error)
	DisableCredentialWithEvents(context.Context, uuid.UUID, uuid.UUID, CredentialToDisable, []identityapp.OutboxEvent) (SavedCredential, error)
}

// DisableCredentialInput identifies the exact active credential to disable.
type DisableCredentialInput struct {
	ProviderID   uuid.UUID
	CredentialID uuid.UUID
	RequestID    string
}

// DisableCredentialCommand persists a safe, audited credential revocation.
type DisableCredentialCommand struct {
	store DisableCredentialStore
	now   func() time.Time
}

// NewDisableCredentialCommand injects the transactional store and clock.
func NewDisableCredentialCommand(store DisableCredentialStore, now func() time.Time) *DisableCredentialCommand {
	return &DisableCredentialCommand{store: store, now: now}
}

// Execute rejects stale credentials and never returns their key or ciphertext.
func (c *DisableCredentialCommand) Execute(ctx context.Context, actor identityapp.Principal, input DisableCredentialInput) (SavedCredential, error) {
	if c == nil || c.store == nil || c.now == nil {
		return SavedCredential{}, ErrInvalidDisableCredential
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return SavedCredential{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if input.ProviderID == uuid.Nil || input.CredentialID == uuid.Nil || err != nil || requestID.String() != input.RequestID {
		return SavedCredential{}, ErrInvalidDisableCredential
	}
	target, err := c.store.FindCredentialForAdmin(ctx, actor.ID, actor.OrgID, input.ProviderID, input.CredentialID)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("find credential to disable: %w", err)
	}
	if target.ID != input.CredentialID || target.ProviderID != input.ProviderID ||
		!utf8.ValidString(target.Last4) || utf8.RuneCountInString(target.Last4) != 4 {
		return SavedCredential{}, ErrInvalidDisableCredential
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return SavedCredential{}, ErrInvalidDisableCredential
	}
	events, err := credentialDisabledEvents(actor, target, input.RequestID, occurredAt)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("build credential disable events: %w", err)
	}
	saved, err := c.store.DisableCredentialWithEvents(ctx, actor.ID, actor.OrgID, target, events)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("disable credential with events: %w", err)
	}
	if saved.ID != target.ID || saved.ProviderID != target.ProviderID || saved.Last4 != target.Last4 ||
		saved.Status != domain.CredentialDisabled || saved.Label == "" ||
		saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return SavedCredential{}, ErrInvalidDisableCredential
	}
	return saved, nil
}

func credentialDisabledEvents(actor identityapp.Principal, target CredentialToDisable, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changed, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": credentialChangedTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "provider_credential", "id": target.ID},
		"data":      map[string]any{"change": "disabled", "provider_id": target.ProviderID},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential disable change: %w", err)
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action":     "credential.disabled",
			"object":     map[string]any{"type": "provider_credential", "id": target.ID},
			"after":      map[string]any{"last4": target.Last4},
			"request_id": requestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential disable audit: %w", err)
	}
	key := actor.OrgID.String()
	return []identityapp.OutboxEvent{
		{ID: changedID, Topic: credentialChangedTopic, PartitionKey: key, Payload: changed},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
