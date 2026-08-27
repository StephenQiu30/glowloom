package workflow_test

import (
	"testing"

	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateOutputMatchesStoryboardCandidateWithAppliedRevision(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ReferenceID:      "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "2",
		ContentHash:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ReferenceID:      candidate.ReferenceID,
		ReferenceVersion: "3",
		ContentHash:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard applied output must preserve Set identity and advance from candidate revision")
	}
	output.ReferenceVersion = candidate.ReferenceVersion
	if workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard applied output accepted without advancing the Set revision")
	}
	output.ReferenceVersion = "3"
	output.ContentHash = candidate.ContentHash
	if workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard applied output accepted the candidate hash as the formal Shot hash")
	}
}

func TestProductionBibleHumanGateOutputCreatesAnImmutableVersionIdentity(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ValueType:        "story_reconciliation_candidate",
		ReferenceID:      "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "2",
		ContentHash:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ValueType:        "production_bible_version",
		ReferenceID:      "77bb2559-b058-4309-957b-6f656e096945",
		ReferenceVersion: "1",
		ContentHash:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.production_bible_review", candidate, output) {
		t.Fatal("Production Bible gate must accept a distinct immutable Version bound by owner evidence")
	}
	output.ReferenceID = candidate.ReferenceID
	if workflow.HumanGateOutputMatchesCandidate("gate.production_bible_review", candidate, output) {
		t.Fatal("Production Bible gate accepted the Candidate identity as a formal Version identity")
	}
}
