package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	AnalyzeEpisodeStage   = "analyze_episode"
	ReconcileEpisodeStage = "reconcile_episode"
)

var episodeAnalysisHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type EpisodeSceneMarker struct {
	Label         string `json:"label"`
	AbsoluteStart int    `json:"absolute_start"`
	AbsoluteEnd   int    `json:"absolute_end"`
}

type EpisodeAnalysisSource struct {
	EpisodeID       string               `json:"episode_id"`
	EpisodePosition int                  `json:"episode_position"`
	ScriptVersionID string               `json:"script_version_id"`
	ContentHash     string               `json:"content_hash"`
	SourceStart     int                  `json:"source_start"`
	SourceEnd       int                  `json:"source_end"`
	Content         string               `json:"content"`
	SceneMarkers    []EpisodeSceneMarker `json:"scene_markers"`
}

type EpisodeAnalysisShard struct {
	Key                string `json:"shard_key"`
	TreePath           string `json:"tree_path"`
	ParentKey          string `json:"parent_shard_key,omitempty"`
	Kind               string `json:"kind"`
	EpisodeID          string `json:"episode_id"`
	EpisodePosition    int    `json:"episode_position"`
	ScriptVersionID    string `json:"script_version_id"`
	LogicalStart       int    `json:"logical_start"`
	LogicalEnd         int    `json:"logical_end"`
	ContextStart       int    `json:"context_start"`
	ContextEnd         int    `json:"context_end"`
	SourceContentHash  string `json:"source_content_hash"`
	InputHash          string `json:"input_hash"`
	MaxShardCodePoints int    `json:"max_shard_code_points"`
	OverlapCodePoints  int    `json:"overlap_code_points"`
	Status             string `json:"status"`
}

type EpisodeReconcileChild struct {
	Stage      string `json:"stage"`
	ShardKey   string `json:"shard_key"`
	SourceHash string `json:"source_hash"`
}

type EpisodeReconcileShard struct {
	Key             string                  `json:"shard_key"`
	TreePath        string                  `json:"tree_path"`
	ParentKey       string                  `json:"parent_shard_key,omitempty"`
	Kind            string                  `json:"kind"`
	EpisodeID       string                  `json:"episode_id"`
	EpisodePosition int                     `json:"episode_position"`
	Level           int                     `json:"level"`
	Children        []EpisodeReconcileChild `json:"children"`
	SubtreeHash     string                  `json:"subtree_hash"`
	Status          string                  `json:"status"`
}

type EpisodeReconcileRoot struct {
	EpisodeID       string `json:"episode_id"`
	EpisodePosition int    `json:"episode_position"`
	ShardKey        string `json:"shard_key"`
	SubtreeHash     string `json:"subtree_hash"`
}

type EpisodeAnalysisManifest struct {
	ManifestID         string                 `json:"manifest_id"`
	Version            int64                  `json:"version"`
	ParentManifestHash *string                `json:"parent_manifest_hash"`
	WorkspaceID        string                 `json:"workspace_id"`
	WorkflowRunID      string                 `json:"workflow_run_id"`
	NodeRunID          string                 `json:"node_run_id"`
	Stage              string                 `json:"stage"`
	RootInputHash      string                 `json:"root_input_hash"`
	MaxShardCodePoints int                    `json:"max_shard_code_points"`
	OverlapCodePoints  int                    `json:"overlap_code_points"`
	Shards             []EpisodeAnalysisShard `json:"shards"`
	CoverageHash       string                 `json:"coverage_hash"`
	ManifestHash       string                 `json:"manifest_hash"`
}

