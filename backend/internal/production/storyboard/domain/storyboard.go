package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/google/uuid"
)

type Unit struct {
	ID, SceneID, Kind, Text string
	DialogueID              *string
	Position                int
	Required                bool
}

type DraftInput struct {
	WorkspaceID, ProjectID, EpisodeID, StructureID, ScriptVersionID string
	StructureResultHash                                             string
	StructureRevision                                               int
	BibleID, BibleResultHash                                        string
	BibleRevision                                                   int
	TargetDurationMS                                                int
	AspectRatio                                                     string
	VisualStyle                                                     *string
	Units                                                           []Unit
	WorldEntries                                                    []map[string]any
}

type StoryGraphDraftInput struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	GraphVersionID, GraphContentHash                 string
	GraphVersionNo                                   int64
	EffectiveStyleSnapshot                           agentcontract.StoryboardStyleSnapshotInput
	Scenes                                           []SceneDraftInput
}

type SceneDraftInput struct {
	EpisodeID, StructureID, ScriptVersionID string
	StageInput                              agentcontract.StoryboardDraftStageInput
}

type DraftManifestShard struct {
	Kind              string `json:"kind"`
	Key               string `json:"key"`
	TreePath          string `json:"tree_path"`
	SceneStoryNodeKey string `json:"scene_story_node_key"`
	EpisodeID         string `json:"episode_id"`
	EpisodePosition   int    `json:"episode_position"`
	ScenePosition     int    `json:"scene_position"`
	InputHash         string `json:"input_hash"`
}

type DraftManifest struct {
	ManifestID, WorkspaceID, WorkflowRunID, NodeRunID string
	Version                                           int64
	Stage, RootInputHash                              string
	Shards                                            []DraftManifestShard
	CoverageHash, ManifestHash                        string
}

type DraftShot struct {
	ProposalKey             string           `json:"proposal_key"`
	Position                int              `json:"position"`
	Title                   string           `json:"title"`
	NarrativeUnitVersionIDs []string         `json:"narrative_unit_version_ids"`
	Spec                    map[string]any   `json:"spec"`
	AssetReferences         []map[string]any `json:"asset_references"`
	RiskCodes               []string         `json:"risk_codes"`
}

type Candidate struct {
	SceneStoryNodeKey string       `json:"scene_story_node_key"`
	ShotIntents       []ShotIntent `json:"shot_intents"`
	AssetReadiness    string       `json:"asset_readiness"`
	Shots             []DraftShot  `json:"-"`
}

type EvidenceRef struct {
	DocumentRevisionID string `json:"document_revision_id"`
	AbsoluteStart      int    `json:"absolute_start"`
	AbsoluteEnd        int    `json:"absolute_end"`
	TextHash           string `json:"text_hash"`
}

type CameraIntent struct {
	Scale, Angle, Movement, Composition string
}

func (value CameraIntent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scale       string `json:"scale"`
		Angle       string `json:"angle"`
		Movement    string `json:"movement"`
		Composition string `json:"composition"`
	}{value.Scale, value.Angle, value.Movement, value.Composition})
}

