// Package application coordinates authorized provider catalog commands.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const (
	credentialChangedTopic = "lanverse.catalog.credential_changed.v1"
	auditTopic             = "lanverse.audit.recorded.v1"
)

// ErrInvalidSetCredential means the command input or committed result is invalid.
var ErrInvalidSetCredential = errors.New("invalid set-credential command")

// SetCredentialStore reads a provider and commits the credential plus events.
type SetCredentialStore interface {
	FindProviderForAdmin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (domain.Provider, error)
	ReplaceWithEvents(context.Context, uuid.UUID, uuid.UUID, domain.Credential, []identityapp.OutboxEvent) (domain.Credential, error)
}

// SecretValidator enforces the selected provider adapter's declared fields.
type SecretValidator interface {
	Validate(string, json.RawMessage) (string, error)
}

// CredentialSealer cannot decrypt a stored credential.
type CredentialSealer interface {
	KeyID() string
	Seal(uuid.UUID, uuid.UUID, json.RawMessage) ([]byte, error)
}

// SetCredentialInput contains the one-time plaintext request and safe metadata.
type SetCredentialInput struct {
	ProviderID uuid.UUID
	Label      string
	Secret     json.RawMessage
	RequestID  string
}

// SavedCredential is the only result exposed beyond the command boundary.
type SavedCredential struct {
	ID             uuid.UUID               `json:"id"`
	ProviderID     uuid.UUID               `json:"provider_id"`
	Label          string                  `json:"label"`
	Last4          string                  `json:"last4"`
	Status         domain.CredentialStatus `json:"status"`
	LastTestResult *domain.TestResult      `json:"last_test_result"`
	CreateTime     time.Time               `json:"create_time"`
	UpdateTime     time.Time               `json:"update_time"`
}

// SetCredentialCommand validates, seals, and atomically records a replacement.
type SetCredentialCommand struct {
	store     SetCredentialStore
	validator SecretValidator
	sealer    CredentialSealer
	now       func() time.Time
}

// NewSetCredentialCommand injects the store, adapter contracts, public key, and clock.
func NewSetCredentialCommand(store SetCredentialStore, validator SecretValidator, sealer CredentialSealer, now func() time.Time) *SetCredentialCommand {
	return &SetCredentialCommand{store: store, validator: validator, sealer: sealer, now: now}
}

// Execute never sends the plaintext to persistence, logs, or Outbox.
func (c *SetCredentialCommand) Execute(ctx context.Context, actor identityapp.Principal, input SetCredentialInput) (SavedCredential, error) {
	if c == nil || c.store == nil || c.validator == nil || c.sealer == nil || c.now == nil {
		return SavedCredential{}, ErrInvalidSetCredential
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return SavedCredential{}, identityapp.ErrForbidden
	}
	label := strings.TrimSpace(input.Label)
	requestID, requestIDErr := uuid.Parse(input.RequestID)
	if input.ProviderID == uuid.Nil || label == "" || utf8.RuneCountInString(label) > 100 ||
		requestIDErr != nil || requestID.String() != input.RequestID {
		return SavedCredential{}, ErrInvalidSetCredential
	}
	provider, err := c.store.FindProviderForAdmin(ctx, actor.ID, actor.OrgID, input.ProviderID)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("read credential provider: %w", err)
	}
	if provider.ID != input.ProviderID {
		return SavedCredential{}, ErrInvalidSetCredential
	}
	if provider.Status != domain.ProviderActive {
		return SavedCredential{}, domain.ErrProviderDisabled
	}
	last4, err := c.validator.Validate(provider.AdapterKey, input.Secret)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("validate provider credential fields: %w", err)
	}
	credential := domain.Credential{
		ID: uuid.New(), ProviderID: provider.ID, Label: label,
		KeyID: c.sealer.KeyID(), Last4: last4, Status: domain.CredentialActive,
	}
	credential.Ciphertext, err = c.sealer.Seal(provider.ID, credential.ID, input.Secret)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("seal provider credential: %w", err)
	}
	if err := credential.Validate(); err != nil {
		return SavedCredential{}, err
	}
	events, err := credentialSetEvents(actor, credential, input.RequestID, c.now().UTC())
	if err != nil {
		return SavedCredential{}, fmt.Errorf("build credential events: %w", err)
	}
	saved, err := c.store.ReplaceWithEvents(ctx, actor.ID, actor.OrgID, credential, events)
	if err != nil {
		return SavedCredential{}, fmt.Errorf("replace credential with events: %w", err)
	}
	if saved.ID != credential.ID || saved.ProviderID != credential.ProviderID ||
		saved.Label != credential.Label || saved.Last4 != credential.Last4 ||
		saved.Status != domain.CredentialActive {
		return SavedCredential{}, ErrInvalidSetCredential
	}
	return SavedCredential{
		ID: saved.ID, ProviderID: saved.ProviderID, Label: saved.Label,
		Last4: saved.Last4, Status: saved.Status,
		CreateTime: saved.CreateTime, UpdateTime: saved.UpdateTime,
	}, nil
}

func credentialSetEvents(actor identityapp.Principal, credential domain.Credential, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": credentialChangedTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "provider_credential", "id": credential.ID},
		"data":      map[string]any{"change": "set", "provider_id": credential.ProviderID},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action":     "credential.set",
			"object":     map[string]any{"type": "provider_credential", "id": credential.ID},
			"after":      map[string]any{"last4": credential.Last4},
			"request_id": requestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential audit: %w", err)
	}
	key := actor.OrgID.String()
	return []identityapp.OutboxEvent{
		{ID: changedID, Topic: credentialChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
