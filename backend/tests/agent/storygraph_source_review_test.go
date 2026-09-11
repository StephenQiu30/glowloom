package agent_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

func TestStoryGraphSourceReviewQuarantinesExecutableExternalSkills(t *testing.T) {
	inventory, _, err := contract.PinnedStoryGraphSourceInventory()
	if err != nil {
		t.Fatal(err)
	}
	review, canonical, err := contract.PinnedStoryGraphSourceReview()
	if err != nil || len(review.Reviews) != len(inventory.Sources) || len(canonical) == 0 {
		t.Fatalf("load pinned StoryGraph Source Review: review=%#v err=%v", review, err)
	}
	if review.SourceInventoryHash != inventory.SourceInventoryHash ||
		review.ReviewBasis != "fixed_source_and_license_bytes" || review.ReviewerID != "project-owner" {
		t.Fatalf("Source Review is not bound to the reviewed inventory: %#v", review)
	}

	want := map[string]struct {
		decision string
		risks    contract.ExternalSkillSourceRisks
		findings []string
	}{
		"agent-skills-specification": {
			decision: "reference_only",
			findings: []string{
				"executable_resource_examples",
				"network_requirement_examples",
				"normative_specification",
				"tool_declaration_examples",
			},
		},
		"anthropic-skill-creator": {
			decision: "quarantined",
			risks: contract.ExternalSkillSourceRisks{
				InstructionInjection: true,
				ExcessToolAuthority:  true,
			},
			findings: []string{
				"active_agent_instructions",
				"browser_review_instructions",
				"filesystem_write_instructions",
				"skill_installation_instructions",
				"subagent_execution_instructions",
			},
		},
		"libtv-public-skill": {
			decision: "quarantined",
			risks: contract.ExternalSkillSourceRisks{
				ImplicitNetworkAccess: true,
				ExcessToolAuthority:   true,
			},
			findings: []string{
				"executable_script_calls",
				"external_network_service",
				"external_session_control",
				"local_file_upload",
				"remote_artifact_download",
			},
		},
	}
	for index, sourceReview := range review.Reviews {
		source := inventory.Sources[index]
		expected, exists := want[sourceReview.SourceID]
		if !exists || sourceReview.SourceID != source.SourceID ||
			sourceReview.SourceCommit != source.SourceCommit ||
			sourceReview.SourceFileSHA256 != source.SourceFileSHA256 ||
			sourceReview.LicenseSHA256 != source.License.SHA256 ||
			sourceReview.Decision != expected.decision || sourceReview.Risks != expected.risks ||
			!slices.Equal(sourceReview.Findings, expected.findings) {
			t.Fatalf("unexpected external source review: %#v", sourceReview)
		}
		if sourceReview.RewriteQueueAllowed || sourceReview.ProductionBundleAllowed ||
			sourceReview.RuntimeExecutionAllowed {
			t.Fatalf("external source escaped quarantine: %#v", sourceReview)
		}
	}

	decoded, roundTrip, err := contract.DecodeStoryGraphSourceReview(canonical)
	if err != nil || decoded.SourceReviewRoot != review.SourceReviewRoot || !slices.Equal(roundTrip, canonical) {
		t.Fatalf("Source Review did not round-trip: decoded=%#v err=%v", decoded, err)
	}
}

func TestStoryGraphSourceReviewRejectsPolicyBypassWithValidContentRoot(t *testing.T) {
	_, canonical, err := contract.PinnedStoryGraphSourceReview()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"unknown field": func(value map[string]any) { value["active"] = true },
		"unclear license bypass": func(value map[string]any) {
			review := value["reviews"].([]any)[0].(map[string]any)
			review["risks"].(map[string]any)["license_unclear"] = true
			review["findings"] = []any{
				"executable_resource_examples", "license_unresolved", "network_requirement_examples",
				"normative_specification", "tool_declaration_examples",
			}
		},
		"prohibited redistribution bypass": func(value map[string]any) {
			review := value["reviews"].([]any)[0].(map[string]any)
			review["risks"].(map[string]any)["redistribution_prohibited"] = true
			review["findings"] = []any{
				"executable_resource_examples", "network_requirement_examples", "normative_specification",
				"redistribution_prohibited", "tool_declaration_examples",
			}
		},
		"embedded credentials bypass": func(value map[string]any) {
			review := value["reviews"].([]any)[0].(map[string]any)
			review["risks"].(map[string]any)["embedded_credentials"] = true
			review["findings"] = []any{
				"embedded_credentials", "executable_resource_examples", "network_requirement_examples",
				"normative_specification", "tool_declaration_examples",
			}
		},
		"untraceable source bypass": func(value map[string]any) {
			review := value["reviews"].([]any)[0].(map[string]any)
			review["risks"].(map[string]any)["untraceable_source"] = true
			review["findings"] = []any{
				"executable_resource_examples", "network_requirement_examples", "normative_specification",
				"tool_declaration_examples", "untraceable_source",
			}
		},
		"inventory drift": func(value map[string]any) {
			value["source_inventory_hash"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"source bytes drift": func(value map[string]any) {
			value["reviews"].([]any)[0].(map[string]any)["source_file_sha256"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"production Bundle bypass": func(value map[string]any) {
			value["reviews"].([]any)[0].(map[string]any)["production_bundle_allowed"] = true
		},
		"rewrite queue bypass": func(value map[string]any) {
			value["reviews"].([]any)[1].(map[string]any)["rewrite_queue_allowed"] = true
		},
		"runtime execution bypass": func(value map[string]any) {
			value["reviews"].([]any)[2].(map[string]any)["runtime_execution_allowed"] = true
		},
		"unsafe source downgraded": func(value map[string]any) {
			value["reviews"].([]any)[1].(map[string]any)["decision"] = "reference_only"
		},
		"unsafe risk hidden": func(value map[string]any) {
			value["reviews"].([]any)[2].(map[string]any)["risks"].(map[string]any)["implicit_network_access"] = false
		},
		"duplicate review": func(value map[string]any) {
			reviews := value["reviews"].([]any)
			reviews[1].(map[string]any)["source_id"] = reviews[0].(map[string]any)["source_id"]
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(canonical, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			refreshSourceReviewRoot(t, candidate)
			if _, _, err := contract.DecodeStoryGraphSourceReview(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid external Source Review was accepted")
			}
		})
	}
}

func refreshSourceReviewRoot(t *testing.T, value map[string]any) {
	t.Helper()
	delete(value, "source_review_root")
	raw := mustJSON(t, value)
	hash, err := platformcanonical.Hash(raw)
	if err != nil {
		t.Fatal(err)
	}
	value["source_review_root"] = hash
}