type EpisodeReconcileManifest struct {
	ManifestID         string                  `json:"manifest_id"`
	Version            int64                   `json:"version"`
	ParentManifestHash *string                 `json:"parent_manifest_hash"`
	WorkspaceID        string                  `json:"workspace_id"`
	WorkflowRunID      string                  `json:"workflow_run_id"`
	NodeRunID          string                  `json:"node_run_id"`
	Stage              string                  `json:"stage"`
	RootInputHash      string                  `json:"root_input_hash"`
	FanIn              int                     `json:"fan_in"`
	Roots              []EpisodeReconcileRoot  `json:"roots"`
	Shards             []EpisodeReconcileShard `json:"shards"`
	CoverageHash       string                  `json:"coverage_hash"`
	ManifestHash       string                  `json:"manifest_hash"`
}

type EpisodeAnalysisManifestInput struct {
	AnalyzeManifestID, ReconcileManifestID               string
	WorkspaceID, WorkflowRunID, NodeRunID, RootInputHash string
	MaxShardCodePoints, OverlapCodePoints, FanIn         int
	Episodes                                             []EpisodeAnalysisSource
}

func BuildEpisodeAnalysisManifests(
	input EpisodeAnalysisManifestInput,
) (EpisodeAnalysisManifest, EpisodeReconcileManifest, error) {
	for _, identifier := range []string{
		input.AnalyzeManifestID, input.ReconcileManifestID, input.WorkspaceID,
		input.WorkflowRunID, input.NodeRunID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, errors.New("invalid Episode analysis manifest identity")
		}
	}
	if !episodeAnalysisHashPattern.MatchString(input.RootInputHash) || input.MaxShardCodePoints < 1 ||
		input.OverlapCodePoints < 0 || input.FanIn != 2 || len(input.Episodes) == 0 {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, errors.New("invalid Episode analysis manifest input")
	}
	episodes := append([]EpisodeAnalysisSource(nil), input.Episodes...)
	slices.SortFunc(episodes, func(left, right EpisodeAnalysisSource) int {
		if left.EpisodePosition != right.EpisodePosition {
			return left.EpisodePosition - right.EpisodePosition
		}
		return strings.Compare(left.EpisodeID, right.EpisodeID)
	})
	analyzeShards := make([]EpisodeAnalysisShard, 0, len(episodes))
	reconcileShards := make([]EpisodeReconcileShard, 0, len(episodes))
	roots := make([]EpisodeReconcileRoot, 0, len(episodes))
	previousSourceEnd := -1
	for episodeIndex, episode := range episodes {
		if err := validateEpisodeAnalysisSource(episode, episodeIndex+1, previousSourceEnd); err != nil {
			return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
		}
		previousSourceEnd = episode.SourceEnd
		ranges := episodeAnalysisRanges(episode, input.MaxShardCodePoints)
		current := make([]EpisodeReconcileChild, len(ranges))
		for shardIndex, sourceRange := range ranges {
			key := fmt.Sprintf("episode:%04d:map:%04d", episode.EpisodePosition, shardIndex)
			shard := EpisodeAnalysisShard{
				Key: key, TreePath: fmt.Sprintf("episode.%04d.map.%04d", episode.EpisodePosition, shardIndex),
				Kind: "episode_map", EpisodeID: episode.EpisodeID,
				EpisodePosition: episode.EpisodePosition, ScriptVersionID: episode.ScriptVersionID,
				LogicalStart: sourceRange[0], LogicalEnd: sourceRange[1],
				ContextStart:      max(episode.SourceStart, sourceRange[0]-input.OverlapCodePoints),
				ContextEnd:        min(episode.SourceEnd, sourceRange[1]+input.OverlapCodePoints),
				SourceContentHash: episode.ContentHash, Status: "active",
				MaxShardCodePoints: input.MaxShardCodePoints, OverlapCodePoints: input.OverlapCodePoints,
			}
			shard.InputHash, _ = episodeAnalysisCanonicalHash(struct {
				Schema string               `json:"schema"`
				Shard  EpisodeAnalysisShard `json:"shard"`
			}{"episode-analysis-shard", shard})
			analyzeShards = append(analyzeShards, shard)
			current[shardIndex] = EpisodeReconcileChild{
				Stage: AnalyzeEpisodeStage, ShardKey: key, SourceHash: shard.InputHash,
			}
		}
		level := 0
		for {
			next := make([]EpisodeReconcileChild, 0, (len(current)+input.FanIn-1)/input.FanIn)
			for start := 0; start < len(current); start += input.FanIn {
				end := min(start+input.FanIn, len(current))
				children := append([]EpisodeReconcileChild(nil), current[start:end]...)
				key := fmt.Sprintf("episode:%04d:reduce:%04d:%04d", episode.EpisodePosition, level, len(next))
				subtreeHash, err := episodeAnalysisCanonicalHash(struct {
					Schema          string                  `json:"schema"`
					EpisodeID       string                  `json:"episode_id"`
					EpisodePosition int                     `json:"episode_position"`
					Level           int                     `json:"level"`
					Children        []EpisodeReconcileChild `json:"children"`
				}{"episode-reconcile-subtree", episode.EpisodeID, episode.EpisodePosition, level, children})
				if err != nil {
					return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
				}
				reconcileShards = append(reconcileShards, EpisodeReconcileShard{
					Key: key, TreePath: fmt.Sprintf("episode.%04d.reduce.%04d.%04d", episode.EpisodePosition, level, len(next)),
					Kind: "episode_reduce", EpisodeID: episode.EpisodeID,
					EpisodePosition: episode.EpisodePosition, Level: level,
					Children: children, SubtreeHash: subtreeHash, Status: "active",
				})
				next = append(next, EpisodeReconcileChild{
					Stage: ReconcileEpisodeStage, ShardKey: key, SourceHash: subtreeHash,
				})
			}
			if len(next) == 1 {
				root := next[0]
				roots = append(roots, EpisodeReconcileRoot{
					EpisodeID: episode.EpisodeID, EpisodePosition: episode.EpisodePosition,
					ShardKey: root.ShardKey, SubtreeHash: root.SourceHash,
				})
				break
			}
			current = next
			level++
		}
	}
	parents := make(map[string]string, len(analyzeShards)+len(reconcileShards))
	for _, shard := range reconcileShards {
		for _, child := range shard.Children {
			parents[child.Stage+"\x00"+child.ShardKey] = shard.Key
		}
	}
	rootKeys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootKeys[root.ShardKey] = struct{}{}
	}
	for index := range analyzeShards {
		analyzeShards[index].ParentKey = parents[AnalyzeEpisodeStage+"\x00"+analyzeShards[index].Key]
	}
	for index := range reconcileShards {
		if _, root := rootKeys[reconcileShards[index].Key]; !root {
			reconcileShards[index].ParentKey = parents[ReconcileEpisodeStage+"\x00"+reconcileShards[index].Key]
		}
	}
	coverageHash, err := episodeAnalysisCanonicalHash(struct {
		Schema   string                  `json:"schema"`
		Episodes []EpisodeAnalysisSource `json:"episodes"`
		Shards   []EpisodeAnalysisShard  `json:"shards"`
	}{"episode-analysis-coverage", episodes, analyzeShards})
	if err != nil {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
	}
	analyze := EpisodeAnalysisManifest{
		ManifestID: input.AnalyzeManifestID, Version: 1, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID,
		Stage: AnalyzeEpisodeStage, RootInputHash: input.RootInputHash,
		MaxShardCodePoints: input.MaxShardCodePoints, OverlapCodePoints: input.OverlapCodePoints,
		Shards: analyzeShards, CoverageHash: coverageHash,
	}
	analyze.ManifestHash, err = episodeAnalysisManifestHash(analyze)
	if err != nil {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
	}
	reconcileCoverage, err := episodeAnalysisCanonicalHash(struct {
		Schema string                 `json:"schema"`
		Roots  []EpisodeReconcileRoot `json:"roots"`
	}{"episode-reconcile-coverage", roots})
	if err != nil {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
	}
	reconcile := EpisodeReconcileManifest{
		ManifestID: input.ReconcileManifestID, Version: 1, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID,
		Stage: ReconcileEpisodeStage, RootInputHash: input.RootInputHash,
		FanIn: input.FanIn, Roots: roots, Shards: reconcileShards, CoverageHash: reconcileCoverage,
	}
	reconcile.ManifestHash, err = episodeReconcileManifestHash(reconcile)
	if err != nil {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
	}
	if err = ValidateEpisodeAnalysisManifests(analyze, reconcile); err != nil {
		return EpisodeAnalysisManifest{}, EpisodeReconcileManifest{}, err
	}
	return analyze, reconcile, nil
}

