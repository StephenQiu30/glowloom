package planning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type episodeAnalysisRepository struct {
	seed        planningapp.EpisodeAnalysisSeed
	preparation planningapp.EpisodeAnalysisPreparation
}

func (repo *episodeAnalysisRepository) LoadEpisodeAnalysisSeed(
	context.Context,
	planningapp.EpisodeAnalysisCommand,
) (planningapp.EpisodeAnalysisSeed, error) {
	return repo.seed, nil
}

func (repo *episodeAnalysisRepository) EnsureEpisodeAnalysis(
	_ context.Context,
	preparation planningapp.EpisodeAnalysisPreparation,
) (planningapp.EpisodeAnalysisState, error) {
	repo.preparation = preparation
	return planningapp.EpisodeAnalysisState{Status: "pending"}, nil
}

func TestEpisodeAnalysisServiceFreezesAdjacentContextBibleAndKnownIdentities(t *testing.T) {
	firstText := "内景 客厅 日\n阿澜：雨停了。"
	secondText := "外景 河岸 晨\n阿澜发现脚印。"
	firstEnd := len([]rune(firstText))
	snapshot := json.RawMessage(`{"canonical_entities":[],"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[]}`)
	snapshotHash, err := agentcontract.CanonicalHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repo := &episodeAnalysisRepository{seed: planningapp.EpisodeAnalysisSeed{
		WorkspaceID:    "74000000-0000-0000-0000-000000000001",
		ProjectID:      "74000000-0000-0000-0000-000000000002",
		WorkflowRunID:  "74000000-0000-0000-0000-000000000003",
		NodeRunID:      "74000000-0000-0000-0000-000000000004",
		EpisodeSetID:   "74000000-0000-0000-0000-000000000005",
		EpisodeSetHash: strings.Repeat("a", 64),
		Episodes: []planningapp.EpisodeAnalysisEpisodeSeed{
			{
				Source: domain.EpisodeAnalysisSource{
					EpisodeID: "74000000-0000-0000-0000-000000000010", EpisodePosition: 1,
					ScriptVersionID: "74000000-0000-0000-0000-000000000011",
					ContentHash:     planningTextHash(firstText), SourceStart: 0, SourceEnd: firstEnd, Content: firstText,
					SceneMarkers: []domain.EpisodeSceneMarker{{Label: "内景 客厅 日", AbsoluteStart: 0, AbsoluteEnd: len([]rune("内景 客厅 日"))}},
				},
				ScriptVersionNo: 1, DocumentRevisionID: "74000000-0000-0000-0000-000000000012",
			},
			{
				Source: domain.EpisodeAnalysisSource{
					EpisodeID: "74000000-0000-0000-0000-000000000020", EpisodePosition: 2,
					ScriptVersionID: "74000000-0000-0000-0000-000000000021",
					ContentHash:     planningTextHash(secondText), SourceStart: firstEnd,
					SourceEnd: firstEnd + len([]rune(secondText)), Content: secondText,
					SceneMarkers: []domain.EpisodeSceneMarker{{Label: "外景 河岸 晨", AbsoluteStart: firstEnd, AbsoluteEnd: firstEnd + len([]rune("外景 河岸 晨"))}},
				},
				ScriptVersionNo: 1, DocumentRevisionID: "74000000-0000-0000-0000-000000000012",
			},
		},
		BibleVersionID: "74000000-0000-0000-0000-000000000030", BibleVersion: 1,
		BibleContentHash: strings.Repeat("b", 64), BibleSnapshotHash: snapshotHash,
		BibleSnapshot: snapshot, MaterializationHash: strings.Repeat("c", 64),
		KnownIdentities: []agentcontract.EpisodeKnownIdentity{{
			EntityKey: "character:alan", Kind: "character",
			AssetID:                "74000000-0000-0000-0000-000000000040",
			SpecificationVersionID: "74000000-0000-0000-0000-000000000041",
			SpecificationHash:      strings.Repeat("d", 64),
			States: []agentcontract.EpisodeKnownState{{
				StateKey: "base", AssetStateID: "74000000-0000-0000-0000-000000000042",
				ContentHash: strings.Repeat("e", 64),
			}},
		}},
	}}
	service := planningapp.NewEpisodeAnalysisService(repo, planningapp.EpisodeAnalysisConfig{
		Now:                func() time.Time { return time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC) },
		NewID:              episodeAnalysisIDs(),
		MaxShardCodePoints: 64, OverlapCodePoints: 4, AdjacentCodePoints: 5, FanIn: 2,
	})
	state, err := service.Ensure(context.Background(), planningapp.EpisodeAnalysisCommand{
		WorkspaceID: repo.seed.WorkspaceID, ProjectID: repo.seed.ProjectID,
		WorkflowRunID: repo.seed.WorkflowRunID, NodeRunID: repo.seed.NodeRunID,
		EpisodeSetID: repo.seed.EpisodeSetID, EpisodeSetHash: repo.seed.EpisodeSetHash,
		BibleVersionID: repo.seed.BibleVersionID, BibleVersion: repo.seed.BibleVersion,
		MaterializationHash: repo.seed.MaterializationHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "pending" || len(repo.preparation.Invocations) != 2 {
		t.Fatalf("unexpected Episode analysis preparation: %#v %#v", state, repo.preparation)
	}
	var payload agentcontract.StageInvocationPayload
	if err = json.Unmarshal(repo.preparation.Invocations[1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var stageInput agentcontract.EpisodeAnalysisStageInput
	if err = json.Unmarshal(payload.StageInput, &stageInput); err != nil {
		t.Fatal(err)
	}
	if len(stageInput.AdjacentEpisodes) != 1 || stageInput.AdjacentEpisodes[0].Side != "previous" ||
		stageInput.AdjacentEpisodes[0].Excerpt != string([]rune(firstText)[firstEnd-5:]) {
		t.Fatalf("adjacent Episode boundary was not frozen: %#v", stageInput.AdjacentEpisodes)
	}
	if stageInput.BibleSnapshotHash != snapshotHash || len(stageInput.KnownIdentities) != 1 ||
		stageInput.KnownIdentities[0].EntityKey != "character:alan" {
		t.Fatalf("Bible or identity index drifted: %#v", stageInput)
	}
	if len(payload.SourceRefs) != 4 {
		t.Fatalf("expected current Episode, adjacent Episode, Bible and materialization refs: %#v", payload.SourceRefs)
	}
	firstAnalyzeID, firstAnalyzeHash := repo.preparation.AnalyzeManifest.ManifestID, repo.preparation.AnalyzeManifest.ManifestHash
	firstReconcileID, firstReconcileHash := repo.preparation.ReconcileManifest.ManifestID, repo.preparation.ReconcileManifest.ManifestHash
	if _, err = service.Ensure(context.Background(), repo.preparation.Command); err != nil {
		t.Fatalf("re-enter Episode analysis service: %v", err)
	}
	if repo.preparation.AnalyzeManifest.ManifestID != firstAnalyzeID ||
		repo.preparation.AnalyzeManifest.ManifestHash != firstAnalyzeHash ||
		repo.preparation.ReconcileManifest.ManifestID != firstReconcileID ||
		repo.preparation.ReconcileManifest.ManifestHash != firstReconcileHash {
		t.Fatalf("Episode analysis manifests changed across the same NodeRun")
	}
}

func episodeAnalysisIDs() func() string {
	index := 0
	return func() string {
		index++
		return fmt.Sprintf("74000000-0000-0000-0000-%012d", 100+index)
	}
}
