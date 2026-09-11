package agent_test

import (
	"bytes"
	"encoding/json"
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
		core.WireSchemaID != contract.SceneAnalysisWireSchemaVersion || len(core.VariantContracts) != 7 {
		t.Fatalf("unexpected Scene Analysis Definition Core: %#v", core)
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
		"missing variant":       func(value map[string]any) { value["variant_contracts"] = variants[:6] },
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
