package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidPreset rejects unsupported protocols and secret-bearing imports.
var ErrInvalidPreset = errors.New("invalid provider preset")

// ProviderConfig is a nonsecret, immutable execution configuration. Endpoints
// belong to the adapter registry; presets cannot introduce arbitrary hosts.
type ProviderConfig struct {
	AdapterKey       string   `json:"adapter_key"`
	BaseURL          string   `json:"base_url"`
	Region           Region   `json:"region"`
	Capabilities     []string `json:"capabilities"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	ConcurrencyLimit int      `json:"concurrency_limit"`
	RateLimitPerMin  int      `json:"rate_limit_per_min"`
}

// Validate admits only implemented protocols and bounded operational settings.
func (c ProviderConfig) Validate() error {
	var allowed []string
	switch c.AdapterKey {
	case "volcengine_ark":
		if c.BaseURL != "https://ark.cn-beijing.volces.com/api/v3" || c.Region != RegionDomestic {
			return ErrInvalidPreset
		}
		allowed = []string{"text", "image", "video"}
	case "volcengine_music":
		if c.BaseURL != "https://open.volcengineapi.com" || c.Region != RegionDomestic {
			return ErrInvalidPreset
		}
		allowed = []string{"audio"}
	case "volcengine_avatar", "volcengine_motion":
		if c.BaseURL != "https://openapi.cv.volces.com" || c.Region != RegionDomestic {
			return ErrInvalidPreset
		}
		allowed = []string{"video"}
	case "volcengine_speech":
		if c.BaseURL != "https://openspeech.bytedance.com/api/v3" || c.Region != RegionDomestic {
			return ErrInvalidPreset
		}
		allowed = []string{"audio"}
	case "openrouter":
		if c.BaseURL != "https://openrouter.ai/api/v1" || c.Region != RegionOverseas {
			return ErrInvalidPreset
		}
		allowed = []string{"text", "image", "video"}
	case "byteplus":
		if c.BaseURL != "https://ark.ap-southeast.bytepluses.com/api/v3" || c.Region != RegionOverseas {
			return ErrInvalidPreset
		}
		allowed = []string{"text", "image"}
	case "deepseek":
		if c.BaseURL != "https://api.deepseek.com" || c.Region != RegionDomestic {
			return ErrInvalidPreset
		}
		allowed = []string{"text"}
	default:
		return ErrInvalidPreset
	}
	if c.TimeoutSeconds < 5 || c.TimeoutSeconds > 300 || c.ConcurrencyLimit < 1 || c.ConcurrencyLimit > 1000 || c.RateLimitPerMin < 1 || c.RateLimitPerMin > 100000 || len(c.Capabilities) == 0 {
		return ErrInvalidPreset
	}
	seen := make(map[string]bool)
	for _, capability := range c.Capabilities {
		if !slices.Contains(allowed, capability) || seen[capability] {
			return ErrInvalidPreset
		}
		seen[capability] = true
	}
	return nil
}

// PresetDocument is the complete portable format. Connection and credential identities,
// headers, validation evidence and executable scripts are unrepresentable.
type PresetDocument struct {
	SchemaVersion  int                   `json:"schema_version"`
	Name           string                `json:"name"`
	Config         ProviderConfig        `json:"config"`
	ModelTemplates []PresetModelTemplate `json:"model_templates,omitempty"`
}

// Validate checks the versioned import/export contract.
func (p PresetDocument) Validate() error {
	if p.SchemaVersion != 1 || strings.TrimSpace(p.Name) != p.Name || len(p.Name) == 0 || len(p.Name) > 200 {
		return ErrInvalidPreset
	}
	if p.Config.Validate() != nil || len(p.ModelTemplates) > 16 {
		return ErrInvalidPreset
	}
	seen := map[string]bool{}
	for _, template := range p.ModelTemplates {
		if seen[template.Key] || template.validate(p.Config) != nil {
			return ErrInvalidPreset
		}
		seen[template.Key] = true
	}
	encoded, err := json.Marshal(p)
	if err != nil || len(encoded) > 262144 {
		return ErrInvalidPreset
	}
	return nil
}

// Digest binds an import preview to its normalized content.
func (p PresetDocument) Digest() string {
	encoded, _ := json.Marshal(p)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// ParsePreset rejects unknown and duplicate fields instead of silently dropping
// credentials or resolving ambiguous security-relevant values.
func ParsePreset(data []byte) (PresetDocument, error) {
	var p PresetDocument
	if len(data) > 262144 || rejectDuplicateFields(json.NewDecoder(bytes.NewReader(data))) != nil {
		return p, ErrInvalidPreset
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil || decoder.Decode(new(any)) != io.EOF || p.Validate() != nil {
		return PresetDocument{}, ErrInvalidPreset
	}
	return p, nil
}

func rejectDuplicateFields(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrInvalidPreset
			}
			seen[name] = true
			if err := rejectDuplicateFields(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := rejectDuplicateFields(decoder); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidPreset
	}
	_, err = decoder.Token()
	return err
}

// Preset keeps lifecycle separate from its immutable versioned content.
type Preset struct {
	ModelCatalog      ModelCandidateCatalog `json:"model_catalog"`
	ID                uuid.UUID             `json:"id"`
	Slug              string                `json:"slug"`
	Origin            string                `json:"origin"`
	ParentID          *uuid.UUID            `json:"parent_id"`
	Archived          bool                  `json:"archived"`
	EmergencyDisabled bool                  `json:"emergency_disabled"`
	Revision          int64                 `json:"revision"`
	Version           int                   `json:"version"`
	VersionID         uuid.UUID             `json:"version_id"`
	Document          PresetDocument        `json:"document"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

// BuiltinPreset declares an installation seed, never a mutable global variable.
type BuiltinPreset struct {
	Slug   string
	Name   string
	Config ProviderConfig
}

// BuiltinPresets returns fresh data so editing a copy cannot change the registry.
func BuiltinPresets() []BuiltinPreset {
	return []BuiltinPreset{
		{Slug: "volcengine-ark", Name: "火山方舟", Config: ProviderConfig{AdapterKey: "volcengine_ark", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Region: RegionDomestic, Capabilities: []string{"video", "text", "image"}, TimeoutSeconds: 120, ConcurrencyLimit: 2, RateLimitPerMin: 30}},
		{Slug: "openrouter", Name: "OpenRouter", Config: ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: RegionOverseas, Capabilities: []string{"text", "image", "video"}, TimeoutSeconds: 120, ConcurrencyLimit: 2, RateLimitPerMin: 30}},
		{Slug: "byteplus", Name: "BytePlus", Config: ProviderConfig{AdapterKey: "byteplus", BaseURL: "https://ark.ap-southeast.bytepluses.com/api/v3", Region: RegionOverseas, Capabilities: []string{"text", "image"}, TimeoutSeconds: 120, ConcurrencyLimit: 2, RateLimitPerMin: 30}},
		{Slug: "deepseek", Name: "DeepSeek", Config: ProviderConfig{AdapterKey: "deepseek", BaseURL: "https://api.deepseek.com", Region: RegionDomestic, Capabilities: []string{"text"}, TimeoutSeconds: 120, ConcurrencyLimit: 2, RateLimitPerMin: 30}},
		{Slug: "volcengine-music", Name: "火山音乐", Config: ProviderConfig{AdapterKey: "volcengine_music", BaseURL: "https://open.volcengineapi.com", Region: RegionDomestic, Capabilities: []string{"audio"}, TimeoutSeconds: 120, ConcurrencyLimit: 1, RateLimitPerMin: 60}},
		{Slug: "volcengine-avatar", Name: "即梦数字人", Config: ProviderConfig{AdapterKey: "volcengine_avatar", BaseURL: "https://openapi.cv.volces.com", Region: RegionDomestic, Capabilities: []string{"video"}, TimeoutSeconds: 120, ConcurrencyLimit: 1, RateLimitPerMin: 30}},
		{Slug: "volcengine-motion", Name: "即梦动作模仿", Config: ProviderConfig{AdapterKey: "volcengine_motion", BaseURL: "https://openapi.cv.volces.com", Region: RegionDomestic, Capabilities: []string{"video"}, TimeoutSeconds: 120, ConcurrencyLimit: 1, RateLimitPerMin: 30}},
		{Slug: "volcengine-speech", Name: "豆包语音", Config: ProviderConfig{AdapterKey: "volcengine_speech", BaseURL: "https://openspeech.bytedance.com/api/v3", Region: RegionDomestic, Capabilities: []string{"audio"}, TimeoutSeconds: 300, ConcurrencyLimit: 2, RateLimitPerMin: 30}},
	}
}

