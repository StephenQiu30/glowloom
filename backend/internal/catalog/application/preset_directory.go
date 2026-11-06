package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	ErrModelDirectoryUnsupported = errors.New("provider model directory unsupported")
	ErrModelDirectoryCredential  = errors.New("provider model directory credential rejected")
	ErrModelDirectoryRateLimited = errors.New("provider model directory rate limited")
	ErrModelDirectoryUnavailable = errors.New("provider model directory unavailable")
)

type PresetDirectoryInput struct {
	VersionID uuid.UUID `json:"version_id"`
	APIKey    string    `json:"api_key"`
}

type PresetDirectoryResult struct {
	ModelIDs []string `json:"model_ids"`
}

// ReadModelIDs is a read-only upstream directory call. IDs carry no assertion
// about the model's supported modalities or inference permissions.
type ReadModelIDs func(context.Context, domain.ProviderConfig, string) ([]string, error)

func (s *ProviderPresets) ReadDirectory(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input PresetDirectoryInput, read ReadModelIDs) (PresetDirectoryResult, error) {
	if !IsPlatformAdmin(actor) {
		return PresetDirectoryResult{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.VersionID == uuid.Nil || len(input.APIKey) > 4096 || strings.TrimSpace(input.APIKey) == "" || strings.ContainsAny(input.APIKey, "\r\n") {
		return PresetDirectoryResult{}, domain.ErrInvalidPreset
	}
	presets, err := s.store.ListPresets(ctx, actor)
	if err != nil {
		return PresetDirectoryResult{}, err
	}
	for _, preset := range presets {
		if preset.ID != id {
			continue
		}
		if preset.Archived || preset.EmergencyDisabled || preset.VersionID != input.VersionID {
			return PresetDirectoryResult{}, ErrPresetUnavailable
		}
		config := preset.Document.Config
		spec, ok := domain.GetAdapterSpec(config.AdapterKey)
		if !ok || config.Validate() != nil || !slices.Contains(spec.CheckCapabilities, "model_discovery") {
			return PresetDirectoryResult{}, ErrModelDirectoryUnsupported
		}
		call, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		ids, err := read(call, config, strings.TrimSpace(input.APIKey))
		return PresetDirectoryResult{ModelIDs: ids}, err
	}
	return PresetDirectoryResult{}, ErrPresetNotFound
}
