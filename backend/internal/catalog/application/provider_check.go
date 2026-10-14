package application

import (
	"context"
	"encoding/json"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
)

// ModelCandidate is a discovered model, not proof of account access or execution.
type ModelCandidate struct {
	ProviderModelID string          `json:"provider_model_id"`
	DisplayName     string          `json:"display_name"`
	Capability      string          `json:"capability"`
	Metadata        json.RawMessage `json:"metadata" swaggertype:"object"`
}

// ProviderCheck separates authentication, protocol and discovery evidence.
type ProviderCheck struct {
	ID              uuid.UUID        `json:"id"`
	ProviderID      uuid.UUID        `json:"provider_id"`
	ConfigVersionID uuid.UUID        `json:"config_version_id"`
	CredentialID    uuid.UUID        `json:"credential_id"`
	Kind            string           `json:"kind"`
	Status          string           `json:"status"`
	AuthResult      string           `json:"auth_result"`
	AdapterResult   string           `json:"adapter_result"`
	DiscoveryResult string           `json:"discovery_result"`
	ErrorCode       *string          `json:"error_code"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	Candidates      []ModelCandidate `json:"candidates" gorm:"-"`
}

// ProviderCheckReader returns authorized nonsecret check progress.
type ProviderCheckReader interface {
	ReadProviderCheck(context.Context, identityapp.Principal, uuid.UUID) (ProviderCheck, error)
}

// ProviderChecks reads the durable check receipt.
type ProviderChecks struct{ store ProviderCheckReader }

// NewProviderChecks injects the authorized check reader.
func NewProviderChecks(store ProviderCheckReader) *ProviderChecks {
	return &ProviderChecks{store: store}
}

// Read always checks platform authorization before querying check results.
func (s *ProviderChecks) Read(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ProviderCheck, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderCheck{}, identityapp.ErrForbidden
	}
	return s.store.ReadProviderCheck(ctx, actor, id)
}
