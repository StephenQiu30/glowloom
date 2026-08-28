package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const StoryGraphWireSchemaVersion = "storygraph-stage-wire-v1"

var storyboardStoryNodePattern = regexp.MustCompile(`^sgn_[0-9a-f]{64}$`)

var storyGraphStages = map[string]struct{}{
	"extract_source_evidence": {}, "analyze_story": {}, "reconcile_story": {}, "segment_episodes": {},
	"analyze_episode": {}, "reconcile_episode": {}, "draft_storyboard": {}, "detail_shots": {},
	"review_storygraph": {}, "repair_candidate": {},
}

type StageExecutionPolicy struct {
	DefinitionKey        string   `json:"definition_key"`
	DefinitionVersion    string   `json:"definition_version"`
	PromptVersion        string   `json:"prompt_version"`
	SkillBundleVersion   string   `json:"skill_bundle_version"`
	SkillBundleHash      string   `json:"skill_bundle_hash"`
	OutputSchemaVersion  string   `json:"output_schema_version"`
	ModelCapability      string   `json:"model_capability"`
	CodexRuntimeContract string   `json:"codex_runtime_contract"`
	AllowedTools         []string `json:"allowed_tools"`
	MaxModelCalls        int      `json:"max_model_calls"`
	MaxExecutionSeconds  int      `json:"max_execution_seconds"`
}

func (value StageExecutionPolicy) Validate() error {
	return StoryGraphDefinition().ValidatePolicy(value)
}

func (value StageExecutionPolicy) Hash() (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return CanonicalHash(encoded)
}

type StageSourceRef struct {
	OwnerKind      string `json:"owner_kind"`
	OwnerLogicalID string `json:"owner_logical_id"`
	OwnerVersionID string `json:"owner_version_id"`
	Revision       int64  `json:"revision"`
	ContentHash    string `json:"content_hash"`
}