func ValidateEpisodeAnalysisManifests(
	analyze EpisodeAnalysisManifest,
	reconcile EpisodeReconcileManifest,
) error {
	if analyze.Stage != AnalyzeEpisodeStage || reconcile.Stage != ReconcileEpisodeStage ||
		analyze.Version < 1 || reconcile.Version < 1 || analyze.WorkspaceID != reconcile.WorkspaceID ||
		analyze.WorkflowRunID != reconcile.WorkflowRunID || analyze.NodeRunID != reconcile.NodeRunID ||
		analyze.RootInputHash != reconcile.RootInputHash || analyze.MaxShardCodePoints < 1 ||
		analyze.OverlapCodePoints < 0 || reconcile.FanIn != 2 || len(analyze.Shards) == 0 ||
		len(reconcile.Shards) == 0 || len(reconcile.Roots) == 0 ||
		!episodeAnalysisHashPattern.MatchString(analyze.RootInputHash) ||
		!episodeAnalysisHashPattern.MatchString(analyze.CoverageHash) ||
		!episodeAnalysisHashPattern.MatchString(reconcile.CoverageHash) {
		return errors.New("invalid Episode analysis manifest pair")
	}
	for _, identifier := range []string{
		analyze.ManifestID, reconcile.ManifestID, analyze.WorkspaceID,
		analyze.WorkflowRunID, analyze.NodeRunID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode analysis manifest identity")
		}
	}
	analyzeHash, err := episodeAnalysisManifestHash(analyze)
	if err != nil || analyzeHash != analyze.ManifestHash {
		return errors.New("Episode analysis manifest hash mismatch")
	}
	reconcileHash, err := episodeReconcileManifestHash(reconcile)
	if err != nil || reconcileHash != reconcile.ManifestHash {
		return errors.New("Episode reconciliation manifest hash mismatch")
	}
	positions := make(map[int]int)
	for _, shard := range analyze.Shards {
		if shard.Kind != "episode_map" || shard.Status != "active" || shard.EpisodePosition < 1 ||
			shard.LogicalEnd <= shard.LogicalStart || shard.LogicalEnd-shard.LogicalStart > analyze.MaxShardCodePoints ||
			shard.ContextStart > shard.LogicalStart || shard.ContextEnd < shard.LogicalEnd ||
			shard.MaxShardCodePoints != analyze.MaxShardCodePoints || shard.OverlapCodePoints != analyze.OverlapCodePoints ||
			!episodeAnalysisHashPattern.MatchString(shard.SourceContentHash) ||
			!episodeAnalysisHashPattern.MatchString(shard.InputHash) {
			return errors.New("invalid Episode analysis shard")
		}
		if previous, exists := positions[shard.EpisodePosition]; exists && previous != shard.LogicalStart {
			return errors.New("Episode analysis shard coverage contains a gap or overlap")
		}
		positions[shard.EpisodePosition] = shard.LogicalEnd
	}
	rootKeys := make(map[string]struct{}, len(reconcile.Roots))
	for index, root := range reconcile.Roots {
		if root.EpisodePosition != index+1 || strings.TrimSpace(root.ShardKey) == "" ||
			!episodeAnalysisHashPattern.MatchString(root.SubtreeHash) {
			return errors.New("invalid Episode reconciliation root")
		}
		rootKeys[root.ShardKey] = struct{}{}
	}
	for _, shard := range reconcile.Shards {
		if shard.Kind != "episode_reduce" || shard.Status != "active" || shard.Level < 0 ||
			len(shard.Children) < 1 || len(shard.Children) > reconcile.FanIn ||
			!episodeAnalysisHashPattern.MatchString(shard.SubtreeHash) {
			return errors.New("invalid Episode reconciliation shard")
		}
		if _, root := rootKeys[shard.Key]; root && shard.ParentKey != "" {
			return errors.New("Episode reconciliation root cannot have a parent")
		}
		for _, child := range shard.Children {
			if !oneOfAnalysisStage(child.Stage, AnalyzeEpisodeStage, ReconcileEpisodeStage) ||
				strings.TrimSpace(child.ShardKey) == "" || !episodeAnalysisHashPattern.MatchString(child.SourceHash) {
				return errors.New("invalid Episode reconciliation child")
			}
		}
	}
	return nil
}