func (value *CameraIntent) UnmarshalJSON(raw []byte) error {
	var decoded struct {
		Scale       string `json:"scale"`
		Angle       string `json:"angle"`
		Movement    string `json:"movement"`
		Composition string `json:"composition"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*value = CameraIntent{decoded.Scale, decoded.Angle, decoded.Movement, decoded.Composition}
	return nil
}

type FrameIntent struct {
	First string `json:"first"`
	Key   string `json:"key"`
	Last  string `json:"last"`
}

type AssetVersionRef struct {
	AssetVersionID string `json:"asset_version_id"`
	Revision       int64  `json:"revision"`
	ContentHash    string `json:"content_hash"`
	LineageHash    string `json:"lineage_hash"`
}

type VisualRequirement struct {
	OccurrenceStoryNodeKey    string           `json:"occurrence_story_node_key"`
	IdentityStoryNodeKey      string           `json:"identity_story_node_key"`
	SpecificationStoryNodeKey string           `json:"specification_story_node_key"`
	AssetStateStoryNodeKey    string           `json:"asset_state_story_node_key"`
	AssetID                   string           `json:"asset_id"`
	SpecificationVersionID    string           `json:"specification_version_id"`
	AssetStateID              string           `json:"asset_state_id"`
	AssetRole                 string           `json:"asset_role"`
	RequiredViewRoles         []string         `json:"required_view_roles"`
	AssetReadiness            string           `json:"asset_readiness"`
	AssetVersionRef           *AssetVersionRef `json:"asset_version_ref"`
}

type ReviewIssue struct {
	Code     string        `json:"code"`
	Severity string        `json:"severity"`
	Summary  string        `json:"summary"`
	Evidence []EvidenceRef `json:"evidence"`
}

type ShotIntent struct {
	ShotKey                 string              `json:"shot_key"`
	IntentOrder             int                 `json:"intent_order"`
	SourceBeatStoryNodeKeys []string            `json:"source_beat_story_node_keys"`
	SourceEvidence          []EvidenceRef       `json:"source_evidence"`
	Purpose                 string              `json:"purpose"`
	ProposedDurationMS      int                 `json:"proposed_duration_ms"`
	Camera                  CameraIntent        `json:"camera"`
	ActionIntent            string              `json:"action_intent"`
	DialogueIntent          *string             `json:"dialogue_intent"`
	SoundIntent             string              `json:"sound_intent"`
	PerformanceIntent       string              `json:"performance_intent"`
	ContinuityIn            string              `json:"continuity_in"`
	ContinuityOut           string              `json:"continuity_out"`
	FrameIntent             FrameIntent         `json:"frame_intent"`
	VisualRequirements      []VisualRequirement `json:"visual_requirements"`
	RiskCodes               []string            `json:"risk_codes"`
	ReviewIssues            []ReviewIssue       `json:"review_issues"`
}

type Batch struct {
	ID, WorkspaceID, ProjectID, EpisodeID, StructureID, ScriptVersionID, TaskID string
	WorkflowRunID, NodeRunID, ManifestID, GraphVersionID, SceneStoryNodeKey     string
	ManifestVersion, GraphVersionNo                                             int64
	Status, InputHash                                                           string
	ResultHash                                                                  *string
	CandidateRevisionID, CandidateRevisionHash                                  *string
	Candidate                                                                   Candidate
	Decisions                                                                   map[string]string
	Error                                                                       json.RawMessage
	Revision                                                                    int
	ApprovedBy                                                                  *string
	ApprovedAt, AppliedAt                                                       *time.Time
	CreatedBy                                                                   string
	CreatedAt, UpdatedAt                                                        time.Time
}

type DraftSetBatch struct {
	BatchID               string  `json:"batch_id"`
	EpisodeID             string  `json:"episode_id"`
	StructureID           string  `json:"structure_id"`
	ScriptVersionID       string  `json:"script_version_id"`
	SceneStoryNodeKey     string  `json:"scene_story_node_key"`
	InputHash             string  `json:"input_hash"`
	BaselineOrderHash     string  `json:"baseline_order_hash"`
	ResultHash            *string `json:"result_hash"`
	CandidateRevisionID   *string `json:"candidate_revision_id"`
	CandidateRevisionHash *string `json:"candidate_revision_hash"`
}

type DraftSet struct {
	ID, WorkspaceID, ProjectID, StructureCommitID, StructureContentHash string
	WorkflowRunID, NodeRunID, GraphVersionID, GraphContentHash          string
	ManifestID, ManifestHash                                            string
	GraphVersionNo, ManifestVersion                                     int64
	StructureRevision                                                   int
	Status, InputHash                                                   string
	ResultHash                                                          *string
	CandidateRevisionID, CandidateRevisionHash                          *string
	Batches                                                             []DraftSetBatch
	Revision                                                            int
	CreatedBy                                                           string
	CreatedAt, UpdatedAt                                                time.Time
}

type Invocation struct {
	ID, WorkspaceID, RequestID, Kind, Stage, ShardKey string
	WorkflowRunID, NodeRunID, ManifestID              string
	ManifestVersion                                   int64
	InputHash, StageInstanceKey, ManifestHash, Status string
	ExecutionPolicy, Payload                          json.RawMessage
	Attempts, ClaimVersion                            int
	LeaseExpiresAt                                    *time.Time
	CreatedAt                                         time.Time
}

type CandidateSetItem struct {
	SceneStoryNodeKey     string `json:"scene_story_node_key"`
	ShardKey              string `json:"shard_key"`
	StageInstanceKey      string `json:"stage_instance_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	AssetReadiness        string `json:"asset_readiness"`
}

type CandidateSet struct {
	SchemaVersion    string             `json:"schema_version"`
	DraftSetID       string             `json:"draft_set_id"`
	DraftSetRevision int                `json:"draft_set_revision"`
	GraphVersionID   string             `json:"graph_version_id"`
	GraphContentHash string             `json:"graph_content_hash"`
	ManifestID       string             `json:"manifest_id"`
	ManifestVersion  int64              `json:"manifest_version"`
	ManifestHash     string             `json:"manifest_hash"`
	Scenes           []CandidateSetItem `json:"scenes"`
	AssetReadiness   string             `json:"asset_readiness"`
}

type ApprovedIntentScene struct {
	SceneStoryNodeKey     string       `json:"scene_story_node_key"`
	BatchID               string       `json:"batch_id"`
	EpisodeID             string       `json:"episode_id"`
	StructureID           string       `json:"structure_id"`
	ScriptVersionID       string       `json:"script_version_id"`
	CandidateRevisionID   string       `json:"candidate_revision_id"`
	CandidateRevisionHash string       `json:"candidate_revision_hash"`
	AssetReadiness        string       `json:"asset_readiness"`
	ShotIntents           []ShotIntent `json:"shot_intents"`
}

type ApprovedIntentSet struct {
	SchemaVersion          string                `json:"schema_version"`
	ID                     string                `json:"id"`
	WorkspaceID            string                `json:"workspace_id"`
	ProjectID              string                `json:"project_id"`
	DraftSetID             string                `json:"draft_set_id"`
	DraftSetRevision       int                   `json:"draft_set_revision"`
	CandidateRevisionID    string                `json:"candidate_revision_id"`
	CandidateRevisionHash  string                `json:"candidate_revision_hash"`
	CandidateRevision      int64                 `json:"candidate_revision"`
	GraphVersionID         string                `json:"graph_version_id"`
	GraphVersionNo         int64                 `json:"graph_version_no"`
	GraphContentHash       string                `json:"graph_content_hash"`
	ManifestID             string                `json:"manifest_id"`
	ManifestVersion        int64                 `json:"manifest_version"`
	ManifestHash           string                `json:"manifest_hash"`
	ReviewDecisionID       string                `json:"review_decision_id"`
	Scenes                 []ApprovedIntentScene `json:"scenes"`
	VisualRequirementsHash string                `json:"visual_requirements_hash"`
	ContentHash            string                `json:"content_hash"`
}

type Shot struct {
	ID, WorkspaceID, ProjectID, EpisodeID, BatchID, ProposalKey, Title string
	Position, Revision                                                 int
	NarrativeUnitIDs                                                   []string
	Spec                                                               map[string]any
	ContentHash, Status, CreatedBy                                     string
	CreatedAt, UpdatedAt                                               time.Time
}

type ExportFile struct {
	Name, MediaType, SHA256 string
	Size                    int
}
type ExportSetReference struct {
	ExportID    string `json:"export_id"`
	EpisodeID   string `json:"episode_id"`
	OrderHash   string `json:"order_hash"`
	ContentHash string `json:"content_hash"`
}
type ExportSet struct {
	ID, WorkspaceID, ProjectID, DraftSetID, Status, InputHash, ContentHash string
	DraftSetRevision                                                       int
	Exports                                                                []ExportSetReference
	Revision                                                               int
	CreatedBy                                                              string
	CreatedAt, UpdatedAt                                                   time.Time
}
type Export struct {
	ID, WorkspaceID, ProjectID, EpisodeID, Status, InputHash, ContentHash string
	ExportSetID                                                           *string
	Manifest                                                              map[string]any
	Files                                                                 []ExportFile
	Package                                                               []byte
	Revision                                                              int
	CreatedBy                                                             string
	CreatedAt, UpdatedAt                                                  time.Time
}

func DecodeAndValidateCandidate(raw json.RawMessage, payload json.RawMessage) (Candidate, error) {
	var candidate Candidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return Candidate{}, errors.New("candidate does not match storyboard schema")
	}
	var input agentcontract.StoryboardDraftStageInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return Candidate{}, errors.New("storyboard invocation payload is invalid")
	}
	if candidate.SceneStoryNodeKey != input.Scene.StoryNodeKey || len(candidate.ShotIntents) < 1 || len(candidate.ShotIntents) > 120 {
		return Candidate{}, errors.New("storyboard candidate must contain 1-120 intents for its exact Scene")
	}
	beats := make(map[string]agentcontract.StoryboardBeatInput, len(input.Beats))
	requiredBeats := make(map[string]bool, len(input.Beats))
	occurrences := make(map[string]agentcontract.StoryboardOccurrenceInput, len(input.Occurrences))
	versions := make(map[string]agentcontract.StoryboardAssetVersionInput, len(input.AssetVersions))
	allowedEvidence := make(map[string]struct{})
	addEvidence := func(values []agentcontract.StoryboardEvidenceRef) {
		for _, value := range values {
			allowedEvidence[evidenceKey(value.DocumentRevisionID, value.AbsoluteStart, value.AbsoluteEnd, value.TextHash)] = struct{}{}
		}
	}
	addEvidence(input.Scene.Evidence)
	for _, beat := range input.Beats {
		beats[beat.StoryNodeKey] = beat
		if beat.RequiredForCoverage {
			requiredBeats[beat.StoryNodeKey] = false
		}
		addEvidence(beat.Evidence)
	}
	for _, dialogue := range input.Dialogues {
		addEvidence(dialogue.Evidence)
	}
	for _, occurrence := range input.Occurrences {
		occurrences[occurrence.StoryNodeKey] = occurrence
		addEvidence(occurrence.Evidence)
	}
	for _, version := range input.AssetVersions {
		versions[version.AssetVersionID] = version
	}
	keys := map[string]struct{}{}
	coveredOccurrences := make(map[string]struct{}, len(occurrences))
	hasMissingAsset := false
	for index, intent := range candidate.ShotIntents {
		if intent.IntentOrder != index+1 || strings.TrimSpace(intent.ShotKey) == "" ||
			len(intent.SourceBeatStoryNodeKeys) == 0 || len(intent.SourceEvidence) == 0 ||
			strings.TrimSpace(intent.Purpose) == "" || intent.ProposedDurationMS < 500 || intent.ProposedDurationMS > 15000 ||
			strings.TrimSpace(intent.Camera.Scale) == "" || strings.TrimSpace(intent.Camera.Angle) == "" ||
			strings.TrimSpace(intent.Camera.Movement) == "" || strings.TrimSpace(intent.Camera.Composition) == "" ||
			strings.TrimSpace(intent.ActionIntent) == "" || strings.TrimSpace(intent.SoundIntent) == "" ||
			strings.TrimSpace(intent.PerformanceIntent) == "" || strings.TrimSpace(intent.ContinuityIn) == "" ||
			strings.TrimSpace(intent.ContinuityOut) == "" || strings.TrimSpace(intent.FrameIntent.First) == "" ||
			strings.TrimSpace(intent.FrameIntent.Key) == "" || strings.TrimSpace(intent.FrameIntent.Last) == "" ||
			len(intent.VisualRequirements) == 0 {
			return Candidate{}, errors.New("storyboard candidate contains an invalid Shot Intent")
		}
		if _, exists := keys[intent.ShotKey]; exists {
			return Candidate{}, errors.New("storyboard Shot Intent keys must be unique")
		}
		keys[intent.ShotKey] = struct{}{}
		seen := map[string]struct{}{}
		previousBeat := ""
		for _, key := range intent.SourceBeatStoryNodeKeys {
			if _, exists := beats[key]; !exists || previousBeat >= key {
				return Candidate{}, errors.New("storyboard intent references an unknown or duplicate Beat")
			}
			if _, exists := seen[key]; exists {
				return Candidate{}, errors.New("storyboard intent Beat references must be unique")
			}
			seen[key] = struct{}{}
			if _, exists := requiredBeats[key]; exists {
				requiredBeats[key] = true
			}
			previousBeat = key
		}
		for _, evidence := range intent.SourceEvidence {
			if _, exists := allowedEvidence[evidenceKey(evidence.DocumentRevisionID, evidence.AbsoluteStart, evidence.AbsoluteEnd, evidence.TextHash)]; !exists {
				return Candidate{}, errors.New("storyboard intent Evidence is outside its exact Scene")
			}
		}
		seenRequirements := make(map[string]struct{}, len(intent.VisualRequirements))
		for _, requirement := range intent.VisualRequirements {
			occurrence, exists := occurrences[requirement.OccurrenceStoryNodeKey]
			if !exists {
				return Candidate{}, errors.New("storyboard visual requirement references an unknown Occurrence")
			}
			if _, duplicate := seenRequirements[requirement.OccurrenceStoryNodeKey]; duplicate {
				return Candidate{}, errors.New("storyboard intent has duplicate visual requirements")
			}
			seenRequirements[requirement.OccurrenceStoryNodeKey] = struct{}{}
			coveredOccurrences[requirement.OccurrenceStoryNodeKey] = struct{}{}
			if requirement.IdentityStoryNodeKey != occurrence.IdentityStoryNodeKey ||
				requirement.SpecificationStoryNodeKey != occurrence.SpecificationStoryNodeKey ||
				requirement.AssetStateStoryNodeKey != occurrence.AssetStateStoryNodeKey ||
				requirement.AssetID != occurrence.AssetID || requirement.SpecificationVersionID != occurrence.SpecificationVersionID ||
				requirement.AssetStateID != occurrence.AssetStateID || requirement.AssetRole != assetRole(occurrence.AssetKind) ||
				!validViewRoles(occurrence.AssetKind, requirement.RequiredViewRoles) {
				return Candidate{}, errors.New("storyboard visual requirement changed its formal identity or state")
			}
			if requirement.AssetVersionRef == nil {
				if requirement.AssetReadiness != "needs_asset" {
					return Candidate{}, errors.New("missing Storyboard AssetVersion must remain needs_asset")
				}
				hasMissingAsset = true
				continue
			}
			version, found := versions[requirement.AssetVersionRef.AssetVersionID]
			if !found || requirement.AssetReadiness != "ready" || version.AssetID != requirement.AssetID ||
				version.AssetStateID != requirement.AssetStateID || version.Revision != requirement.AssetVersionRef.Revision ||
				version.ContentHash != requirement.AssetVersionRef.ContentHash || version.LineageHash != requirement.AssetVersionRef.LineageHash ||
				version.StyleSnapshotHash != input.EffectiveStyleSnapshot.ContentHash || !containsRoles(version.ViewRoles, requirement.RequiredViewRoles) {
				return Candidate{}, errors.New("storyboard visual requirement references a drifted AssetVersion")
			}
		}
		for _, issue := range intent.ReviewIssues {
			if strings.TrimSpace(issue.Code) == "" || strings.TrimSpace(issue.Summary) == "" ||
				(issue.Severity != "warning" && issue.Severity != "blocking") {
				return Candidate{}, errors.New("storyboard candidate contains an invalid Review Issue")
			}
			for _, evidence := range issue.Evidence {
				if _, exists := allowedEvidence[evidenceKey(evidence.DocumentRevisionID, evidence.AbsoluteStart, evidence.AbsoluteEnd, evidence.TextHash)]; !exists {
					return Candidate{}, errors.New("storyboard Review Issue Evidence is outside its exact Scene")
				}
			}
		}
	}
	for _, covered := range requiredBeats {
		if !covered {
			return Candidate{}, errors.New("storyboard candidate does not cover every required Beat")
		}
	}
	if len(coveredOccurrences) != len(occurrences) {
		return Candidate{}, errors.New("storyboard candidate does not cover every formal Occurrence")
	}
	expectedReadiness := "ready"
	if hasMissingAsset {
		expectedReadiness = "needs_asset"
	}
	if candidate.AssetReadiness != expectedReadiness {
		return Candidate{}, errors.New("storyboard candidate readiness does not match its visual requirements")
	}
	return candidate, nil
}

