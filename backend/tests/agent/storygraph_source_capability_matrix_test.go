package agent_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

func TestStoryGraphSourceCapabilityMatrixAllowsOnlyReviewedRewriteCapabilities(t *testing.T) {
	mapping, _, err := contract.PinnedStoryGraphSourceMapping()
	if err != nil {
		t.Fatal(err)
	}
	matrix, canonical, err := contract.PinnedStoryGraphSourceCapabilityMatrix()
	if err != nil {
		t.Fatal(err)
	}
	wantCapabilities := []string{
		"build-production-bible",
		"design-reference-assets",
		"direct-storyboard",
		"map-scene-continuity",
		"parse-script-structure",
		"resolve-visual-foundation",
		"review-production",
	}
	if matrix.SourceMappingRoot != mapping.SourceMappingRoot ||
		!slices.Equal(matrix.AllowedCapabilities, wantCapabilities) || len(matrix.RewriteMappings) != 0 ||
		matrix.RuntimePolicy.ExternalSkillDownload || matrix.RuntimePolicy.NetworkDiscovery ||
		matrix.RuntimePolicy.SourceProjectPathLoading || len(canonical) == 0 {
		t.Fatalf("unexpected external Skill capability matrix: %#v", matrix)
	}
	for _, source := range mapping.Mappings {
		if source.Classification == "rewrite" {
			t.Fatalf("reviewed rewrite source is missing from the capability matrix: %#v", source)
		}
	}
}

func TestStoryGraphSourceCapabilityMatrixRejectsRuntimeAndCapabilityBypass(t *testing.T) {
	_, canonical, err := contract.PinnedStoryGraphSourceCapabilityMatrix()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"unknown capability": func(value map[string]any) {
			value["allowed_capabilities"].([]any)[0] = "arbitrary-tool-runner"
		},
		"missing capability": func(value map[string]any) {
			value["allowed_capabilities"] = value["allowed_capabilities"].([]any)[1:]
		},
		"reference source enters rewrite": func(value map[string]any) {
			value["rewrite_mappings"] = []any{map[string]any{
				"source_id":                  "agent-skills-specification",
				"capability":                 "parse-script-structure",
				"rewritten_production_paths": []any{"agent/skills/build-storygraph/SKILL.md"},
			}}
		},
		"runtime downloads external skill": func(value map[string]any) {
			value["runtime_policy"].(map[string]any)["external_skill_download"] = true
		},
		"runtime discovers network guidance": func(value map[string]any) {
			value["runtime_policy"].(map[string]any)["network_discovery"] = true
		},
		"runtime loads source project path": func(value map[string]any) {
			value["runtime_policy"].(map[string]any)["source_project_path_loading"] = true
		},
		"unknown field": func(value map[string]any) { value["active"] = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(canonical, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			delete(candidate, "capability_matrix_root")
			hash, hashErr := platformcanonical.Hash(mustJSON(t, candidate))
			if hashErr != nil {
				t.Fatal(hashErr)
			}
			candidate["capability_matrix_root"] = hash
			if _, _, err = contract.DecodeStoryGraphSourceCapabilityMatrix(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid external Skill capability matrix was accepted")
			}
		})
	}
}
