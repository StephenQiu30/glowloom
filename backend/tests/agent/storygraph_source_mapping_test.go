package agent_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

func TestStoryGraphSourceMappingRejectsQuarantinedSkillsWithoutCopyingBytes(t *testing.T) {
	review, _, err := contract.PinnedStoryGraphSourceReview()
	if err != nil {
		t.Fatal(err)
	}
	mapping, canonical, err := contract.PinnedStoryGraphSourceMapping()
	if err != nil || len(mapping.Mappings) != len(review.Reviews) || len(canonical) == 0 {
		t.Fatalf("load pinned StoryGraph Source Mapping: mapping=%#v err=%v", mapping, err)
	}
	if mapping.SourceInventoryHash != review.SourceInventoryHash || mapping.SourceReviewRoot != review.SourceReviewRoot {
		t.Fatalf("Source Mapping is not bound to the reviewed inventory: %#v", mapping)
	}

	want := map[string]struct {
		reviewDecision, classification, artifactKind, reason string
	}{
		"agent-skills-specification": {
			"reference_only", "reference_only", "reference_knowledge", "normative_specification_only",
		},
		"anthropic-skill-creator": {
			"quarantined", "reject", "none", "quarantined_active_agent_and_tool_instructions",
		},
		"libtv-public-skill": {
			"quarantined", "reject", "none", "quarantined_remote_service_and_file_transfer",
		},
	}
	for index, sourceMapping := range mapping.Mappings {
		sourceReview := review.Reviews[index]
		expected, exists := want[sourceMapping.SourceID]
		if !exists || sourceMapping.SourceID != sourceReview.SourceID ||
			sourceMapping.SourceCommit != sourceReview.SourceCommit ||
			sourceMapping.ReviewDecision != expected.reviewDecision ||
			sourceMapping.Classification != expected.classification ||
			sourceMapping.ArtifactKind != expected.artifactKind ||
			sourceMapping.DispositionReason != expected.reason ||
			len(sourceMapping.CopiedProductionPaths) != 0 || len(sourceMapping.RewrittenProductionPaths) != 0 {
			t.Fatalf("unexpected external source mapping: %#v", sourceMapping)
		}
	}

	decoded, roundTrip, err := contract.DecodeStoryGraphSourceMapping(canonical)
	if err != nil || decoded.SourceMappingRoot != mapping.SourceMappingRoot || !slices.Equal(roundTrip, canonical) {
		t.Fatalf("Source Mapping did not round-trip: decoded=%#v err=%v", decoded, err)
	}
}

func TestStoryGraphSourceMappingRejectsCopyAndClassificationBypassWithValidRoot(t *testing.T) {
	_, canonical, err := contract.PinnedStoryGraphSourceMapping()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"unknown field": func(value map[string]any) { value["active"] = true },
		"review drift": func(value map[string]any) {
			value["source_review_root"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"review decision drift": func(value map[string]any) {
			value["mappings"].([]any)[0].(map[string]any)["review_decision"] = "quarantined"
		},
		"quarantine reference bypass": func(value map[string]any) {
			mapping := value["mappings"].([]any)[1].(map[string]any)
			mapping["classification"] = "reference_only"
			mapping["artifact_kind"] = "reference_knowledge"
		},
		"quarantine rewrite bypass": func(value map[string]any) {
			mapping := value["mappings"].([]any)[2].(map[string]any)
			mapping["classification"] = "rewrite"
			mapping["artifact_kind"] = "typed_recipe"
			mapping["rewritten_production_paths"] = []any{"agent/skills/build-storygraph/SKILL.md"}
		},
		"copied runtime bytes": func(value map[string]any) {
			value["mappings"].([]any)[0].(map[string]any)["copied_production_paths"] = []any{
				"agent/skills/build-storygraph/references/external.md",
			}
		},
		"reference writes production": func(value map[string]any) {
			value["mappings"].([]any)[0].(map[string]any)["rewritten_production_paths"] = []any{
				"agent/skills/build-storygraph/SKILL.md",
			}
		},
		"duplicate mapping": func(value map[string]any) {
			mappings := value["mappings"].([]any)
			mappings[1].(map[string]any)["source_id"] = mappings[0].(map[string]any)["source_id"]
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(canonical, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			refreshSourceMappingRoot(t, candidate)
			if _, _, err := contract.DecodeStoryGraphSourceMapping(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid external Source Mapping was accepted")
			}
		})
	}
}

func refreshSourceMappingRoot(t *testing.T, value map[string]any) {
	t.Helper()
	delete(value, "source_mapping_root")
	hash, err := platformcanonical.Hash(mustJSON(t, value))
	if err != nil {
		t.Fatal(err)
	}
	value["source_mapping_root"] = hash
}