func evidenceKey(documentID string, start, end int, hash string) string {
	return documentID + "\x00" + fmt.Sprint(start) + "\x00" + fmt.Sprint(end) + "\x00" + hash
}

func assetRole(kind string) string {
	switch kind {
	case "character":
		return "subject"
	case "location":
		return "environment"
	case "prop":
		return "prop"
	default:
		return ""
	}
}

func validViewRoles(kind string, roles []string) bool {
	want := map[string][]string{
		"character": {"front", "profile", "back"},
		"location":  {"environment"},
		"prop":      {"prop"},
	}[kind]
	if len(roles) != len(want) {
		return false
	}
	for index := range roles {
		if roles[index] != want[index] {
			return false
		}
	}
	return true
}

func containsRoles(available, required []string) bool {
	set := make(map[string]struct{}, len(available))
	for _, role := range available {
		set[role] = struct{}{}
	}
	for _, role := range required {
		if _, exists := set[role]; !exists {
			return false
		}
	}
	return true
}

func BuildDraftManifest(input StoryGraphDraftInput) (DraftManifest, error) {
	for _, identifier := range []string{
		input.WorkspaceID, input.ProjectID, input.WorkflowRunID, input.NodeRunID, input.GraphVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return DraftManifest{}, errors.New("invalid Storyboard Draft manifest owner")
		}
	}
	if input.GraphVersionNo < 1 || len(input.GraphContentHash) != 64 || len(input.Scenes) == 0 {
		return DraftManifest{}, errors.New("Storyboard Draft requires an exact non-empty StoryGraph")
	}
	rootInputHash, err := agentcontract.CanonicalHash(mustJSON(struct {
		GraphVersionID string                                     `json:"graph_version_id"`
		GraphVersionNo int64                                      `json:"graph_version_no"`
		GraphHash      string                                     `json:"graph_hash"`
		Style          agentcontract.StoryboardStyleSnapshotInput `json:"effective_style_snapshot"`
	}{input.GraphVersionID, input.GraphVersionNo, input.GraphContentHash, input.EffectiveStyleSnapshot}))
	if err != nil {
		return DraftManifest{}, err
	}
	scenes := append([]SceneDraftInput(nil), input.Scenes...)
	sort.Slice(scenes, func(i, j int) bool {
		left, right := scenes[i].StageInput.Scene, scenes[j].StageInput.Scene
		if left.EpisodePosition != right.EpisodePosition {
			return left.EpisodePosition < right.EpisodePosition
		}
		if left.ScenePosition != right.ScenePosition {
			return left.ScenePosition < right.ScenePosition
		}
		return left.StoryNodeKey < right.StoryNodeKey
	})
	shards := make([]DraftManifestShard, len(scenes))
	seen := make(map[string]struct{}, len(scenes))
	for index, scene := range scenes {
		key := "scene:" + scene.StageInput.Scene.StoryNodeKey
		if _, duplicate := seen[key]; duplicate {
			return DraftManifest{}, errors.New("Storyboard Draft Scene shards must be unique")
		}
		seen[key] = struct{}{}
		inputHash, hashErr := agentcontract.CanonicalHash(mustJSON(scene.StageInput))
		if hashErr != nil {
			return DraftManifest{}, hashErr
		}
		shards[index] = DraftManifestShard{
			Kind: "story_scene", Key: key,
			TreePath:          fmt.Sprintf("episodes/%08d/scenes/%08d", scene.StageInput.Scene.EpisodePosition, scene.StageInput.Scene.ScenePosition),
			SceneStoryNodeKey: scene.StageInput.Scene.StoryNodeKey, EpisodeID: scene.EpisodeID,
			EpisodePosition: scene.StageInput.Scene.EpisodePosition, ScenePosition: scene.StageInput.Scene.ScenePosition,
			InputHash: inputHash,
		}
	}
	coverageHash, err := agentcontract.CanonicalHash(mustJSON(shards))
	if err != nil {
		return DraftManifest{}, err
	}
	manifestID := uuid.NewSHA1(uuid.MustParse(input.NodeRunID), []byte("storyboard-draft-manifest-v1")).String()
	manifest := DraftManifest{
		ManifestID: manifestID, WorkspaceID: input.WorkspaceID, WorkflowRunID: input.WorkflowRunID,
		NodeRunID: input.NodeRunID, Version: 1, Stage: "draft_storyboard", RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash,
	}
	manifest.ManifestHash, err = agentcontract.CanonicalHash(mustJSON(struct {
		ManifestID, WorkspaceID, WorkflowRunID, NodeRunID string
		Version                                           int64
		Stage, RootInputHash                              string
		Shards                                            []DraftManifestShard
		CoverageHash                                      string
	}{
		manifest.ManifestID, manifest.WorkspaceID, manifest.WorkflowRunID, manifest.NodeRunID,
		manifest.Version, manifest.Stage, manifest.RootInputHash, manifest.Shards, manifest.CoverageHash,
	}))
	return manifest, err
}