type StageUpstreamCandidateRef struct {
	Stage                 string `json:"stage"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
}

type ShardManifestRef struct {
	ManifestID string `json:"manifest_id"`
	Version    int64  `json:"version"`
	Hash       string `json:"hash"`
}

type InvocationShard struct {
	Kind          string `json:"kind"`
	Key           string `json:"key"`
	TreePath      string `json:"tree_path"`
	ParentKey     string `json:"parent_key,omitempty"`
	AbsoluteStart *int   `json:"absolute_start,omitempty"`
	AbsoluteEnd   *int   `json:"absolute_end,omitempty"`
}

type SourceEvidenceEpisodeMarkerHint struct {
	EpisodeNumber int    `json:"episode_number"`
	Label         string `json:"label"`
	AbsoluteStart int    `json:"absolute_start"`
	AbsoluteEnd   int    `json:"absolute_end"`
}

type SourceEvidenceStageInput struct {
	DocumentRevisionID string                            `json:"document_revision_id"`
	NormalizedHash     string                            `json:"normalized_hash"`
	LogicalSourceHash  string                            `json:"logical_source_hash"`
	LogicalStart       int                               `json:"logical_start"`
	LogicalEnd         int                               `json:"logical_end"`
	ContextStart       int                               `json:"context_start"`
	ContextEnd         int                               `json:"context_end"`
	NormalizedText     string                            `json:"normalized_text"`
	EpisodeMarkerHints []SourceEvidenceEpisodeMarkerHint `json:"episode_marker_hints"`
}

type StoryAnalysisStageInput struct {
	EvidenceShardKey              string          `json:"evidence_shard_key"`
	EvidenceCandidateRevisionID   string          `json:"evidence_candidate_revision_id"`
	EvidenceCandidateRevisionHash string          `json:"evidence_candidate_revision_hash"`
	LogicalStart                  int             `json:"logical_start"`
	LogicalEnd                    int             `json:"logical_end"`
	CandidateItemStart            int             `json:"candidate_item_start"`
	CandidateItemEnd              int             `json:"candidate_item_end"`
	EvidenceCandidate             json.RawMessage `json:"evidence_candidate"`
}

type EpisodeSegmentationEvidence struct {
	SourceStart   int    `json:"source_start"`
	SourceEnd     int    `json:"source_end"`
	TextHash      string `json:"text_hash"`
	ExactAnchor   string `json:"exact_anchor"`
	EpisodeNumber *int   `json:"episode_number"`
}

type EpisodeSegmentationEvidenceLeaf struct {
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
}

type EpisodeSegmentationMarkerHint struct {
	EpisodeNumber int                         `json:"episode_number"`
	Label         string                      `json:"label"`
	Evidence      EpisodeSegmentationEvidence `json:"evidence"`
}

type EpisodeSegmentationEvidenceIndexItem struct {
	IndexKey              string                      `json:"index_key"`
	Kind                  string                      `json:"kind"`
	Label                 string                      `json:"label"`
	ShardKey              string                      `json:"shard_key"`
	CandidateRevisionID   string                      `json:"candidate_revision_id"`
	CandidateRevisionHash string                      `json:"candidate_revision_hash"`
	Evidence              EpisodeSegmentationEvidence `json:"evidence"`
}

type EpisodeSegmentationStageInput struct {
	DocumentRevisionID            string                                 `json:"document_revision_id"`
	NormalizedHash                string                                 `json:"normalized_hash"`
	SourceCodePoints              int                                    `json:"source_code_points"`
	TargetDurationMS              int                                    `json:"target_duration_ms"`
	BibleVersionID                string                                 `json:"bible_version_id"`
	BibleVersion                  int                                    `json:"bible_version"`
	BibleContentHash              string                                 `json:"bible_content_hash"`
	MaterializationHash           string                                 `json:"materialization_hash"`
	EvidenceAggregateRevisionID   string                                 `json:"evidence_aggregate_revision_id"`
	EvidenceAggregateRevisionHash string                                 `json:"evidence_aggregate_revision_hash"`
	EvidenceLeaves                []EpisodeSegmentationEvidenceLeaf      `json:"evidence_leaves"`
	MarkerHints                   []EpisodeSegmentationMarkerHint        `json:"marker_hints"`
	EvidenceIndex                 []EpisodeSegmentationEvidenceIndexItem `json:"evidence_index"`
}

type EpisodeSceneMarkerHint struct {
	Label         string `json:"label"`
	AbsoluteStart int    `json:"absolute_start"`
	AbsoluteEnd   int    `json:"absolute_end"`
}

type EpisodeAdjacentContext struct {
	Side            string `json:"side"`
	EpisodeID       string `json:"episode_id"`
	EpisodePosition int    `json:"episode_position"`
	ScriptVersionID string `json:"script_version_id"`
	ScriptVersionNo int    `json:"script_version_no"`
	SourceStart     int    `json:"source_start"`
	SourceEnd       int    `json:"source_end"`
	ContentHash     string `json:"content_hash"`
	ExcerptStart    int    `json:"excerpt_start"`
	ExcerptEnd      int    `json:"excerpt_end"`
	Excerpt         string `json:"excerpt"`
	ExcerptHash     string `json:"excerpt_hash"`
}

type EpisodeKnownState struct {
	StateKey     string `json:"state_key"`
	AssetStateID string `json:"asset_state_id"`
	ContentHash  string `json:"content_hash"`
}

type EpisodeKnownIdentity struct {
	EntityKey              string              `json:"entity_key"`
	Kind                   string              `json:"kind"`
	AssetID                string              `json:"asset_id"`
	SpecificationVersionID string              `json:"specification_version_id"`
	SpecificationHash      string              `json:"specification_hash"`
	States                 []EpisodeKnownState `json:"states"`
}

type EpisodeAnalysisStageInput struct {
	EpisodeID           string                   `json:"episode_id"`
	EpisodePosition     int                      `json:"episode_position"`
	ScriptVersionID     string                   `json:"script_version_id"`
	ScriptVersionNo     int                      `json:"script_version_no"`
	DocumentRevisionID  string                   `json:"document_revision_id"`
	EpisodeSourceStart  int                      `json:"episode_source_start"`
	EpisodeSourceEnd    int                      `json:"episode_source_end"`
	ScriptContentHash   string                   `json:"script_content_hash"`
	LogicalStart        int                      `json:"logical_start"`
	LogicalEnd          int                      `json:"logical_end"`
	ContextStart        int                      `json:"context_start"`
	ContextEnd          int                      `json:"context_end"`
	ContextText         string                   `json:"context_text"`
	LogicalTextHash     string                   `json:"logical_text_hash"`
	SceneMarkerHints    []EpisodeSceneMarkerHint `json:"scene_marker_hints"`
	AdjacentEpisodes    []EpisodeAdjacentContext `json:"adjacent_episodes"`
	BibleVersionID      string                   `json:"bible_version_id"`
	BibleVersion        int                      `json:"bible_version"`
	BibleContentHash    string                   `json:"bible_content_hash"`
	BibleSnapshotHash   string                   `json:"bible_snapshot_hash"`
	BibleSnapshot       json.RawMessage          `json:"bible_snapshot"`
	MaterializationHash string                   `json:"materialization_hash"`
	KnownIdentities     []EpisodeKnownIdentity   `json:"known_identities"`
}

type EpisodeReconciliationInputCandidate struct {
	ShardKey              string          `json:"shard_key"`
	CandidateRevisionID   string          `json:"candidate_revision_id"`
	CandidateRevisionHash string          `json:"candidate_revision_hash"`
	Candidate             json.RawMessage `json:"candidate"`
}

type EpisodeReconciliationStageInput struct {
	EpisodeID           string                                `json:"episode_id"`
	EpisodePosition     int                                   `json:"episode_position"`
	ScriptVersionID     string                                `json:"script_version_id"`
	ScriptVersionNo     int                                   `json:"script_version_no"`
	EpisodeSourceStart  int                                   `json:"episode_source_start"`
	EpisodeSourceEnd    int                                   `json:"episode_source_end"`
	ScriptContentHash   string                                `json:"script_content_hash"`
	BibleVersionID      string                                `json:"bible_version_id"`
	BibleVersion        int                                   `json:"bible_version"`
	BibleContentHash    string                                `json:"bible_content_hash"`
	MaterializationHash string                                `json:"materialization_hash"`
	KnownIdentities     []EpisodeKnownIdentity                `json:"known_identities"`
	Level               int                                   `json:"level"`
	CandidateType       string                                `json:"candidate_type"`
	Candidates          []EpisodeReconciliationInputCandidate `json:"candidates"`
}

type StoryboardEvidenceRef struct {
	DocumentRevisionID string `json:"document_revision_id"`
	AbsoluteStart      int    `json:"absolute_start"`
	AbsoluteEnd        int    `json:"absolute_end"`
	TextHash           string `json:"text_hash"`
}

type StoryboardSceneInput struct {
	StoryNodeKey    string                  `json:"story_node_key"`
	OwnerVersionID  string                  `json:"owner_version_id"`
	OwnerRevision   int64                   `json:"owner_revision"`
	OwnerHash       string                  `json:"owner_hash"`
	EpisodeID       string                  `json:"episode_id"`
	EpisodePosition int                     `json:"episode_position"`
	ScenePosition   int                     `json:"scene_position"`
	Heading         string                  `json:"heading"`
	Evidence        []StoryboardEvidenceRef `json:"evidence"`
}

type StoryboardBeatInput struct {
	StoryNodeKey        string                  `json:"story_node_key"`
	Summary             string                  `json:"summary"`
	RequiredForCoverage bool                    `json:"required_for_coverage"`
	Evidence            []StoryboardEvidenceRef `json:"evidence"`
}

type StoryboardDialogueInput struct {
	StoryNodeKey string                  `json:"story_node_key"`
	Speaker      string                  `json:"speaker"`
	Text         string                  `json:"text"`
	Evidence     []StoryboardEvidenceRef `json:"evidence"`
}

type StoryboardOccurrenceInput struct {
	StoryNodeKey              string                  `json:"story_node_key"`
	IdentityStoryNodeKey      string                  `json:"identity_story_node_key"`
	SpecificationStoryNodeKey string                  `json:"specification_story_node_key"`
	AssetStateStoryNodeKey    string                  `json:"asset_state_story_node_key"`
	AssetID                   string                  `json:"asset_id"`
	SpecificationVersionID    string                  `json:"specification_version_id"`
	AssetStateID              string                  `json:"asset_state_id"`
	AssetKind                 string                  `json:"asset_kind"`
	Summary                   string                  `json:"summary"`
	Evidence                  []StoryboardEvidenceRef `json:"evidence"`
}

type StoryboardStyleSnapshotInput struct {
	OwnerVersionID string `json:"owner_version_id"`
	Revision       int64  `json:"revision"`
	ContentHash    string `json:"content_hash"`
	VisualStyle    string `json:"visual_style"`
	AspectRatio    string `json:"aspect_ratio"`
}

type StoryboardAssetVersionInput struct {
	AssetID           string   `json:"asset_id"`
	AssetStateID      string   `json:"asset_state_id"`
	AssetVersionID    string   `json:"asset_version_id"`
	Revision          int64    `json:"revision"`
	ContentHash       string   `json:"content_hash"`
	LineageHash       string   `json:"lineage_hash"`
	StyleSnapshotHash string   `json:"style_snapshot_hash"`
	ViewRoles         []string `json:"view_roles"`
	Status            string   `json:"status"`
}

type StoryboardDraftStageInput struct {
	GraphVersionNo         int64                         `json:"graph_version_no"`
	Scene                  StoryboardSceneInput          `json:"scene"`
	Beats                  []StoryboardBeatInput         `json:"beats"`
	Dialogues              []StoryboardDialogueInput     `json:"dialogues"`
	Occurrences            []StoryboardOccurrenceInput   `json:"occurrences"`
	EffectiveStyleSnapshot StoryboardStyleSnapshotInput  `json:"effective_style_snapshot"`
	TargetDurationMS       int                           `json:"target_duration_ms"`
	AssetVersions          []StoryboardAssetVersionInput `json:"asset_versions"`
}

type StoryReconciliationInputCandidate struct {
	ShardKey              string          `json:"shard_key"`
	CandidateRevisionID   string          `json:"candidate_revision_id"`
	CandidateRevisionHash string          `json:"candidate_revision_hash"`
	CandidateItemStart    *int            `json:"candidate_item_start,omitempty"`
	CandidateItemEnd      *int            `json:"candidate_item_end,omitempty"`
	Candidate             json.RawMessage `json:"candidate"`
}

type StoryReconciliationStageInput struct {
	Level         int                                 `json:"level"`
	CandidateType string                              `json:"candidate_type"`
	Candidates    []StoryReconciliationInputCandidate `json:"candidates"`
}

type StageInvocationPayload struct {
	Stage                   string                      `json:"stage"`
	ShardKey                string                      `json:"shard_key"`
	WorkspaceID             string                      `json:"workspace_id"`
	ProjectID               string                      `json:"project_id"`
	SourceRefs              []StageSourceRef            `json:"source_refs"`
	BaseStoryGraphVersionID string                      `json:"base_storygraph_version_id,omitempty"`
	BaseStoryGraphHash      string                      `json:"base_storygraph_hash,omitempty"`
	UpstreamCandidates      []StageUpstreamCandidateRef `json:"upstream_candidates"`
	ShardManifestRef        ShardManifestRef            `json:"shard_manifest_ref"`
	Shard                   InvocationShard             `json:"shard"`
	StageInput              json.RawMessage             `json:"stage_input"`
}

type StageInvocation struct {
	InvocationID      string                 `json:"invocation_id"`
	Kind              string                 `json:"kind"`
	WireSchemaVersion string                 `json:"wire_schema_version"`
	InputHash         string                 `json:"input_hash"`
	ExecutionPolicy   StageExecutionPolicy   `json:"execution_policy"`
	Payload           StageInvocationPayload `json:"payload"`
}

func NewStageInvocation(invocationID string, policy StageExecutionPolicy, payload StageInvocationPayload) (StageInvocation, error) {
	value := StageInvocation{
		InvocationID: invocationID, Kind: "storygraph_stage",
		WireSchemaVersion: StoryGraphWireSchemaVersion, ExecutionPolicy: policy, Payload: payload,
	}
	inputHash, err := value.ComputeInputHash()
	if err != nil {
		return StageInvocation{}, err
	}
	value.InputHash = inputHash
	if err = value.Validate(); err != nil {
		return StageInvocation{}, err
	}
	return value, nil
}

func DecodeStageInvocation(raw []byte) (StageInvocation, error) {
	var value StageInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return StageInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return StageInvocation{}, err
	}
	return value, nil
}

func (value StageInvocation) Validate() error {
	if _, err := uuid.Parse(value.InvocationID); err != nil || value.Kind != "storygraph_stage" || value.WireSchemaVersion != StoryGraphWireSchemaVersion || !hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid StoryGraph stage invocation identity")
	}
	if err := value.ExecutionPolicy.Validate(); err != nil {
		return err
	}
	if _, ok := storyGraphStages[value.Payload.Stage]; !ok || strings.TrimSpace(value.Payload.ShardKey) == "" || value.Payload.ShardKey != value.Payload.Shard.Key || strings.TrimSpace(value.Payload.Shard.Kind) == "" || strings.TrimSpace(value.Payload.Shard.TreePath) == "" || !jsonObject(value.Payload.StageInput) {
		return errors.New("invalid StoryGraph stage payload")
	}
	if _, err := uuid.Parse(value.Payload.WorkspaceID); err != nil {
		return errors.New("invalid StoryGraph workspace")
	}
	if _, err := uuid.Parse(value.Payload.ProjectID); err != nil {
		return errors.New("invalid StoryGraph project")
	}
	if err := validateStageRefs(value.Payload); err != nil {
		return err
	}
	if err := validateStageInput(value.Payload); err != nil {
		return err
	}
	computed, err := value.ComputeInputHash()
	if err != nil {
		return err
	}
	if computed != value.InputHash {
		return fmt.Errorf("StoryGraph input hash mismatch: got %s want %s", value.InputHash, computed)
	}
	return nil
}

func validateStageInput(payload StageInvocationPayload) error {
	switch payload.Stage {
	case "extract_source_evidence":
		return validateSourceEvidenceStageInput(payload)
	case "segment_episodes":
		return validateEpisodeSegmentationStageInput(payload)
	case "analyze_episode":
		return validateEpisodeAnalysisStageInput(payload)
	case "reconcile_episode":
		return validateEpisodeReconciliationStageInput(payload)
	case "draft_storyboard":
		return validateStoryboardDraftStageInput(payload)
	case "review_storygraph":
		return validateStoryGraphReviewStageInput(payload)
	case "repair_candidate":
		return validateStoryGraphRepairStageInput(payload)
	}
	return nil
}

func validateStoryboardDraftStageInput(payload StageInvocationPayload) error {
	var input StoryboardDraftStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Storyboard Draft stage input")
	}
	if input.GraphVersionNo < 1 || input.TargetDurationMS < 1000 || input.TargetDurationMS > 7_200_000 ||
		len(input.Beats) == 0 || len(input.Occurrences) == 0 || len(payload.SourceRefs) != 2 ||
		len(payload.UpstreamCandidates) != 0 || payload.BaseStoryGraphVersionID == "" ||
		payload.BaseStoryGraphHash == "" || payload.Shard.Kind != "story_scene" ||
		payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil ||
		payload.ShardKey != "scene:"+input.Scene.StoryNodeKey ||
		!storyboardStoryNodePattern.MatchString(input.Scene.StoryNodeKey) || !hashPattern.MatchString(input.Scene.OwnerHash) ||
		input.Scene.OwnerRevision < 1 || input.Scene.EpisodePosition < 1 || input.Scene.ScenePosition < 1 ||
		strings.TrimSpace(input.Scene.Heading) == "" || len(input.Scene.Evidence) == 0 ||
		input.EffectiveStyleSnapshot.Revision < 1 ||
		!hashPattern.MatchString(input.EffectiveStyleSnapshot.ContentHash) ||
		strings.TrimSpace(input.EffectiveStyleSnapshot.VisualStyle) == "" ||
		strings.TrimSpace(input.EffectiveStyleSnapshot.AspectRatio) == "" {
		return errors.New("invalid Storyboard Draft exact dependencies")
	}
	for _, identifier := range []string{
		payload.BaseStoryGraphVersionID, input.Scene.OwnerVersionID, input.Scene.EpisodeID,
		input.EffectiveStyleSnapshot.OwnerVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Storyboard Draft exact revision")
		}
	}
	if err := validateStoryboardEvidence(input.Scene.Evidence); err != nil {
		return err
	}
	previous := ""
	for _, beat := range input.Beats {
		if !storyboardStoryNodePattern.MatchString(beat.StoryNodeKey) || previous >= beat.StoryNodeKey ||
			strings.TrimSpace(beat.Summary) == "" || len(beat.Evidence) == 0 {
			return errors.New("Storyboard Draft Beats must be exact, unique, and sorted")
		}
		if err := validateStoryboardEvidence(beat.Evidence); err != nil {
			return err
		}
		previous = beat.StoryNodeKey
	}
	previous = ""
	for _, dialogue := range input.Dialogues {
		if !storyboardStoryNodePattern.MatchString(dialogue.StoryNodeKey) || previous >= dialogue.StoryNodeKey ||
			strings.TrimSpace(dialogue.Speaker) == "" || strings.TrimSpace(dialogue.Text) == "" || len(dialogue.Evidence) == 0 {
			return errors.New("Storyboard Draft Dialogues must be exact, unique, and sorted")
		}
		if err := validateStoryboardEvidence(dialogue.Evidence); err != nil {
			return err
		}
		previous = dialogue.StoryNodeKey
	}
	previous = ""
	allowedKinds := map[string]struct{}{"character": {}, "location": {}, "prop": {}}
	for _, occurrence := range input.Occurrences {
		if !storyboardStoryNodePattern.MatchString(occurrence.StoryNodeKey) || previous >= occurrence.StoryNodeKey ||
			!storyboardStoryNodePattern.MatchString(occurrence.IdentityStoryNodeKey) ||
			!storyboardStoryNodePattern.MatchString(occurrence.SpecificationStoryNodeKey) ||
			!storyboardStoryNodePattern.MatchString(occurrence.AssetStateStoryNodeKey) ||
			strings.TrimSpace(occurrence.Summary) == "" || len(occurrence.Evidence) == 0 {
			return errors.New("Storyboard Draft Occurrences must be exact, unique, and sorted")
		}
		if _, ok := allowedKinds[occurrence.AssetKind]; !ok {
			return errors.New("Storyboard Draft Occurrence has an invalid Asset kind")
		}
		for _, identifier := range []string{occurrence.AssetID, occurrence.SpecificationVersionID, occurrence.AssetStateID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("Storyboard Draft Occurrence has an invalid formal reference")
			}
		}
		if err := validateStoryboardEvidence(occurrence.Evidence); err != nil {
			return err
		}
		previous = occurrence.StoryNodeKey
	}
	previous = ""
	for _, version := range input.AssetVersions {
		key := strings.Join([]string{version.AssetID, version.AssetStateID, version.AssetVersionID}, "\x00")
		if previous >= key || version.Revision < 1 || version.Status != "READY" ||
			!hashPattern.MatchString(version.ContentHash) || !hashPattern.MatchString(version.LineageHash) ||
			!hashPattern.MatchString(version.StyleSnapshotHash) || len(version.ViewRoles) == 0 {
			return errors.New("Storyboard Draft AssetVersions must be exact, READY, unique, and sorted")
		}
		for _, identifier := range []string{version.AssetID, version.AssetStateID, version.AssetVersionID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("Storyboard Draft AssetVersion has an invalid formal reference")
			}
		}
		previous = key
	}
	expectedSources := []StageSourceRef{
		{
			OwnerKind: "production/storygraph", OwnerLogicalID: payload.ProjectID,
			OwnerVersionID: payload.BaseStoryGraphVersionID, Revision: input.GraphVersionNo,
			ContentHash: payload.BaseStoryGraphHash,
		},
		{
			OwnerKind: "preset/effective-style", OwnerLogicalID: payload.ProjectID,
			OwnerVersionID: input.EffectiveStyleSnapshot.OwnerVersionID,
			Revision:       input.EffectiveStyleSnapshot.Revision, ContentHash: input.EffectiveStyleSnapshot.ContentHash,
		},
	}
	return validateExactStageSources(payload.SourceRefs, expectedSources, "Storyboard Draft")
}

func validateStoryboardEvidence(values []StoryboardEvidenceRef) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.AbsoluteStart < 0 || value.AbsoluteEnd <= value.AbsoluteStart ||
			!hashPattern.MatchString(value.TextHash) {
			return errors.New("invalid Storyboard Draft Evidence")
		}
		if _, err := uuid.Parse(value.DocumentRevisionID); err != nil {
			return errors.New("invalid Storyboard Draft Evidence")
		}
		key := fmt.Sprintf("%s:%d:%d:%s", value.DocumentRevisionID, value.AbsoluteStart, value.AbsoluteEnd, value.TextHash)
		if _, duplicate := seen[key]; duplicate {
			return errors.New("duplicate Storyboard Draft Evidence")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ValidateEpisodeAnalysisInvocation applies the Production Episode owner
// contract after the shared StoryGraph wire contract has been decoded.
func ValidateEpisodeAnalysisInvocation(value StageInvocation) error {
	if err := value.Validate(); err != nil {
		return err
	}
	switch value.Payload.Stage {
	case "analyze_episode":
		return validateEpisodeAnalysisStageInput(value.Payload)
	case "reconcile_episode":
		return validateEpisodeReconciliationStageInput(value.Payload)
	default:
		return errors.New("invalid Episode analysis invocation stage")
	}
}

func validateEpisodeAnalysisStageInput(payload StageInvocationPayload) error {
	var input EpisodeAnalysisStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Episode analysis stage input")
	}
	if len(input.AdjacentEpisodes) > 2 || len(payload.UpstreamCandidates) != 0 ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "episode_map" || payload.Shard.AbsoluteStart == nil ||
		payload.Shard.AbsoluteEnd == nil || input.EpisodePosition < 1 || input.ScriptVersionNo < 1 ||
		input.BibleVersion < 1 || input.EpisodeSourceStart < 0 ||
		input.EpisodeSourceEnd <= input.EpisodeSourceStart || input.LogicalStart < input.EpisodeSourceStart ||
		input.LogicalEnd <= input.LogicalStart || input.LogicalEnd > input.EpisodeSourceEnd ||
		input.ContextStart < input.EpisodeSourceStart || input.ContextStart > input.LogicalStart ||
		input.ContextEnd < input.LogicalEnd || input.ContextEnd > input.EpisodeSourceEnd ||
		input.ContextEnd-input.ContextStart != utf8.RuneCountInString(input.ContextText) ||
		*payload.Shard.AbsoluteStart != input.LogicalStart || *payload.Shard.AbsoluteEnd != input.LogicalEnd ||
		!hashPattern.MatchString(input.ScriptContentHash) || !hashPattern.MatchString(input.LogicalTextHash) ||
		!hashPattern.MatchString(input.BibleContentHash) || !hashPattern.MatchString(input.BibleSnapshotHash) ||
		!hashPattern.MatchString(input.MaterializationHash) || !jsonObject(input.BibleSnapshot) {
		return errors.New("invalid Episode analysis stage dependencies")
	}
	for _, identifier := range []string{
		input.EpisodeID, input.ScriptVersionID, input.DocumentRevisionID, input.BibleVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode analysis exact revision")
		}
	}
	contextRunes := []rune(input.ContextText)
	relativeStart := input.LogicalStart - input.ContextStart
	relativeEnd := input.LogicalEnd - input.ContextStart
	if sourceTextHash(string(contextRunes[relativeStart:relativeEnd])) != input.LogicalTextHash {
		return errors.New("Episode analysis logical text hash mismatch")
	}
	if input.ContextStart == input.EpisodeSourceStart && input.ContextEnd == input.EpisodeSourceEnd &&
		sourceTextHash(input.ContextText) != input.ScriptContentHash {
		return errors.New("Episode script content hash mismatch")
	}
	snapshotHash, err := CanonicalHash(input.BibleSnapshot)
	if err != nil || snapshotHash != input.BibleSnapshotHash {
		return errors.New("Episode Bible snapshot hash mismatch")
	}
	previousMarkerStart := -1
	for _, marker := range input.SceneMarkerHints {
		if strings.TrimSpace(marker.Label) == "" || marker.AbsoluteStart <= previousMarkerStart ||
			marker.AbsoluteStart < input.ContextStart || marker.AbsoluteEnd <= marker.AbsoluteStart ||
			marker.AbsoluteEnd > input.ContextEnd ||
			string(contextRunes[marker.AbsoluteStart-input.ContextStart:marker.AbsoluteEnd-input.ContextStart]) != marker.Label {
			return errors.New("Episode scene marker does not match its frozen context")
		}
		previousMarkerStart = marker.AbsoluteStart
	}
	previousSide := -1
	for _, adjacent := range input.AdjacentEpisodes {
		side := 0
		switch adjacent.Side {
		case "previous":
			if adjacent.EpisodePosition >= input.EpisodePosition {
				return errors.New("invalid previous Episode context")
			}
		case "next":
			side = 1
			if adjacent.EpisodePosition <= input.EpisodePosition {
				return errors.New("invalid next Episode context")
			}
		default:
			return errors.New("invalid Episode adjacent side")
		}
		if side <= previousSide || adjacent.EpisodeID == input.EpisodeID ||
			adjacent.ScriptVersionID == input.ScriptVersionID || adjacent.EpisodePosition < 1 ||
			adjacent.ScriptVersionNo < 1 || adjacent.SourceStart < 0 || adjacent.SourceEnd <= adjacent.SourceStart ||
			adjacent.ExcerptStart < adjacent.SourceStart || adjacent.ExcerptEnd <= adjacent.ExcerptStart ||
			adjacent.ExcerptEnd > adjacent.SourceEnd ||
			adjacent.ExcerptEnd-adjacent.ExcerptStart != utf8.RuneCountInString(adjacent.Excerpt) ||
			strings.TrimSpace(adjacent.Excerpt) == "" || sourceTextHash(adjacent.Excerpt) != adjacent.ExcerptHash ||
			!hashPattern.MatchString(adjacent.ContentHash) || !hashPattern.MatchString(adjacent.ExcerptHash) {
			return errors.New("invalid Episode adjacent context")
		}
		for _, identifier := range []string{adjacent.EpisodeID, adjacent.ScriptVersionID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("invalid Episode adjacent exact revision")
			}
		}
		previousSide = side
	}
	if err := validateEpisodeKnownIdentities(input.KnownIdentities); err != nil {
		return err
	}
	expectedSources := []StageSourceRef{
		{OwnerKind: "production/episode-script", OwnerLogicalID: input.EpisodeID, OwnerVersionID: input.ScriptVersionID, Revision: int64(input.ScriptVersionNo), ContentHash: input.ScriptContentHash},
		{OwnerKind: "production/bible-version", OwnerLogicalID: input.BibleVersionID, OwnerVersionID: input.BibleVersionID, Revision: int64(input.BibleVersion), ContentHash: input.BibleContentHash},
		{OwnerKind: "production/bible-materialization", OwnerLogicalID: input.BibleVersionID, OwnerVersionID: input.BibleVersionID, Revision: int64(input.BibleVersion), ContentHash: input.MaterializationHash},
	}
	for _, adjacent := range input.AdjacentEpisodes {
		expectedSources = append(expectedSources, StageSourceRef{
			OwnerKind: "production/episode-script", OwnerLogicalID: adjacent.EpisodeID,
			OwnerVersionID: adjacent.ScriptVersionID, Revision: int64(adjacent.ScriptVersionNo),
			ContentHash: adjacent.ContentHash,
		})
	}
	return validateExactStageSources(payload.SourceRefs, expectedSources, "Episode analysis")
}

func validateEpisodeReconciliationStageInput(payload StageInvocationPayload) error {
	var input EpisodeReconciliationStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Episode reconciliation stage input")
	}
	if input.EpisodePosition < 1 || input.ScriptVersionNo < 1 || input.BibleVersion < 1 || input.Level < 1 ||
		input.EpisodeSourceStart < 0 || input.EpisodeSourceEnd <= input.EpisodeSourceStart ||
		len(input.Candidates) < 1 || len(input.Candidates) > 2 ||
		len(payload.UpstreamCandidates) != len(input.Candidates) ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "episode_reduce" || payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil ||
		!hashPattern.MatchString(input.ScriptContentHash) || !hashPattern.MatchString(input.BibleContentHash) ||
		!hashPattern.MatchString(input.MaterializationHash) {
		return errors.New("invalid Episode reconciliation stage dependencies")
	}
	for _, identifier := range []string{input.EpisodeID, input.ScriptVersionID, input.BibleVersionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode reconciliation exact revision")
		}
	}
	if err := validateEpisodeKnownIdentities(input.KnownIdentities); err != nil {
		return err
	}
	expectedStage := ""
	switch input.CandidateType {
	case "episode_analysis_candidate":
		expectedStage = "analyze_episode"
	case "episode_reconciliation_candidate":
		expectedStage = "reconcile_episode"
	default:
		return errors.New("invalid Episode reconciliation candidate type")
	}
	expectedChildren := make(map[string]struct{}, len(input.Candidates))
	previousShard := ""
	for _, candidate := range input.Candidates {
		if strings.TrimSpace(candidate.ShardKey) == "" || previousShard >= candidate.ShardKey ||
			!hashPattern.MatchString(candidate.CandidateRevisionHash) || !jsonObject(candidate.Candidate) {
			return errors.New("invalid Episode reconciliation child candidate")
		}
		if _, err := uuid.Parse(candidate.CandidateRevisionID); err != nil {
			return errors.New("invalid Episode reconciliation child revision")
		}
		var identity struct {
			EpisodeID       string `json:"episode_id"`
			ScriptVersionID string `json:"script_version_id"`
			LogicalStart    *int   `json:"logical_start"`
			LogicalEnd      *int   `json:"logical_end"`
			SourceStart     *int   `json:"source_start"`
			SourceEnd       *int   `json:"source_end"`
		}
		if err := json.Unmarshal(candidate.Candidate, &identity); err != nil ||
			identity.EpisodeID != input.EpisodeID || identity.ScriptVersionID != input.ScriptVersionID {
			return errors.New("Episode reconciliation child belongs to another Episode")
		}
		start, end := identity.LogicalStart, identity.LogicalEnd
		if input.CandidateType == "episode_reconciliation_candidate" {
			start, end = identity.SourceStart, identity.SourceEnd
		}
		if start == nil || end == nil || *start < input.EpisodeSourceStart || *end <= *start || *end > input.EpisodeSourceEnd {
			return errors.New("Episode reconciliation child escaped its frozen Episode")
		}
		key := episodeSegmentationLeafKey(candidate.ShardKey, candidate.CandidateRevisionID, candidate.CandidateRevisionHash)
		expectedChildren[key] = struct{}{}
		previousShard = candidate.ShardKey
	}
	for _, upstream := range payload.UpstreamCandidates {
		key := episodeSegmentationLeafKey(upstream.ShardKey, upstream.CandidateRevisionID, upstream.CandidateRevisionHash)
		if upstream.Stage != expectedStage {
			return errors.New("Episode reconciliation child stage has drifted")
		}
		if _, exists := expectedChildren[key]; !exists {
			return errors.New("Episode reconciliation input does not match exact child revisions")
		}
		delete(expectedChildren, key)
	}
	if len(expectedChildren) != 0 {
		return errors.New("Episode reconciliation input is missing exact child revisions")
	}
	expectedSources := []StageSourceRef{
		{OwnerKind: "production/episode-script", OwnerLogicalID: input.EpisodeID, OwnerVersionID: input.ScriptVersionID, Revision: int64(input.ScriptVersionNo), ContentHash: input.ScriptContentHash},
		{OwnerKind: "production/bible-version", OwnerLogicalID: input.BibleVersionID, OwnerVersionID: input.BibleVersionID, Revision: int64(input.BibleVersion), ContentHash: input.BibleContentHash},
		{OwnerKind: "production/bible-materialization", OwnerLogicalID: input.BibleVersionID, OwnerVersionID: input.BibleVersionID, Revision: int64(input.BibleVersion), ContentHash: input.MaterializationHash},
	}
	return validateExactStageSources(payload.SourceRefs, expectedSources, "Episode reconciliation")
}

func validateEpisodeKnownIdentities(values []EpisodeKnownIdentity) error {
	allowedKinds := map[string]struct{}{
		"character": {}, "location": {}, "prop": {}, "costume": {}, "visual_style": {}, "voice": {},
	}
	previousEntity := ""
	for _, identity := range values {
		if strings.TrimSpace(identity.EntityKey) == "" || previousEntity >= identity.EntityKey ||
			!hashPattern.MatchString(identity.SpecificationHash) {
			return errors.New("Episode known identities must be unique and sorted")
		}
		if _, ok := allowedKinds[identity.Kind]; !ok {
			return errors.New("invalid Episode known identity kind")
		}
		for _, identifier := range []string{identity.AssetID, identity.SpecificationVersionID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("invalid Episode known identity revision")
			}
		}
		previousState := ""
		for _, state := range identity.States {
			if strings.TrimSpace(state.StateKey) == "" || previousState >= state.StateKey ||
				!hashPattern.MatchString(state.ContentHash) {
				return errors.New("Episode known states must be unique and sorted")
			}
			if _, err := uuid.Parse(state.AssetStateID); err != nil {
				return errors.New("invalid Episode known state revision")
			}
			previousState = state.StateKey
		}
		previousEntity = identity.EntityKey
	}
	return nil
}

func validateExactStageSources(actual, expected []StageSourceRef, label string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("%s exact sources are incomplete", label)
	}
	remaining := make(map[string]struct{}, len(expected))
	for _, ref := range expected {
		key := stageSourceRefKey(ref)
		if _, exists := remaining[key]; exists {
			return fmt.Errorf("%s expected sources contain a duplicate", label)
		}
		remaining[key] = struct{}{}
	}
	for _, ref := range actual {
		key := stageSourceRefKey(ref)
		if _, exists := remaining[key]; !exists {
			return fmt.Errorf("%s source reference has drifted", label)
		}
		delete(remaining, key)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("%s exact sources are incomplete", label)
	}
	return nil
}

func stageSourceRefKey(value StageSourceRef) string {
	return strings.Join([]string{
		value.OwnerKind, value.OwnerLogicalID, value.OwnerVersionID,
		fmt.Sprint(value.Revision), value.ContentHash,
	}, "\x00")
}

func sourceTextHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func ValidateEpisodeSegmentationInvocation(value StageInvocation) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Payload.Stage != "segment_episodes" {
		return errors.New("invalid Episode segmentation invocation stage")
	}
	return validateEpisodeSegmentationStageInput(value.Payload)
}

func validateEpisodeSegmentationStageInput(payload StageInvocationPayload) error {
	var input EpisodeSegmentationStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Episode segmentation stage input")
	}
	if len(payload.SourceRefs) != 2 || len(input.EvidenceLeaves) == 0 || len(input.EvidenceIndex) == 0 ||
		len(input.EvidenceIndex) > 512 || len(payload.UpstreamCandidates) != len(input.EvidenceLeaves) ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "episode_segmentation" || payload.Shard.AbsoluteStart == nil ||
		payload.Shard.AbsoluteEnd == nil || *payload.Shard.AbsoluteStart != 0 ||
		*payload.Shard.AbsoluteEnd != input.SourceCodePoints || input.SourceCodePoints < 1 ||
		input.TargetDurationMS < 1000 || input.TargetDurationMS > 7_200_000 || input.BibleVersion < 1 ||
		!hashPattern.MatchString(input.NormalizedHash) || !hashPattern.MatchString(input.BibleContentHash) ||
		!hashPattern.MatchString(input.MaterializationHash) || !hashPattern.MatchString(input.EvidenceAggregateRevisionHash) {
		return errors.New("invalid Episode segmentation stage dependencies")
	}
	for _, identifier := range []string{input.DocumentRevisionID, input.BibleVersionID, input.EvidenceAggregateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode segmentation exact revision")
		}
	}
	scriptMatches, materializationMatches := false, false
	for _, ref := range payload.SourceRefs {
		switch {
		case ref.OwnerKind == "production/script" && ref.OwnerVersionID == input.DocumentRevisionID && ref.ContentHash == input.NormalizedHash:
			scriptMatches = true
		case ref.OwnerKind == "production/bible-materialization" && ref.OwnerLogicalID == input.BibleVersionID &&
			ref.OwnerVersionID == input.BibleVersionID && ref.Revision == int64(input.BibleVersion) && ref.ContentHash == input.MaterializationHash:
			materializationMatches = true
		default:
			return errors.New("Episode segmentation source reference has drifted")
		}
	}
	if !scriptMatches || !materializationMatches {
		return errors.New("Episode segmentation exact sources are incomplete")
	}
	leaves := make(map[string]EpisodeSegmentationEvidenceLeaf, len(input.EvidenceLeaves))
	previousShard := ""
	for _, leaf := range input.EvidenceLeaves {
		if strings.TrimSpace(leaf.ShardKey) == "" || previousShard >= leaf.ShardKey ||
			!hashPattern.MatchString(leaf.CandidateRevisionHash) {
			return errors.New("invalid Episode segmentation Evidence leaf")
		}
		if _, err := uuid.Parse(leaf.CandidateRevisionID); err != nil {
			return errors.New("invalid Episode segmentation Evidence leaf revision")
		}
		key := episodeSegmentationLeafKey(leaf.ShardKey, leaf.CandidateRevisionID, leaf.CandidateRevisionHash)
		leaves[key] = leaf
		previousShard = leaf.ShardKey
	}
	upstreams := make(map[string]struct{}, len(payload.UpstreamCandidates))
	for _, upstream := range payload.UpstreamCandidates {
		if upstream.Stage != "extract_source_evidence" {
			return errors.New("Episode segmentation has a non-Evidence upstream candidate")
		}
		key := episodeSegmentationLeafKey(upstream.ShardKey, upstream.CandidateRevisionID, upstream.CandidateRevisionHash)
		if _, exists := leaves[key]; !exists {
			return errors.New("Episode segmentation upstream revision has drifted")
		}
		upstreams[key] = struct{}{}
	}
	if len(upstreams) != len(leaves) {
		return errors.New("Episode segmentation exact Evidence leaves are incomplete")
	}
	indexKeys := make(map[string]struct{}, len(input.EvidenceIndex))
	markerEvidence := make(map[string]struct{}, len(input.MarkerHints))
	for _, item := range input.EvidenceIndex {
		leafKey := episodeSegmentationLeafKey(item.ShardKey, item.CandidateRevisionID, item.CandidateRevisionHash)
		if strings.TrimSpace(item.IndexKey) == "" || strings.TrimSpace(item.Label) == "" ||
			(item.Kind != "marker" && item.Kind != "event" && item.Kind != "evidence") || !validEpisodeSegmentationEvidence(item.Evidence, input.SourceCodePoints) {
			return errors.New("invalid Episode segmentation Evidence index item")
		}
		if _, exists := leaves[leafKey]; !exists {
			return errors.New("Episode segmentation Evidence index has no exact leaf")
		}
		if _, exists := indexKeys[item.IndexKey]; exists {
			return errors.New("Episode segmentation Evidence index keys must be unique")
		}
		indexKeys[item.IndexKey] = struct{}{}
		if item.Kind == "marker" {
			markerEvidence[episodeSegmentationEvidenceKey(item.Evidence)] = struct{}{}
		}
	}
	markerStarts := make(map[int]struct{}, len(input.MarkerHints))
	for _, marker := range input.MarkerHints {
		if marker.EpisodeNumber < 1 || strings.TrimSpace(marker.Label) == "" ||
			!validEpisodeSegmentationEvidence(marker.Evidence, input.SourceCodePoints) ||
			marker.Evidence.EpisodeNumber == nil || *marker.Evidence.EpisodeNumber != marker.EpisodeNumber {
			return errors.New("invalid Episode segmentation marker hint")
		}
		if _, exists := markerStarts[marker.Evidence.SourceStart]; exists {
			return errors.New("Episode segmentation marker starts must be unique")
		}
		if _, exists := markerEvidence[episodeSegmentationEvidenceKey(marker.Evidence)]; !exists {
			return errors.New("Episode segmentation marker is absent from its bounded Evidence index")
		}
		markerStarts[marker.Evidence.SourceStart] = struct{}{}
	}
	return nil
}

func episodeSegmentationLeafKey(shardKey, revisionID, revisionHash string) string {
	return strings.Join([]string{shardKey, revisionID, revisionHash}, "\x00")
}

func episodeSegmentationEvidenceKey(value EpisodeSegmentationEvidence) string {
	episode := ""
	if value.EpisodeNumber != nil {
		episode = fmt.Sprint(*value.EpisodeNumber)
	}
	return fmt.Sprintf("%d:%d:%s:%s:%s", value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor, episode)
}

func validEpisodeSegmentationEvidence(value EpisodeSegmentationEvidence, sourceCodePoints int) bool {
	return value.SourceStart >= 0 && value.SourceEnd > value.SourceStart && value.SourceEnd <= sourceCodePoints &&
		hashPattern.MatchString(value.TextHash) && strings.TrimSpace(value.ExactAnchor) != "" &&
		(value.EpisodeNumber == nil || *value.EpisodeNumber >= 1)
}

func validateStoryGraphReviewStageInput(payload StageInvocationPayload) error {
	var input StoryGraphReviewStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil || input.Validate() != nil {
		return errors.New("invalid StoryGraph review stage input")
	}
	if len(payload.SourceRefs) != 1 || len(payload.UpstreamCandidates) != 1 ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "story_review" || payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil {
		return errors.New("invalid StoryGraph review stage dependencies")
	}
	upstream := payload.UpstreamCandidates[0]
	if upstream.Stage != input.ReviewedStage || upstream.CandidateRevisionID != input.TargetCandidateRevisionID ||
		upstream.CandidateRevisionHash != input.TargetCandidateRevisionHash {
		return errors.New("StoryGraph review input does not match its exact candidate revision")
	}
	return nil
}

func validateStoryGraphRepairStageInput(payload StageInvocationPayload) error {
	var input StoryGraphRepairStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil || input.Validate() != nil {
		return errors.New("invalid StoryGraph repair stage input")
	}
	if len(payload.SourceRefs) != 1 || len(payload.UpstreamCandidates) != 2 ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "candidate_repair" || payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil {
		return errors.New("invalid StoryGraph repair stage dependencies")
	}
	matchedTarget, matchedReview := false, false
	for _, upstream := range payload.UpstreamCandidates {
		switch {
		case upstream.Stage == "reconcile_story" && upstream.CandidateRevisionID == input.TargetCandidateRevisionID &&
			upstream.CandidateRevisionHash == input.TargetCandidateRevisionHash:
			matchedTarget = true
		case upstream.Stage == "review_storygraph" && upstream.CandidateRevisionID == input.ReviewCandidateRevisionID &&
			upstream.CandidateRevisionHash == input.ReviewCandidateRevisionHash:
			matchedReview = true
		default:
			return errors.New("StoryGraph repair input does not match its exact candidate revisions")
		}
	}
	if !matchedTarget || !matchedReview {
		return errors.New("StoryGraph repair input is missing an exact candidate revision")
	}
	return nil
}

// ValidateStoryAnalysisInvocation applies the Production Bible Story Analysis
// owner contract after the shared StoryGraph wire contract has been decoded.
// The stage names are part of the shared harness registry, while the exact
// upstream shape belongs to the backend owner that schedules these shards.
func ValidateStoryAnalysisInvocation(value StageInvocation) error {
	if err := value.Validate(); err != nil {
		return err
	}
	switch value.Payload.Stage {
	case "analyze_story":
		return validateStoryAnalysisStageInput(value.Payload)
	case "reconcile_story":
		return validateStoryReconciliationStageInput(value.Payload)
	default:
		return errors.New("invalid Story analysis invocation stage")
	}
}

func validateSourceEvidenceStageInput(payload StageInvocationPayload) error {
	var input SourceEvidenceStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Source Evidence stage input")
	}
	if len(payload.SourceRefs) != 1 || len(payload.UpstreamCandidates) != 0 ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "source_slice" || payload.Shard.AbsoluteStart == nil ||
		payload.Shard.AbsoluteEnd == nil {
		return errors.New("invalid Source Evidence stage dependencies")
	}
	ref := payload.SourceRefs[0]
	if ref.OwnerKind != "production/script" || ref.OwnerVersionID != input.DocumentRevisionID ||
		ref.ContentHash != input.NormalizedHash || !hashPattern.MatchString(input.NormalizedHash) ||
		!hashPattern.MatchString(input.LogicalSourceHash) {
		return errors.New("Source Evidence stage input does not match its source revision")
	}
	if _, err := uuid.Parse(input.DocumentRevisionID); err != nil {
		return errors.New("invalid Source Evidence document revision")
	}
	if input.LogicalStart < 0 || input.LogicalEnd <= input.LogicalStart ||
		input.ContextStart < 0 || input.ContextStart > input.LogicalStart ||
		input.ContextEnd < input.LogicalEnd || input.ContextEnd-input.ContextStart != utf8.RuneCountInString(input.NormalizedText) ||
		*payload.Shard.AbsoluteStart != input.LogicalStart || *payload.Shard.AbsoluteEnd != input.LogicalEnd {
		return errors.New("invalid Source Evidence stage range")
	}
	contextRunes := []rune(input.NormalizedText)
	logicalStart := input.LogicalStart - input.ContextStart
	logicalEnd := input.LogicalEnd - input.ContextStart
	logicalHash := sha256.Sum256([]byte(string(contextRunes[logicalStart:logicalEnd])))
	if hex.EncodeToString(logicalHash[:]) != input.LogicalSourceHash {
		return errors.New("Source Evidence logical source hash mismatch")
	}
	for _, marker := range input.EpisodeMarkerHints {
		if marker.EpisodeNumber < 1 || strings.TrimSpace(marker.Label) == "" ||
			marker.AbsoluteStart < input.ContextStart || marker.AbsoluteEnd <= marker.AbsoluteStart ||
			marker.AbsoluteEnd > input.ContextEnd {
			return errors.New("invalid Source Evidence episode marker hint")
		}
	}
	return nil
}

func validateStoryAnalysisStageInput(payload StageInvocationPayload) error {
	var input StoryAnalysisStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Story analysis stage input")
	}
	if len(payload.SourceRefs) != 1 || len(payload.UpstreamCandidates) != 1 ||
		payload.BaseStoryGraphVersionID != "" || payload.BaseStoryGraphHash != "" ||
		payload.Shard.Kind != "story_map" || payload.Shard.AbsoluteStart == nil ||
		payload.Shard.AbsoluteEnd == nil || input.LogicalStart < 0 || input.LogicalEnd <= input.LogicalStart ||
		input.CandidateItemStart < 0 || input.CandidateItemEnd < input.CandidateItemStart ||
		*payload.Shard.AbsoluteStart != input.LogicalStart || *payload.Shard.AbsoluteEnd != input.LogicalEnd ||
		strings.TrimSpace(input.EvidenceShardKey) == "" || !jsonObject(input.EvidenceCandidate) ||
		!hashPattern.MatchString(input.EvidenceCandidateRevisionHash) {
		return errors.New("invalid Story analysis stage dependencies")
	}
	if _, err := uuid.Parse(input.EvidenceCandidateRevisionID); err != nil {
		return errors.New("invalid Story analysis Evidence revision")
	}
	upstream := payload.UpstreamCandidates[0]
	if upstream.Stage != "extract_source_evidence" || upstream.ShardKey != input.EvidenceShardKey ||
		upstream.CandidateRevisionID != input.EvidenceCandidateRevisionID ||
		upstream.CandidateRevisionHash != input.EvidenceCandidateRevisionHash {
		return errors.New("Story analysis input does not match its exact Evidence revision")
	}
	return nil
}

func validateStoryReconciliationStageInput(payload StageInvocationPayload) error {
	var input StoryReconciliationStageInput
	if err := decodeStrict(payload.StageInput, &input); err != nil {
		return errors.New("invalid Story reconciliation stage input")
	}
	if len(payload.SourceRefs) != 1 || input.Level < 0 || len(input.Candidates) < 1 || len(input.Candidates) > 2 ||
		len(payload.UpstreamCandidates) != len(input.Candidates) || payload.BaseStoryGraphVersionID != "" ||
		payload.BaseStoryGraphHash != "" || payload.Shard.Kind != "story_reduce" ||
		payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil {
		return errors.New("invalid Story reconciliation stage dependencies")
	}
	expectedStage := ""
	switch input.CandidateType {
	case "story_analysis_candidate":
		expectedStage = "analyze_story"
	case "story_reconciliation_candidate":
		expectedStage = "reconcile_story"
	default:
		return errors.New("invalid Story reconciliation candidate type")
	}
	expected := make(map[string]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if strings.TrimSpace(candidate.ShardKey) == "" || !jsonObject(candidate.Candidate) ||
			!hashPattern.MatchString(candidate.CandidateRevisionHash) {
			return errors.New("invalid Story reconciliation child candidate")
		}
		if (candidate.CandidateItemStart == nil) != (candidate.CandidateItemEnd == nil) ||
			candidate.CandidateItemStart != nil && (*candidate.CandidateItemStart < 0 ||
				*candidate.CandidateItemEnd <= *candidate.CandidateItemStart) {
			return errors.New("invalid Story reconciliation child candidate range")
		}
		if _, err := uuid.Parse(candidate.CandidateRevisionID); err != nil {
			return errors.New("invalid Story reconciliation child revision")
		}
		key := strings.Join([]string{candidate.ShardKey, candidate.CandidateRevisionID, candidate.CandidateRevisionHash}, "\x00")
		if _, exists := expected[key]; exists {
			return errors.New("duplicate Story reconciliation child revision")
		}
		expected[key] = struct{}{}
	}
	for _, upstream := range payload.UpstreamCandidates {
		key := strings.Join([]string{upstream.ShardKey, upstream.CandidateRevisionID, upstream.CandidateRevisionHash}, "\x00")
		if upstream.Stage != expectedStage {
			return errors.New("Story reconciliation child stage has drifted")
		}
		if _, exists := expected[key]; !exists {
			return errors.New("Story reconciliation input does not match exact child revisions")
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		return errors.New("Story reconciliation input is missing exact child revisions")
	}
	return nil
}

func validateStageRefs(payload StageInvocationPayload) error {
	if _, err := uuid.Parse(payload.ShardManifestRef.ManifestID); err != nil || payload.ShardManifestRef.Version < 1 || !hashPattern.MatchString(payload.ShardManifestRef.Hash) {
		return errors.New("invalid shard manifest reference")
	}
	for _, ref := range payload.SourceRefs {
		if strings.TrimSpace(ref.OwnerKind) == "" || strings.TrimSpace(ref.OwnerLogicalID) == "" || ref.Revision < 1 || !hashPattern.MatchString(ref.ContentHash) {
			return errors.New("invalid source reference")
		}
		if _, err := uuid.Parse(ref.OwnerVersionID); err != nil {
			return errors.New("invalid source reference")
		}
	}
	for _, ref := range payload.UpstreamCandidates {
		if _, ok := storyGraphStages[ref.Stage]; !ok || strings.TrimSpace(ref.ShardKey) == "" || !hashPattern.MatchString(ref.CandidateRevisionHash) || !hashPattern.MatchString(ref.SourceResultHash) {
			return errors.New("invalid upstream candidate reference")
		}
		for _, identifier := range []string{ref.CandidateRevisionID, ref.SourceInvocationID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("invalid upstream candidate reference")
			}
		}
	}
	if payload.BaseStoryGraphVersionID == "" != (payload.BaseStoryGraphHash == "") {
		return errors.New("incomplete base StoryGraph reference")
	}
	if payload.BaseStoryGraphVersionID != "" {
		if _, err := uuid.Parse(payload.BaseStoryGraphVersionID); err != nil || !hashPattern.MatchString(payload.BaseStoryGraphHash) {
			return errors.New("invalid base StoryGraph reference")
		}
	}
	if payload.Shard.AbsoluteStart != nil || payload.Shard.AbsoluteEnd != nil {
		if payload.Shard.AbsoluteStart == nil || payload.Shard.AbsoluteEnd == nil || *payload.Shard.AbsoluteStart < 0 || *payload.Shard.AbsoluteEnd <= *payload.Shard.AbsoluteStart {
			return errors.New("invalid shard absolute range")
		}
	}
	return nil
}

func (value StageInvocation) ComputeInputHash() (string, error) {
	encoded, err := value.CanonicalInput()
	if err != nil {
		return "", err
	}
	return CanonicalHash(encoded)
}

func (value StageInvocation) CanonicalInput() ([]byte, error) {
	payload := value.Payload
	payload.SourceRefs = append(make([]StageSourceRef, 0, len(payload.SourceRefs)), payload.SourceRefs...)
	sort.Slice(payload.SourceRefs, func(i, j int) bool {
		left, right := payload.SourceRefs[i], payload.SourceRefs[j]
		return strings.Join([]string{left.OwnerKind, left.OwnerLogicalID, left.OwnerVersionID, fmt.Sprint(left.Revision), left.ContentHash}, "\x00") < strings.Join([]string{right.OwnerKind, right.OwnerLogicalID, right.OwnerVersionID, fmt.Sprint(right.Revision), right.ContentHash}, "\x00")
	})
	payload.UpstreamCandidates = append(make([]StageUpstreamCandidateRef, 0, len(payload.UpstreamCandidates)), payload.UpstreamCandidates...)
	sort.Slice(payload.UpstreamCandidates, func(i, j int) bool {
		left, right := payload.UpstreamCandidates[i], payload.UpstreamCandidates[j]
		return strings.Join([]string{left.Stage, left.ShardKey, left.CandidateRevisionID, left.CandidateRevisionHash}, "\x00") < strings.Join([]string{right.Stage, right.ShardKey, right.CandidateRevisionID, right.CandidateRevisionHash}, "\x00")
	})
	material := struct {
		WireSchemaVersion string                 `json:"wire_schema_version"`
		ExecutionPolicy   StageExecutionPolicy   `json:"execution_policy"`
		Payload           StageInvocationPayload `json:"payload"`
	}{value.WireSchemaVersion, value.ExecutionPolicy, payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return nil, err
	}
	var canonicalValue any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err = decoder.Decode(&canonicalValue); err != nil {
		return nil, err
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(canonicalValue); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(canonical.Bytes()), nil
}

func (value StageInvocation) StageInstanceKey() (string, error) {
	if !hashPattern.MatchString(value.InputHash) || !hashPattern.MatchString(value.Payload.ShardManifestRef.Hash) {
		return "", errors.New("invalid stage identity hash")
	}
	material := "storygraph-stage-v1" + value.Payload.Stage + value.Payload.ShardKey + value.Payload.ShardManifestRef.Hash + value.InputHash
	hash := sha256.Sum256([]byte(material))
	return hex.EncodeToString(hash[:]), nil
}

type StageIssue struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

type StageResult struct {
	InvocationID      string          `json:"invocation_id"`
	Kind              string          `json:"kind"`
	WireSchemaVersion string          `json:"wire_schema_version"`
	Stage             string          `json:"stage"`
	ShardKey          string          `json:"shard_key"`
	Status            string          `json:"status"`
	CandidateType     string          `json:"candidate_type"`
	Candidate         json.RawMessage `json:"candidate"`
	InputHash         string          `json:"input_hash"`
	ResultHash        *string         `json:"result_hash"`
	Issues            []StageIssue    `json:"issues"`
	Executor          Executor        `json:"executor"`
	Error             *ResultError    `json:"error"`
}

func DecodeStageResult(raw []byte) (StageResult, error) {
	var value StageResult
	if err := decodeStrict(raw, &value); err != nil {
		return StageResult{}, err
	}
	if value.Status == "succeeded" {
		computed, err := value.ComputeResultHash()
		if err != nil {
			return StageResult{}, err
		}
		if value.ResultHash == nil || *value.ResultHash != computed {
			got := "null"
			if value.ResultHash != nil {
				got = *value.ResultHash
			}
			return StageResult{}, fmt.Errorf("StoryGraph result hash mismatch: got %s want %s", got, computed)
		}
	}
	return value, nil
}

func (value StageResult) ComputeResultHash() (string, error) {
	if !jsonObject(value.Candidate) {
		return "", errors.New("successful StoryGraph result candidate must be an object")
	}
	return CanonicalHash(value.Candidate)
}

func (value StageResult) ValidateFor(invocation StageInvocation) error {
	expectedCandidateType, ok := CandidateTypeForStage(invocation.Payload.Stage)
	if value.InvocationID != invocation.InvocationID || value.Kind != "storygraph_stage" || value.WireSchemaVersion != StoryGraphWireSchemaVersion || value.Stage != invocation.Payload.Stage || value.ShardKey != invocation.Payload.ShardKey || value.InputHash != invocation.InputHash || !ok || value.CandidateType != expectedCandidateType || value.Issues == nil || value.Executor.Name == "" || value.Executor.Version == "" || value.Executor.Model == "" {
		return errors.New("StoryGraph result identity does not match invocation")
	}
	if value.Status == "succeeded" {
		if value.Error != nil || value.ResultHash == nil || !hashPattern.MatchString(*value.ResultHash) {
			return errors.New("successful StoryGraph result is incomplete")
		}
		computed, err := value.ComputeResultHash()
		if err != nil || computed != *value.ResultHash {
			return errors.New("StoryGraph result hash mismatch")
		}
		return nil
	}
	if value.Status != "failed" && value.Status != "unknown" {
		return errors.New("invalid StoryGraph result status")
	}
	if !bytes.Equal(bytes.TrimSpace(value.Candidate), []byte("null")) || value.ResultHash != nil || value.Error == nil || value.Error.Code == "" || value.Error.Summary == "" || !validStageErrorSemantics(value.Status, *value.Error) {
		return errors.New("failed or unknown StoryGraph result is incomplete")
	}
	return nil
}

func validStageErrorSemantics(status string, resultError ResultError) bool {
	switch resultError.Code {
	case "skill_bundle_invalid", "invocation_policy_invalid", "candidate_schema_invalid", "evidence_invalid", "upstream_candidate_stale", "execution_budget_exceeded", "execution_deadline_exceeded", "tool_not_allowed":
		return status == "failed" && !resultError.Retryable
	case "skill_bundle_unavailable", "runtime_unavailable", "agent_execution_unknown":
		return status == "unknown" && resultError.Retryable
	default:
		return false
	}
}

type StageExecutionGrantClaims struct {
	InvocationID        string `json:"invocation_id"`
	InputHash           string `json:"input_hash"`
	ExecutionPolicyHash string `json:"execution_policy_hash"`
	ExpiresAt           int64  `json:"expires_at"`
	Attempt             int    `json:"attempt"`
	FencingToken        int64  `json:"fencing_token"`
}

func (value StageExecutionGrantClaims) ValidateFor(invocation StageInvocation, nowUnix int64) error {
	policyHash, err := invocation.ExecutionPolicy.Hash()
	if err != nil {
		return err
	}
	if value.InvocationID != invocation.InvocationID || value.InputHash != invocation.InputHash || value.ExecutionPolicyHash != policyHash || value.ExpiresAt <= nowUnix || value.Attempt < 1 || value.FencingToken < 1 {
		return errors.New("invalid StoryGraph execution grant claims")
	}
	return nil
}

func decodeStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}
