package application

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidProviderDetail means a requested or stored summary is invalid.
var ErrInvalidProviderDetail = errors.New("invalid provider detail")

// ErrUnsupportedCredentialSchema means no input contract exists for the adapter.
var ErrUnsupportedCredentialSchema = errors.New("unsupported provider credential schema")

// CredentialField is a declared input field, never a saved secret value.
type CredentialField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// ProviderCredentialSummary contains only metadata safe for an administrator.
type ProviderCredentialSummary struct {
	ID             uuid.UUID               `json:"id"`
	ProviderID     uuid.UUID               `json:"provider_id"`
	Label          string                  `json:"label"`
	Last4          string                  `json:"last4"`
	Status         domain.CredentialStatus `json:"status"`
	LastTestedAt   *time.Time              `json:"last_tested_at"`
	LastTestResult *domain.TestResult      `json:"last_test_result"`
	CreateTime     time.Time               `json:"create_time"`
	UpdateTime     time.Time               `json:"update_time"`
}

// ProviderDetail combines safe provider settings and current credential metadata.
type ProviderDetail struct {
	Provider         CreatedProvider            `json:"provider"`
	Credential       *ProviderCredentialSummary `json:"credential"`
	CredentialSchema []CredentialField          `json:"credential_schema"`
}

// ProviderDetailStore reads one consistent, currently authorized snapshot.
type ProviderDetailStore interface {
	FindProviderDetailForAdmin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (domain.Provider, *ProviderCredentialSummary, error)
}

// CredentialFieldCatalog describes input fields for a supported adapter.
type CredentialFieldCatalog interface {
	Fields(string) ([]CredentialField, bool)
}

// ProviderDetailQuery obtains the administrator's safe display data.
type ProviderDetailQuery struct {
	store  ProviderDetailStore
	fields CredentialFieldCatalog
}

// NewProviderDetailQuery injects the authorized store and adapter contracts.
func NewProviderDetailQuery(store ProviderDetailStore, fields CredentialFieldCatalog) *ProviderDetailQuery {
	return &ProviderDetailQuery{store: store, fields: fields}
}

// Execute never loads a saved credential's ciphertext or key identifier.
func (q *ProviderDetailQuery) Execute(ctx context.Context, actor identityapp.Principal, providerID uuid.UUID) (ProviderDetail, error) {
	if q == nil || q.store == nil || q.fields == nil || providerID == uuid.Nil {
		return ProviderDetail{}, ErrInvalidProviderDetail
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return ProviderDetail{}, identityapp.ErrForbidden
	}
	provider, credential, err := q.store.FindProviderDetailForAdmin(ctx, actor.ID, actor.OrgID, providerID)
	if err != nil {
		return ProviderDetail{}, fmt.Errorf("read provider detail: %w", err)
	}
	if provider.ID != providerID || provider.Validate() != nil ||
		provider.CreateTime.IsZero() || provider.UpdateTime.IsZero() {
		return ProviderDetail{}, ErrInvalidProviderDetail
	}
	fields, supported := q.fields.Fields(provider.AdapterKey)
	if !supported || len(fields) == 0 {
		return ProviderDetail{}, ErrUnsupportedCredentialSchema
	}
	if credential != nil && !validProviderCredentialSummary(*credential, providerID) {
		return ProviderDetail{}, ErrInvalidProviderDetail
	}
	return ProviderDetail{
		Provider: CreatedProvider{
			ID: provider.ID, Key: provider.Key, Name: provider.Name,
			AdapterKey: provider.AdapterKey, Region: provider.Region, Status: provider.Status,
			ConcurrencyLimit: provider.ConcurrencyLimit, RateLimitPerMin: provider.RateLimitPerMin,
			Revision: provider.Revision, CreateTime: provider.CreateTime, UpdateTime: provider.UpdateTime,
		},
		Credential: credential, CredentialSchema: fields,
	}, nil
}

func validProviderCredentialSummary(credential ProviderCredentialSummary, providerID uuid.UUID) bool {
	if credential.ID == uuid.Nil || credential.ProviderID != providerID || credential.Label == "" ||
		!utf8.ValidString(credential.Last4) || utf8.RuneCountInString(credential.Last4) != 4 ||
		credential.Status != domain.CredentialActive || credential.CreateTime.IsZero() ||
		credential.UpdateTime.IsZero() || (credential.LastTestedAt == nil) != (credential.LastTestResult == nil) {
		return false
	}
	if credential.LastTestResult == nil {
		return true
	}
	if credential.LastTestedAt.IsZero() {
		return false
	}
	switch *credential.LastTestResult {
	case domain.TestOK, domain.TestAuthFailed, domain.TestUnreachable, domain.TestTimeout, domain.TestUnsupported:
		return true
	default:
		return false
	}
}
