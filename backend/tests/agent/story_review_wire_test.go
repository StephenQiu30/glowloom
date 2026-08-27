package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/google/uuid"
)

func TestStoryGraphReviewInvocationBindsTheExactCandidateAndGate(t *testing.T) {
	targetID := uuid.NewString()
	targetHash := strings.Repeat("a", 64)
	stageInput := contract.StoryGraphReviewStageInput{
		ReviewedStage: "reconcile_story", TargetCandidateRevisionID: targetID,
		TargetCandidateRevisionHash: targetHash, CandidateItemStart: 0, CandidateItemEnd: 1,
		TargetCandidate: json.RawMessage(`{
			"canonical_entities":[{"entity_key":"character:lin-yi","evidence":[]}],
			"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[]
		}`),
		DeterministicGate: contract.StoryGraphDeterministicGateResult{
			GateVersion:               contract.BibleDeterministicGateVersion,
			TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: targetHash,
			Blockers: []contract.StoryGraphGateBlocker{},
		},
	}
	payload := reviewWirePayload(t, "review:0000", "story_review", stageInput)
	payload.UpstreamCandidates = []contract.StageUpstreamCandidateRef{{
		Stage: "reconcile_story", ShardKey: "story-reduce:root", CandidateRevisionID: targetID,
		CandidateRevisionHash: targetHash, SourceInvocationID: uuid.NewString(), SourceResultHash: strings.Repeat("b", 64),
	}}
	if _, err := contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload); err != nil {
		t.Fatalf("valid frozen StoryGraph review invocation was rejected: %v", err)
	}

	payload.UpstreamCandidates[0].CandidateRevisionHash = strings.Repeat("c", 64)
	if _, err := contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload); err == nil {
		t.Fatal("StoryGraph review invocation accepted a candidate revision that drifted from its gate")
	}
}

func TestStoryGraphRepairInvocationBindsTargetReviewAndAllowlist(t *testing.T) {
	targetID, reviewID := uuid.NewString(), uuid.NewString()
	fragment := json.RawMessage(`{"entity_key":"character:lin-yi","canonical_name":"林一"}`)
	fragmentHash, err := contract.StoryGraphCandidateFragmentHash(fragment)
	if err != nil {
		t.Fatal(err)
	}
	input := contract.StoryGraphRepairStageInput{
		TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: strings.Repeat("a", 64),
		ReviewCandidateRevisionID: reviewID, ReviewCandidateRevisionHash: strings.Repeat("b", 64),
		TargetIssue: contract.StoryGraphReviewIssue{
			IssueKey: "issue:canonical", Code: "canonical_name", Severity: "blocking", Scope: "entity",
			SubjectKey: wireStringPointer("character:lin-yi"), Summary: "规范名冲突",
			Evidence: []contract.StoryGraphEvidence{{SourceStart: 0, SourceEnd: 2, TextHash: strings.Repeat("c", 64), ExactAnchor: "林一"}},
		},
		AllowedTargets: []contract.StoryGraphRepairAllowedTarget{{
			CandidateKey: "character:lin-yi", AllowedFields: []string{"canonical_name"},
			BaseFragmentHash: fragmentHash, Fragment: fragment,
		}},
		ReadOnlyAdjacency: []contract.StoryGraphRepairReadOnlyFragment{}, RepairRound: 1, MaxRepairRounds: 2,
	}
	payload := reviewWirePayload(t, "repair:0000", "candidate_repair", input)
	payload.UpstreamCandidates = []contract.StageUpstreamCandidateRef{
		{Stage: "reconcile_story", ShardKey: "story-reduce:root", CandidateRevisionID: targetID, CandidateRevisionHash: input.TargetCandidateRevisionHash, SourceInvocationID: uuid.NewString(), SourceResultHash: strings.Repeat("d", 64)},
		{Stage: "review_storygraph", ShardKey: "review:0000", CandidateRevisionID: reviewID, CandidateRevisionHash: input.ReviewCandidateRevisionHash, SourceInvocationID: uuid.NewString(), SourceResultHash: strings.Repeat("e", 64)},
	}
	if _, err = contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload); err != nil {
		t.Fatalf("valid frozen StoryGraph repair invocation was rejected: %v", err)
	}
	published := payload
	published.BaseStoryGraphVersionID = uuid.NewString()
	published.BaseStoryGraphHash = strings.Repeat("2", 64)
	if _, err = contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), published); err == nil {
		t.Fatal("StoryGraph repair invocation accepted a published StoryGraph target")
	}

	payload.UpstreamCandidates = payload.UpstreamCandidates[:1]
	if _, err = contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload); err == nil {
		t.Fatal("StoryGraph repair invocation accepted a missing exact review revision")
	}
}

func reviewWirePayload(t *testing.T, shardKey, shardKind string, stageInput any) contract.StageInvocationPayload {
	t.Helper()
	encoded, err := json.Marshal(stageInput)
	if err != nil {
		t.Fatal(err)
	}
	stage := "review_storygraph"
	if shardKind == "candidate_repair" {
		stage = "repair_candidate"
	}
	return contract.StageInvocationPayload{
		Stage: stage, ShardKey: shardKey, WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		SourceRefs: []contract.StageSourceRef{{
			OwnerKind: "production/script", OwnerLogicalID: uuid.NewString(), OwnerVersionID: uuid.NewString(),
			Revision: 1, ContentHash: strings.Repeat("f", 64),
		}},
		UpstreamCandidates: []contract.StageUpstreamCandidateRef{},
		ShardManifestRef:   contract.ShardManifestRef{ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("1", 64)},
		Shard:              contract.InvocationShard{Kind: shardKind, Key: shardKey, TreePath: shardKey}, StageInput: encoded,
	}
}

func wireStringPointer(value string) *string { return &value }
