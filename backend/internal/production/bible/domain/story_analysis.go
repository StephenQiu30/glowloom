package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	AnalyzeStoryStage   = "analyze_story"
	ReconcileStoryStage = "reconcile_story"
)

var ErrStoryCandidateCannotSplit = errors.New("Story candidate shard cannot be split further")

type StoryAnalysisEvidenceFragment struct {
	ShardKey              string `json:"evidence_shard_key"`
	LogicalStart          int    `json:"logical_start"`
	LogicalEnd            int    `json:"logical_end"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	CandidateItemCount    int    `json:"candidate_item_count"`
}

type StoryAnalysisShard struct {
	Key                       string   `json:"shard_key"`
	TreePath                  string   `json:"tree_path"`
	ParentKey                 string   `json:"parent_shard_key,omitempty"`
	Kind                      string   `json:"kind"`
	EvidenceShardKey          string   `json:"evidence_shard_key"`
	LogicalStart              int      `json:"logical_start"`
	LogicalEnd                int      `json:"logical_end"`
	UpstreamCandidateRevision string   `json:"upstream_candidate_revision_id"`
	UpstreamCandidateHash     string   `json:"upstream_candidate_revision_hash"`
	CandidateItemStart        int      `json:"candidate_item_start"`
	CandidateItemEnd          int      `json:"candidate_item_end"`
	SourceHashes              []string `json:"source_hashes"`
	Status                    string   `json:"status"`
}

type StoryReconcileChild struct {
	Stage              string `json:"stage"`
	ShardKey           string `json:"shard_key"`
	SourceHash         string `json:"source_hash"`
	CandidateItemStart *int   `json:"candidate_item_start,omitempty"`
	CandidateItemEnd   *int   `json:"candidate_item_end,omitempty"`
}

type StoryReconcileShard struct {
	Key          string                `json:"shard_key"`
	TreePath     string                `json:"tree_path"`
	ParentKey    string                `json:"parent_shard_key,omitempty"`
	Kind         string                `json:"kind"`
	Level        int                   `json:"level"`
	Children     []StoryReconcileChild `json:"children"`
	SourceHashes []string              `json:"source_hashes"`
	SubtreeHash  string                `json:"subtree_hash"`
	Status       string                `json:"status"`
}

type StoryAnalysisManifest struct {
	ManifestID         string               `json:"manifest_id"`
	Version            int64                `json:"version"`
	ParentManifestHash *string              `json:"parent_manifest_hash"`
	WorkspaceID        string               `json:"workspace_id"`
	WorkflowRunID      string               `json:"workflow_run_id"`
	NodeRunID          string               `json:"node_run_id"`
	Stage              string               `json:"stage"`
	RootInputHash      string               `json:"root_input_hash"`
	Shards             []StoryAnalysisShard `json:"shards"`
	CoverageHash       string               `json:"coverage_hash"`
	ManifestHash       string               `json:"manifest_hash"`
}

type StoryReconcileManifest struct {
	ManifestID         string                `json:"manifest_id"`
	Version            int64                 `json:"version"`
	ParentManifestHash *string               `json:"parent_manifest_hash"`
	WorkspaceID        string                `json:"workspace_id"`
	WorkflowRunID      string                `json:"workflow_run_id"`
	NodeRunID          string                `json:"node_run_id"`
	Stage              string                `json:"stage"`
	RootInputHash      string                `json:"root_input_hash"`
	FanIn              int                   `json:"fan_in"`
	RootShardKey       string                `json:"root_shard_key"`
	Shards             []StoryReconcileShard `json:"shards"`
	CoverageHash       string                `json:"coverage_hash"`
	ManifestHash       string                `json:"manifest_hash"`
}

type StoryAnalysisManifestInput struct {
	AnalyzeManifestID, ReconcileManifestID               string
	WorkspaceID, WorkflowRunID, NodeRunID, RootInputHash string
	FanIn                                                int
	EvidenceFragments                                    []StoryAnalysisEvidenceFragment
}

func BuildStoryAnalysisManifests(input StoryAnalysisManifestInput) (StoryAnalysisManifest, StoryReconcileManifest, error) {
	for _, identifier := range []string{
		input.AnalyzeManifestID, input.ReconcileManifestID, input.WorkspaceID,
		input.WorkflowRunID, input.NodeRunID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("invalid Story analysis manifest identity")
		}
	}
	if input.FanIn != 2 || !hashPattern.MatchString(input.RootInputHash) || len(input.EvidenceFragments) == 0 {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("invalid Story analysis manifest input")
	}
	fragments := append([]StoryAnalysisEvidenceFragment(nil), input.EvidenceFragments...)
	slices.SortFunc(fragments, func(left, right StoryAnalysisEvidenceFragment) int {
		if left.LogicalStart != right.LogicalStart {
			return left.LogicalStart - right.LogicalStart
		}
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	analyzeShards := make([]StoryAnalysisShard, len(fragments))
	current := make([]StoryReconcileChild, len(fragments))
	position := 0
	seen := make(map[string]struct{}, len(fragments))
	for index, fragment := range fragments {
		if strings.TrimSpace(fragment.ShardKey) == "" || fragment.LogicalStart != position ||
			fragment.LogicalEnd <= fragment.LogicalStart || fragment.CandidateItemCount < 0 ||
			!hashPattern.MatchString(fragment.CandidateRevisionHash) {
			return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("invalid Story analysis Evidence coverage")
		}
		if _, err := uuid.Parse(fragment.CandidateRevisionID); err != nil {
			return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("invalid Story analysis Evidence revision")
		}
		if _, exists := seen[fragment.ShardKey]; exists {
			return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("duplicate Story analysis Evidence shard")
		}
		seen[fragment.ShardKey] = struct{}{}
		key := fmt.Sprintf("story-map:%04d", index)
		analyzeShards[index] = StoryAnalysisShard{
			Key: key, TreePath: fmt.Sprintf("map.%04d", index), Kind: "story_map",
			EvidenceShardKey: fragment.ShardKey, LogicalStart: fragment.LogicalStart,
			LogicalEnd: fragment.LogicalEnd, UpstreamCandidateRevision: fragment.CandidateRevisionID,
			UpstreamCandidateHash: fragment.CandidateRevisionHash,
			CandidateItemStart:    0, CandidateItemEnd: fragment.CandidateItemCount,
			SourceHashes: []string{fragment.CandidateRevisionHash}, Status: "active",
		}
		current[index] = StoryReconcileChild{
			Stage: AnalyzeStoryStage, ShardKey: key, SourceHash: fragment.CandidateRevisionHash,
		}
		position = fragment.LogicalEnd
	}

	reconcileShards := make([]StoryReconcileShard, 0, len(fragments)*2)
	level := 0
	for {
		next := make([]StoryReconcileChild, 0, (len(current)+input.FanIn-1)/input.FanIn)
		for start := 0; start < len(current); start += input.FanIn {
			end := min(start+input.FanIn, len(current))
			children := append([]StoryReconcileChild(nil), current[start:end]...)
			key := fmt.Sprintf("story-reduce:%04d:%04d", level, len(next))
			sourceHashes := make([]string, len(children))
			for index := range children {
				sourceHashes[index] = children[index].SourceHash
			}
			subtreeHash, err := storyReconcileSubtreeHash(level, children)
			if err != nil {
				return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
			}
			reconcileShards = append(reconcileShards, StoryReconcileShard{
				Key: key, TreePath: fmt.Sprintf("reduce.%04d.%04d", level, len(next)),
				Kind: "story_reduce", Level: level, Children: children,
				SourceHashes: sourceHashes, SubtreeHash: subtreeHash, Status: "active",
			})
			next = append(next, StoryReconcileChild{
				Stage: ReconcileStoryStage, ShardKey: key, SourceHash: subtreeHash,
			})
		}
		if len(next) == 1 {
			break
		}
		current = next
		level++
	}
	rootKey := reconcileShards[len(reconcileShards)-1].Key
	parents := make(map[string]string, len(analyzeShards)+len(reconcileShards))
	for _, shard := range reconcileShards {
		for _, child := range shard.Children {
			parents[child.Stage+"\x00"+child.ShardKey] = shard.Key
		}
	}
	for index := range reconcileShards {
		if reconcileShards[index].Key != rootKey {
			reconcileShards[index].ParentKey = parents[ReconcileStoryStage+"\x00"+reconcileShards[index].Key]
		}
	}

	coverageHash, err := storyAnalysisCoverageHash(fragments)
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	analyze := StoryAnalysisManifest{
		ManifestID: input.AnalyzeManifestID, Version: 1, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID, Stage: AnalyzeStoryStage,
		RootInputHash: input.RootInputHash, Shards: analyzeShards, CoverageHash: coverageHash,
	}
	analyze.ManifestHash, err = storyAnalysisManifestHash(analyze)
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	reconcileCoverage, err := CanonicalStoryHash(struct {
		Schema      string `json:"schema"`
		RootKey     string `json:"root_shard_key"`
		SubtreeHash string `json:"subtree_hash"`
	}{"story-reconcile-coverage-v1", rootKey, reconcileShards[len(reconcileShards)-1].SubtreeHash})
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	reconcile := StoryReconcileManifest{
		ManifestID: input.ReconcileManifestID, Version: 1, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID, Stage: ReconcileStoryStage,
		RootInputHash: analyze.ManifestHash, FanIn: input.FanIn, RootShardKey: rootKey,
		Shards: reconcileShards, CoverageHash: reconcileCoverage,
	}
	reconcile.ManifestHash, err = storyReconcileManifestHash(reconcile)
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	if err = ValidateStoryAnalysisManifests(analyze, reconcile); err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	return analyze, reconcile, nil
}

type StoryReconcileCandidateSize struct {
	Stage, ShardKey string
	ItemCount       int
}

func ReshardStoryAnalysisMap(
	currentAnalyze StoryAnalysisManifest,
	currentReconcile StoryReconcileManifest,
	parentKey string,
) (StoryAnalysisManifest, StoryReconcileManifest, error) {
	if err := ValidateStoryAnalysisManifests(currentAnalyze, currentReconcile); err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	parentIndex := -1
	for index, shard := range currentAnalyze.Shards {
		if shard.Key == parentKey && shard.Status == "active" {
			parentIndex = index
			break
		}
	}
	if parentIndex < 0 {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, errors.New("active Story analysis parent shard not found")
	}
	parent := currentAnalyze.Shards[parentIndex]
	if parent.CandidateItemEnd-parent.CandidateItemStart < 2 {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, ErrStoryCandidateCannotSplit
	}
	middle := parent.CandidateItemStart + (parent.CandidateItemEnd-parent.CandidateItemStart)/2
	ranges := [][2]int{{parent.CandidateItemStart, middle}, {middle, parent.CandidateItemEnd}}
	shards := append([]StoryAnalysisShard(nil), currentAnalyze.Shards...)
	shards[parentIndex].Status = "superseded"
	replacements := make([]StoryReconcileChild, 0, len(ranges))
	for index, itemRange := range ranges {
		child := parent
		child.Key = fmt.Sprintf("%s.%04d", parent.Key, index)
		child.TreePath = fmt.Sprintf("%s.%04d", parent.TreePath, index)
		child.ParentKey = parent.Key
		child.CandidateItemStart = itemRange[0]
		child.CandidateItemEnd = itemRange[1]
		child.Status = "active"
		partitionHash, err := storyCandidatePartitionHash(
			AnalyzeStoryStage, parent.Key, parent.UpstreamCandidateHash, itemRange[0], itemRange[1],
		)
		if err != nil {
			return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
		}
		child.SourceHashes = []string{partitionHash}
		shards = append(shards, child)
		replacements = append(replacements, StoryReconcileChild{
			Stage: AnalyzeStoryStage, ShardKey: child.Key, SourceHash: partitionHash,
		})
	}
	parentHash := currentAnalyze.ManifestHash
	nextAnalyze := currentAnalyze
	nextAnalyze.Version++
	nextAnalyze.ParentManifestHash = &parentHash
	nextAnalyze.Shards = shards
	nextAnalyze.ManifestHash = ""
	manifestHash, err := storyAnalysisManifestHash(nextAnalyze)
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	nextAnalyze.ManifestHash = manifestHash

	target := StoryReconcileChild{
		Stage: AnalyzeStoryStage, ShardKey: parent.Key, SourceHash: parent.SourceHashes[0],
	}
	nextReconcile, err := replaceStoryReconcileReference(
		currentReconcile, target, replacements, nextAnalyze.ManifestHash, "map",
	)
	if err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	if err = ValidateStoryAnalysisManifests(nextAnalyze, nextReconcile); err != nil {
		return StoryAnalysisManifest{}, StoryReconcileManifest{}, err
	}
	return nextAnalyze, nextReconcile, nil
}

func ReshardStoryReconcile(
	current StoryReconcileManifest,
	targetKey string,
	sizes []StoryReconcileCandidateSize,
) (StoryReconcileManifest, error) {
	targetIndex := -1
	for index, shard := range current.Shards {
		if shard.Key == targetKey && shard.Status == "active" {
			targetIndex = index
			break
		}
	}
	if targetIndex < 0 {
		return StoryReconcileManifest{}, errors.New("active Story reconcile shard not found")
	}
	target := current.Shards[targetIndex]
	counts := make(map[string]int, len(sizes))
	for _, size := range sizes {
		if (size.Stage != AnalyzeStoryStage && size.Stage != ReconcileStoryStage) ||
			strings.TrimSpace(size.ShardKey) == "" || size.ItemCount < 0 {
			return StoryReconcileManifest{}, errors.New("invalid Story reconcile candidate size")
		}
		key := size.Stage + "\x00" + size.ShardKey
		if _, exists := counts[key]; exists {
			return StoryReconcileManifest{}, errors.New("duplicate Story reconcile candidate size")
		}
		counts[key] = size.ItemCount
	}
	type rangedChild struct {
		child      StoryReconcileChild
		start, end int
	}
	ranged := make([]rangedChild, len(target.Children))
	total := 0
	for index, child := range target.Children {
		count, exists := counts[child.Stage+"\x00"+child.ShardKey]
		if !exists {
			return StoryReconcileManifest{}, errors.New("Story reconcile candidate size is incomplete")
		}
		start, end := 0, count
		if child.CandidateItemStart != nil || child.CandidateItemEnd != nil {
			if child.CandidateItemStart == nil || child.CandidateItemEnd == nil {
				return StoryReconcileManifest{}, errors.New("Story reconcile candidate range is incomplete")
			}
			start, end = *child.CandidateItemStart, *child.CandidateItemEnd
		}
		if start < 0 || end <= start || end > count {
			return StoryReconcileManifest{}, errors.New("Story reconcile candidate range has drifted")
		}
		ranged[index] = rangedChild{child: child, start: start, end: end}
		total += end - start
	}
	if len(counts) != len(target.Children) || total < 2 {
		return StoryReconcileManifest{}, ErrStoryCandidateCannotSplit
	}
	cut := total / 2
	groups := make([][]StoryReconcileChild, 2)
	position := 0
	for _, value := range ranged {
		for position < total && value.start < value.end {
			group := 0
			boundary := cut
			if position >= cut {
				group, boundary = 1, total
			}
			take := min(value.end-value.start, boundary-position)
			if take == 0 {
				position = boundary
				continue
			}
			start, end := value.start, value.start+take
			child := value.child
			child.CandidateItemStart = intPointer(start)
			child.CandidateItemEnd = intPointer(end)
			groups[group] = append(groups[group], child)
			value.start, position = end, position+take
		}
	}
	if len(groups[0]) == 0 || len(groups[1]) == 0 {
		return StoryReconcileManifest{}, errors.New("Story reconcile reshard did not produce two candidate partitions")
	}
	shards := append([]StoryReconcileShard(nil), current.Shards...)
	shards[targetIndex].Status = "superseded"
	newShards := make([]StoryReconcileShard, 0, 4)
	partitionRefs := make([]StoryReconcileChild, 0, 2)
	for index, children := range groups {
		key := fmt.Sprintf("%s.partition.v%04d.%04d", target.Key, current.Version+1, index)
		shard, err := buildStoryReconcileShard(key, target.TreePath+fmt.Sprintf(".partition.%04d", index), target.Level, children)
		if err != nil {
			return StoryReconcileManifest{}, err
		}
		newShards = append(newShards, shard)
		partitionRefs = append(partitionRefs, StoryReconcileChild{
			Stage: ReconcileStoryStage, ShardKey: shard.Key, SourceHash: shard.SubtreeHash,
		})
	}
	combined, err := buildStoryReconcileShard(
		fmt.Sprintf("%s.combine.v%04d", target.Key, current.Version+1),
		target.TreePath+".combine", target.Level+1, partitionRefs,
	)
	if err != nil {
		return StoryReconcileManifest{}, err
	}
	newShards = append(newShards, combined)
	shards = append(shards, newShards...)
	next, err := replaceStoryReconcileNodePath(current, shards, target, combined, "reduce")
	if err != nil {
		return StoryReconcileManifest{}, err
	}
	return next, nil
}

func intPointer(value int) *int { return &value }

func storyCandidatePartitionHash(stage, shardKey, sourceHash string, start, end int) (string, error) {
	return CanonicalStoryHash(struct {
		Schema, Stage, ShardKey, SourceHash string
		Start, End                          int
	}{"story-candidate-partition-v1", stage, shardKey, sourceHash, start, end})
}

func buildStoryReconcileShard(key, treePath string, level int, children []StoryReconcileChild) (StoryReconcileShard, error) {
	if len(children) == 0 || len(children) > 2 {
		return StoryReconcileShard{}, errors.New("Story reconcile shard exceeds fixed fan-in")
	}
	stage := children[0].Stage
	sourceHashes := make([]string, len(children))
	for index, child := range children {
		if child.Stage != stage {
			return StoryReconcileShard{}, errors.New("Story reconcile shard mixed candidate layers")
		}
		sourceHashes[index] = child.SourceHash
	}
	subtreeHash, err := storyReconcileSubtreeHash(level, children)
	if err != nil {
		return StoryReconcileShard{}, err
	}
	return StoryReconcileShard{
		Key: key, TreePath: treePath, Kind: "story_reduce", Level: level,
		Children: children, SourceHashes: sourceHashes, SubtreeHash: subtreeHash, Status: "active",
	}, nil
}

func replaceStoryReconcileReference(
	current StoryReconcileManifest,
	target StoryReconcileChild,
	replacements []StoryReconcileChild,
	rootInputHash string,
	label string,
) (StoryReconcileManifest, error) {
	bridge, err := buildStoryReconcileShard(
		fmt.Sprintf("%s.partition.v%04d", target.ShardKey, current.Version+1),
		"reshard."+target.ShardKey+".partition", 0, replacements,
	)
	if err != nil {
		return StoryReconcileManifest{}, err
	}
	shards := append([]StoryReconcileShard(nil), current.Shards...)
	shards = append(shards, bridge)
	return replaceStoryReconcileChildPath(current, shards, target, bridge, rootInputHash, label)
}

func replaceStoryReconcileNodePath(
	current StoryReconcileManifest,
	shards []StoryReconcileShard,
	target StoryReconcileShard,
	replacement StoryReconcileShard,
	label string,
) (StoryReconcileManifest, error) {
	oldRef := StoryReconcileChild{Stage: ReconcileStoryStage, ShardKey: target.Key, SourceHash: target.SubtreeHash}
	return replaceStoryReconcileChildPath(current, shards, oldRef, replacement, current.RootInputHash, label)
}

func replaceStoryReconcileChildPath(
	current StoryReconcileManifest,
	shards []StoryReconcileShard,
	oldRef StoryReconcileChild,
	replacement StoryReconcileShard,
	rootInputHash string,
	label string,
) (StoryReconcileManifest, error) {
	newRef := StoryReconcileChild{Stage: ReconcileStoryStage, ShardKey: replacement.Key, SourceHash: replacement.SubtreeHash}
	sequence := 0
	for {
		parentIndex := -1
		var parent StoryReconcileShard
		for index, shard := range current.Shards {
			if shard.Status != "active" {
				continue
			}
			for _, child := range shard.Children {
				if child.Stage == oldRef.Stage && child.ShardKey == oldRef.ShardKey && child.SourceHash == oldRef.SourceHash {
					parentIndex, parent = index, shard
					break
				}
			}
			if parentIndex >= 0 {
				break
			}
		}
		if parentIndex < 0 {
			break
		}
		for index := range shards {
			if shards[index].Key == parent.Key && shards[index].Status == "active" {
				shards[index].Status = "superseded"
				break
			}
		}
		children := append([]StoryReconcileChild(nil), parent.Children...)
		for index, child := range children {
			if child.Stage == oldRef.Stage && child.ShardKey == oldRef.ShardKey && child.SourceHash == oldRef.SourceHash {
				children[index] = newRef
				break
			}
		}
		stage := children[0].Stage
		mixed := false
		for _, child := range children[1:] {
			mixed = mixed || child.Stage != stage
		}
		if mixed {
			for index, child := range children {
				if child.Stage == ReconcileStoryStage {
					continue
				}
				key := fmt.Sprintf("%s.%s.v%04d.wrap.%04d", parent.Key, label, current.Version+1, sequence)
				sequence++
				wrapper, err := buildStoryReconcileShard(key, parent.TreePath+".wrap", 0, []StoryReconcileChild{child})
				if err != nil {
					return StoryReconcileManifest{}, err
				}
				shards = append(shards, wrapper)
				children[index] = StoryReconcileChild{
					Stage: ReconcileStoryStage, ShardKey: wrapper.Key, SourceHash: wrapper.SubtreeHash,
				}
			}
		}
		level := 0
		if children[0].Stage == ReconcileStoryStage {
			for _, child := range children {
				for _, candidate := range shards {
					if candidate.Key == child.ShardKey && candidate.Status == "active" && candidate.Level >= level {
						level = candidate.Level + 1
					}
				}
			}
		}
		key := fmt.Sprintf("%s.%s.v%04d.%04d", parent.Key, label, current.Version+1, sequence)
		sequence++
		clone, err := buildStoryReconcileShard(key, parent.TreePath+"."+label, level, children)
		if err != nil {
			return StoryReconcileManifest{}, err
		}
		shards = append(shards, clone)
		oldRef = StoryReconcileChild{Stage: ReconcileStoryStage, ShardKey: parent.Key, SourceHash: parent.SubtreeHash}
		newRef = StoryReconcileChild{Stage: ReconcileStoryStage, ShardKey: clone.Key, SourceHash: clone.SubtreeHash}
	}
	parentHash := current.ManifestHash
	next := current
	next.Version++
	next.ParentManifestHash = &parentHash
	next.RootInputHash = rootInputHash
	next.RootShardKey = newRef.ShardKey
	next.Shards = shards
	recomputeStoryReconcileParents(next.Shards, next.RootShardKey)
	root, exists := storyReconcileShardByKey(next.Shards, next.RootShardKey)
	if !exists || root.Status != "active" {
		return StoryReconcileManifest{}, errors.New("Story reconcile reshard lost its root")
	}
	coverageHash, err := storyReconcileCoverageHash(root.Key, root.SubtreeHash)
	if err != nil {
		return StoryReconcileManifest{}, err
	}
	next.CoverageHash = coverageHash
	next.ManifestHash = ""
	manifestHash, err := storyReconcileManifestHash(next)
	if err != nil {
		return StoryReconcileManifest{}, err
	}
	next.ManifestHash = manifestHash
	return next, nil
}

func recomputeStoryReconcileParents(shards []StoryReconcileShard, rootKey string) {
	parents := make(map[string][]string)
	for _, shard := range shards {
		if shard.Status != "active" {
			continue
		}
		for _, child := range shard.Children {
			if child.Stage == ReconcileStoryStage {
				parents[child.ShardKey] = append(parents[child.ShardKey], shard.Key)
			}
		}
	}
	for index := range shards {
		if shards[index].Status != "active" {
			continue
		}
		values := parents[shards[index].Key]
		if shards[index].Key == rootKey || len(values) != 1 {
			shards[index].ParentKey = ""
		} else {
			shards[index].ParentKey = values[0]
		}
	}
}

func storyReconcileShardByKey(shards []StoryReconcileShard, key string) (StoryReconcileShard, bool) {
	for _, shard := range shards {
		if shard.Key == key {
			return shard, true
		}
	}
	return StoryReconcileShard{}, false
}

func storyReconcileCoverageHash(rootKey, subtreeHash string) (string, error) {
	return CanonicalStoryHash(struct {
		Schema      string `json:"schema"`
		RootKey     string `json:"root_shard_key"`
		SubtreeHash string `json:"subtree_hash"`
	}{"story-reconcile-coverage-v1", rootKey, subtreeHash})
}

func ValidateStoryAnalysisManifests(analyze StoryAnalysisManifest, reconcile StoryReconcileManifest) error {
	for _, identifier := range []string{
		analyze.ManifestID, reconcile.ManifestID, analyze.WorkspaceID, analyze.WorkflowRunID, analyze.NodeRunID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Story analysis manifest owner")
		}
	}
	if analyze.Version < 1 || reconcile.Version < 1 || analyze.Stage != AnalyzeStoryStage ||
		reconcile.Stage != ReconcileStoryStage || analyze.WorkspaceID != reconcile.WorkspaceID ||
		analyze.WorkflowRunID != reconcile.WorkflowRunID || analyze.NodeRunID != reconcile.NodeRunID ||
		reconcile.FanIn != 2 || reconcile.RootInputHash != analyze.ManifestHash ||
		!hashPattern.MatchString(analyze.RootInputHash) || len(analyze.Shards) == 0 || len(reconcile.Shards) == 0 {
		return errors.New("invalid Story analysis manifest relationship")
	}
	if analyze.Version == 1 && analyze.ParentManifestHash != nil || analyze.Version > 1 &&
		(analyze.ParentManifestHash == nil || !hashPattern.MatchString(*analyze.ParentManifestHash)) ||
		reconcile.Version == 1 && reconcile.ParentManifestHash != nil || reconcile.Version > 1 &&
		(reconcile.ParentManifestHash == nil || !hashPattern.MatchString(*reconcile.ParentManifestHash)) {
		return errors.New("invalid Story analysis manifest lineage")
	}
	analyzeKeys := make(map[string]StoryAnalysisShard, len(analyze.Shards))
	activeAnalyze := make(map[string]StoryAnalysisShard)
	childrenByParent := make(map[string][]StoryAnalysisShard)
	for _, shard := range analyze.Shards {
		if strings.TrimSpace(shard.Key) == "" || strings.TrimSpace(shard.TreePath) == "" ||
			shard.Kind != "story_map" || (shard.Status != "active" && shard.Status != "superseded") ||
			shard.LogicalStart < 0 || shard.LogicalEnd <= shard.LogicalStart ||
			shard.CandidateItemStart < 0 || shard.CandidateItemEnd < shard.CandidateItemStart ||
			len(shard.SourceHashes) != 1 || !hashPattern.MatchString(shard.SourceHashes[0]) ||
			!hashPattern.MatchString(shard.UpstreamCandidateHash) {
			return errors.New("invalid Story analysis map shard")
		}
		if _, err := uuid.Parse(shard.UpstreamCandidateRevision); err != nil {
			return errors.New("invalid Story analysis upstream revision")
		}
		if _, exists := analyzeKeys[shard.Key]; exists {
			return errors.New("duplicate Story analysis map shard")
		}
		analyzeKeys[shard.Key] = shard
		if shard.Status == "active" {
			activeAnalyze[shard.Key] = shard
		}
		if shard.ParentKey != "" {
			childrenByParent[shard.ParentKey] = append(childrenByParent[shard.ParentKey], shard)
		}
	}
	roots := make([]StoryAnalysisShard, 0)
	for _, shard := range analyze.Shards {
		if shard.ParentKey == "" {
			roots = append(roots, shard)
			continue
		}
		parent, exists := analyzeKeys[shard.ParentKey]
		if !exists || parent.EvidenceShardKey != shard.EvidenceShardKey ||
			parent.LogicalStart != shard.LogicalStart || parent.LogicalEnd != shard.LogicalEnd ||
			parent.UpstreamCandidateRevision != shard.UpstreamCandidateRevision ||
			parent.UpstreamCandidateHash != shard.UpstreamCandidateHash ||
			shard.CandidateItemStart < parent.CandidateItemStart || shard.CandidateItemEnd > parent.CandidateItemEnd {
			return errors.New("Story analysis map shard lineage has drifted")
		}
	}
	slices.SortFunc(roots, func(left, right StoryAnalysisShard) int { return left.LogicalStart - right.LogicalStart })
	position := 0
	for _, root := range roots {
		if root.LogicalStart != position {
			return errors.New("Story analysis Evidence coverage contains a gap or overlap")
		}
		position = root.LogicalEnd
		leaves := make([]StoryAnalysisShard, 0)
		for _, shard := range activeAnalyze {
			if shard.EvidenceShardKey == root.EvidenceShardKey && shard.LogicalStart == root.LogicalStart &&
				shard.LogicalEnd == root.LogicalEnd && shard.UpstreamCandidateRevision == root.UpstreamCandidateRevision {
				leaves = append(leaves, shard)
			}
		}
		slices.SortFunc(leaves, func(left, right StoryAnalysisShard) int {
			return left.CandidateItemStart - right.CandidateItemStart
		})
		itemPosition := root.CandidateItemStart
		for _, leaf := range leaves {
			if leaf.CandidateItemStart != itemPosition {
				return errors.New("Story analysis candidate coverage contains a gap or overlap")
			}
			itemPosition = leaf.CandidateItemEnd
		}
		if len(leaves) == 0 || itemPosition != root.CandidateItemEnd {
			return errors.New("Story analysis candidate coverage is incomplete")
		}
	}
	for parentKey, children := range childrenByParent {
		parent := analyzeKeys[parentKey]
		if parent.Status != "superseded" || len(children) < 2 {
			return errors.New("Story analysis reshard parent is not superseded")
		}
	}
	reconcileKeys := make(map[string]StoryReconcileShard, len(reconcile.Shards))
	for _, shard := range reconcile.Shards {
		if strings.TrimSpace(shard.Key) == "" || strings.TrimSpace(shard.TreePath) == "" ||
			shard.Kind != "story_reduce" || (shard.Status != "active" && shard.Status != "superseded") ||
			shard.Level < 0 || len(shard.Children) == 0 || len(shard.Children) > reconcile.FanIn ||
			len(shard.SourceHashes) != len(shard.Children) || !hashPattern.MatchString(shard.SubtreeHash) {
			return errors.New("invalid Story reconcile shard")
		}
		childStage := shard.Children[0].Stage
		for index, child := range shard.Children {
			if child.Stage != childStage || (child.Stage != AnalyzeStoryStage && child.Stage != ReconcileStoryStage) ||
				strings.TrimSpace(child.ShardKey) == "" || !hashPattern.MatchString(child.SourceHash) ||
				shard.SourceHashes[index] != child.SourceHash {
				return errors.New("invalid Story reconcile child")
			}
			if (child.CandidateItemStart == nil) != (child.CandidateItemEnd == nil) || child.CandidateItemStart != nil &&
				(*child.CandidateItemStart < 0 || *child.CandidateItemEnd <= *child.CandidateItemStart) {
				return errors.New("invalid Story reconcile candidate partition")
			}
		}
		expected, err := storyReconcileSubtreeHash(shard.Level, shard.Children)
		if err != nil || expected != shard.SubtreeHash {
			return errors.New("Story reconcile subtree hash mismatch")
		}
		if _, exists := reconcileKeys[shard.Key]; exists {
			return errors.New("duplicate Story reconcile shard")
		}
		reconcileKeys[shard.Key] = shard
	}
	root, exists := reconcileKeys[reconcile.RootShardKey]
	if !exists || root.Status != "active" {
		return errors.New("Story reconcile root key mismatch")
	}
	reachableReduce := make(map[string]struct{})
	reachableAnalyze := make(map[string]struct{})
	visiting := make(map[string]struct{})
	var visit func(string) error
	visit = func(key string) error {
		if _, done := reachableReduce[key]; done {
			return nil
		}
		if _, cycle := visiting[key]; cycle {
			return errors.New("Story reconcile graph contains a cycle")
		}
		shard, exists := reconcileKeys[key]
		if !exists || shard.Status != "active" {
			return errors.New("Story reconcile active path references a stale shard")
		}
		visiting[key] = struct{}{}
		for _, child := range shard.Children {
			if child.Stage == AnalyzeStoryStage {
				value, exists := activeAnalyze[child.ShardKey]
				if !exists || value.SourceHashes[0] != child.SourceHash {
					return errors.New("Story reconcile map child has drifted")
				}
				reachableAnalyze[child.ShardKey] = struct{}{}
				continue
			}
			value, exists := reconcileKeys[child.ShardKey]
			if !exists || value.Status != "active" || value.SubtreeHash != child.SourceHash || value.Level >= shard.Level {
				return errors.New("Story reconcile tree child has drifted")
			}
			if err := visit(child.ShardKey); err != nil {
				return err
			}
		}
		delete(visiting, key)
		reachableReduce[key] = struct{}{}
		return nil
	}
	if err := visit(root.Key); err != nil {
		return err
	}
	for _, shard := range reconcile.Shards {
		if shard.Status == "active" {
			if _, exists := reachableReduce[shard.Key]; !exists {
				return errors.New("Story reconcile manifest contains an unreachable active shard")
			}
		}
	}
	if len(reachableAnalyze) != len(activeAnalyze) {
		return errors.New("Story reconcile manifest omitted an active map shard")
	}
	expectedCoverage, err := storyReconcileCoverageHash(root.Key, root.SubtreeHash)
	if err != nil || expectedCoverage != reconcile.CoverageHash {
		return errors.New("Story reconcile coverage hash mismatch")
	}
	if expected, err := storyAnalysisManifestHash(analyze); err != nil || expected != analyze.ManifestHash {
		return errors.New("Story analysis manifest hash mismatch")
	}
	if expected, err := storyReconcileManifestHash(reconcile); err != nil || expected != reconcile.ManifestHash {
		return errors.New("Story reconcile manifest hash mismatch")
	}
	return nil
}

func storyAnalysisCoverageHash(fragments []StoryAnalysisEvidenceFragment) (string, error) {
	return CanonicalStoryHash(struct {
		Schema    string                          `json:"schema"`
		Fragments []StoryAnalysisEvidenceFragment `json:"fragments"`
	}{"story-analysis-coverage-v1", fragments})
}

func storyReconcileSubtreeHash(level int, children []StoryReconcileChild) (string, error) {
	return CanonicalStoryHash(struct {
		Schema   string                `json:"schema"`
		Level    int                   `json:"level"`
		Children []StoryReconcileChild `json:"children"`
	}{"story-reconcile-subtree-v1", level, children})
}

func storyAnalysisManifestHash(value StoryAnalysisManifest) (string, error) {
	return CanonicalStoryHash(struct {
		Schema             string               `json:"schema"`
		Version            int64                `json:"version"`
		ParentManifestHash *string              `json:"parent_manifest_hash"`
		WorkspaceID        string               `json:"workspace_id"`
		WorkflowRunID      string               `json:"workflow_run_id"`
		NodeRunID          string               `json:"node_run_id"`
		Stage              string               `json:"stage"`
		RootInputHash      string               `json:"root_input_hash"`
		Shards             []StoryAnalysisShard `json:"shards"`
		CoverageHash       string               `json:"coverage_hash"`
	}{"story-analysis-shard-manifest-v1", value.Version, value.ParentManifestHash, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.Shards, value.CoverageHash})
}

func storyReconcileManifestHash(value StoryReconcileManifest) (string, error) {
	return CanonicalStoryHash(struct {
		Schema             string                `json:"schema"`
		Version            int64                 `json:"version"`
		ParentManifestHash *string               `json:"parent_manifest_hash"`
		WorkspaceID        string                `json:"workspace_id"`
		WorkflowRunID      string                `json:"workflow_run_id"`
		NodeRunID          string                `json:"node_run_id"`
		Stage              string                `json:"stage"`
		RootInputHash      string                `json:"root_input_hash"`
		FanIn              int                   `json:"fan_in"`
		RootShardKey       string                `json:"root_shard_key"`
		Shards             []StoryReconcileShard `json:"shards"`
		CoverageHash       string                `json:"coverage_hash"`
	}{"story-reconcile-shard-manifest-v1", value.Version, value.ParentManifestHash, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.FanIn, value.RootShardKey,
		value.Shards, value.CoverageHash})
}

func CanonicalStoryHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return SourceTextHash(string(encoded)), nil
}

type AssetSpecCandidate struct {
	Identity            *string  `json:"identity"`
	Appearance          *string  `json:"appearance"`
	AgeImpression       *string  `json:"age_impression"`
	Temperament         []string `json:"temperament"`
	Goals               []string `json:"goals"`
	Relationships       []string `json:"relationships"`
	ArcSummary          *string  `json:"arc_summary"`
	VoiceProfile        *string  `json:"voice_profile"`
	SpatialDescription  *string  `json:"spatial_description"`
	TimeWeather         *string  `json:"time_weather"`
	VisualElements      []string `json:"visual_elements"`
	Lighting            *string  `json:"lighting"`
	Material            *string  `json:"material"`
	UsageContext        *string  `json:"usage_context"`
	VisualLanguage      *string  `json:"visual_language"`
	Palette             *string  `json:"palette"`
	LightingLanguage    *string  `json:"lighting_language"`
	NegativeConstraints []string `json:"negative_constraints"`
	SourceKind          *string  `json:"source_kind"`
	Language            *string  `json:"language"`
	PerformanceTraits   []string `json:"performance_traits"`
	AllowedUsage        []string `json:"allowed_usage"`
}

type StoryEntityStateCandidate struct {
	StateKey       string             `json:"state_key"`
	Label          string             `json:"label"`
	StateSpec      AssetSpecCandidate `json:"state_spec"`
	EpisodeNumbers []int              `json:"episode_numbers"`
	Evidence       []Evidence         `json:"evidence"`
	Ambiguities    []string           `json:"ambiguities"`
}

type StoryEntityCandidate struct {
	EntityKey      string                      `json:"entity_key"`
	Kind           string                      `json:"kind"`
	CanonicalName  string                      `json:"canonical_name"`
	NormalizedName string                      `json:"normalized_name"`
	Aliases        []string                    `json:"aliases"`
	StableSpec     AssetSpecCandidate          `json:"stable_spec"`
	EpisodeNumbers []int                       `json:"episode_numbers"`
	Evidence       []Evidence                  `json:"evidence"`
	States         []StoryEntityStateCandidate `json:"states"`
	Ambiguities    []string                    `json:"ambiguities"`
}

type StoryWorldEntryCandidate struct {
	EntryKey       string     `json:"entry_key"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	Facts          []string   `json:"facts"`
	Rules          []string   `json:"rules"`
	EntityKeys     []string   `json:"entity_keys"`
	EpisodeNumbers []int      `json:"episode_numbers"`
	Evidence       []Evidence `json:"evidence"`
	Ambiguities    []string   `json:"ambiguities"`
}

