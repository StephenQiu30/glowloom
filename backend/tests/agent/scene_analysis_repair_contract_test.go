package agent_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStructureIdentityRepairDirectiveTargetsOneSceneAnalysisStage(t *testing.T) {
	directive := contract.StructureIdentityRepairDirective{
		ReviewDecisionID:    "10000000-0000-4000-8000-000000000001",
		DecisionPayloadHash: strings.Repeat("a", 64),
		IssueRefs:           []string{"issue_identity_ambiguous"},
		EvidenceRefs: []contract.StructureIdentityRepairEvidence{{
			SourceVersionID: "20000000-0000-4000-8000-000000000001",
			SourceStart:     8, SourceEnd: 10, TextHash: strings.Repeat("b", 64),
		}},
		ChangeSpec: contract.StructureIdentityRepairChange{
			Operation: "resolve_mention", TargetKeys: []string{"mention:8:10"},
			AffectedScopeKeys: []string{"scene:30000000-0000-4000-8000-000000000001"},
		},
		ReasonCode: "identity_resolution_incorrect",
	}
	if err := directive.ValidateFor("resolve_identities"); err != nil {
		t.Fatalf("valid identity repair rejected: %v", err)
	}
	if err := directive.ValidateFor("propose_script_spans"); err == nil {
		t.Fatal("identity repair was accepted by the script span stage")
	}

	directive.ChangeSpec.Operation = "adjust_scene_boundary"
	directive.ReasonCode = "structure_boundary_incorrect"
	if err := directive.ValidateFor("propose_script_spans"); err != nil {
		t.Fatalf("valid span repair rejected: %v", err)
	}
	if err := directive.ValidateFor("resolve_identities"); err == nil {
		t.Fatal("span repair was accepted by the identity stage")
	}
}

func TestStructureIdentityRepairDirectiveRejectsExpandedOrUnstableScope(t *testing.T) {
	directive := contract.StructureIdentityRepairDirective{
		ReviewDecisionID:    "10000000-0000-4000-8000-000000000001",
		DecisionPayloadHash: strings.Repeat("a", 64),
		IssueRefs:           []string{"issue_z", "issue_a"},
		EvidenceRefs: []contract.StructureIdentityRepairEvidence{{
			SourceVersionID: "20000000-0000-4000-8000-000000000001",
			SourceStart:     8, SourceEnd: 10, TextHash: strings.Repeat("b", 64),
		}},
		ChangeSpec: contract.StructureIdentityRepairChange{
			Operation: "resolve_mention", TargetKeys: []string{"mention:8:10"},
			AffectedScopeKeys: []string{"project:30000000-0000-4000-8000-000000000001"},
		},
		ReasonCode: "identity_resolution_incorrect",
	}
	if err := directive.ValidateFor("resolve_identities"); err == nil {
		t.Fatal("unsorted issues and project-wide repair scope were accepted")
	}
}