func BuildCandidateSet(
	set DraftSet,
	items []CandidateSetItem,
) (CandidateSet, []byte, string, string, error) {
	ordered := append([]CandidateSetItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SceneStoryNodeKey < ordered[j].SceneStoryNodeKey })
	if len(ordered) == 0 {
		return CandidateSet{}, nil, "", "", errors.New("Storyboard Candidate Set has no Scene candidates")
	}
	for index, item := range ordered {
		if strings.TrimSpace(item.SceneStoryNodeKey) == "" || strings.TrimSpace(item.ShardKey) == "" ||
			len(item.StageInstanceKey) != 64 || len(item.CandidateRevisionHash) != 64 ||
			(item.AssetReadiness != "ready" && item.AssetReadiness != "needs_asset") {
			return CandidateSet{}, nil, "", "", errors.New("invalid Storyboard Scene candidate reference")
		}
		if _, err := uuid.Parse(item.CandidateRevisionID); err != nil {
			return CandidateSet{}, nil, "", "", errors.New("invalid Storyboard Scene candidate revision")
		}
		if index > 0 && ordered[index-1].SceneStoryNodeKey == item.SceneStoryNodeKey {
			return CandidateSet{}, nil, "", "", errors.New("duplicate Storyboard Scene candidate")
		}
	}
	readiness := "ready"
	for _, item := range ordered {
		if item.AssetReadiness == "needs_asset" {
			readiness = "needs_asset"
			break
		}
	}
	candidate := CandidateSet{
		SchemaVersion: "storyboard-intent-candidate-set-v1", DraftSetID: set.ID, DraftSetRevision: set.Revision,
		GraphVersionID:   set.GraphVersionID,
		GraphContentHash: set.GraphContentHash, ManifestID: set.ManifestID,
		ManifestVersion: set.ManifestVersion, ManifestHash: set.ManifestHash,
		Scenes: ordered, AssetReadiness: readiness,
	}
	encoded := mustJSON(candidate)
	contentHash, err := agentcontract.CanonicalHash(encoded)
	if err != nil {
		return CandidateSet{}, nil, "", "", err
	}
	stageKey, err := agentcontract.CanonicalHash(mustJSON(struct {
		Schema, NodeRunID, ManifestHash, GraphContentHash string
	}{"storyboard-intent-candidate-set-stage-v1", set.NodeRunID, set.ManifestHash, set.GraphContentHash}))
	return candidate, encoded, contentHash, stageKey, err
}

