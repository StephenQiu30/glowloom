package storyboard_test

import (
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func TestBuildApprovedIntentSetFreezesCanonicalIntentsAndVisualRequirements(t *testing.T) {
	set := domain.DraftSet{
		ID: "81000000-0000-0000-0000-000000000001", WorkspaceID: "81000000-0000-0000-0000-000000000002",
		ProjectID: "81000000-0000-0000-0000-000000000003", GraphVersionID: "81000000-0000-0000-0000-000000000004",
		GraphVersionNo: 3, GraphContentHash: hashOf("1"), ManifestID: "81000000-0000-0000-0000-000000000005",
		ManifestVersion: 1, ManifestHash: hashOf("2"), Revision: 2,
	}
	candidateID, candidateHash := "81000000-0000-0000-0000-000000000006", hashOf("3")
	set.CandidateRevisionID, set.CandidateRevisionHash = &candidateID, &candidateHash
	set.Batches = []domain.DraftSetBatch{
		draftSetBatch("scene:b", "81000000-0000-0000-0000-000000000012", "81000000-0000-0000-0000-000000000022"),
		draftSetBatch("scene:a", "81000000-0000-0000-0000-000000000011", "81000000-0000-0000-0000-000000000021"),
	}
	batches := []domain.Batch{
		intentBatch(set, set.Batches[0], "shot:b", "occurrence:b"),
		intentBatch(set, set.Batches[1], "shot:a", "occurrence:a"),
	}

	approved, err := domain.BuildApprovedIntentSet(
		set, batches, 1, "81000000-0000-0000-0000-000000000007", "81000000-0000-0000-0000-000000000008",
	)
	if err != nil {
		t.Fatal(err)
	}
	if approved.SchemaVersion != "approved-storyboard-intents-v1" || approved.ID != "81000000-0000-0000-0000-000000000008" ||
		approved.DraftSetID != set.ID || approved.DraftSetRevision != set.Revision ||
		approved.CandidateRevisionID != candidateID || approved.CandidateRevisionHash != candidateHash ||
		approved.CandidateRevision != 1 || len(approved.Scenes) != 2 ||
		approved.Scenes[0].SceneStoryNodeKey != "scene:a" || approved.Scenes[0].ShotIntents[0].ShotKey != "shot:a" ||
		approved.Scenes[1].SceneStoryNodeKey != "scene:b" || approved.Scenes[1].ShotIntents[0].ShotKey != "shot:b" ||
		len(approved.VisualRequirementsHash) != 64 || len(approved.ContentHash) != 64 ||
		approved.VisualRequirementsHash == approved.ContentHash {
		t.Fatalf("approved Storyboard Intent Set is incomplete: %#v", approved)
	}

	reversed, err := domain.BuildApprovedIntentSet(
		set, []domain.Batch{batches[1], batches[0]}, 1,
		"81000000-0000-0000-0000-000000000007", "81000000-0000-0000-0000-000000000008",
	)
	if err != nil {
		t.Fatal(err)
	}
	if reversed.ContentHash != approved.ContentHash || reversed.VisualRequirementsHash != approved.VisualRequirementsHash {
		t.Fatalf("freeze hashes depend on repository order: first=%#v reversed=%#v", approved, reversed)
	}
}

func draftSetBatch(sceneKey, batchID, candidateID string) domain.DraftSetBatch {
	resultHash, candidateHash := hashOf("4"), hashOf("5")
	return domain.DraftSetBatch{
		BatchID: batchID, EpisodeID: "81000000-0000-0000-0000-000000000031",
		StructureID: "81000000-0000-0000-0000-000000000032", ScriptVersionID: "81000000-0000-0000-0000-000000000033",
		SceneStoryNodeKey: sceneKey, InputHash: hashOf("6"), ResultHash: &resultHash,
		CandidateRevisionID: &candidateID, CandidateRevisionHash: &candidateHash,
	}
}

func intentBatch(set domain.DraftSet, reference domain.DraftSetBatch, shotKey, occurrenceKey string) domain.Batch {
	return domain.Batch{
		ID: reference.BatchID, WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID,
		EpisodeID: reference.EpisodeID, StructureID: reference.StructureID, ScriptVersionID: reference.ScriptVersionID,
		WorkflowRunID: set.WorkflowRunID, NodeRunID: set.NodeRunID, ManifestID: set.ManifestID,
		ManifestVersion: set.ManifestVersion, GraphVersionID: set.GraphVersionID, GraphVersionNo: set.GraphVersionNo,
		SceneStoryNodeKey: reference.SceneStoryNodeKey, Status: "needs_asset", InputHash: reference.InputHash,
		ResultHash: reference.ResultHash, CandidateRevisionID: reference.CandidateRevisionID,
		CandidateRevisionHash: reference.CandidateRevisionHash,
		Candidate: domain.Candidate{SceneStoryNodeKey: reference.SceneStoryNodeKey, AssetReadiness: "needs_asset", ShotIntents: []domain.ShotIntent{{
			ShotKey: shotKey, IntentOrder: 1, Purpose: "推进剧情", ProposedDurationMS: 1200,
			VisualRequirements: []domain.VisualRequirement{{
				OccurrenceStoryNodeKey: occurrenceKey, IdentityStoryNodeKey: "identity:" + occurrenceKey,
				SpecificationStoryNodeKey: "specification:" + occurrenceKey, AssetStateStoryNodeKey: "state:" + occurrenceKey,
				AssetID: "81000000-0000-0000-0000-000000000041", SpecificationVersionID: "81000000-0000-0000-0000-000000000042",
				AssetStateID: "81000000-0000-0000-0000-000000000043", AssetRole: "subject",
				RequiredViewRoles: []string{"front", "profile", "back"}, AssetReadiness: "needs_asset",
			}},
		}}},
	}
}

func hashOf(character string) string {
	result := ""
	for len(result) < 64 {
		result += character
	}
	return result[:64]
}
