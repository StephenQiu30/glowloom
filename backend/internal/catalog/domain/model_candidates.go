package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ModelCandidatePreset is public configuration guidance, never account access,
// a price, a release, or evidence that inference succeeded.
type ModelCandidatePreset struct {
	Key                    string                   `json:"key"`
	AdapterKey             string                   `json:"adapter_key"`
	DisplayName            string                   `json:"display_name"`
	ProviderModelID        string                   `json:"provider_model_id"`
	Capability             string                   `json:"capability"`
	Modes                  []string                 `json:"modes"`
	SourceURL              string                   `json:"source_url"`
	ReviewedAt             string                   `json:"reviewed_at"`
	Lifecycle              string                   `json:"lifecycle"`
	RequiresModelSelection bool                     `json:"requires_model_selection"`
	MaxOutputs             *int                     `json:"max_outputs,omitempty"`
	Execution              *ModelCandidateExecution `json:"execution,omitempty"`
	MeteringStrategy       string                   `json:"metering_strategy,omitempty"`
}

// MaxPresetModels bounds atomic onboarding selections and management readback.
const MaxPresetModels = 64

// ModelCandidateCatalog versions selections independently of priced v1 templates.
type ModelCandidateCatalog struct {
	Version string                 `json:"version"`
	Models  []ModelCandidatePreset `json:"models"`
}

// ModelCandidateSelection preserves the public facts chosen at connection time.
// Later catalog updates never overwrite administrator configuration.
type ModelCandidateSelection struct {
	CatalogVersion string               `json:"catalog_version"`
	Model          ModelCandidatePreset `json:"model"`
}

// Select resolves an ordered selection against one exact catalog snapshot.
func (c ModelCandidateCatalog) Select(version string, keys []string) ([]ModelCandidatePreset, error) {
	if len(keys) > MaxPresetModels || (len(keys) > 0 && version != c.Version) {
		return nil, ErrInvalidPreset
	}
	selected := make([]ModelCandidatePreset, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			return nil, ErrInvalidPreset
		}
		seen[key] = true
		found := false
		for _, candidate := range c.Models {
			if candidate.Key == key {
				selected = append(selected, candidate)
				found = true
				break
			}
		}
		if !found {
			return nil, ErrInvalidPreset
		}
	}
	return selected, nil
}

// ModelCatalogForPreset returns candidate models matching the config's capabilities,
// merging a persisted snapshot with explicit administrator model templates.
func ModelCatalogForPreset(document PresetDocument, source ModelCandidateCatalog) ModelCandidateCatalog {
	catalog := ModelCandidateCatalog{Version: source.Version, Models: []ModelCandidatePreset{}}
	for _, model := range source.Models {
		if model.AdapterKey == document.Config.AdapterKey && slices.Contains(document.Config.Capabilities, strings.TrimSuffix(model.Capability, ".generate")) {
			catalog.Models = append(catalog.Models, model)
		}
	}

	existingKeys := make(map[string]int, len(catalog.Models))
	for index, model := range catalog.Models {
		existingKeys[model.Key] = index
	}
	for _, template := range document.ModelTemplates {
		reviewedAt := ""
		if !template.UpstreamCost.ReviewedAt.IsZero() {
			reviewedAt = template.UpstreamCost.ReviewedAt.Format("2006-01-02")
		}
		candidate := ModelCandidatePreset{
			Key:             template.Key,
			AdapterKey:      document.Config.AdapterKey,
			DisplayName:     template.Name,
			ProviderModelID: template.ProviderModelID,
			Capability:      template.Capability,
			Modes:           slices.Clone(template.Modes),
			SourceURL:       template.UpstreamCost.Source,
			ReviewedAt:      reviewedAt,
			Lifecycle:       "unknown",
		}
		// An explicit v1 template supplies its own version contract. Combining
		// its configuration with a same-key SQL meter changes legacy semantics.
		if index, exists := existingKeys[template.Key]; exists {
			catalog.Models[index] = candidate
			continue
		}
		existingKeys[template.Key] = len(catalog.Models)
		catalog.Models = append(catalog.Models, candidate)
	}
	raw, _ := json.Marshal(struct {
		Source   string
		Document PresetDocument
		Models   []ModelCandidatePreset
	}{source.Version, document, catalog.Models})
	// jsonb may reorder object fields; selection identity follows semantic JSON.
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) == nil {
		raw, _ = json.Marshal(value)
	}
	digest := sha256.Sum256(raw)
	catalog.Version = "catalog:" + hex.EncodeToString(digest[:])
	return catalog
}

// ModelCandidateExecution carries public, conservative parameters without account identities or a price.
type ModelCandidateExecution struct {
	Limits        json.RawMessage `json:"limits"`
	ParamSchema   json.RawMessage `json:"param_schema"`
	ExpectedMaxMS int             `json:"expected_max_ms"`
	SupportsQuery bool            `json:"supports_query"`
}

