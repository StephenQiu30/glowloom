package planning_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func TestEpisodeAnalysisManifestUsesSceneBoundariesAndOneReduceRootPerEpisode(t *testing.T) {
	firstText := "内景 客厅 日\n阿澜进门。\n内景 厨房 夜\n炉火熄灭。"
	secondText := "外景 河岸 晨\n阿澜发现脚印。"
	firstStart := 40
	secondStart := firstStart + len([]rune(firstText))
	input := domain.EpisodeAnalysisManifestInput{
		AnalyzeManifestID:   "73000000-0000-0000-0000-000000000001",
		ReconcileManifestID: "73000000-0000-0000-0000-000000000002",
		WorkspaceID:         "73000000-0000-0000-0000-000000000003",
		WorkflowRunID:       "73000000-0000-0000-0000-000000000004",
		NodeRunID:           "73000000-0000-0000-0000-000000000005",
		RootInputHash:       strings.Repeat("a", 64),
		MaxShardCodePoints:  18,
		OverlapCodePoints:   3,
		FanIn:               2,
		Episodes: []domain.EpisodeAnalysisSource{
			{
				EpisodeID: "73000000-0000-0000-0000-000000000010", EpisodePosition: 1,
				ScriptVersionID: "73000000-0000-0000-0000-000000000011",
				ContentHash:     planningTextHash(firstText), SourceStart: firstStart,
				SourceEnd: firstStart + len([]rune(firstText)), Content: firstText,
				SceneMarkers: []domain.EpisodeSceneMarker{
					{Label: "内景 客厅 日", AbsoluteStart: firstStart, AbsoluteEnd: firstStart + len([]rune("内景 客厅 日"))},
					{Label: "内景 厨房 夜", AbsoluteStart: firstStart + len([]rune("内景 客厅 日\n阿澜进门。\n")), AbsoluteEnd: firstStart + len([]rune("内景 客厅 日\n阿澜进门。\n内景 厨房 夜"))},
				},
			},
			{
				EpisodeID: "73000000-0000-0000-0000-000000000020", EpisodePosition: 2,
				ScriptVersionID: "73000000-0000-0000-0000-000000000021",
				ContentHash:     planningTextHash(secondText), SourceStart: secondStart,
				SourceEnd: secondStart + len([]rune(secondText)), Content: secondText,
				SceneMarkers: []domain.EpisodeSceneMarker{{
					Label: "外景 河岸 晨", AbsoluteStart: secondStart,
					AbsoluteEnd: secondStart + len([]rune("外景 河岸 晨")),
				}},
			},
		},
	}

	analyze, reconcile, err := domain.BuildEpisodeAnalysisManifests(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(analyze.Shards) < 3 {
		t.Fatalf("expected bounded fan-out across two Episodes, got %#v", analyze.Shards)
	}
	position := map[int]int{1: input.Episodes[0].SourceStart, 2: input.Episodes[1].SourceStart}
	ends := map[int]int{1: input.Episodes[0].SourceEnd, 2: input.Episodes[1].SourceEnd}
	for _, shard := range analyze.Shards {
		if shard.LogicalStart != position[shard.EpisodePosition] {
			t.Fatalf("Episode %d has a gap or overlap at %#v", shard.EpisodePosition, shard)
		}
		if shard.LogicalEnd-shard.LogicalStart > input.MaxShardCodePoints {
			t.Fatalf("shard exceeds the accepted bound: %#v", shard)
		}
		if shard.ContextStart < input.Episodes[shard.EpisodePosition-1].SourceStart ||
			shard.ContextEnd > input.Episodes[shard.EpisodePosition-1].SourceEnd {
			t.Fatalf("context escaped its Episode: %#v", shard)
		}
		position[shard.EpisodePosition] = shard.LogicalEnd
	}
	for episode, end := range ends {
		if position[episode] != end {
			t.Fatalf("Episode %d coverage ended at %d, want %d", episode, position[episode], end)
		}
	}
	if len(reconcile.Roots) != 2 || reconcile.Roots[0].EpisodePosition != 1 || reconcile.Roots[1].EpisodePosition != 2 ||
		reconcile.Roots[0].ShardKey == reconcile.Roots[1].ShardKey {
		t.Fatalf("expected one deterministic reduce root per Episode, got %#v", reconcile.Roots)
	}

	reversed := input
	reversed.AnalyzeManifestID = "73000000-0000-0000-0000-000000000101"
	reversed.ReconcileManifestID = "73000000-0000-0000-0000-000000000102"
	reversed.Episodes = []domain.EpisodeAnalysisSource{input.Episodes[1], input.Episodes[0]}
	rebuiltAnalyze, rebuiltReconcile, err := domain.BuildEpisodeAnalysisManifests(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(analyze.Shards, rebuiltAnalyze.Shards) || !reflect.DeepEqual(reconcile.Roots, rebuiltReconcile.Roots) {
		t.Fatal("Episode analysis partition or roots changed with input order")
	}
}

func TestEpisodeAnalysisManifestRejectsDuplicateEpisodePosition(t *testing.T) {
	input := domain.EpisodeAnalysisManifestInput{
		AnalyzeManifestID: "73000000-0000-0000-0000-000000000001", ReconcileManifestID: "73000000-0000-0000-0000-000000000002",
		WorkspaceID: "73000000-0000-0000-0000-000000000003", WorkflowRunID: "73000000-0000-0000-0000-000000000004",
		NodeRunID: "73000000-0000-0000-0000-000000000005", RootInputHash: strings.Repeat("a", 64),
		MaxShardCodePoints: 10, OverlapCodePoints: 2, FanIn: 2,
		Episodes: []domain.EpisodeAnalysisSource{
			{EpisodeID: "73000000-0000-0000-0000-000000000010", EpisodePosition: 1, ScriptVersionID: "73000000-0000-0000-0000-000000000011", ContentHash: planningTextHash("甲乙丙"), SourceStart: 0, SourceEnd: 3, Content: "甲乙丙"},
			{EpisodeID: "73000000-0000-0000-0000-000000000020", EpisodePosition: 1, ScriptVersionID: "73000000-0000-0000-0000-000000000021", ContentHash: planningTextHash("丁戊己"), SourceStart: 3, SourceEnd: 6, Content: "丁戊己"},
		},
	}
	if _, _, err := domain.BuildEpisodeAnalysisManifests(input); err == nil {
		t.Fatal("duplicate Episode position was accepted")
	}
}

func TestEpisodeCandidatesRejectUnknownIdentityAndReconcileLoss(t *testing.T) {
	text := "内景 客厅 日\n阿澜：我回来了。"
	start := 100
	end := start + len([]rune(text))
	scope := domain.EpisodeCandidateScope{
		EpisodeID: "75000000-0000-0000-0000-000000000001", EpisodePosition: 2,
		ScriptVersionID: "75000000-0000-0000-0000-000000000002",
		SourceStart:     start, SourceEnd: end, ContextStart: start, ContextText: text,
		KnownIdentities: []domain.EpisodeCandidateIdentity{{
			EntityKey: "character:alan", StateKeys: []string{"base"},
		}},
	}
	evidence := map[string]any{
		"source_start": start, "source_end": end, "text_hash": planningTextHash(text),
		"exact_anchor": text, "episode_number": 2,
	}
	candidateMap := map[string]any{
		"episode_id": scope.EpisodeID, "script_version_id": scope.ScriptVersionID,
		"logical_start": start, "logical_end": end,
		"fragments": []any{map[string]any{
			"temporary_key": "scene:0001", "kind": "scene",
			"source_keys":  []string{"episode:" + scope.EpisodeID},
			"source_start": start, "source_end": end, "summary": "阿澜回到客厅",
			"evidence": []any{evidence},
			"attributes": map[string]any{
				"scene_key": nil, "speaker_key": nil,
				"participant_keys": []string{"character:alan"}, "location_key": nil,
				"time_hint": "日", "dialogue_text": nil, "action": "阿澜进入客厅",
				"occurrence_entity_key": nil, "state_key": nil, "continuity_notes": []string{},
			},
		}},
		"claims": []any{}, "review_issues": []any{},
	}
	raw, _ := json.Marshal(candidateMap)
	analysis, err := domain.DecodeEpisodeAnalysisCandidate(raw, scope)
	if err != nil {
		t.Fatal(err)
	}
	unknown := candidateMap["fragments"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	unknown["participant_keys"] = []string{"character:invented"}
	unknownRaw, _ := json.Marshal(candidateMap)
	if _, err = domain.DecodeEpisodeAnalysisCandidate(unknownRaw, scope); err == nil {
		t.Fatal("Backend accepted an unknown Episode candidate identity")
	}

	reconcileMap := map[string]any{
		"episode_id": scope.EpisodeID, "script_version_id": scope.ScriptVersionID,
		"source_start": start, "source_end": end,
		"ordered_fragments": []any{}, "claims": []any{}, "conflicts": []any{}, "review_issues": []any{},
	}
	reconcileRaw, _ := json.Marshal(reconcileMap)
	if _, err = domain.DecodeEpisodeReconciliationCandidate(
		reconcileRaw, scope, []domain.EpisodeAnalysisCandidate{analysis}, nil,
	); err == nil {
		t.Fatal("Backend accepted reconciliation that dropped an exact child fragment")
	}
}

func planningTextHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
