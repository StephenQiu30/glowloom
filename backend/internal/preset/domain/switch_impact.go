package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const SwitchImpactContractID = "preset-switch-impact-production"

var presetSwitchPreservedFamilies = []string{
	"asset_state",
	"identity",
	"identity_resolution_candidate",
	"interaction",
	"interaction_continuity_candidate",
	"production_entity_candidate",
	"production_world",
	"scene",
	"scene_binding_candidate",
	"scene_fact_candidate",
	"script_source",
	"script_span_candidate",
	"specification",
}

var presetSwitchRecommendedActions = map[string]string{
	"asset_version":       "regenerate_reference_asset",
	"production_packet":   "compile_production_packet",
	"reference_binding":   "rebuild_reference_binding",
	"reference_brief":     "compile_reference_brief",
	"reference_bundle":    "regenerate_reference_bundle",
	"reference_plan":      "plan_reference_assets",
	"reference_selection": "review_reference_selection",
	"storyboard":          "direct_storyboard",
	"visual_foundation":   "resolve_visual_foundation",
}

var presetSwitchInvalidatedFamilies = []string{
	"asset_version",
	"production_packet",
	"reference_binding",
	"reference_brief",
	"reference_bundle",
	"reference_plan",
	"reference_selection",
	"storyboard",
	"visual_foundation",
}

type ReleaseIdentity struct {
	Key         string `json:"key"`
	Release     string `json:"release"`
	ContentHash string `json:"content_hash"`
}

type SwitchImpactInput struct {
	ProjectID         string          `json:"project_id"`
	PreviousRelease   ReleaseIdentity `json:"previous_release"`
	NextRelease       ReleaseIdentity `json:"next_release"`
	AffectedScopeKeys []string        `json:"affected_scope_keys"`
}

type StaleEffect struct {
	Family            string          `json:"family"`
	StaleReason       string          `json:"stale_reason"`
	CausedByIdentity  ReleaseIdentity `json:"caused_by_identity"`
	AffectedScopeKeys []string        `json:"affected_scope_keys"`
	RecommendedAction string          `json:"recommended_action"`
}

type SwitchImpact struct {
	ContractID string `json:"contract_id"`
	SwitchImpactInput
	PreservedFamilies []string      `json:"preserved_families"`
	Invalidated       []StaleEffect `json:"invalidated"`
	ContentHash       string        `json:"content_hash"`
}

func NewSwitchImpact(input SwitchImpactInput) (SwitchImpact, json.RawMessage, error) {
	input.AffectedScopeKeys = append([]string(nil), input.AffectedScopeKeys...)
	for index := range input.AffectedScopeKeys {
		input.AffectedScopeKeys[index] = strings.TrimSpace(input.AffectedScopeKeys[index])
	}
	sort.Strings(input.AffectedScopeKeys)

	impact := SwitchImpact{
		ContractID:        SwitchImpactContractID,
		SwitchImpactInput: input,
		PreservedFamilies: append([]string(nil), presetSwitchPreservedFamilies...),
		Invalidated:       buildPresetSwitchStaleEffects(input.NextRelease, input.AffectedScopeKeys),
	}
	if err := validateSwitchImpact(impact); err != nil {
		return SwitchImpact{}, nil, err
	}
	hash, err := switchImpactHash(impact)
	if err != nil {
		return SwitchImpact{}, nil, err
	}
	impact.ContentHash = hash
	encoded, err := encodeSwitchImpact(impact)
	if err != nil {
		return SwitchImpact{}, nil, err
	}
	return impact, encoded, nil
}

func DecodeSwitchImpact(raw json.RawMessage) (SwitchImpact, json.RawMessage, error) {
	var impact SwitchImpact
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&impact); err != nil {
		return SwitchImpact{}, nil, errors.New("invalid Preset switch impact")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SwitchImpact{}, nil, errors.New("invalid Preset switch impact")
	}
	if !hashPattern.MatchString(impact.ContentHash) || validateSwitchImpact(impact) != nil {
		return SwitchImpact{}, nil, errors.New("invalid Preset switch impact")
	}
	hash, err := switchImpactHash(impact)
	if err != nil || hash != impact.ContentHash {
		return SwitchImpact{}, nil, errors.New("Preset switch impact content hash has drifted")
	}
	encoded, err := encodeSwitchImpact(impact)
	if err != nil {
		return SwitchImpact{}, nil, err
	}
	return impact, encoded, nil
}

