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

type StoryAnalysisEvidenceFragment struct {
	ShardKey              string `json:"evidence_shard_key"`
	LogicalStart          int    `json:"logical_start"`
	LogicalEnd            int    `json:"logical_end"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
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
	SourceHashes              []string `json:"source_hashes"`
	Status                    string   `json:"status"`
}

type StoryReconcileChild struct {
	Stage      string `json:"stage"`
	ShardKey   string `json:"shard_key"`
	SourceHash string `json:"source_hash"`
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
	ManifestID    string               `json:"manifest_id"`
	Version       int64                `json:"version"`
	WorkspaceID   string               `json:"workspace_id"`
	WorkflowRunID string               `json:"workflow_run_id"`
	NodeRunID     string               `json:"node_run_id"`
	Stage         string               `json:"stage"`
	RootInputHash string               `json:"root_input_hash"`
	Shards        []StoryAnalysisShard `json:"shards"`
	CoverageHash  string               `json:"coverage_hash"`
	ManifestHash  string               `json:"manifest_hash"`
}

type StoryReconcileManifest struct {
	ManifestID    string                `json:"manifest_id"`
	Version       int64                 `json:"version"`
	WorkspaceID   string                `json:"workspace_id"`
	WorkflowRunID string                `json:"workflow_run_id"`
	NodeRunID     string                `json:"node_run_id"`
	Stage         string                `json:"stage"`
	RootInputHash string                `json:"root_input_hash"`
	FanIn         int                   `json:"fan_in"`
	RootShardKey  string                `json:"root_shard_key"`
	Shards        []StoryReconcileShard `json:"shards"`
	CoverageHash  string                `json:"coverage_hash"`
	ManifestHash  string                `json:"manifest_hash"`
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
			fragment.LogicalEnd <= fragment.LogicalStart || !hashPattern.MatchString(fragment.CandidateRevisionHash) {
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
			SourceHashes:          []string{fragment.CandidateRevisionHash}, Status: "active",
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
	for index := range analyzeShards {
		analyzeShards[index].ParentKey = parents[AnalyzeStoryStage+"\x00"+analyzeShards[index].Key]
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

func ValidateStoryAnalysisManifests(analyze StoryAnalysisManifest, reconcile StoryReconcileManifest) error {
	for _, identifier := range []string{
		analyze.ManifestID, reconcile.ManifestID, analyze.WorkspaceID, analyze.WorkflowRunID, analyze.NodeRunID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Story analysis manifest owner")
		}
	}
	if analyze.Version != 1 || reconcile.Version != 1 || analyze.Stage != AnalyzeStoryStage ||
		reconcile.Stage != ReconcileStoryStage || analyze.WorkspaceID != reconcile.WorkspaceID ||
		analyze.WorkflowRunID != reconcile.WorkflowRunID || analyze.NodeRunID != reconcile.NodeRunID ||
		reconcile.FanIn != 2 || reconcile.RootInputHash != analyze.ManifestHash ||
		!hashPattern.MatchString(analyze.RootInputHash) || len(analyze.Shards) == 0 || len(reconcile.Shards) == 0 {
		return errors.New("invalid Story analysis manifest relationship")
	}
	analyzeKeys := make(map[string]StoryAnalysisShard, len(analyze.Shards))
	position := 0
	for _, shard := range analyze.Shards {
		if strings.TrimSpace(shard.Key) == "" || strings.TrimSpace(shard.ParentKey) == "" ||
			shard.Kind != "story_map" || shard.Status != "active" || shard.LogicalStart != position ||
			shard.LogicalEnd <= shard.LogicalStart || len(shard.SourceHashes) != 1 ||
			shard.SourceHashes[0] != shard.UpstreamCandidateHash || !hashPattern.MatchString(shard.UpstreamCandidateHash) {
			return errors.New("invalid Story analysis map shard")
		}
		if _, err := uuid.Parse(shard.UpstreamCandidateRevision); err != nil {
			return errors.New("invalid Story analysis upstream revision")
		}
		if _, exists := analyzeKeys[shard.Key]; exists {
			return errors.New("duplicate Story analysis map shard")
		}
		analyzeKeys[shard.Key] = shard
		position = shard.LogicalEnd
	}
	reconcileKeys := make(map[string]StoryReconcileShard, len(reconcile.Shards))
	rootCount := 0
	for _, shard := range reconcile.Shards {
		if strings.TrimSpace(shard.Key) == "" || shard.Kind != "story_reduce" || shard.Status != "active" ||
			shard.Level < 0 || len(shard.Children) == 0 || len(shard.Children) > reconcile.FanIn ||
			len(shard.SourceHashes) != len(shard.Children) || !hashPattern.MatchString(shard.SubtreeHash) {
			return errors.New("invalid Story reconcile shard")
		}
		for index, child := range shard.Children {
			if (child.Stage != AnalyzeStoryStage && child.Stage != ReconcileStoryStage) ||
				strings.TrimSpace(child.ShardKey) == "" || !hashPattern.MatchString(child.SourceHash) ||
				shard.SourceHashes[index] != child.SourceHash {
				return errors.New("invalid Story reconcile child")
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
		if shard.ParentKey == "" {
			rootCount++
			if shard.Key != reconcile.RootShardKey {
				return errors.New("Story reconcile root key mismatch")
			}
		}
	}
	if rootCount != 1 {
		return errors.New("Story reconcile tree must have one root")
	}
	for _, shard := range reconcile.Shards {
		for _, child := range shard.Children {
			if child.Stage == AnalyzeStoryStage {
				value, exists := analyzeKeys[child.ShardKey]
				if !exists || value.ParentKey != shard.Key || value.UpstreamCandidateHash != child.SourceHash {
					return errors.New("Story reconcile map child has drifted")
				}
				continue
			}
			value, exists := reconcileKeys[child.ShardKey]
			if !exists || value.ParentKey != shard.Key || value.SubtreeHash != child.SourceHash || value.Level >= shard.Level {
				return errors.New("Story reconcile tree child has drifted")
			}
		}
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
		Schema        string               `json:"schema"`
		Version       int64                `json:"version"`
		WorkspaceID   string               `json:"workspace_id"`
		WorkflowRunID string               `json:"workflow_run_id"`
		NodeRunID     string               `json:"node_run_id"`
		Stage         string               `json:"stage"`
		RootInputHash string               `json:"root_input_hash"`
		Shards        []StoryAnalysisShard `json:"shards"`
		CoverageHash  string               `json:"coverage_hash"`
	}{"story-analysis-shard-manifest-v1", value.Version, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.Shards, value.CoverageHash})
}

func storyReconcileManifestHash(value StoryReconcileManifest) (string, error) {
	return CanonicalStoryHash(struct {
		Schema        string                `json:"schema"`
		Version       int64                 `json:"version"`
		WorkspaceID   string                `json:"workspace_id"`
		WorkflowRunID string                `json:"workflow_run_id"`
		NodeRunID     string                `json:"node_run_id"`
		Stage         string                `json:"stage"`
		RootInputHash string                `json:"root_input_hash"`
		FanIn         int                   `json:"fan_in"`
		RootShardKey  string                `json:"root_shard_key"`
		Shards        []StoryReconcileShard `json:"shards"`
		CoverageHash  string                `json:"coverage_hash"`
	}{"story-reconcile-shard-manifest-v1", value.Version, value.WorkspaceID, value.WorkflowRunID,
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