func BuildApprovedIntentSet(
	set DraftSet,
	batches []Batch,
	candidateRevision int64,
	reviewDecisionID string,
	approvedID string,
) (ApprovedIntentSet, error) {
	for _, identifier := range []string{
		set.ID, set.WorkspaceID, set.ProjectID, set.GraphVersionID, set.ManifestID,
		reviewDecisionID, approvedID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ApprovedIntentSet{}, errors.New("invalid approved Storyboard Intent identity")
		}
	}
	if set.CandidateRevisionID == nil || set.CandidateRevisionHash == nil {
		return ApprovedIntentSet{}, errors.New("Storyboard Draft Set has no Candidate Revision")
	}
	if _, err := uuid.Parse(*set.CandidateRevisionID); err != nil || len(*set.CandidateRevisionHash) != 64 ||
		set.Revision < 1 || candidateRevision < 1 || set.GraphVersionNo < 1 || len(set.GraphContentHash) != 64 ||
		set.ManifestVersion < 1 || len(set.ManifestHash) != 64 || len(set.Batches) == 0 || len(batches) != len(set.Batches) {
		return ApprovedIntentSet{}, errors.New("Storyboard Draft Set is incomplete")
	}
	byBatch := make(map[string]Batch, len(batches))
	for _, batch := range batches {
		if _, duplicate := byBatch[batch.ID]; duplicate {
			return ApprovedIntentSet{}, errors.New("duplicate Storyboard Draft Batch")
		}
		byBatch[batch.ID] = batch
	}
	scenes := make([]ApprovedIntentScene, len(set.Batches))
	for index, reference := range set.Batches {
		batch, found := byBatch[reference.BatchID]
		if !found || reference.CandidateRevisionID == nil || reference.CandidateRevisionHash == nil ||
			reference.ResultHash == nil || len(*reference.ResultHash) != 64 ||
			batch.CandidateRevisionID == nil || batch.CandidateRevisionHash == nil || batch.ResultHash == nil ||
			batch.ID != reference.BatchID || batch.WorkspaceID != set.WorkspaceID || batch.ProjectID != set.ProjectID ||
			batch.EpisodeID != reference.EpisodeID || batch.StructureID != reference.StructureID ||
			batch.ScriptVersionID != reference.ScriptVersionID || batch.SceneStoryNodeKey != reference.SceneStoryNodeKey ||
			batch.ManifestID != set.ManifestID || batch.ManifestVersion != set.ManifestVersion ||
			batch.GraphVersionID != set.GraphVersionID || batch.GraphVersionNo != set.GraphVersionNo ||
			batch.InputHash != reference.InputHash || *batch.ResultHash != *reference.ResultHash ||
			*batch.CandidateRevisionID != *reference.CandidateRevisionID ||
			*batch.CandidateRevisionHash != *reference.CandidateRevisionHash ||
			batch.Candidate.SceneStoryNodeKey != reference.SceneStoryNodeKey ||
			(batch.Status != "ready" && batch.Status != "needs_asset") ||
			batch.Candidate.AssetReadiness != batch.Status || len(batch.Candidate.ShotIntents) == 0 {
			return ApprovedIntentSet{}, errors.New("Storyboard Draft Batch changed before Intent freeze")
		}
		intents := append([]ShotIntent(nil), batch.Candidate.ShotIntents...)
		sort.Slice(intents, func(left, right int) bool {
			if intents[left].IntentOrder != intents[right].IntentOrder {
				return intents[left].IntentOrder < intents[right].IntentOrder
			}
			return intents[left].ShotKey < intents[right].ShotKey
		})
		for intentIndex := range intents {
			if intents[intentIndex].IntentOrder != intentIndex+1 || strings.TrimSpace(intents[intentIndex].ShotKey) == "" {
				return ApprovedIntentSet{}, errors.New("Storyboard Shot Intent order changed before freeze")
			}
			intents[intentIndex].VisualRequirements = append([]VisualRequirement(nil), intents[intentIndex].VisualRequirements...)
			sort.Slice(intents[intentIndex].VisualRequirements, func(left, right int) bool {
				return intents[intentIndex].VisualRequirements[left].OccurrenceStoryNodeKey <
					intents[intentIndex].VisualRequirements[right].OccurrenceStoryNodeKey
			})
		}
		scenes[index] = ApprovedIntentScene{
			SceneStoryNodeKey: reference.SceneStoryNodeKey, BatchID: batch.ID,
			EpisodeID: batch.EpisodeID, StructureID: batch.StructureID, ScriptVersionID: batch.ScriptVersionID,
			CandidateRevisionID: *batch.CandidateRevisionID, CandidateRevisionHash: *batch.CandidateRevisionHash,
			AssetReadiness: batch.Candidate.AssetReadiness, ShotIntents: intents,
		}
	}
	sort.Slice(scenes, func(left, right int) bool { return scenes[left].SceneStoryNodeKey < scenes[right].SceneStoryNodeKey })
	for index := 1; index < len(scenes); index++ {
		if scenes[index-1].SceneStoryNodeKey == scenes[index].SceneStoryNodeKey {
			return ApprovedIntentSet{}, errors.New("duplicate Storyboard Intent Scene")
		}
	}
	visualHash, err := ApprovedIntentVisualRequirementsHash(scenes)
	if err != nil {
		return ApprovedIntentSet{}, err
	}
	approved := ApprovedIntentSet{
		SchemaVersion: "approved-storyboard-intents-v1", ID: approvedID,
		WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID, DraftSetID: set.ID, DraftSetRevision: set.Revision,
		CandidateRevisionID: *set.CandidateRevisionID, CandidateRevisionHash: *set.CandidateRevisionHash,
		CandidateRevision: candidateRevision, GraphVersionID: set.GraphVersionID, GraphVersionNo: set.GraphVersionNo,
		GraphContentHash: set.GraphContentHash, ManifestID: set.ManifestID, ManifestVersion: set.ManifestVersion,
		ManifestHash: set.ManifestHash, ReviewDecisionID: reviewDecisionID, Scenes: scenes,
		VisualRequirementsHash: visualHash,
	}
	approved.ContentHash, err = ApprovedIntentSetContentHash(approved)
	return approved, err
}

