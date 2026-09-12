package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestReferenceBriefCandidateAcceptsSixStrictPurposeBranches(t *testing.T) {
	targetKinds := []string{
		"character_appearance",
		"character_identity_anchor",
		"interaction_composition",
		"location_board",
		"prop_sheet",
		"scene_composition",
	}
	for _, targetKind := range targetKinds {
		t.Run(targetKind, func(t *testing.T) {
			raw := referenceBriefCandidateJSON(t, targetKind)
			candidate, canonical, err := agentcontract.DecodeReferenceBriefCandidate(raw)
			if err != nil {
				t.Fatal(err)
			}
			if candidate.TargetKind != targetKind || len(canonical) == 0 {
				t.Fatalf("decoded Reference Brief lost its purpose: %#v", candidate)
			}
		})
	}
}

func TestReferenceBriefCandidateRejectsProviderFieldsWrongViewsAndBranchDrift(t *testing.T) {
	raw := referenceBriefCandidateJSON(t, "character_appearance")
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["provider"] = map[string]any{"model": "current"}
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted Provider-specific configuration")
	}

	delete(document, "provider")
	document["required_view_roles"] = []string{"front"}
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted an incomplete view-role contract")
	}

	document = referenceBriefCandidateDocument(t, "character_appearance")
	document["brief"] = referenceBriefPurpose("prop_sheet")
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted a purpose branch that drifted from target_kind")
	}

	document = referenceBriefCandidateDocument(t, "character_identity_anchor")
	targetRef := document["reference_plan_target_ref"].(map[string]any)
	targetRef["owner_version_id"] = "latest"
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted a latest Target pointer")
	}

	document = referenceBriefCandidateDocument(t, "scene_composition")
	document["brief"].(map[string]any)["prompt"] = "free-form provider prompt"
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted a free-form Prompt in its purpose branch")
	}

	document = referenceBriefCandidateDocument(t, "character_appearance")
	document["dependency_selections"] = []any{}
	if _, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document)); err == nil {
		t.Fatal("Reference Brief accepted Appearance without its exact Anchor selection")
	}
}

func referenceBriefCandidateJSON(t *testing.T, targetKind string) json.RawMessage {
	t.Helper()
	return mustReferenceBriefJSON(t, referenceBriefCandidateDocument(t, targetKind))
}

func referenceBriefCandidateDocument(t *testing.T, targetKind string) map[string]any {
	t.Helper()
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	targetKey := referenceBriefTargetBusinessKey(targetKind)
	dependencies := []any{}
	if targetKind == "character_appearance" || strings.HasSuffix(targetKind, "_composition") {
		dependencies = []any{map[string]any{
			"target_version_ref":         referenceBriefOwnerRef(workspaceID, projectID, "production/reference", "reference_plan_set", `["character_identity_anchor",["asset","asset_identity_state_set","character:one",""]]`),
			"selected_asset_version_ref": referenceBriefOwnerRef(workspaceID, projectID, "asset", "asset_base_reference_set", "asset:one"),
		}}
	}
	sourceRefs := map[string]any{
		"identity":      []any{referenceBriefOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "identity:one")},
		"specification": []any{referenceBriefOwnerRef(workspaceID, projectID, "production/bible", "bible_production_world_set", "specification:one")},
		"state":         []any{referenceBriefOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "state:one")},
		"scene":         []any{referenceBriefOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "scene:one")},
		"occurrence":    []any{referenceBriefOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "occurrence:one")},
		"interaction":   []any{},
	}
	if targetKind == "interaction_composition" {
		sourceRefs["interaction"] = []any{referenceBriefOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "interaction:one")}
	}
	return map[string]any{
		"workspace_id":                        workspaceID,
		"project_id":                          projectID,
		"approved_reference_plan_version_ref": referenceBriefOwnerRef(workspaceID, projectID, "production/reference", "reference_plan_set", "reference-plan:one"),
		"reference_plan_target_ref":           referenceBriefOwnerRef(workspaceID, projectID, "production/reference", "reference_plan_set", targetKey),
		"target_business_key":                 targetKey,
		"target_kind":                         targetKind,
		"visual_foundation_version_ref":       referenceBriefOwnerRef(workspaceID, projectID, "preset", "preset_effective_set", projectID),
		"effective_style_snapshot_ref":        referenceBriefOwnerRef(workspaceID, projectID, "preset", "preset_effective_set", projectID+":style"),
		"effective_policy_snapshot_ref":       referenceBriefOwnerRef(workspaceID, projectID, "preset", "preset_effective_set", projectID+":policy"),
		"dependency_selections":               dependencies,
		"stage_release": map[string]any{
			"stage_key": "compile_reference_brief", "stage_release_hash": strings.Repeat("a", 64),
		},
		"typed_read_set_root": strings.Repeat("b", 64),
		"source_design_slots": []any{map[string]any{
			"slot_key": "primary_form", "source_requirement": "preserve confirmed production fact", "design_requirement": "apply the approved visual policy only",
		}},
		"positive_instructions":   []string{"preserve exact approved identity and state"},
		"negative_instructions":   []string{"do not add unapproved identities or props"},
		"required_view_roles":     referenceBriefViewRoles(targetKind),
		"layout_requirements":     []string{"keep every required view independently assessable"},
		"scale_requirements":      []string{"preserve approved relative scale"},
		"rights_requirements":     []string{"use only authorized source and dependency material"},
		"provenance_requirements": []string{"preserve exact source and dependency lineage"},
		"qc_rubric_refs": []any{map[string]any{
			"contract_id": "reference-visual-qc-production", "content_hash": strings.Repeat("c", 64),
		}},
		"source_refs": sourceRefs,
		"brief":       referenceBriefPurpose(targetKind),
	}
}

