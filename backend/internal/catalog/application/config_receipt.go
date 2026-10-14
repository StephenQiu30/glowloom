package application

import (
	"context"
	"encoding/json"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
)

// ConfigReceipt contains only a safe committed result for a scoped request.
type ConfigReceipt struct {
	Action    string          `json:"action"`
	TargetID  uuid.UUID       `json:"target_id"`
	RequestID uuid.UUID       `json:"request_id"`
	Result    json.RawMessage `json:"result" swaggertype:"object"`
}

// ConfigReceiptStore never returns a credential fingerprint or encrypted payload.
type ConfigReceiptStore interface {
	ReadConfigReceipt(context.Context, identityapp.Principal, string, uuid.UUID, uuid.UUID) (ConfigReceipt, error)
}

// ConfigReceipts recovers requests after a lost HTTP response.
type ConfigReceipts struct{ store ConfigReceiptStore }

// NewConfigReceipts injects the authorized receipt store.
func NewConfigReceipts(store ConfigReceiptStore) *ConfigReceipts {
	return &ConfigReceipts{store: store}
}

// Read binds recovery to the authenticated actor, action and target.
func (s *ConfigReceipts) Read(ctx context.Context, actor identityapp.Principal, action string, target, id uuid.UUID) (ConfigReceipt, error) {
	if !IsPlatformAdmin(actor) {
		return ConfigReceipt{}, identityapp.ErrForbidden
	}
	if len(action) > 64 || id == uuid.Nil {
		return ConfigReceipt{}, ErrInvalidProviderDetail
	}
	return s.store.ReadConfigReceipt(ctx, actor, action, target, id)
}