func ApprovedIntentVisualRequirementsHash(scenes []ApprovedIntentScene) (string, error) {
	visualRequirements := make([]struct {
		SceneStoryNodeKey string              `json:"scene_story_node_key"`
		ShotKey           string              `json:"shot_key"`
		Requirements      []VisualRequirement `json:"requirements"`
	}, 0)
	for _, scene := range scenes {
		for _, intent := range scene.ShotIntents {
			visualRequirements = append(visualRequirements, struct {
				SceneStoryNodeKey string              `json:"scene_story_node_key"`
				ShotKey           string              `json:"shot_key"`
				Requirements      []VisualRequirement `json:"requirements"`
			}{scene.SceneStoryNodeKey, intent.ShotKey, intent.VisualRequirements})
		}
	}
	return agentcontract.CanonicalHash(mustJSON(visualRequirements))
}

func ApprovedIntentSetContentHash(approved ApprovedIntentSet) (string, error) {
	return agentcontract.CanonicalHash(mustJSON(struct {
		SchemaVersion          string                `json:"schema_version"`
		ID                     string                `json:"id"`
		WorkspaceID            string                `json:"workspace_id"`
		ProjectID              string                `json:"project_id"`
		DraftSetID             string                `json:"draft_set_id"`
		DraftSetRevision       int                   `json:"draft_set_revision"`
		CandidateRevisionID    string                `json:"candidate_revision_id"`
		CandidateRevisionHash  string                `json:"candidate_revision_hash"`
		CandidateRevision      int64                 `json:"candidate_revision"`
		GraphVersionID         string                `json:"graph_version_id"`
		GraphVersionNo         int64                 `json:"graph_version_no"`
		GraphContentHash       string                `json:"graph_content_hash"`
		ManifestID             string                `json:"manifest_id"`
		ManifestVersion        int64                 `json:"manifest_version"`
		ManifestHash           string                `json:"manifest_hash"`
		ReviewDecisionID       string                `json:"review_decision_id"`
		Scenes                 []ApprovedIntentScene `json:"scenes"`
		VisualRequirementsHash string                `json:"visual_requirements_hash"`
	}{
		approved.SchemaVersion, approved.ID, approved.WorkspaceID, approved.ProjectID,
		approved.DraftSetID, approved.DraftSetRevision, approved.CandidateRevisionID,
		approved.CandidateRevisionHash, approved.CandidateRevision, approved.GraphVersionID,
		approved.GraphVersionNo, approved.GraphContentHash, approved.ManifestID, approved.ManifestVersion,
		approved.ManifestHash, approved.ReviewDecisionID, approved.Scenes, approved.VisualRequirementsHash,
	}))
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func OrderHash(shots []Shot) (string, error) {
	ordered := append([]Shot(nil), shots...)
	values := make([]struct {
		ID, ContentHash string
		Position        int
	}, len(ordered))
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	for index, shot := range ordered {
		values[index] = struct {
			ID, ContentHash string
			Position        int
		}{shot.ID, shot.ContentHash, shot.Position}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return canonicalHash(encoded)
}
func numberAsInt(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), number == float64(int(number))
	case json.Number:
		value, err := number.Int64()
		return int(value), err == nil
	case int:
		return number, true
	default:
		return 0, false
	}
}
func canonicalHash(raw []byte) (string, error) {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
