package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type MediaFactoryDescriptor struct {
	ProviderKey, Modality, AdapterContractVersion string
}

type MediaAdapterFactory interface {
	Descriptor() MediaFactoryDescriptor
}

type MediaFactoryRegistry struct {
	entries map[string]registeredMediaFactory
}

type registeredMediaFactory struct {
	descriptor MediaFactoryDescriptor
	factory    MediaAdapterFactory
}

type MediaPresetSummary struct {
	PresetKey, ProviderKey, DisplayName, Modality string
	PresetVersion                                 int64
}

type MediaPresetCatalogView struct {
	Connections []MediaPresetSummary
	Models      []MediaPresetSummary
}

type MediaPresetCatalog struct {
	connections map[mediaPresetIdentity]domain.MediaConnectionPreset
	models      map[mediaPresetIdentity]domain.MediaModelPreset
	registry    *MediaFactoryRegistry
}

type mediaPresetIdentity struct {
	key     string
	version int64
}

var mediaPresetKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{2,119}$`)

var ErrProviderPresetNotFound = errors.New("Media Provider preset not found")

func NewMediaFactoryRegistry(factories []MediaAdapterFactory) (*MediaFactoryRegistry, error) {
	registry := &MediaFactoryRegistry{entries: make(map[string]registeredMediaFactory, len(factories))}
	for _, factory := range factories {
		if factory == nil {
			return nil, errors.New("Media Provider factory is required")
		}
		descriptor := factory.Descriptor()
		descriptor.ProviderKey = strings.TrimSpace(descriptor.ProviderKey)
		descriptor.Modality = strings.TrimSpace(descriptor.Modality)
		descriptor.AdapterContractVersion = strings.TrimSpace(descriptor.AdapterContractVersion)
		if !providerIdentifierPattern.MatchString(descriptor.ProviderKey) ||
			(descriptor.Modality != domain.MediaModalityImage && descriptor.Modality != domain.MediaModalityVideo) ||
			!providerIdentifierPattern.MatchString(descriptor.AdapterContractVersion) {
			return nil, errors.New("Media Provider factory descriptor is invalid")
		}
		key := mediaFactoryKey(descriptor.ProviderKey, descriptor.Modality, descriptor.AdapterContractVersion)
		if _, exists := registry.entries[key]; exists {
			return nil, errors.New("Media Provider factory descriptor is duplicated")
		}
		registry.entries[key] = registeredMediaFactory{descriptor: descriptor, factory: factory}
	}
	return registry, nil
}

func (registry *MediaFactoryRegistry) Resolve(
	providerKey, modality, contractVersion string,
) (MediaAdapterFactory, error) {
	if registry == nil {
		return nil, ErrProviderPresetNotFound
	}
	entry, exists := registry.entries[mediaFactoryKey(providerKey, modality, contractVersion)]
	if !exists || entry.factory == nil {
		return nil, ErrProviderPresetNotFound
	}
	return entry.factory, nil
}

func (registry *MediaFactoryRegistry) Has(providerKey, modality, contractVersion string) bool {
	if registry == nil {
		return false
	}
	_, exists := registry.entries[mediaFactoryKey(providerKey, modality, contractVersion)]
	return exists
}

func NewMediaPresetCatalog(presets domain.MediaPresets, registry *MediaFactoryRegistry) (*MediaPresetCatalog, error) {
	if registry == nil {
		return nil, errors.New("Media Provider factory registry is required")
	}
	catalog := &MediaPresetCatalog{
		connections: make(map[mediaPresetIdentity]domain.MediaConnectionPreset, len(presets.Connections)),
		models:      make(map[mediaPresetIdentity]domain.MediaModelPreset, len(presets.Models)),
		registry:    registry,
	}
	for _, preset := range presets.Connections {
		preset, err := cloneConnectionPreset(preset)
		if err != nil {
			return nil, err
		}
		if err := validateConnectionPreset(preset); err != nil {
			return nil, err
		}
		identity := mediaPresetIdentity{key: preset.PresetKey, version: preset.PresetVersion}
		if _, exists := catalog.connections[identity]; exists {
			return nil, fmt.Errorf(
				"duplicate Media Provider connection preset %q version %d",
				preset.PresetKey,
				preset.PresetVersion,
			)
		}
		catalog.connections[identity] = preset
	}
	for _, preset := range presets.Models {
		preset, err := cloneModelPreset(preset)
		if err != nil {
			return nil, err
		}
		if err := validateModelPreset(preset); err != nil {
			return nil, err
		}
		identity := mediaPresetIdentity{key: preset.PresetKey, version: preset.PresetVersion}
		if _, exists := catalog.models[identity]; exists {
			return nil, fmt.Errorf(
				"duplicate Media Provider model preset %q version %d",
				preset.PresetKey,
				preset.PresetVersion,
			)
		}
		catalog.models[identity] = preset
	}
	return catalog, nil
}

func (catalog *MediaPresetCatalog) List() MediaPresetCatalogView {
	view := MediaPresetCatalogView{Connections: []MediaPresetSummary{}, Models: []MediaPresetSummary{}}
	if catalog == nil || catalog.registry == nil {
		return view
	}
	latestConnections := map[string]domain.MediaConnectionPreset{}
	for _, preset := range catalog.connections {
		latest, exists := latestConnections[preset.PresetKey]
		if catalog.connectionAvailable(preset) && (!exists || preset.PresetVersion > latest.PresetVersion) {
			latestConnections[preset.PresetKey] = preset
		}
	}
	for _, preset := range latestConnections {
		view.Connections = append(view.Connections, MediaPresetSummary{
			PresetKey: preset.PresetKey, PresetVersion: preset.PresetVersion,
			ProviderKey: preset.ProviderKey, DisplayName: preset.DisplayName,
		})
	}
	latestModels := map[string]domain.MediaModelPreset{}
	for _, preset := range catalog.models {
		latest, exists := latestModels[preset.PresetKey]
		if catalog.registry.Has(preset.ProviderKey, preset.Modality, preset.AdapterContractVersion) &&
			(!exists || preset.PresetVersion > latest.PresetVersion) {
			latestModels[preset.PresetKey] = preset
		}
	}
	for _, preset := range latestModels {
		view.Models = append(view.Models, MediaPresetSummary{
			PresetKey: preset.PresetKey, PresetVersion: preset.PresetVersion,
			ProviderKey: preset.ProviderKey, DisplayName: preset.DisplayName, Modality: preset.Modality,
		})
	}
	sort.Slice(view.Connections, func(left, right int) bool {
		return view.Connections[left].PresetKey < view.Connections[right].PresetKey
	})
	sort.Slice(view.Models, func(left, right int) bool { return view.Models[left].PresetKey < view.Models[right].PresetKey })
	return view
}

func (catalog *MediaPresetCatalog) ResolveConnection(
	presetKey string,
	presetVersion int64,
) (domain.MediaConnectionPreset, error) {
	if catalog == nil {
		return domain.MediaConnectionPreset{}, ErrProviderPresetNotFound
	}
	preset, exists := catalog.connections[mediaPresetIdentity{key: strings.TrimSpace(presetKey), version: presetVersion}]
	if !exists || !catalog.connectionAvailable(preset) {
		return domain.MediaConnectionPreset{}, ErrProviderPresetNotFound
	}
	return cloneConnectionPreset(preset)
}

func (catalog *MediaPresetCatalog) ResolveModel(
	presetKey string,
	presetVersion int64,
) (domain.MediaModelPreset, error) {
	if catalog == nil {
		return domain.MediaModelPreset{}, ErrProviderPresetNotFound
	}
	preset, exists := catalog.models[mediaPresetIdentity{key: strings.TrimSpace(presetKey), version: presetVersion}]
	if !exists || !catalog.registry.Has(preset.ProviderKey, preset.Modality, preset.AdapterContractVersion) {
		return domain.MediaModelPreset{}, ErrProviderPresetNotFound
	}
	return cloneModelPreset(preset)
}

func (catalog *MediaPresetCatalog) connectionAvailable(preset domain.MediaConnectionPreset) bool {
	for _, modality := range preset.SupportedFactoryModes {
		if catalog.registry.Has(preset.ProviderKey, modality, preset.AdapterContractVersion) {
			return true
		}
	}
	return false
}

func BuiltinMediaPresets() domain.MediaPresets {
	apiKey := []domain.MediaPresetField{{Key: "api_key", Type: "string", Required: true, WriteOnly: true, Minimum: 8, Maximum: 4096}}
	connections := []domain.MediaConnectionPreset{
		{PresetKey: "volcengine.ark-cn-beijing", PresetVersion: 1, ProviderKey: domain.MediaProviderVolcengine,
			DisplayName: "火山引擎方舟（北京）", Description: "火山引擎方舟官方北京地域连接",
			ProviderHomeURL: "https://console.volcengine.com/ark", AdapterContractVersion: "volcengine-ark-media",
			FixedConfig: map[string]any{"region": "cn-beijing"}, CredentialFields: apiKey,
			SupportedFactoryModes: []string{domain.MediaModalityImage, domain.MediaModalityVideo}},
		{PresetKey: "openai.official-api", PresetVersion: 1, ProviderKey: domain.MediaProviderOpenAI,
			DisplayName: "OpenAI 官方 API", Description: "OpenAI 官方图片 API 连接",
			ProviderHomeURL: "https://platform.openai.com/", AdapterContractVersion: "openai-image-api",
			FixedConfig: map[string]any{}, CredentialFields: apiKey, SupportedFactoryModes: []string{domain.MediaModalityImage}},
		{PresetKey: "google.gemini-api", PresetVersion: 1, ProviderKey: domain.MediaProviderGoogle,
			DisplayName: "Google Gemini API", Description: "Google Gemini 官方图片 API 连接",
			ProviderHomeURL: "https://ai.google.dev/", AdapterContractVersion: "google-gemini-image",
			FixedConfig: map[string]any{}, CredentialFields: apiKey, SupportedFactoryModes: []string{domain.MediaModalityImage}},
	}
	models := []domain.MediaModelPreset{
		mediaModel("volcengine.seedream-5-0-pro-260628", domain.MediaProviderVolcengine, "Seedream 5.0 Pro", "seedream", domain.MediaModalityImage, "doubao-seedream-5-0-pro-260628", "volcengine-ark-media", "ark-image-generation"),
		mediaModel("volcengine.seedance-2-0-260128", domain.MediaProviderVolcengine, "Seedance 2.0", "seedance", domain.MediaModalityVideo, "doubao-seedance-2-0-260128", "volcengine-ark-media", "ark-video-generation"),
		mediaModel("volcengine.seedance-2-0-fast-260128", domain.MediaProviderVolcengine, "Seedance 2.0 Fast", "seedance", domain.MediaModalityVideo, "doubao-seedance-2-0-fast-260128", "volcengine-ark-media", "ark-video-generation"),
		mediaModel("volcengine.seedance-2-0-mini", domain.MediaProviderVolcengine, "Seedance 2.0 Mini", "seedance", domain.MediaModalityVideo, "", "volcengine-ark-media", "ark-video-generation"),
		mediaModel("volcengine.seedance-2-5", domain.MediaProviderVolcengine, "Seedance 2.5", "seedance", domain.MediaModalityVideo, "", "volcengine-ark-media", "ark-video-generation"),
		mediaModel("openai.gpt-image-2", domain.MediaProviderOpenAI, "GPT Image 2", "gpt_image", domain.MediaModalityImage, "gpt-image-2", "openai-image-api", "openai-image-api-nonstreaming"),
		mediaModel("google.nano-banana-2-lite", domain.MediaProviderGoogle, "Nano Banana 2 Lite", "gemini_image", domain.MediaModalityImage, "gemini-3.1-flash-lite-image", "google-gemini-image", "generate-content-image"),
		mediaModel("google.nano-banana-2", domain.MediaProviderGoogle, "Nano Banana 2", "gemini_image", domain.MediaModalityImage, "gemini-3.1-flash-image", "google-gemini-image", "interactions-v1beta-image"),
		mediaModel("google.nano-banana-pro", domain.MediaProviderGoogle, "Nano Banana Pro", "gemini_image", domain.MediaModalityImage, "gemini-3-pro-image", "google-gemini-image", "interactions-v1beta-image"),
		mediaModel("google.nano-banana-legacy", domain.MediaProviderGoogle, "Nano Banana Legacy", "gemini_image", domain.MediaModalityImage, "gemini-2.5-flash-image", "google-gemini-image", "generate-content-image"),
	}
	return domain.MediaPresets{Connections: connections, Models: models}
}

func mediaModel(presetKey, providerKey, displayName, family, modality, externalModelID, adapterContract, transport string) domain.MediaModelPreset {
	metric := "generation.image.call"
	if modality == domain.MediaModalityVideo {
		metric = "generation.video.call"
	}
	return domain.MediaModelPreset{
		PresetKey: presetKey, PresetVersion: 1, ProviderKey: providerKey, DisplayName: displayName,
		Family: family, Modality: modality, ExternalModelID: externalModelID,
		AdapterContractVersion: adapterContract, AdapterTransportContract: transport,
		CapabilitySchemaVersion: family + "-capability", BillingMetric: metric, FixedDefaults: map[string]any{},
	}
}

func validateConnectionPreset(preset domain.MediaConnectionPreset) error {
	if !mediaPresetKeyPattern.MatchString(preset.PresetKey) || preset.PresetVersion < 1 ||
		!providerIdentifierPattern.MatchString(preset.ProviderKey) || strings.TrimSpace(preset.DisplayName) == "" ||
		!providerIdentifierPattern.MatchString(preset.AdapterContractVersion) || len(preset.SupportedFactoryModes) == 0 {
		return fmt.Errorf("Media Provider connection preset %q is invalid", preset.PresetKey)
	}
	for _, modality := range preset.SupportedFactoryModes {
		if modality != domain.MediaModalityImage && modality != domain.MediaModalityVideo {
			return fmt.Errorf("Media Provider connection preset %q has an invalid modality", preset.PresetKey)
		}
	}
	if slices.ContainsFunc(preset.CredentialFields, func(field domain.MediaPresetField) bool { return !field.WriteOnly }) {
		return fmt.Errorf("Media Provider connection preset %q has a readable credential field", preset.PresetKey)
	}
	return nil
}

func validateModelPreset(preset domain.MediaModelPreset) error {
	if !mediaPresetKeyPattern.MatchString(preset.PresetKey) || preset.PresetVersion < 1 ||
		!providerIdentifierPattern.MatchString(preset.ProviderKey) || strings.TrimSpace(preset.DisplayName) == "" ||
		(preset.Modality != domain.MediaModalityImage && preset.Modality != domain.MediaModalityVideo) ||
		!providerIdentifierPattern.MatchString(preset.AdapterContractVersion) ||
		!providerIdentifierPattern.MatchString(preset.AdapterTransportContract) ||
		!providerIdentifierPattern.MatchString(preset.CapabilitySchemaVersion) {
		return fmt.Errorf("Media Provider model preset %q is invalid", preset.PresetKey)
	}
	return nil
}

func mediaFactoryKey(providerKey, modality, contractVersion string) string {
	return strings.Join([]string{providerKey, modality, contractVersion}, "\x00")
}

func cloneConnectionPreset(value domain.MediaConnectionPreset) (domain.MediaConnectionPreset, error) {
	fixedConfig, err := clonePresetMap(value.FixedConfig)
	if err != nil {
		return domain.MediaConnectionPreset{}, fmt.Errorf(
			"clone Media Provider connection preset %q: %w",
			value.PresetKey,
			err,
		)
	}
	value.FixedConfig = fixedConfig
	value.EditableFields = slices.Clone(value.EditableFields)
	value.CredentialFields = slices.Clone(value.CredentialFields)
	value.SupportedFactoryModes = slices.Clone(value.SupportedFactoryModes)
	return value, nil
}

func cloneModelPreset(value domain.MediaModelPreset) (domain.MediaModelPreset, error) {
	fixedDefaults, err := clonePresetMap(value.FixedDefaults)
	if err != nil {
		return domain.MediaModelPreset{}, fmt.Errorf(
			"clone Media Provider model preset %q: %w",
			value.PresetKey,
			err,
		)
	}
	value.FixedDefaults = fixedDefaults
	value.EditableOverrides = slices.Clone(value.EditableOverrides)
	return value, nil
}

func clonePresetMap(value map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}
