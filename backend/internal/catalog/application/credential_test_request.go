package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ErrCredentialTestKeyReused rejects reuse of a test request for another credential.
var ErrCredentialTestKeyReused = errors.New("credential test request key reused")

// CredentialTestAccepted reports scheduling, never successful provider credentials.
type CredentialTestAccepted struct {
	TestID       uuid.UUID `json:"test_id"`
	EventID      uuid.UUID `json:"event_id"`
	ProviderID   uuid.UUID `json:"provider_id"`
	CredentialID uuid.UUID `json:"credential_id"`
	Accepted     bool      `json:"accepted"`
}

// CredentialTestRequestStore records the authorized request, receipt, and Outbox.
type CredentialTestRequestStore interface {
	RequestCredentialTest(context.Context, identityapp.Principal, CredentialTestRequest) (CredentialTestAccepted, error)
}

// RequestCredentialTestCommand schedules the existing credential test workflow.
type RequestCredentialTestCommand struct{ store CredentialTestRequestStore }

// NewRequestCredentialTestCommand injects the transaction that authorizes scheduling.
func NewRequestCredentialTestCommand(store CredentialTestRequestStore) *RequestCredentialTestCommand {
	return &RequestCredentialTestCommand{store: store}
}

// Execute accepts only administrators and derives a stable test identity.
func (c *RequestCredentialTestCommand) Execute(ctx context.Context, actor identityapp.Principal, providerID, credentialID uuid.UUID, key string) (CredentialTestAccepted, error) {
	if err := requireAdmin(actor); err != nil {
		return CredentialTestAccepted{}, err
	}
	requestID, err := uuid.Parse(key)
	if c == nil || c.store == nil || err != nil || requestID == uuid.Nil || requestID.String() != key || providerID == uuid.Nil || credentialID == uuid.Nil {
		return CredentialTestAccepted{}, ErrInvalidCredentialTest
	}
	request := CredentialTestRequest{TestID: uuid.NewSHA1(requestID, []byte(actor.ID.String()+actor.OrgID.String()+providerID.String()+credentialID.String())), ProviderID: providerID, CredentialID: credentialID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: key}
	result, err := c.store.RequestCredentialTest(ctx, actor, request)
	if err != nil {
		return CredentialTestAccepted{}, fmt.Errorf("request credential test: %w", err)
	}
	return result, nil
}