func oneOfAnalysisStage(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func validateEpisodeAnalysisSource(value EpisodeAnalysisSource, expectedPosition, previousEnd int) error {
	for _, identifier := range []string{value.EpisodeID, value.ScriptVersionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode analysis source identity")
		}
	}
	if value.EpisodePosition != expectedPosition || value.SourceStart < 0 || value.SourceEnd <= value.SourceStart ||
		value.SourceEnd-value.SourceStart != len([]rune(value.Content)) ||
		!episodeAnalysisHashPattern.MatchString(value.ContentHash) || episodeAnalysisTextHash(value.Content) != value.ContentHash ||
		previousEnd >= 0 && value.SourceStart != previousEnd {
		return errors.New("invalid Episode analysis published source coverage")
	}
	previousMarkerStart := -1
	runes := []rune(value.Content)
	for _, marker := range value.SceneMarkers {
		if strings.TrimSpace(marker.Label) == "" || marker.AbsoluteStart <= previousMarkerStart ||
			marker.AbsoluteStart < value.SourceStart || marker.AbsoluteEnd <= marker.AbsoluteStart ||
			marker.AbsoluteEnd > value.SourceEnd ||
			string(runes[marker.AbsoluteStart-value.SourceStart:marker.AbsoluteEnd-value.SourceStart]) != marker.Label {
			return errors.New("invalid Episode analysis scene marker")
		}
		previousMarkerStart = marker.AbsoluteStart
	}
	return nil
}

