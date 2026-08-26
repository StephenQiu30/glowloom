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

func TestHumanGateOutputMatchesImmutableCandidateForOtherGates(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ReferenceID:      "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "2",
		ContentHash:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ReferenceID:      candidate.ReferenceID,
		ReferenceVersion: "3",
		ContentHash:      candidate.ContentHash,
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.production_bible_review", candidate, output) {
		t.Fatal("existing human gates must keep matching the frozen candidate hash")
	}
	output.ContentHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if workflow.HumanGateOutputMatchesCandidate("gate.production_bible_review", candidate, output) {
		t.Fatal("existing human gate accepted a different content hash")
	}
}
