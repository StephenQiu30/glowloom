package preset_test

import (
	"encoding/json"
	"slices"
	"testing"

	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

func TestPresetSwitchImpactPreservesTextWorldAndInvalidatesOnlyVisualChain(t *testing.T) {
	input := validPresetSwitchImpactInput()
	left, leftJSON, err := preset.NewSwitchImpact(input)
	if err != nil {
		t.Fatalf("new Preset switch impact: %v", err)
	}

	reordered := validPresetSwitchImpactInput()
	slices.Reverse(reordered.AffectedScopeKeys)
	right, rightJSON, err := preset.NewSwitchImpact(reordered)
	if err != nil {
		t.Fatalf("new reordered Preset switch impact: %v", err)
	}
	if left.ContentHash != right.ContentHash || !slices.Equal(leftJSON, rightJSON) {
		t.Fatal("Preset switch impact depends on scope arrival order")
	}

	wantPreserved := []string{
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
	if !slices.Equal(left.PreservedFamilies, wantPreserved) {
		t.Fatalf("unexpected preserved families: %v", left.PreservedFamilies)
	}
	wantInvalidated := []string{
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
	if len(left.Invalidated) != len(wantInvalidated) {
		t.Fatalf("unexpected invalidated effects: %#v", left.Invalidated)
	}
	for index, effect := range left.Invalidated {
		if effect.Family != wantInvalidated[index] || effect.StaleReason != "preset_release_changed" ||
			effect.CausedByIdentity != left.NextRelease ||
			!slices.Equal(effect.AffectedScopeKeys, left.AffectedScopeKeys) || effect.RecommendedAction == "" {
			t.Fatalf("invalid stale effect: %#v", effect)
		}
	}

	decoded, canonical, err := preset.DecodeSwitchImpact(leftJSON)
	if err != nil {
		t.Fatalf("decode Preset switch impact: %v", err)
	}
	if decoded.ContentHash != left.ContentHash || !slices.Equal(canonical, leftJSON) {
		t.Fatal("Preset switch impact did not round-trip canonically")
	}
}

func TestPresetSwitchImpactRejectsP0InvalidationAndIdentityDrift(t *testing.T) {
	impact, encoded, err := preset.NewSwitchImpact(validPresetSwitchImpactInput())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}

	invalidated := payload["invalidated"].([]any)
	first := invalidated[0].(map[string]any)
	first["family"] = "scene_fact_candidate"
	payload["content_hash"] = impact.ContentHash
	if _, _, err = preset.DecodeSwitchImpact(mustPresetJSON(t, payload)); err == nil {
		t.Fatal("Preset switch impact accepted a P0 family as stale")
	}

	input := validPresetSwitchImpactInput()
	input.NextRelease.ContentHash = input.PreviousRelease.ContentHash
	if _, _, err = preset.NewSwitchImpact(input); err == nil {
		t.Fatal("Preset switch impact accepted an unchanged release identity")
	}

	input = validPresetSwitchImpactInput()
	input.AffectedScopeKeys = []string{"scene:00000000-0000-0000-0000-000000000001"}
	if _, _, err = preset.NewSwitchImpact(input); err == nil {
		t.Fatal("Preset switch impact accepted a scope set without its project root")
	}
}

func TestPresetSwitchImpactRejectsUnknownFieldsAndHashDrift(t *testing.T) {
	_, encoded, err := preset.NewSwitchImpact(validPresetSwitchImpactInput())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["fallback"] = "invalidate_everything"
	if _, _, err = preset.DecodeSwitchImpact(mustPresetJSON(t, payload)); err == nil {
		t.Fatal("unknown Preset switch impact field was accepted")
	}
	delete(payload, "fallback")
	payload["affected_scope_keys"] = append(
		payload["affected_scope_keys"].([]any),
		"scene:00000000-0000-0000-0000-000000000003",
	)
	if _, _, err = preset.DecodeSwitchImpact(mustPresetJSON(t, payload)); err == nil {
		t.Fatal("Preset switch impact hash drift was accepted")
	}
}

func validPresetSwitchImpactInput() preset.SwitchImpactInput {
	return preset.SwitchImpactInput{
		ProjectID: "00000000-0000-0000-0000-000000000001",
		PreviousRelease: preset.ReleaseIdentity{
			Key: "urban-cinematic-realism", Release: "2026.09.11", ContentHash: presetTestHash("qc"),
		},
		NextRelease: preset.ReleaseIdentity{
			Key: "xianxia-animation", Release: "2026.09.12", ContentHash: presetTestHash("model"),
		},
		AffectedScopeKeys: []string{
			"scene:00000000-0000-0000-0000-000000000002",
			"project:00000000-0000-0000-0000-000000000001",
		},
	}
}

func mustPresetJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
