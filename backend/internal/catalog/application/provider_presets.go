package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrPresetNotFound hides absent or inaccessible preset identities.
	ErrPresetNotFound = errors.New("provider preset not found")
	// ErrPresetConflict signals a stale revision, immutable builtin, or reused request.
	ErrPresetConflict = errors.New("provider preset conflict")
	// ErrPresetUnavailable prevents adoption of archived or emergency-stopped presets.
	ErrPresetUnavailable = errors.New("provider preset unavailable")
)

// IsPlatformAdmin does not infer platform authority from a team or legacy role.
func IsPlatformAdmin(actor identityapp.Principal) bool {
	return actor.ID != uuid.Nil && actor.PlatformRole == "platform_admin"
}

// SavePresetInput creates a custom preset or appends an immutable version.
type SavePresetInput struct {
	ID               uuid.UUID             `json:"-"`
	Slug             string                `json:"slug"`
	ParentID         *uuid.UUID            `json:"parent_id"`
	ExpectedRevision int64                 `json:"expected_revision"`
	RequestID        uuid.UUID             `json:"request_id"`
	PreviewHash      string                `json:"preview_hash"`
	Document         domain.PresetDocument `json:"document"`
}

// PresetStateInput changes lifecycle with optimistic locking.
type PresetStateInput struct {
	ExpectedRevision  int64     `json:"expected_revision"`
	RequestID         uuid.UUID `json:"request_id"`
	Archived          bool      `json:"archived"`
	EmergencyDisabled bool      `json:"emergency_disabled"`
}

// ConnectPresetInput creates a disabled connection from a pinned preset version.
type ConnectPresetInput struct {
	CatalogVersion string    `json:"catalog_version,omitempty"`
	CandidateKeys  []string  `json:"candidate_keys,omitempty"`
	VersionID      uuid.UUID `json:"version_id"`
	Key            string    `json:"key"`
	Name           string    `json:"name"`
	RequestID      uuid.UUID `json:"request_id"`
}

// ConnectionConfig preserves the nonsecret settings used by a connection.
type ConnectionConfig struct {
	ID              uuid.UUID             `json:"id"`
	ProviderID      uuid.UUID             `json:"provider_id"`
	PresetVersionID uuid.UUID             `json:"preset_version_id"`
	Version         int                   `json:"version"`
	Config          domain.ProviderConfig `json:"config"`
}

// PresetConnection is the recoverable receipt for a connection creation.
type PresetConnection struct {
	ProviderID uuid.UUID        `json:"provider_id"`
	Config     ConnectionConfig `json:"config"`
	ModelIDs   []uuid.UUID      `json:"model_ids,omitempty"`
}

// PresetStore checks live platform rights inside every read/write transaction.
type PresetStore interface {
	ListPresets(context.Context, identityapp.Principal) ([]domain.Preset, error)
	SavePreset(context.Context, identityapp.Principal, SavePresetInput) (domain.Preset, error)
	SetPresetState(context.Context, identityapp.Principal, uuid.UUID, PresetStateInput) (domain.Preset, error)
	ConnectPreset(context.Context, identityapp.Principal, uuid.UUID, ConnectPresetInput) (PresetConnection, error)
}

// ProviderPresets owns preset validation and management commands.
type ProviderPresets struct{ store PresetStore }

// NewProviderPresets injects the authorized transactional store.
func NewProviderPresets(store PresetStore) *ProviderPresets { return &ProviderPresets{store: store} }

// List returns portable settings without credential or account data.
func (s *ProviderPresets) List(ctx context.Context, actor identityapp.Principal) ([]domain.Preset, error) {
	if !IsPlatformAdmin(actor) {
		return nil, identityapp.ErrForbidden
	}
	presets, err := s.store.ListPresets(ctx, actor)
	if err != nil {
		return nil, err
	}
	for i := range presets {
		presets[i].ModelCatalog = domain.ModelCatalogForConfig(presets[i].Document.Config)
	}
	return presets, nil
}

// Save verifies the exact preview before committing a custom preset version.
func (s *ProviderPresets) Save(ctx context.Context, actor identityapp.Principal, input SavePresetInput) (domain.Preset, error) {
	if !IsPlatformAdmin(actor) {
		return domain.Preset{}, identityapp.ErrForbidden
	}
	if !validProviderKey(input.Slug) || input.RequestID == uuid.Nil || input.Document.Validate() != nil || input.PreviewHash != input.Document.Digest() || (input.ID == uuid.Nil) != (input.ExpectedRevision == 0) || input.ExpectedRevision < 0 {
		return domain.Preset{}, domain.ErrInvalidPreset
	}
	return s.store.SavePreset(ctx, actor, input)
}

// SetState archives or emergency-stops a preset without mutating its versions.
func (s *ProviderPresets) SetState(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input PresetStateInput) (domain.Preset, error) {
	if !IsPlatformAdmin(actor) {
		return domain.Preset{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 {
		return domain.Preset{}, domain.ErrInvalidPreset
	}
	return s.store.SetPresetState(ctx, actor, id, input)
}

// Connect pins a known preset and leaves the connection disabled until verified.
func (s *ProviderPresets) Connect(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ConnectPresetInput) (PresetConnection, error) {
	if !IsPlatformAdmin(actor) {
		return PresetConnection{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.VersionID == uuid.Nil || input.RequestID == uuid.Nil || (input.Key != "" && !validProviderKey(input.Key)) || (input.Name != "" && strings.TrimSpace(input.Name) == "") || len(input.Name) > 200 {
		return PresetConnection{}, domain.ErrInvalidPreset
	}
	if input.Key == "" {
		input.Key = "connection-" + input.RequestID.String()
	}
	return s.store.ConnectPreset(ctx, actor, id, input)
}
