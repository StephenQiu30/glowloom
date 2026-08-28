package agent_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestEpisodeAnalysisWireBindsPublishedSliceBibleMaterializationAndNeighbors(t *testing.T) {
	contextText := "内景 客厅 日\n阿澜：我回来了。"
	logicalStart := 100
	logicalEnd := logicalStart + len([]rune(contextText))
	scriptContentHash := sourceTextHash(contextText)
	bibleSnapshot := json.RawMessage(`{"canonical_entities":[],"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[]}`)
	bibleSnapshotHash, err := agentcontract.CanonicalHash(bibleSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	stageInput, err := json.Marshal(agentcontract.EpisodeAnalysisStageInput{
		EpisodeID: "72000000-0000-0000-0000-000000000010", EpisodePosition: 2,
		ScriptVersionID: "72000000-0000-0000-0000-000000000011", ScriptVersionNo: 1,
		DocumentRevisionID: "72000000-0000-0000-0000-000000000012",
		EpisodeSourceStart: logicalStart, EpisodeSourceEnd: logicalEnd,
		ScriptContentHash: scriptContentHash, LogicalStart: logicalStart, LogicalEnd: logicalEnd,
		ContextStart: logicalStart, ContextEnd: logicalEnd, ContextText: contextText,
		LogicalTextHash: sourceTextHash(contextText),
		SceneMarkerHints: []agentcontract.EpisodeSceneMarkerHint{{
			Label: "内景 客厅 日", AbsoluteStart: logicalStart, AbsoluteEnd: logicalStart + len([]rune("内景 客厅 日")),
		}},
		AdjacentEpisodes: []agentcontract.EpisodeAdjacentContext{{
			Side: "previous", EpisodeID: "72000000-0000-0000-0000-000000000020", EpisodePosition: 1,
			ScriptVersionID: "72000000-0000-0000-0000-000000000021", ScriptVersionNo: 1,
			SourceStart: 0, SourceEnd: logicalStart, ContentHash: strings.Repeat("2", 64),
			ExcerptStart: logicalStart - 4, ExcerptEnd: logicalStart, Excerpt: "雨停了。",
			ExcerptHash: sourceTextHash("雨停了。"),
		}},
		BibleVersionID: "72000000-0000-0000-0000-000000000030", BibleVersion: 1,
		BibleContentHash: strings.Repeat("3", 64), BibleSnapshotHash: bibleSnapshotHash,
		BibleSnapshot:       bibleSnapshot,
		MaterializationHash: strings.Repeat("5", 64),
		KnownIdentities: []agentcontract.EpisodeKnownIdentity{{
			EntityKey: "character:alan", Kind: "character",
			AssetID:                "72000000-0000-0000-0000-000000000040",
			SpecificationVersionID: "72000000-0000-0000-0000-000000000041", SpecificationHash: strings.Repeat("6", 64),
			States: []agentcontract.EpisodeKnownState{{
				StateKey: "base", AssetStateID: "72000000-0000-0000-0000-000000000042", ContentHash: strings.Repeat("7", 64),
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	start, end := logicalStart, logicalEnd
	invocation, err := agentcontract.NewStageInvocation(
		"72000000-0000-0000-0000-000000000001",
		agentcontract.StoryGraphDefinition().ExecutionPolicy(),
		agentcontract.StageInvocationPayload{
			Stage: "analyze_episode", ShardKey: "episode:0002:map:0000",
			WorkspaceID: "72000000-0000-0000-0000-000000000002", ProjectID: "72000000-0000-0000-0000-000000000003",
			SourceRefs: []agentcontract.StageSourceRef{
				{OwnerKind: "production/episode-script", OwnerLogicalID: "72000000-0000-0000-0000-000000000010", OwnerVersionID: "72000000-0000-0000-0000-000000000011", Revision: 1, ContentHash: scriptContentHash},
				{OwnerKind: "production/episode-script", OwnerLogicalID: "72000000-0000-0000-0000-000000000020", OwnerVersionID: "72000000-0000-0000-0000-000000000021", Revision: 1, ContentHash: strings.Repeat("2", 64)},
				{OwnerKind: "production/bible-version", OwnerLogicalID: "72000000-0000-0000-0000-000000000030", OwnerVersionID: "72000000-0000-0000-0000-000000000030", Revision: 1, ContentHash: strings.Repeat("3", 64)},
				{OwnerKind: "production/bible-materialization", OwnerLogicalID: "72000000-0000-0000-0000-000000000030", OwnerVersionID: "72000000-0000-0000-0000-000000000030", Revision: 1, ContentHash: strings.Repeat("5", 64)},
			},
			ShardManifestRef: agentcontract.ShardManifestRef{ManifestID: "72000000-0000-0000-0000-000000000004", Version: 1, Hash: strings.Repeat("8", 64)},
			Shard:            agentcontract.InvocationShard{Kind: "episode_map", Key: "episode:0002:map:0000", TreePath: "episode/0002/map/0000", AbsoluteStart: &start, AbsoluteEnd: &end},
			StageInput:       stageInput,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = agentcontract.ValidateEpisodeAnalysisInvocation(invocation); err != nil {
		t.Fatal(err)
	}

	var drifted agentcontract.EpisodeAnalysisStageInput
	if err = json.Unmarshal(stageInput, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted.LogicalEnd--
	invocation.Payload.StageInput, _ = json.Marshal(drifted)
	invocation.InputHash, _ = invocation.ComputeInputHash()
	if err = agentcontract.ValidateEpisodeAnalysisInvocation(invocation); err == nil {
		t.Fatal("Episode analysis accepted a shard range that drifted from the published slice")
	}
}

func sourceTextHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