func referenceBriefPurpose(targetKind string) map[string]any {
	switch targetKind {
	case "character_identity_anchor":
		return map[string]any{"target_kind": targetKind, "identity_invariant_slots": []string{"body_shape", "facial_structure", "hair", "permanent_marks", "proportions"}}
	case "character_appearance":
		return map[string]any{
			"target_kind":              targetKind,
			"identity_invariant_slots": []string{"body_shape", "facial_structure", "hair", "permanent_marks", "proportions"},
			"approved_variable_slots":  []string{"wardrobe"},
		}
	case "location_board":
		return map[string]any{
			"target_kind": targetKind, "topology_constraints": []string{"preserve entrance and exit topology"},
			"scale_anchors": []string{"preserve confirmed human scale"}, "material_slots": []string{"approved wall material"}, "occupancy_policy": "empty",
		}
	case "prop_sheet":
		return map[string]any{
			"target_kind": targetKind, "physical_dimensions": "preserve confirmed dimensions", "structural_slots": []string{"outer structure"},
			"state_slots": []string{"approved state"}, "content_or_mechanism_slots": []string{"approved mechanism"}, "occupancy_policy": "no_hands_no_people",
		}
	case "scene_composition":
		return map[string]any{"target_kind": targetKind, "composition_purpose": "show the approved scene relationship", "spatial_constraints": []string{"preserve approved occurrence positions"}}
	default:
		return map[string]any{
			"target_kind": targetKind, "hand_side": "right", "grip_or_contact_point": "approved handle",
			"orientation": "toward the approved counterparty", "body_prop_scale_constraints": []string{"preserve approved body-to-prop scale"}, "transfer_or_use_state": "held",
		}
	}
}

func referenceBriefViewRoles(targetKind string) []string {
	switch targetKind {
	case "character_identity_anchor", "character_appearance":
		return []string{"back", "front", "profile"}
	case "location_board":
		return []string{"empty_establishing", "material_scale_detail", "spatial_orientation"}
	case "prop_sheet":
		return []string{"back", "front", "side", "state_detail"}
	case "scene_composition":
		return []string{"composition_master"}
	default:
		return []string{"interaction_master"}
	}
}

func referenceBriefTargetBusinessKey(targetKind string) string {
	reference := `["asset","asset_identity_state_set","subject:one",""]`
	count := map[string]int{
		"character_identity_anchor": 1, "character_appearance": 3, "location_board": 3,
		"prop_sheet": 3, "scene_composition": 1, "interaction_composition": 1,
	}[targetKind]
	parts := make([]string, count+1)
	parts[0] = `"` + targetKind + `"`
	for index := 1; index <= count; index++ {
		parts[index] = reference
	}
	return `[` + strings.Join(parts, ",") + `]`
}

func referenceBriefOwnerRef(workspaceID, projectID, ownerKind, versionFamily, logicalID string) map[string]any {
	return map[string]any{
		"workspace_id": workspaceID, "project_id": projectID, "owner_kind": ownerKind, "version_family": versionFamily,
		"owner_logical_id": logicalID, "owner_version_id": uuid.NewString(), "owner_revision": 1,
		"owner_content_hash": strings.Repeat("d", 64), "fragment_key": nil, "fragment_content_hash": nil,
	}
}

func mustReferenceBriefJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