func episodeAnalysisRanges(value EpisodeAnalysisSource, maximum int) [][2]int {
	boundaries := []int{value.SourceStart}
	for _, marker := range value.SceneMarkers {
		if marker.AbsoluteStart > value.SourceStart {
			boundaries = append(boundaries, marker.AbsoluteStart)
		}
	}
	boundaries = append(boundaries, value.SourceEnd)
	expanded := []int{boundaries[0]}
	for index := 1; index < len(boundaries); index++ {
		for expanded[len(expanded)-1]+maximum < boundaries[index] {
			expanded = append(expanded, expanded[len(expanded)-1]+maximum)
		}
		expanded = append(expanded, boundaries[index])
	}
	ranges := make([][2]int, 0, len(expanded)-1)
	start := expanded[0]
	for index := 1; index < len(expanded); index++ {
		if expanded[index]-start > maximum {
			ranges = append(ranges, [2]int{start, expanded[index-1]})
			start = expanded[index-1]
		}
	}
	if start < value.SourceEnd {
		ranges = append(ranges, [2]int{start, value.SourceEnd})
	}
	return ranges
}

func episodeAnalysisManifestHash(value EpisodeAnalysisManifest) (string, error) {
	value.ManifestHash = ""
	return episodeAnalysisCanonicalHash(value)
}

func episodeReconcileManifestHash(value EpisodeReconcileManifest) (string, error) {
	value.ManifestHash = ""
	return episodeAnalysisCanonicalHash(value)
}

func episodeAnalysisCanonicalHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func episodeAnalysisTextHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
