package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CustomProviderInput creates an empty disabled connection without credentials.
type CustomProviderInput struct {
	RequestID   uuid.UUID             `json:"request_id"`
	Name        string                `json:"name"`
	Config      domain.ProviderConfig `json:"config"`
	LogoAssetID *uuid.UUID            `json:"logo_asset_id,omitempty"`
}

// ProviderLogoInput changes display metadata with optimistic locking.
type ProviderLogoInput struct {
	RequestID        uuid.UUID  `json:"request_id"`
	ExpectedRevision int64      `json:"expected_revision"`
	LogoAssetID      *uuid.UUID `json:"logo_asset_id" extensions:"x-nullable"`
}

// CustomProviderStore rechecks live administrator authority inside each transaction.
type CustomProviderStore interface {
	CreateCustomProvider(context.Context, identityapp.Principal, CustomProviderInput) (PresetConnection, error)
	SetProviderLogo(context.Context, identityapp.Principal, uuid.UUID, ProviderLogoInput) (CreatedProvider, error)
}

// CustomProviders validates custom connection and logo mutations.
type CustomProviders struct{ store CustomProviderStore }

// NewCustomProviders injects the transactional catalog store.
func NewCustomProviders(store CustomProviderStore) *CustomProviders {
	return &CustomProviders{store: store}
}

func validDisplayName(name string) bool {
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 100 && strings.IndexFunc(name, unicode.IsControl) < 0
}

// Create creates a disabled, empty connection without credential handling.
func (s *CustomProviders) Create(ctx context.Context, actor identityapp.Principal, input CustomProviderInput) (PresetConnection, error) {
	if !IsPlatformAdmin(actor) {
		return PresetConnection{}, identityapp.ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.RequestID == uuid.Nil || !validDisplayName(input.Name) || input.Config.Validate() != nil || (input.LogoAssetID != nil && *input.LogoAssetID == uuid.Nil) {
		return PresetConnection{}, domain.ErrInvalidPreset
	}
	return s.store.CreateCustomProvider(ctx, actor, input)
}

// SetLogo changes only the dedicated provider logo binding.
func (s *CustomProviders) SetLogo(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ProviderLogoInput) (CreatedProvider, error) {
	if !IsPlatformAdmin(actor) {
		return CreatedProvider{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || (input.LogoAssetID != nil && *input.LogoAssetID == uuid.Nil) {
		return CreatedProvider{}, domain.ErrInvalidPreset
	}
	return s.store.SetProviderLogo(ctx, actor, id, input)
}