func buildPresetSwitchStaleEffects(cause ReleaseIdentity, scopes []string) []StaleEffect {
	effects := make([]StaleEffect, 0, len(presetSwitchInvalidatedFamilies))
	for _, family := range presetSwitchInvalidatedFamilies {
		effects = append(effects, StaleEffect{
			Family: family, StaleReason: "preset_release_changed", CausedByIdentity: cause,
			AffectedScopeKeys: append([]string(nil), scopes...),
			RecommendedAction: presetSwitchRecommendedActions[family],
		})
	}
	return effects
}

func validateSwitchImpact(value SwitchImpact) error {
	if value.ContractID != SwitchImpactContractID {
		return errors.New("invalid Preset switch impact contract")
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil || projectID == uuid.Nil || validateReleaseIdentity(value.PreviousRelease) != nil ||
		validateReleaseIdentity(value.NextRelease) != nil || value.PreviousRelease == value.NextRelease ||
		value.PreviousRelease.ContentHash == value.NextRelease.ContentHash ||
		validatePresetSwitchScopes(value.ProjectID, value.AffectedScopeKeys) != nil ||
		!slices.Equal(value.PreservedFamilies, presetSwitchPreservedFamilies) ||
		len(value.Invalidated) != len(presetSwitchInvalidatedFamilies) {
		return errors.New("invalid Preset switch impact contract")
	}
	for index, effect := range value.Invalidated {
		family := presetSwitchInvalidatedFamilies[index]
		if effect.Family != family || effect.StaleReason != "preset_release_changed" ||
			effect.CausedByIdentity != value.NextRelease ||
			!slices.Equal(effect.AffectedScopeKeys, value.AffectedScopeKeys) ||
			effect.RecommendedAction != presetSwitchRecommendedActions[family] ||
			slices.Contains(value.PreservedFamilies, effect.Family) {
			return errors.New("invalid Preset switch stale effect")
		}
	}
	return nil
}

func validateReleaseIdentity(value ReleaseIdentity) error {
	if !releaseKeyPattern.MatchString(value.Key) || !validReleaseDate(value.Release) ||
		!hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid Preset release identity")
	}
	return nil
}

func validatePresetSwitchScopes(projectID string, values []string) error {
	if len(values) < 2 || !sort.StringsAreSorted(values) {
		return errors.New("invalid Preset switch impact scopes")
	}
	projectScope := "project:" + projectID
	projectCount := 0
	for index, value := range values {
		if value == "" || index > 0 && values[index-1] == value {
			return errors.New("invalid Preset switch impact scopes")
		}
		kind, identifier, found := strings.Cut(value, ":")
		parsed, err := uuid.Parse(identifier)
		if !found || err != nil || parsed == uuid.Nil {
			return errors.New("invalid Preset switch impact scope")
		}
		switch kind {
		case "project":
			if value != projectScope {
				return errors.New("Preset switch impact crosses project scope")
			}
			projectCount++
		case "scene":
		default:
			return errors.New("invalid Preset switch impact scope kind")
		}
	}
	if projectCount != 1 {
		return errors.New("Preset switch impact requires its project scope")
	}
	return nil
}

func switchImpactHash(impact SwitchImpact) (string, error) {
	impact.ContentHash = ""
	raw, err := json.Marshal(impact)
	if err != nil {
		return "", err
	}
	var material map[string]json.RawMessage
	if err = json.Unmarshal(raw, &material); err != nil {
		return "", err
	}
	delete(material, "content_hash")
	raw, err = json.Marshal(material)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeSwitchImpact(impact SwitchImpact) (json.RawMessage, error) {
	raw, err := json.Marshal(impact)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
