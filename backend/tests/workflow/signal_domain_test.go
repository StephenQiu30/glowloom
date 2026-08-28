package workflow_test

import (
	"testing"

	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateOutputMatchesStoryboardIntentFreezeReceipt(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ValueType:        "storyboard_intent_candidate_set",
		ReferenceID:      "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "1",
		ContentHash:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ValueType:        "approved_storyboard_intents",
		ReferenceID:      "77bb2559-b058-4309-957b-6f656e096945",
		ReferenceVersion: "1",
		ContentHash:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard Intent gate must publish a distinct frozen intent receipt reference")
	}
	output.ReferenceID = candidate.ReferenceID
	if workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard Intent gate accepted the Agent Candidate as the frozen Owner result")
	}
	output.ReferenceID = "77bb2559-b058-4309-957b-6f656e096945"
	output.ContentHash = candidate.ContentHash
	if workflow.HumanGateOutputMatchesCandidate("gate.storyboard_review", candidate, output) {
		t.Fatal("Storyboard Intent gate accepted the unfrozen Candidate hash as Owner output")
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

func TestEpisodePlanHumanGateOutputUsesMaterializedSetIdentityAndHash(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ValueType: "episode_segmentation_candidate", ReferenceID: "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "1", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ValueType: "episode_set", ReferenceID: "77bb2559-b058-4309-957b-6f656e096945",
		ReferenceVersion: "1", ContentHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.episode_plan_review", candidate, output) {
		t.Fatal("Episode Plan gate must publish a distinct materialized Episode set")
	}
	output.ReferenceID = candidate.ReferenceID
	if workflow.HumanGateOutputMatchesCandidate("gate.episode_plan_review", candidate, output) {
		t.Fatal("Episode Plan gate accepted the Agent Candidate as the formal Episode set")
	}
}

func TestEpisodePlanningHumanGateOutputUsesDistinctOwnerSetIdentityAndHash(t *testing.T) {
	candidate := workflow.NodeInputBinding{
		ValueType: "episode_planning_candidate_set", ReferenceID: "47fb8568-bbb9-4071-898a-01bcf812356c",
		ReferenceVersion: "1", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	output := workflow.NodeOutputBinding{
		ValueType: "planning_owner_set", ReferenceID: "77bb2559-b058-4309-957b-6f656e096945",
		ReferenceVersion: "1", ContentHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if !workflow.HumanGateOutputMatchesCandidate("gate.episode_structure_review", candidate, output) {
		t.Fatal("Episode Planning gate must publish a distinct formal owner set")
	}
	output.ReferenceID = candidate.ReferenceID
	if workflow.HumanGateOutputMatchesCandidate("gate.episode_structure_review", candidate, output) {
		t.Fatal("Episode Planning gate accepted its Agent Candidate as formal facts")
	}
}