// OnboardingVersion instantiates a pinned public contract without copying prices or evidence.
func (m ModelCandidatePreset) OnboardingVersion(modelID, actorID uuid.UUID) (ModelVersion, error) {
	if m.RequiresModelSelection || m.ProviderModelID == "" || !IsManagedCloud(m.AdapterKey) || m.Execution == nil {
		return ModelVersion{}, ErrInvalidPreset
	}
	contract := m.Execution
	var limits map[string]json.RawMessage
	if json.Unmarshal(contract.Limits, &limits) != nil || limits == nil {
		return ModelVersion{}, ErrInvalidPreset
	}
	if m.MeteringStrategy != "" {
		raw, _ := json.Marshal(m.MeteringStrategy)
		limits["metering_strategy"] = raw
	}
	normalized, err := json.Marshal(limits)
	if err != nil {
		return ModelVersion{}, ErrInvalidPreset
	}
	version := ModelVersion{ID: uuid.New(), ModelID: modelID, VersionNo: 1, ProviderModelID: m.ProviderModelID, Modes: slices.Clone(m.Modes), Limits: normalized, ParamSchema: slices.Clone(contract.ParamSchema), ExpectedMaxMS: contract.ExpectedMaxMS, SupportsQuery: contract.SupportsQuery, Moderation: ModerationProvider, Queue: "provider", CreateBy: actorID}
	return version, version.Validate()
}

// ValidatePinnedVersion prevents settings edits from dropping a SQL candidate's immutable identity and meter.
// Snapshots predating public execution contracts retain their historical semantics.
func (m ModelCandidatePreset) ValidatePinnedVersion(version ModelVersion) error {
	if m.Execution == nil && m.MeteringStrategy == "" {
		return nil
	}
	if version.ProviderModelID != m.ProviderModelID {
		return ErrInvalidModelVersion
	}
	for _, mode := range version.Modes {
		if !slices.Contains(m.Modes, mode) {
			return ErrInvalidModelVersion
		}
	}
	var limits struct {
		MeteringStrategy string `json:"metering_strategy"`
	}
	if json.Unmarshal(version.Limits, &limits) != nil || m.MeteringStrategy != "" && limits.MeteringStrategy != m.MeteringStrategy {
		return ErrInvalidModelVersion
	}
	if m.Execution != nil && !pinnedParametersWithin(m.Execution.ParamSchema, version.ParamSchema, version.Modes) {
		return ErrInvalidModelVersion
	}
	return nil
}

type pinnedParameter struct {
	Field    string            `json:"field"`
	Type     string            `json:"type"`
	Required bool              `json:"required"`
	Default  json.RawMessage   `json:"default"`
	Enum     []json.RawMessage `json:"enum"`
	Min      json.RawMessage   `json:"min"`
	Max      json.RawMessage   `json:"max"`
	ForModes []string          `json:"for_modes"`
}

func pinnedParametersWithin(source, proposed json.RawMessage, modes []string) bool {
	parse := func(raw json.RawMessage) (map[string]pinnedParameter, bool) {
		var fields []pinnedParameter
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return nil, false
		}
		result := make(map[string]pinnedParameter, len(fields))
		for _, field := range fields {
			if field.Field == "" {
				return nil, false
			}
			if _, exists := result[field.Field]; exists {
				return nil, false
			}
			result[field.Field] = field
		}
		return result, true
	}
	baseline, sourceOK := parse(source)
	fields, proposedOK := parse(proposed)
	if !sourceOK || !proposedOK {
		return false
	}
	active := func(field pinnedParameter, mode string) bool {
		return len(field.ForModes) == 0 || slices.Contains(field.ForModes, mode)
	}
	for name, field := range fields {
		original, exists := baseline[name]
		if !exists || field.Type != original.Type {
			return false
		}
		retained := false
		for _, mode := range modes {
			if active(field, mode) {
				retained = true
				if !active(original, mode) {
					return false
				}
			}
		}
		if !retained {
			continue
		}
		if original.Required && !field.Required {
			return false
		}
		if !pinnedBoundWithin(original.Min, field.Min, true) || !pinnedBoundWithin(original.Max, field.Max, false) {
			return false
		}
		if len(original.Enum) > 0 {
			if len(field.Enum) == 0 {
				return false
			}
			for _, option := range field.Enum {
				if !slices.ContainsFunc(original.Enum, func(value json.RawMessage) bool { return pinnedOptionEqual(option, value, field.Type) }) {
					return false
				}
			}
		}
	}
	for name, original := range baseline {
		if !original.Required && len(original.Default) == 0 {
			continue
		}
		field, exists := fields[name]
		for _, mode := range modes {
			if active(original, mode) && (!exists || !active(field, mode) || !field.Required && len(field.Default) == 0) {
				return false
			}
		}
	}
	return true
}

func pinnedBoundWithin(source, proposed json.RawMessage, minimum bool) bool {
	if len(source) == 0 {
		return true
	}
	base, baseOK := pinnedNumber(source)
	value, valueOK := pinnedNumber(proposed)
	if !baseOK || !valueOK {
		return false
	}
	comparison := value.Cmp(base)
	return minimum && comparison >= 0 || !minimum && comparison <= 0
}

func pinnedOptionEqual(left, right json.RawMessage, fieldType string) bool {
	if fieldType == "integer" || fieldType == "number" {
		a, aOK := pinnedNumber(left)
		b, bOK := pinnedNumber(right)
		return aOK && bOK && a.Cmp(b) == 0
	}
	var a, b string
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && a == b
}

func pinnedNumber(raw json.RawMessage) (*big.Rat, bool) {
	text := string(bytes.TrimSpace(raw))
	if len(text) == 0 || len(text) > 512 || !json.Valid([]byte(text)) || text[0] != '-' && (text[0] < '0' || text[0] > '9') {
		return nil, false
	}
	if at := strings.IndexAny(text, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(text[at+1:])
		if err != nil || exponent < -512 || exponent > 512 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(text)
}
