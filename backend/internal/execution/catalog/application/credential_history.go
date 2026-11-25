package application

import (
	"context"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CredentialHistoryStore exposes display metadata only, including draining keys.
type CredentialHistoryStore interface {
	ListCredentialHistory(context.Context, identityapp.Principal, uuid.UUID) ([]ProviderCredentialSummary, error)
}

// CredentialHistory provides the platform's key rotation history.
type CredentialHistory struct{ store CredentialHistoryStore }

// NewCredentialHistory injects the authorized metadata reader.
func NewCredentialHistory(store CredentialHistoryStore) *CredentialHistory {
	return &CredentialHistory{store: store}
}

// List never reads encrypted or plaintext secrets.
func (s *CredentialHistory) List(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]ProviderCredentialSummary, error) {
	if !IsPlatformAdmin(actor) {
		return nil, identityapp.ErrForbidden
	}
	if id == uuid.Nil {
		return nil, ErrInvalidProviderDetail
	}
	return s.store.ListCredentialHistory(ctx, actor, id)
}
