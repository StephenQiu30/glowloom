package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestEpisodeSegmentationInvocationBindsScriptMaterializationAndEvidenceLeaves(t *testing.T) {
	leaf := contract.EpisodeSegmentationEvidenceLeaf{
		ShardKey: "source:00000000:00000012", CandidateRevisionID: uuid.NewString(),
		CandidateRevisionHash: strings.Repeat("a", 64),
	}
	marker := contract.EpisodeSegmentationEvidence{
		SourceStart: 0, SourceEnd: 3, TextHash: strings.Repeat("b", 64), ExactAnchor: "第一集", EpisodeNumber: intWirePointer(1),
	}
	input := contract.EpisodeSegmentationStageInput{
		DocumentRevisionID: uuid.NewString(), NormalizedHash: strings.Repeat("c", 64), SourceCodePoints: 24,
		TargetDurationMS: 90_000, BibleVersionID: uuid.NewString(), BibleVersion: 1,
		BibleContentHash: strings.Repeat("d", 64), MaterializationHash: strings.Repeat("e", 64),
		EvidenceAggregateRevisionID: uuid.NewString(), EvidenceAggregateRevisionHash: strings.Repeat("f", 64),
		EvidenceLeaves: []contract.EpisodeSegmentationEvidenceLeaf{leaf},
		MarkerHints:    []contract.EpisodeSegmentationMarkerHint{{EpisodeNumber: 1, Label: "第一集", Evidence: marker}},
		EvidenceIndex: []contract.EpisodeSegmentationEvidenceIndexItem{{
			IndexKey: "marker:0000", Kind: "marker", Label: "第一集", ShardKey: leaf.ShardKey,
			CandidateRevisionID: leaf.CandidateRevisionID, CandidateRevisionHash: leaf.CandidateRevisionHash, Evidence: marker,
		}},
	}
	stageInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 0, input.SourceCodePoints
	payload := contract.StageInvocationPayload{
		Stage: "segment_episodes", ShardKey: "episode-segmentation:global", WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		SourceRefs: []contract.StageSourceRef{
			{OwnerKind: "production/script", OwnerLogicalID: uuid.NewString(), OwnerVersionID: input.DocumentRevisionID, Revision: 1, ContentHash: input.NormalizedHash},
			{OwnerKind: "production/bible-materialization", OwnerLogicalID: input.BibleVersionID, OwnerVersionID: input.BibleVersionID, Revision: int64(input.BibleVersion), ContentHash: input.MaterializationHash},
		},
		UpstreamCandidates: []contract.StageUpstreamCandidateRef{{
			Stage: "extract_source_evidence", ShardKey: leaf.ShardKey, CandidateRevisionID: leaf.CandidateRevisionID,
			CandidateRevisionHash: leaf.CandidateRevisionHash, SourceInvocationID: uuid.NewString(), SourceResultHash: strings.Repeat("1", 64),
		}},
		ShardManifestRef: contract.ShardManifestRef{ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("2", 64)},
		Shard:            contract.InvocationShard{Kind: "episode_segmentation", Key: "episode-segmentation:global", TreePath: "global", AbsoluteStart: &start, AbsoluteEnd: &end},
		StageInput:       stageInput,
	}
	invocation, err := contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload)
	if err != nil {
		t.Fatalf("valid Episode segmentation invocation rejected: %v", err)
	}
	if err = contract.ValidateEpisodeSegmentationInvocation(invocation); err != nil {
		t.Fatal(err)
	}

	payload.UpstreamCandidates[0].CandidateRevisionHash = strings.Repeat("3", 64)
	if _, err = contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), payload); err == nil {
		t.Fatal("Episode segmentation accepted a leaf revision that drifted from its bounded evidence index")
	}
}

func intWirePointer(value int) *int { return &value }
