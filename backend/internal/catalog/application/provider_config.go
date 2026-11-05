package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ConfigUpgradeInput pins the source revision and target preset version.
type ConfigUpgradeInput struct {
	ExpectedRevision int64     `json:"expected_revision"`
	ExpectedConfigID uuid.UUID `json:"expected_config_id"`
	PresetVersionID  uuid.UUID `json:"preset_version_id"`
}

// ConfigUpgradePreview is a three-way merge; conflicts require explicit resolution.
type ConfigUpgradePreview struct {
	Current   ConnectionConfig      `json:"current"`
	Proposed  domain.ProviderConfig `json:"proposed"`
	Conflicts []string              `json:"conflicts"`
	Revision  int64                 `json:"revision"`
	Hash      string                `json:"hash"`
}

// ApplyConfigInput binds an explicit resolution to a freshly checked preview.
type ApplyConfigInput struct {
	Upgrade     ConfigUpgradeInput    `json:"upgrade"`
	RequestID   uuid.UUID             `json:"request_id"`
	PreviewHash string                `json:"preview_hash"`
	Resolution  domain.ProviderConfig `json:"resolution"`
}

// AddPresetModelInput pins the selected model to the current connection snapshot.
type AddPresetModelInput struct {
	ConfigID       uuid.UUID `json:"config_id"`
	CatalogVersion string    `json:"catalog_version"`
	CandidateKey   string    `json:"candidate_key,omitempty"`
	CustomModelID  string    `json:"custom_model_id,omitempty"`
	Capability     string    `json:"capability,omitempty"`
	RequestID      uuid.UUID `json:"request_id"`
}

// ConfigStore persists connection snapshots without changing active releases.
type ConfigStore interface {
	ReadConnectionConfig(context.Context, identityapp.Principal, uuid.UUID) (ConnectionConfig, error)
	CheckModelConfiguration(context.Context, identityapp.Principal, uuid.UUID) (ModelConfigurationCheck, error)
	AddPresetModel(context.Context, identityapp.Principal, uuid.UUID, AddPresetModelInput) (CreatedModel, error)
	PreviewConfigUpgrade(context.Context, identityapp.Principal, uuid.UUID, ConfigUpgradeInput) (ConfigUpgradePreview, error)
	ApplyConfigUpgrade(context.Context, identityapp.Principal, uuid.UUID, ApplyConfigInput) (ConnectionConfig, error)
}

// ProviderConfigs coordinates immutable connection configuration changes.
type ProviderConfigs struct{ store ConfigStore }

// NewProviderConfigs injects the authorized snapshot store.
func NewProviderConfigs(store ConfigStore) *ProviderConfigs { return &ProviderConfigs{store: store} }

// Read returns the current editing snapshot.
func (s *ProviderConfigs) Read(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ConnectionConfig, error) {
	if !IsPlatformAdmin(actor) {
		return ConnectionConfig{}, identityapp.ErrForbidden
	}
	return s.store.ReadConnectionConfig(ctx, actor, id)
}

// AddModel creates a draft from the selected preset without dispatching it.
func (s *ProviderConfigs) AddModel(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input AddPresetModelInput) (CreatedModel, error) {
	if !IsPlatformAdmin(actor) {
		return CreatedModel{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.ConfigID == uuid.Nil || input.RequestID == uuid.Nil || input.CatalogVersion == "" || (input.CandidateKey == "") == (input.CustomModelID == "") {
		return CreatedModel{}, domain.ErrInvalidPreset
	}
	return s.store.AddPresetModel(ctx, actor, id, input)
}

// ModelConfigurationCheck identifies the exact saved snapshot checked locally.
type ModelConfigurationCheck struct {
	ProviderID      uuid.UUID  `json:"provider_id"`
	ConfigVersionID uuid.UUID  `json:"config_version_id"`
	ModelVersionID  uuid.UUID  `json:"model_version_id"`
	CredentialID    *uuid.UUID `json:"credential_id,omitempty"`
	ProviderModelID string     `json:"provider_model_id"`
	ParametersValid bool       `json:"parameters_valid"`
}

func (s *ProviderConfigs) CheckModel(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ModelConfigurationCheck, error) {
	if !IsPlatformAdmin(actor) {
		return ModelConfigurationCheck{}, identityapp.ErrForbidden
	}
	return s.store.CheckModelConfiguration(ctx, actor, id)
}

// Preview recomputes a merge from immutable base, target and current snapshots.
func (s *ProviderConfigs) Preview(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ConfigUpgradeInput) (ConfigUpgradePreview, error) {
	if !IsPlatformAdmin(actor) {
		return ConfigUpgradePreview{}, identityapp.ErrForbidden
	}
	if !validUpgrade(id, input) {
		return ConfigUpgradePreview{}, domain.ErrInvalidPreset
	}
	return s.store.PreviewConfigUpgrade(ctx, actor, id, input)
}

// Apply changes only the candidate configuration, after exact-preview validation.
func (s *ProviderConfigs) Apply(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ApplyConfigInput) (ConnectionConfig, error) {
	if !IsPlatformAdmin(actor) {
		return ConnectionConfig{}, identityapp.ErrForbidden
	}
	if !validUpgrade(id, input.Upgrade) || input.RequestID == uuid.Nil || len(input.PreviewHash) != 64 || input.Resolution.Validate() != nil {
		return ConnectionConfig{}, domain.ErrInvalidPreset
	}
	return s.store.ApplyConfigUpgrade(ctx, actor, id, input)
}
func validUpgrade(id uuid.UUID, input ConfigUpgradeInput) bool {
	return id != uuid.Nil && input.ExpectedRevision > 0 && input.ExpectedConfigID != uuid.Nil && input.PresetVersionID != uuid.Nil
}

// ConfigPreviewHash binds all reviewed source IDs, revisions, values and conflicts.
func ConfigPreviewHash(preview ConfigUpgradePreview, target uuid.UUID) string {
	preview.Hash = ""
	data, _ := json.Marshal(struct {
		Preview ConfigUpgradePreview
		Target  uuid.UUID
	}{preview, target})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
