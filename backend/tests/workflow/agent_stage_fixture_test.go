package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func mustStageInvocationRecord(
	t *testing.T,
	id, workspaceID, requestID uuid.UUID,
	requestType, stage, status string,
	now time.Time,
) model.AgentInvocation {
	t.Helper()
	policy := contract.StoryGraphDefinition().ExecutionPolicy()
	payload := contract.StageInvocationPayload{
		Stage: stage, ShardKey: "test-shard", WorkspaceID: workspaceID.String(), ProjectID: uuid.NewString(),
		SourceRefs: []contract.StageSourceRef{}, UpstreamCandidates: []contract.StageUpstreamCandidateRef{},
		ShardManifestRef: contract.ShardManifestRef{ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("c", 64)},
		Shard:            contract.InvocationShard{Kind: "test", Key: "test-shard", TreePath: "0"}, StageInput: json.RawMessage(`{}`),
	}
	if stage == "draft_storyboard" {
		payload = storyboardInvocationFixture(t, workspaceID)
	}
	invocation, err := contract.NewStageInvocation(id.String(), policy, payload)
	if err != nil {
		t.Fatal(err)
	}
	stageInstanceKey, err := invocation.StageInstanceKey()
	if err != nil {
		t.Fatal(err)
	}
	encodedPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return model.AgentInvocation{
		ID: id, WorkspaceID: workspaceID, RequestType: requestType, RequestID: requestID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.StoryGraphWireSchemaVersion,
		Stage: stage, ShardKey: payload.ShardKey, StageInstanceKey: stageInstanceKey,
		ShardManifestHash: payload.ShardManifestRef.Hash, InputHash: invocation.InputHash,
		ExecutionPolicy: encodedPolicy, Payload: encodedPayload,
		Status: status, CreatedAt: now, UpdatedAt: now,
	}
}

func storyboardInvocationFixture(t *testing.T, workspaceID uuid.UUID) contract.StageInvocationPayload {
	t.Helper()
	projectID, graphID, styleID := uuid.New(), uuid.New(), uuid.New()
	documentRevisionID, sceneOwnerID, episodeID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	assetID, specificationID, stateID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	graphHash, styleHash, ownerHash, evidenceHash := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)
	nodeKey := func(value string) string { return "sgn_" + strings.Repeat(value, 64) }
	evidence := []contract.StoryboardEvidenceRef{{
		DocumentRevisionID: documentRevisionID, AbsoluteStart: 0, AbsoluteEnd: 4, TextHash: evidenceHash,
	}}
	stageInput, err := json.Marshal(contract.StoryboardDraftStageInput{
		GraphVersionNo: 1,
		Scene: contract.StoryboardSceneInput{
			StoryNodeKey: nodeKey("1"), OwnerVersionID: sceneOwnerID, OwnerRevision: 1, OwnerHash: ownerHash,
			EpisodeID: episodeID, EpisodePosition: 1, ScenePosition: 1, Heading: "雨巷，夜", Evidence: evidence,
		},
		Beats: []contract.StoryboardBeatInput{{
			StoryNodeKey: nodeKey("2"), Summary: "人物进入雨巷", RequiredForCoverage: true, Evidence: evidence,
		}},
		Dialogues: []contract.StoryboardDialogueInput{},
		Occurrences: []contract.StoryboardOccurrenceInput{{
			StoryNodeKey: nodeKey("3"), IdentityStoryNodeKey: nodeKey("4"),
			SpecificationStoryNodeKey: nodeKey("5"), AssetStateStoryNodeKey: nodeKey("6"),
			AssetID: assetID, SpecificationVersionID: specificationID, AssetStateID: stateID,
			AssetKind: "character", Summary: "人物出现", Evidence: evidence,
		}},
		EffectiveStyleSnapshot: contract.StoryboardStyleSnapshotInput{
			OwnerVersionID: styleID.String(), Revision: 1, ContentHash: styleHash,
			VisualStyle: "cinematic noir", AspectRatio: "9:16",
		},
		TargetDurationMS: 90_000, AssetVersions: []contract.StoryboardAssetVersionInput{},
	})
	if err != nil {
		t.Fatalf("encode exact Storyboard input: %v", err)
	}
	return contract.StageInvocationPayload{
		Stage: "draft_storyboard", ShardKey: "scene:" + nodeKey("1"),
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(),
		SourceRefs: []contract.StageSourceRef{
			{OwnerKind: "production/storygraph", OwnerLogicalID: projectID.String(), OwnerVersionID: graphID.String(), Revision: 1, ContentHash: graphHash},
			{OwnerKind: "preset/effective-style", OwnerLogicalID: projectID.String(), OwnerVersionID: styleID.String(), Revision: 1, ContentHash: styleHash},
		},
		BaseStoryGraphVersionID: graphID.String(), BaseStoryGraphHash: graphHash,
		UpstreamCandidates: []contract.StageUpstreamCandidateRef{},
		ShardManifestRef:   contract.ShardManifestRef{ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("c", 64)},
		Shard: contract.InvocationShard{
			Kind: "story_scene", Key: "scene:" + nodeKey("1"), TreePath: "scene/0001",
		},
		StageInput: stageInput,
	}
}
