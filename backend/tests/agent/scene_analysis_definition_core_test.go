package agent_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestSceneAnalysisDefinitionCoreFreezesOnlyPreReleaseContracts(t *testing.T) {
	core, encoded, err := contract.BuildSceneAnalysisDefinitionCore()
	if err != nil {
		t.Fatalf("build Scene Analysis Definition Core: %v", err)
	}
	if core.ContractID != contract.SceneAnalysisDefinitionCoreContractID ||
		core.WireSchemaID != contract.SceneAnalysisWireSchemaVersion || len(core.VariantContracts) != 10 {
		t.Fatalf("unexpected Scene Analysis Definition Core: %#v", core)
	}
	visualIndex := slices.IndexFunc(core.VariantContracts, func(candidate contract.SceneAnalysisDefinitionVariant) bool {
		return candidate.VariantKey.StageKey == "resolve_visual_foundation"
	})
	if visualIndex < 0 {
		t.Fatal("Visual Foundation Definition variant is missing")
	}
	visual := core.VariantContracts[visualIndex]
	if visual.VariantKey.ProfileKey != "default" || visual.CapabilityKey != "resolve-visual-foundation" ||
		visual.Lane != "preset_visual" || visual.RuntimeClass != "vision" ||
		visual.InputContractID != contract.VisualFoundationInputContractID ||
		visual.InputSchemaHash != contract.VisualFoundationInputSchemaHash ||
		visual.OutputContractID != "visual_foundation_candidate" ||
		visual.OutputSchemaHash != contract.VisualFoundationCandidateSchemaHash {
		t.Fatalf("unexpected Visual Foundation Definition variant: %#v", visual)
	}
	referencePlanIndex := slices.IndexFunc(core.VariantContracts, func(candidate contract.SceneAnalysisDefinitionVariant) bool {
		return candidate.VariantKey.StageKey == "plan_reference_assets"
	})
	if referencePlanIndex < 0 {
		t.Fatal("Reference Plan Definition variant is missing")
	}
	referencePlan := core.VariantContracts[referencePlanIndex]
	if referencePlan.VariantKey.ProfileKey != "default" || referencePlan.CapabilityKey != "plan-reference-assets" ||
		referencePlan.Lane != "preset_visual" || referencePlan.RuntimeClass != "text" ||
		referencePlan.InputContractID != contract.ReferencePlanInputContractID ||
		referencePlan.InputSchemaHash != contract.ReferencePlanInputSchemaHash ||
		referencePlan.OutputContractID != "reference_plan_candidate" ||
		referencePlan.OutputSchemaHash != contract.ReferencePlanCandidateSchemaHash {
		t.Fatalf("unexpected Reference Plan Definition variant: %#v", referencePlan)
	}
	referenceBriefIndex := slices.IndexFunc(core.VariantContracts, func(candidate contract.SceneAnalysisDefinitionVariant) bool {
		return candidate.VariantKey.StageKey == "compile_reference_brief"
	})
	if referenceBriefIndex < 0 {
		t.Fatal("Reference Brief Definition variant is missing")
	}
	referenceBrief := core.VariantContracts[referenceBriefIndex]
	if referenceBrief.VariantKey.ProfileKey != "default" || referenceBrief.CapabilityKey != "compile-reference-brief" ||
		referenceBrief.Lane != "preset_visual" || referenceBrief.RuntimeClass != "text" ||
		referenceBrief.InputContractID != contract.ReferenceBriefInputContractID ||
		referenceBrief.InputSchemaHash != contract.ReferenceBriefInputSchemaHash ||
		referenceBrief.OutputContractID != "reference_brief_candidate" ||
		referenceBrief.OutputSchemaHash != contract.ReferenceBriefCandidateSchemaHash {
		t.Fatalf("unexpected Reference Brief Definition variant: %#v", referenceBrief)
	}
	for _, forbidden := range []string{"stage_release", "skill_release", "signature", "control", "current", "latest"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("Definition Core contains forbidden activation reference %q", forbidden)
		}
	}
	decoded, canonical, err := contract.DecodeSceneAnalysisDefinitionCore(encoded)
	if err != nil {
		t.Fatalf("decode Scene Analysis Definition Core: %v", err)
	}
	if decoded.DefinitionCoreHash != core.DefinitionCoreHash || !bytes.Equal(canonical, encoded) {
		t.Fatal("Definition Core did not round-trip canonically")
	}
}

func TestSceneAnalysisDefinitionCoreRejectsIncompleteOrMutableContracts(t *testing.T) {
	_, encoded, err := contract.BuildSceneAnalysisDefinitionCore()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	variants := payload["variant_contracts"].([]any)

	tests := map[string]func(map[string]any){
		"unknown release field": func(value map[string]any) { value["stage_release_hash"] = strings.Repeat("a", 64) },
		"missing variant":       func(value map[string]any) { value["variant_contracts"] = variants[:len(variants)-1] },
		"mutable resource ref": func(value map[string]any) {
			value["variant_contracts"].([]any)[0].(map[string]any)["resource_policy_ref"] = "current"
		},
		"hash drift": func(value map[string]any) { value["definition_core_hash"] = strings.Repeat("a", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(encoded, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			if _, _, err := contract.DecodeSceneAnalysisDefinitionCore(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid Definition Core was accepted")
			}
		})
	}
}