// MergePreset performs a field-wise three-way merge. Conflicting administrator
// overrides remain in the result and require explicit acceptance before saving.
func MergePreset(old, next, current ProviderConfig) (ProviderConfig, []string) {
	merged := current
	conflicts := []string{}
	merge := func(name string, previous, proposed, override, output any) {
		if reflect.DeepEqual(previous, override) {
			reflect.ValueOf(output).Elem().Set(reflect.ValueOf(proposed))
			return
		}
		if !reflect.DeepEqual(previous, proposed) && !reflect.DeepEqual(proposed, override) {
			conflicts = append(conflicts, name)
		}
	}
	merge("adapter_key", old.AdapterKey, next.AdapterKey, current.AdapterKey, &merged.AdapterKey)
	merge("base_url", old.BaseURL, next.BaseURL, current.BaseURL, &merged.BaseURL)
	merge("region", old.Region, next.Region, current.Region, &merged.Region)
	merge("capabilities", old.Capabilities, next.Capabilities, current.Capabilities, &merged.Capabilities)
	merge("timeout_seconds", old.TimeoutSeconds, next.TimeoutSeconds, current.TimeoutSeconds, &merged.TimeoutSeconds)
	merge("concurrency_limit", old.ConcurrencyLimit, next.ConcurrencyLimit, current.ConcurrencyLimit, &merged.ConcurrencyLimit)
	merge("rate_limit_per_min", old.RateLimitPerMin, next.RateLimitPerMin, current.RateLimitPerMin, &merged.RateLimitPerMin)
	merged.Capabilities = slices.Clone(merged.Capabilities)
	return merged, conflicts
}