type StoryClaimCandidate struct {
	ClaimKey        string     `json:"claim_key"`
	ClaimType       string     `json:"claim_type"`
	ParticipantKeys []string   `json:"participant_keys"`
	AnchorKeys      []string   `json:"anchor_keys"`
	Scope           string     `json:"scope"`
	Polarity        string     `json:"polarity"`
	Status          string     `json:"status"`
	Evidence        []Evidence `json:"evidence"`
}

type StoryArcCandidate struct {
	ArcKey   string     `json:"arc_key"`
	Title    string     `json:"title"`
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`
}

type StoryAnalysisCandidate struct {
	Entities     []StoryEntityCandidate     `json:"entities"`
	WorldEntries []StoryWorldEntryCandidate `json:"world_entries"`
	Claims       []StoryClaimCandidate      `json:"claims"`
	Arcs         []StoryArcCandidate        `json:"arcs"`
	ReviewIssues []ReviewIssue              `json:"review_issues"`
}

type StoryReconciliationCandidate struct {
	CanonicalEntities     []StoryEntityCandidate     `json:"canonical_entities"`
	CanonicalWorldEntries []StoryWorldEntryCandidate `json:"canonical_world_entries"`
	MergedClaims          []StoryClaimCandidate      `json:"merged_claims"`
	MergedArcs            []StoryArcCandidate        `json:"merged_arcs"`
	Conflicts             []ReviewIssue              `json:"conflicts"`
	ReviewIssues          []ReviewIssue              `json:"review_issues"`
}

func SourceEvidenceCandidateItemCount(value SourceEvidenceCandidate) int {
	return len(value.Observations) + len(value.ReviewIssues)
}

func SliceSourceEvidenceCandidate(value SourceEvidenceCandidate, start, end int) (SourceEvidenceCandidate, error) {
	total := SourceEvidenceCandidateItemCount(value)
	if start < 0 || end < start || end > total {
		return SourceEvidenceCandidate{}, errors.New("invalid Source Evidence candidate item range")
	}
	result := SourceEvidenceCandidate{
		Observations: []SourceObservation{}, ReviewIssues: []SourceEvidenceIssue{},
	}
	cursor := 0
	appendCandidateItems(&result.Observations, value.Observations, start, end, &cursor)
	appendCandidateItems(&result.ReviewIssues, value.ReviewIssues, start, end, &cursor)
	return result, nil
}

func StoryAnalysisCandidateItemCount(value StoryAnalysisCandidate) int {
	return len(value.Entities) + len(value.WorldEntries) + len(value.Claims) + len(value.Arcs) + len(value.ReviewIssues)
}

func SliceStoryAnalysisCandidate(value StoryAnalysisCandidate, start, end int) (StoryAnalysisCandidate, error) {
	total := StoryAnalysisCandidateItemCount(value)
	if start < 0 || end <= start || end > total {
		return StoryAnalysisCandidate{}, errors.New("invalid Story analysis candidate item range")
	}
	result := StoryAnalysisCandidate{
		Entities: []StoryEntityCandidate{}, WorldEntries: []StoryWorldEntryCandidate{},
		Claims: []StoryClaimCandidate{}, Arcs: []StoryArcCandidate{}, ReviewIssues: []ReviewIssue{},
	}
	cursor := 0
	appendCandidateItems(&result.Entities, value.Entities, start, end, &cursor)
	appendCandidateItems(&result.WorldEntries, value.WorldEntries, start, end, &cursor)
	appendCandidateItems(&result.Claims, value.Claims, start, end, &cursor)
	appendCandidateItems(&result.Arcs, value.Arcs, start, end, &cursor)
	appendCandidateItems(&result.ReviewIssues, value.ReviewIssues, start, end, &cursor)
	return result, nil
}

func StoryReconciliationCandidateItemCount(value StoryReconciliationCandidate) int {
	return len(value.CanonicalEntities) + len(value.CanonicalWorldEntries) + len(value.MergedClaims) +
		len(value.MergedArcs) + len(value.Conflicts) + len(value.ReviewIssues)
}

func SliceStoryReconciliationCandidate(value StoryReconciliationCandidate, start, end int) (StoryReconciliationCandidate, error) {
	total := StoryReconciliationCandidateItemCount(value)
	if start < 0 || end <= start || end > total {
		return StoryReconciliationCandidate{}, errors.New("invalid Story reconciliation candidate item range")
	}
	result := StoryReconciliationCandidate{
		CanonicalEntities: []StoryEntityCandidate{}, CanonicalWorldEntries: []StoryWorldEntryCandidate{},
		MergedClaims: []StoryClaimCandidate{}, MergedArcs: []StoryArcCandidate{},
		Conflicts: []ReviewIssue{}, ReviewIssues: []ReviewIssue{},
	}
	cursor := 0
	appendCandidateItems(&result.CanonicalEntities, value.CanonicalEntities, start, end, &cursor)
	appendCandidateItems(&result.CanonicalWorldEntries, value.CanonicalWorldEntries, start, end, &cursor)
	appendCandidateItems(&result.MergedClaims, value.MergedClaims, start, end, &cursor)
	appendCandidateItems(&result.MergedArcs, value.MergedArcs, start, end, &cursor)
	appendCandidateItems(&result.Conflicts, value.Conflicts, start, end, &cursor)
	appendCandidateItems(&result.ReviewIssues, value.ReviewIssues, start, end, &cursor)
	return result, nil
}

func appendCandidateItems[T any](target *[]T, source []T, start, end int, cursor *int) {
	sectionStart, sectionEnd := *cursor, *cursor+len(source)
	from, to := max(start, sectionStart), min(end, sectionEnd)
	if from < to {
		*target = append(*target, source[from-sectionStart:to-sectionStart]...)
	}
	*cursor = sectionEnd
}

func DecodeStoryAnalysisCandidate(raw json.RawMessage, allowed []Evidence) (StoryAnalysisCandidate, error) {
	var value StoryAnalysisCandidate
	if err := decodeStoryCandidate(raw, &value); err != nil {
		return StoryAnalysisCandidate{}, err
	}
	if err := ValidateStoryAnalysisCandidate(value, allowed); err != nil {
		return StoryAnalysisCandidate{}, err
	}
	return value, nil
}

func DecodeStoryReconciliationCandidate(raw json.RawMessage, allowed []Evidence) (StoryReconciliationCandidate, error) {
	var value StoryReconciliationCandidate
	if err := decodeStoryCandidate(raw, &value); err != nil {
		return StoryReconciliationCandidate{}, err
	}
	if err := ValidateStoryReconciliationCandidate(value, allowed); err != nil {
		return StoryReconciliationCandidate{}, err
	}
	return value, nil
}

func decodeStoryCandidate(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("candidate does not match Story analysis schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("Story analysis candidate contains multiple JSON values")
	}
	return nil
}

func ValidateStoryAnalysisCandidate(value StoryAnalysisCandidate, allowed []Evidence) error {
	return validateStoryCandidate(value.Entities, value.WorldEntries, value.Claims, value.Arcs, value.ReviewIssues, nil, allowed)
}

func ValidateStoryReconciliationCandidate(value StoryReconciliationCandidate, allowed []Evidence) error {
	return validateStoryCandidate(value.CanonicalEntities, value.CanonicalWorldEntries, value.MergedClaims, value.MergedArcs, value.ReviewIssues, value.Conflicts, allowed)
}

func ValidateStoryReconciliationConservation(
	value StoryReconciliationCandidate,
	analysisSources []StoryAnalysisCandidate,
	reconciliationSources []StoryReconciliationCandidate,
) error {
	if (len(analysisSources) == 0) == (len(reconciliationSources) == 0) {
		return errors.New("Story reconciliation requires one exact upstream candidate type")
	}
	expected := newStoryCandidateKeys()
	for _, source := range analysisSources {
		expected.add(source.Entities, source.WorldEntries, source.Claims, source.Arcs)
	}
	for _, source := range reconciliationSources {
		expected.add(source.CanonicalEntities, source.CanonicalWorldEntries, source.MergedClaims, source.MergedArcs)
	}
	actual := newStoryCandidateKeys()
	if !actual.addUnique(value.CanonicalEntities, value.CanonicalWorldEntries, value.MergedClaims, value.MergedArcs) {
		return errors.New("Story reconciliation contains duplicate candidate keys")
	}
	if !sameStoryCandidateKeys(expected, actual) {
		return errors.New("Story reconciliation changed exact candidate keys without an explicit reviewed identity link")
	}
	return nil
}

type storyCandidateKeys struct {
	entities map[string]struct{}
	states   map[string]struct{}
	world    map[string]struct{}
	claims   map[string]struct{}
	arcs     map[string]struct{}
}

func newStoryCandidateKeys() storyCandidateKeys {
	return storyCandidateKeys{
		entities: map[string]struct{}{}, states: map[string]struct{}{}, world: map[string]struct{}{},
		claims: map[string]struct{}{}, arcs: map[string]struct{}{},
	}
}

func (keys storyCandidateKeys) add(
	entities []StoryEntityCandidate,
	world []StoryWorldEntryCandidate,
	claims []StoryClaimCandidate,
	arcs []StoryArcCandidate,
) {
	for _, entity := range entities {
		keys.entities[entity.EntityKey] = struct{}{}
		for _, state := range entity.States {
			keys.states[entity.EntityKey+"\x00"+state.StateKey] = struct{}{}
		}
	}
	for _, entry := range world {
		keys.world[entry.EntryKey] = struct{}{}
	}
	for _, claim := range claims {
		keys.claims[claim.ClaimKey] = struct{}{}
	}
	for _, arc := range arcs {
		keys.arcs[arc.ArcKey] = struct{}{}
	}
}

func (keys storyCandidateKeys) addUnique(
	entities []StoryEntityCandidate,
	world []StoryWorldEntryCandidate,
	claims []StoryClaimCandidate,
	arcs []StoryArcCandidate,
) bool {
	for _, entity := range entities {
		if _, exists := keys.entities[entity.EntityKey]; exists {
			return false
		}
		keys.entities[entity.EntityKey] = struct{}{}
		for _, state := range entity.States {
			key := entity.EntityKey + "\x00" + state.StateKey
			if _, exists := keys.states[key]; exists {
				return false
			}
			keys.states[key] = struct{}{}
		}
	}
	for _, entry := range world {
		if _, exists := keys.world[entry.EntryKey]; exists {
			return false
		}
		keys.world[entry.EntryKey] = struct{}{}
	}
	for _, claim := range claims {
		if _, exists := keys.claims[claim.ClaimKey]; exists {
			return false
		}
		keys.claims[claim.ClaimKey] = struct{}{}
	}
	for _, arc := range arcs {
		if _, exists := keys.arcs[arc.ArcKey]; exists {
			return false
		}
		keys.arcs[arc.ArcKey] = struct{}{}
	}
	return true
}

func sameStoryCandidateKeys(left, right storyCandidateKeys) bool {
	return sameStoryKeySet(left.entities, right.entities) && sameStoryKeySet(left.states, right.states) &&
		sameStoryKeySet(left.world, right.world) && sameStoryKeySet(left.claims, right.claims) &&
		sameStoryKeySet(left.arcs, right.arcs)
}

func sameStoryKeySet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, exists := right[key]; !exists {
			return false
		}
	}
	return true
}

func validateStoryCandidate(
	entities []StoryEntityCandidate,
	world []StoryWorldEntryCandidate,
	claims []StoryClaimCandidate,
	arcs []StoryArcCandidate,
	issues []ReviewIssue,
	conflicts []ReviewIssue,
	allowed []Evidence,
) error {
	if entities == nil || world == nil || claims == nil || arcs == nil || issues == nil ||
		len(entities)+len(world)+len(claims)+len(arcs)+len(issues)+len(conflicts) > 10_000 {
		return errors.New("Story candidate is incomplete or exceeds limits")
	}
	allowedEvidence := make(map[string]struct{}, len(allowed))
	for _, evidence := range allowed {
		allowedEvidence[storyEvidenceKey(evidence)] = struct{}{}
	}
	checkEvidence := func(values []Evidence, required bool) error {
		if required && len(values) == 0 {
			return errors.New("Story candidate fact has no Evidence")
		}
		for _, evidence := range values {
			if _, ok := allowedEvidence[storyEvidenceKey(evidence)]; !ok {
				return errors.New("Story candidate Evidence is absent from exact upstream revisions")
			}
		}
		return nil
	}
	entityKeys := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		if !keyPattern.MatchString(entity.EntityKey) || !oneOf(entity.Kind, "character", "location", "prop", "costume", "visual_style", "voice") ||
			strings.TrimSpace(entity.CanonicalName) == "" || strings.TrimSpace(entity.NormalizedName) == "" ||
			entity.Aliases == nil || entity.EpisodeNumbers == nil || entity.States == nil || entity.Ambiguities == nil ||
			!validAssetSpec(entity.StableSpec) {
			return errors.New("Story candidate contains an invalid entity")
		}
		if _, exists := entityKeys[entity.EntityKey]; exists {
			return errors.New("Story candidate entity keys must be unique")
		}
		entityKeys[entity.EntityKey] = struct{}{}
		if err := checkEvidence(entity.Evidence, true); err != nil {
			return err
		}
		stateKeys := map[string]struct{}{}
		for _, state := range entity.States {
			if !statePattern.MatchString(state.StateKey) || strings.TrimSpace(state.Label) == "" ||
				state.EpisodeNumbers == nil || state.Ambiguities == nil || !validAssetSpec(state.StateSpec) {
				return errors.New("Story candidate contains an invalid entity state")
			}
			if _, exists := stateKeys[state.StateKey]; exists {
				return errors.New("Story candidate state keys must be unique per entity")
			}
			stateKeys[state.StateKey] = struct{}{}
			if err := checkEvidence(state.Evidence, true); err != nil {
				return err
			}
		}
	}
	worldKeys := map[string]struct{}{}
	for _, entry := range world {
		if !keyPattern.MatchString(entry.EntryKey) || strings.TrimSpace(entry.Category) == "" ||
			strings.TrimSpace(entry.Title) == "" || entry.Facts == nil || entry.Rules == nil ||
			entry.EntityKeys == nil || entry.EpisodeNumbers == nil || entry.Ambiguities == nil {
			return errors.New("Story candidate contains an invalid world entry")
		}
		if _, exists := worldKeys[entry.EntryKey]; exists {
			return errors.New("Story candidate world entry keys must be unique")
		}
		worldKeys[entry.EntryKey] = struct{}{}
		if err := checkEvidence(entry.Evidence, true); err != nil {
			return err
		}
	}
	claimKeys := map[string]struct{}{}
	for _, claim := range claims {
		if !keyPattern.MatchString(claim.ClaimKey) || !oneOf(claim.ClaimType, "relationship", "causal", "continuity", "foreshadowing") ||
			len(claim.ParticipantKeys) == 0 || len(claim.AnchorKeys) == 0 || strings.TrimSpace(claim.Scope) == "" ||
			!oneOf(claim.Polarity, "positive", "negative", "mixed", "unknown") ||
			!oneOf(claim.Status, "proposed", "ambiguous", "conflicted") {
			return errors.New("Story candidate contains an invalid claim")
		}
		if _, exists := claimKeys[claim.ClaimKey]; exists {
			return errors.New("Story candidate claim keys must be unique")
		}
		claimKeys[claim.ClaimKey] = struct{}{}
		if err := checkEvidence(claim.Evidence, true); err != nil {
			return err
		}
	}
	arcKeys := map[string]struct{}{}
	for _, arc := range arcs {
		if !keyPattern.MatchString(arc.ArcKey) || strings.TrimSpace(arc.Title) == "" || strings.TrimSpace(arc.Summary) == "" {
			return errors.New("Story candidate contains an invalid arc")
		}
		if _, exists := arcKeys[arc.ArcKey]; exists {
			return errors.New("Story candidate arc keys must be unique")
		}
		arcKeys[arc.ArcKey] = struct{}{}
		if err := checkEvidence(arc.Evidence, true); err != nil {
			return err
		}
	}
	issueKeys := map[string]struct{}{}
	for _, issue := range append(append([]ReviewIssue(nil), issues...), conflicts...) {
		if !keyPattern.MatchString(issue.IssueKey) || strings.TrimSpace(issue.Code) == "" ||
			!oneOf(issue.Severity, "warning", "blocking") || strings.TrimSpace(issue.Scope) == "" ||
			strings.TrimSpace(issue.Summary) == "" {
			return errors.New("Story candidate contains an invalid issue")
		}
		if _, exists := issueKeys[issue.IssueKey]; exists {
			return errors.New("Story candidate issue keys must be unique")
		}
		issueKeys[issue.IssueKey] = struct{}{}
		if err := checkEvidence(issue.Evidence, false); err != nil {
			return err
		}
	}
	return nil
}

func validAssetSpec(value AssetSpecCandidate) bool {
	if value.Temperament == nil || value.Goals == nil || value.Relationships == nil ||
		value.VisualElements == nil || value.NegativeConstraints == nil || value.PerformanceTraits == nil || value.AllowedUsage == nil {
		return false
	}
	return value.SourceKind == nil || oneOf(*value.SourceKind, "synthetic_recording", "human_recording", "voice_clone")
}

func storyEvidenceKey(value Evidence) string {
	episode := ""
	if value.EpisodeNumber != nil {
		episode = fmt.Sprint(*value.EpisodeNumber)
	}
	return fmt.Sprintf("%d:%d:%s:%s:%s", value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor, episode)
}

func StoryAnalysisCandidateEvidence(value StoryAnalysisCandidate) []Evidence {
	return storyCandidateEvidence(value.Entities, value.WorldEntries, value.Claims, value.Arcs, value.ReviewIssues, nil)
}

func StoryReconciliationCandidateEvidence(value StoryReconciliationCandidate) []Evidence {
	return storyCandidateEvidence(value.CanonicalEntities, value.CanonicalWorldEntries, value.MergedClaims, value.MergedArcs, value.ReviewIssues, value.Conflicts)
}

func storyCandidateEvidence(
	entities []StoryEntityCandidate,
	world []StoryWorldEntryCandidate,
	claims []StoryClaimCandidate,
	arcs []StoryArcCandidate,
	issues []ReviewIssue,
	conflicts []ReviewIssue,
) []Evidence {
	result := []Evidence{}
	for _, entity := range entities {
		result = append(result, entity.Evidence...)
		for _, state := range entity.States {
			result = append(result, state.Evidence...)
		}
	}
	for _, entry := range world {
		result = append(result, entry.Evidence...)
	}
	for _, claim := range claims {
		result = append(result, claim.Evidence...)
	}
	for _, arc := range arcs {
		result = append(result, arc.Evidence...)
	}
	for _, issue := range append(append([]ReviewIssue(nil), issues...), conflicts...) {
		result = append(result, issue.Evidence...)
	}
	return uniqueStoryEvidence(result)
}

func uniqueStoryEvidence(values []Evidence) []Evidence {
	seen := make(map[string]struct{}, len(values))
	result := make([]Evidence, 0, len(values))
	for _, value := range values {
		key := storyEvidenceKey(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(left, right Evidence) int {
		return strings.Compare(storyEvidenceKey(left), storyEvidenceKey(right))
	})
	return result
}
