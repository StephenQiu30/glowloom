package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	SceneAnalysisWireSchemaVersion                = "storygraph-stage-wire-production"
	ScriptSpanCandidateSchemaVersion              = "script-span-candidate-production"
	SceneFactCandidateSchemaVersion               = "scene-fact-candidate-production"
	IdentityResolutionCandidateSchemaVersion      = "identity-resolution-candidate-production"
	StructureIdentityReviewCandidateSchemaVersion = "structure-identity-review-candidate-production"
	SceneAnalysisSkillBundleHash                  = StoryGraphSkillBundleHash
)

var structureIdentityRepairIssuePattern = regexp.MustCompile(`^issue_[a-z0-9_]{1,80}$`)

type SceneAnalysisStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value SceneAnalysisStageVariant) Validate() error {
	expectedSchema := map[string]string{
		"propose_script_spans\x00default":             ScriptSpanCandidateSchemaVersion,
		"extract_scene_facts\x00default":              SceneFactCandidateSchemaVersion,
		"resolve_identities\x00default":               IdentityResolutionCandidateSchemaVersion,
		"review_candidate\x00structure_identity":      StructureIdentityReviewCandidateSchemaVersion,
		"derive_production_entities\x00default":       ProductionEntityFragmentCandidateSchemaVersion,
		"bind_scene_occurrences\x00default":           SceneBindingFragmentCandidateSchemaVersion,
		"reconcile_interaction_continuity\x00default": InteractionContinuityCandidateSchemaVersion,
	}[value.StageKey+"\x00"+value.ProfileKey]
	if value.LaneKey != "primary" ||
		expectedSchema == "" || value.OutputSchemaVersion != expectedSchema {
		return errors.New("invalid Scene Analysis stage variant")
	}
	return nil
}

type ScriptSourceVersionIdentity struct {
	OwnerKind   string    `json:"owner_kind"`
	LogicalID   string    `json:"logical_id"`
	VersionID   string    `json:"version_id"`
	Revision    int64     `json:"revision"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

func (value ScriptSourceVersionIdentity) Validate() error {
	if value.OwnerKind != "production/script" || strings.TrimSpace(value.LogicalID) == "" ||
		value.Revision < 1 || !hashPattern.MatchString(value.ContentHash) || value.CreatedAt.IsZero() {
		return errors.New("invalid script source version identity")
	}
	if _, err := uuid.Parse(value.VersionID); err != nil {
		return errors.New("invalid script source version identity")
	}
	return nil
}

type SceneAnalysisCandidateRevisionIdentity struct {
	StageKey              string `json:"stage_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
}

func (value SceneAnalysisCandidateRevisionIdentity) Validate() error {
	if (value.StageKey != "propose_script_spans" && value.StageKey != "extract_scene_facts" &&
		value.StageKey != "resolve_identities" && value.StageKey != "review_candidate" &&
		value.StageKey != "derive_production_entities" && value.StageKey != "bind_scene_occurrences" &&
		value.StageKey != "reconcile_interaction_continuity") ||
		strings.TrimSpace(value.ShardKey) == "" ||
		!hashPattern.MatchString(value.CandidateRevisionHash) ||
		!hashPattern.MatchString(value.SourceResultHash) {
		return errors.New("invalid Scene Analysis candidate revision identity")
	}
	for _, identifier := range []string{value.CandidateRevisionID, value.SourceInvocationID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Scene Analysis candidate revision identity")
		}
	}
	return nil
}

type SceneAnalysisReleaseIdentity struct {
	SkillReleaseID    string `json:"skill_release_id"`
	SkillReleaseHash  string `json:"skill_release_hash"`
	StageReleaseHash  string `json:"stage_release_hash"`
	BundleContentHash string `json:"bundle_content_hash"`
	AgentImageDigest  string `json:"agent_image_digest"`
}

func (value SceneAnalysisReleaseIdentity) Validate() error {
	if _, err := uuid.Parse(value.SkillReleaseID); err != nil {
		return errors.New("invalid Scene Analysis release identity")
	}
	if !hashPattern.MatchString(value.SkillReleaseHash) ||
		!hashPattern.MatchString(value.StageReleaseHash) ||
		value.BundleContentHash != SceneAnalysisSkillBundleHash ||
		!strings.HasPrefix(value.AgentImageDigest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(value.AgentImageDigest, "sha256:")) {
		return errors.New("invalid Scene Analysis release identity")
	}
	return nil
}

type SceneAnalysisControlProof struct {
	ControlRecordID string `json:"control_record_id"`
	ControlRevision int64  `json:"control_revision"`
	Status          string `json:"status"`
	ControlHash     string `json:"control_hash"`
	ReleaseFence    int64  `json:"release_fence"`
}

func (value SceneAnalysisControlProof) Validate() error {
	if _, err := uuid.Parse(value.ControlRecordID); err != nil {
		return errors.New("invalid Scene Analysis Control proof")
	}
	if value.ControlRevision < 1 || value.Status != "approved" ||
		!hashPattern.MatchString(value.ControlHash) || value.ReleaseFence < 0 {
		return errors.New("invalid Scene Analysis Control proof")
	}
	return nil
}

type SceneAnalysisExecutionBudget struct {
	MaxAttempts         int `json:"max_attempts"`
	MaxModelCalls       int `json:"max_model_calls"`
	MaxExecutionSeconds int `json:"max_execution_seconds"`
	MaxOutputBytes      int `json:"max_output_bytes"`
}

func (value SceneAnalysisExecutionBudget) Validate() error {
	if value.MaxAttempts < 1 || value.MaxAttempts > 3 ||
		value.MaxModelCalls < 1 || value.MaxModelCalls > 2 ||
		value.MaxExecutionSeconds < 1 || value.MaxExecutionSeconds > 600 ||
		value.MaxOutputBytes < 1024 || value.MaxOutputBytes > 1_048_576 {
		return errors.New("invalid Scene Analysis execution budget")
	}
	return nil
}

type SceneAnalysisScope struct {
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   string  `json:"project_id"`
	EpisodeID   *string `json:"episode_id"`
	SceneID     *string `json:"scene_id"`
	EntityID    *string `json:"entity_id"`
	TargetID    *string `json:"target_id"`
}

func (value SceneAnalysisScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Scene Analysis scope")
		}
	}
	for _, identifier := range []*string{value.EpisodeID, value.SceneID, value.EntityID, value.TargetID} {
		if identifier != nil {
			if _, err := uuid.Parse(*identifier); err != nil {
				return errors.New("invalid Scene Analysis scope")
			}
		}
	}
	if value.EpisodeID == nil && (value.SceneID != nil || value.EntityID != nil || value.TargetID != nil) {
		return errors.New("invalid Scene Analysis scope")
	}
	return nil
}

type SceneAnalysisShard struct {
	ManifestID     string `json:"manifest_id"`
	ManifestHash   string `json:"manifest_hash"`
	ShardKey       string `json:"shard_key"`
	CodepointStart int    `json:"codepoint_start"`
	CodepointEnd   int    `json:"codepoint_end"`
}

func (value SceneAnalysisShard) Validate() error {
	if _, err := uuid.Parse(value.ManifestID); err != nil {
		return errors.New("invalid Scene Analysis shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) || strings.TrimSpace(value.ShardKey) == "" ||
		value.CodepointStart < 0 || value.CodepointEnd <= value.CodepointStart {
		return errors.New("invalid Scene Analysis shard")
	}
	return nil
}

type ScriptSpanProposalInput struct {
	SourceVersionID      string                            `json:"source_version_id"`
	SourceHash           string                            `json:"source_hash"`
	NormalizedText       string                            `json:"normalized_text"`
	CodepointCount       int                               `json:"codepoint_count"`
	NewlineNormalization string                            `json:"newline_normalization"`
	Repair               *StructureIdentityRepairDirective `json:"repair,omitempty"`
}

func (value ScriptSpanProposalInput) Validate() error {
	if _, err := uuid.Parse(value.SourceVersionID); err != nil {
		return errors.New("invalid script span source identity")
	}
	if !utf8.ValidString(value.NormalizedText) || value.NormalizedText == "" ||
		strings.Contains(value.NormalizedText, "\r") ||
		utf8.RuneCountInString(value.NormalizedText) != value.CodepointCount ||
		hashUTF8(value.NormalizedText) != value.SourceHash || value.NewlineNormalization != "lf" {
		return errors.New("invalid script span source")
	}
	if value.Repair != nil && value.Repair.ValidateFor("propose_script_spans") != nil {
		return errors.New("invalid script span repair directive")
	}
	return nil
}

type StructureIdentityRepairEvidence struct {
	SourceVersionID string `json:"source_version_id"`
	SourceStart     int    `json:"source_start"`
	SourceEnd       int    `json:"source_end"`
	TextHash        string `json:"text_hash"`
}

type StructureIdentityRepairChange struct {
	Operation         string   `json:"operation"`
	TargetKeys        []string `json:"target_keys"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type StructureIdentityRepairDirective struct {
	ReviewDecisionID    string                            `json:"review_decision_id"`
	DecisionPayloadHash string                            `json:"decision_payload_hash"`
	IssueRefs           []string                          `json:"issue_refs"`
	EvidenceRefs        []StructureIdentityRepairEvidence `json:"evidence_refs"`
	ChangeSpec          StructureIdentityRepairChange     `json:"change_spec"`
	ReasonCode          string                            `json:"reason_code"`
}

func (value StructureIdentityRepairDirective) ValidateFor(stageKey string) error {
	if _, err := uuid.Parse(value.ReviewDecisionID); err != nil || !hashPattern.MatchString(value.DecisionPayloadHash) ||
		len(value.IssueRefs) == 0 || len(value.EvidenceRefs) == 0 || len(value.ChangeSpec.TargetKeys) == 0 ||
		len(value.ChangeSpec.AffectedScopeKeys) == 0 {
		return errors.New("invalid structure identity repair directive")
	}
	for index, issue := range value.IssueRefs {
		if !structureIdentityRepairIssuePattern.MatchString(issue) ||
			(index > 0 && value.IssueRefs[index-1] >= issue) {
			return errors.New("invalid structure identity repair issue")
		}
	}
	for index, evidence := range value.EvidenceRefs {
		if _, err := uuid.Parse(evidence.SourceVersionID); err != nil || evidence.SourceStart < 0 ||
			evidence.SourceEnd <= evidence.SourceStart || !hashPattern.MatchString(evidence.TextHash) ||
			(index > 0 && compareStructureIdentityRepairEvidence(value.EvidenceRefs[index-1], evidence) >= 0) {
			return errors.New("invalid structure identity repair evidence")
		}
	}
	for index, key := range value.ChangeSpec.TargetKeys {
		if strings.TrimSpace(key) == "" || (index > 0 && value.ChangeSpec.TargetKeys[index-1] >= key) {
			return errors.New("invalid structure identity repair target")
		}
	}
	for index, scope := range value.ChangeSpec.AffectedScopeKeys {
		identifier, found := strings.CutPrefix(scope, "scene:")
		if !found || (index > 0 && value.ChangeSpec.AffectedScopeKeys[index-1] >= scope) {
			return errors.New("invalid structure identity repair scope")
		}
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity repair scope")
		}
	}
	spanRepair := slices.Contains([]string{"inspect_source", "adjust_episode_boundary", "adjust_scene_boundary"}, value.ChangeSpec.Operation)
	identityRepair := slices.Contains([]string{"separate_identity", "merge_identity", "resolve_mention", "reject_mention"}, value.ChangeSpec.Operation)
	if (stageKey == "propose_script_spans") != spanRepair || (stageKey == "resolve_identities") != identityRepair {
		return errors.New("structure identity repair operation targets another stage")
	}
	if (spanRepair && value.ReasonCode != "source_interpretation_incorrect" && value.ReasonCode != "insufficient_evidence" &&
		value.ReasonCode != "structure_boundary_incorrect") ||
		(identityRepair && value.ReasonCode != "identity_resolution_incorrect") {
		return errors.New("invalid structure identity repair reason")
	}
	return nil
}

func compareStructureIdentityRepairEvidence(left, right StructureIdentityRepairEvidence) int {
	return strings.Compare(
		fmt.Sprintf("%s\x00%012d\x00%012d\x00%s", left.SourceVersionID, left.SourceStart, left.SourceEnd, left.TextHash),
		fmt.Sprintf("%s\x00%012d\x00%012d\x00%s", right.SourceVersionID, right.SourceStart, right.SourceEnd, right.TextHash),
	)
}

type SceneFactExtractionInput struct {
	SourceVersionID           string          `json:"source_version_id"`
	SourceHash                string          `json:"source_hash"`
	NormalizedText            string          `json:"normalized_text"`
	SpanCandidateRevisionID   string          `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash string          `json:"span_candidate_revision_hash"`
	SpanCandidate             json.RawMessage `json:"span_candidate"`
}

type IdentityResolutionInput struct {
	SourceVersionID                string                            `json:"source_version_id"`
	SourceHash                     string                            `json:"source_hash"`
	NormalizedText                 string                            `json:"normalized_text"`
	SceneFactCandidateRevisionID   string                            `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash string                            `json:"scene_fact_candidate_revision_hash"`
	SceneFactCandidate             json.RawMessage                   `json:"scene_fact_candidate"`
	AllowedReuseIdentityKeys       []string                          `json:"allowed_reuse_identity_keys"`
	Repair                         *StructureIdentityRepairDirective `json:"repair,omitempty"`
}

type StructureIdentityReviewInput struct {
	SourceVersionID                string                 `json:"source_version_id"`
	SourceHash                     string                 `json:"source_hash"`
	NormalizedText                 string                 `json:"normalized_text"`
	SpanCandidateRevisionID        string                 `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash      string                 `json:"span_candidate_revision_hash"`
	SpanCandidate                  json.RawMessage        `json:"span_candidate"`
	SceneFactCandidateRevisionID   string                 `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash string                 `json:"scene_fact_candidate_revision_hash"`
	SceneFactCandidate             json.RawMessage        `json:"scene_fact_candidate"`
	IdentityCandidateRevisionID    string                 `json:"identity_candidate_revision_id"`
	IdentityCandidateRevisionHash  string                 `json:"identity_candidate_revision_hash"`
	IdentityCandidate              json.RawMessage        `json:"identity_candidate"`
	DeterministicIssues            []CandidateReviewIssue `json:"deterministic_issues"`
}

func (value StructureIdentityReviewInput) Validate() error {
	for _, identifier := range []string{
		value.SourceVersionID, value.SpanCandidateRevisionID,
		value.SceneFactCandidateRevisionID, value.IdentityCandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity review identity")
		}
	}
	if value.NormalizedText == "" || hashUTF8(value.NormalizedText) != value.SourceHash ||
		!hashPattern.MatchString(value.SpanCandidateRevisionHash) ||
		!hashPattern.MatchString(value.SceneFactCandidateRevisionHash) ||
		!hashPattern.MatchString(value.IdentityCandidateRevisionHash) ||
		ValidateScriptSpanCandidate(value.SpanCandidate, value.NormalizedText) != nil ||
		ValidateSceneFactCandidate(value.SceneFactCandidate, value.NormalizedText, value.SpanCandidate) != nil ||
		value.DeterministicIssues == nil {
		return errors.New("invalid structure identity review input")
	}
	var spans ScriptSpanCandidate
	var facts SceneFactCandidate
	var identities IdentityResolutionCandidate
	if decodeStrict(value.SpanCandidate, &spans) != nil ||
		decodeStrict(value.SceneFactCandidate, &facts) != nil ||
		decodeStrict(value.IdentityCandidate, &identities) != nil {
		return errors.New("invalid structure identity review candidate chain")
	}
	allowedReuse := make(map[string]struct{})
	for _, cluster := range identities.ResolvedClusters {
		if cluster.ReuseIdentityKey != nil {
			allowedReuse[*cluster.ReuseIdentityKey] = struct{}{}
		}
	}
	if ValidateIdentityResolutionCandidate(value.IdentityCandidate, value.SceneFactCandidate, allowedReuse) != nil ||
		spans.SourceVersionID != value.SourceVersionID || spans.SourceHash != value.SourceHash ||
		facts.SourceVersionID != value.SourceVersionID || facts.SourceHash != value.SourceHash ||
		facts.SpanCandidateRevisionID != value.SpanCandidateRevisionID ||
		facts.SpanCandidateRevisionHash != value.SpanCandidateRevisionHash ||
		identities.SourceVersionID != value.SourceVersionID || identities.SourceHash != value.SourceHash ||
		identities.SceneFactCandidateRevisionID != value.SceneFactCandidateRevisionID ||
		identities.SceneFactCandidateRevisionHash != value.SceneFactCandidateRevisionHash {
		return errors.New("structure identity review candidate chain drifted")
	}
	previous := ""
	for _, issue := range value.DeterministicIssues {
		if issue.IssueKey <= previous || validateCandidateReviewIssue(issue, []rune(value.NormalizedText), true) != nil {
			return errors.New("deterministic review issues must be unique and sorted")
		}
		previous = issue.IssueKey
	}
	return nil
}

func (value IdentityResolutionInput) Validate() error {
	for _, identifier := range []string{value.SourceVersionID, value.SceneFactCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid identity resolution source identity")
		}
	}
	var sceneFacts SceneFactCandidate
	if value.NormalizedText == "" || hashUTF8(value.NormalizedText) != value.SourceHash ||
		!hashPattern.MatchString(value.SceneFactCandidateRevisionHash) ||
		decodeStrict(value.SceneFactCandidate, &sceneFacts) != nil ||
		sceneFacts.SourceVersionID != value.SourceVersionID || sceneFacts.SourceHash != value.SourceHash {
		return errors.New("invalid identity resolution source")
	}
	for index, key := range value.AllowedReuseIdentityKeys {
		if strings.TrimSpace(key) == "" || index > 0 && value.AllowedReuseIdentityKeys[index-1] >= key {
			return errors.New("identity reuse allowlist must be sorted and unique")
		}
	}
	if value.Repair != nil && value.Repair.ValidateFor("resolve_identities") != nil {
		return errors.New("invalid identity resolution repair directive")
	}
	return nil
}

func (value SceneFactExtractionInput) Validate() error {
	for _, identifier := range []string{value.SourceVersionID, value.SpanCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid scene fact source identity")
		}
	}
	if value.NormalizedText == "" || hashUTF8(value.NormalizedText) != value.SourceHash ||
		!hashPattern.MatchString(value.SpanCandidateRevisionHash) ||
		ValidateScriptSpanCandidate(value.SpanCandidate, value.NormalizedText) != nil {
		return errors.New("invalid scene fact source")
	}
	return nil
}

type SceneAnalysisPayload struct {
	Variant            SceneAnalysisStageVariant                `json:"variant"`
	Scope              SceneAnalysisScope                       `json:"scope"`
	SourceRefs         []ScriptSourceVersionIdentity            `json:"source_refs"`
	UpstreamCandidates []SceneAnalysisCandidateRevisionIdentity `json:"upstream_candidates"`
	Shard              SceneAnalysisShard                       `json:"shard"`
	StageInput         json.RawMessage                          `json:"stage_input"`
	ProductionRepair   *ProductionWorldRepairDirective          `json:"production_world_repair,omitempty"`
}

func (value SceneAnalysisPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil || value.Shard.Validate() != nil ||
		len(value.SourceRefs) != 1 || value.SourceRefs[0].Validate() != nil || !jsonObject(value.StageInput) {
		return errors.New("invalid Scene Analysis payload")
	}
	if value.ProductionRepair != nil && value.ProductionRepair.ValidateFor(value.Variant.StageKey) != nil {
		return errors.New("invalid Production World repair payload")
	}
	source := value.SourceRefs[0]
	switch value.Variant.StageKey {
	case "propose_script_spans":
		var input ScriptSpanProposalInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 0 || source.VersionID != input.SourceVersionID ||
			source.ContentHash != input.SourceHash || value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != input.CodepointCount {
			return errors.New("script span input does not match its frozen source")
		}
	case "extract_scene_facts":
		var input SceneFactExtractionInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 1 || value.UpstreamCandidates[0].Validate() != nil ||
			source.VersionID != input.SourceVersionID || source.ContentHash != input.SourceHash ||
			value.UpstreamCandidates[0].CandidateRevisionID != input.SpanCandidateRevisionID ||
			value.UpstreamCandidates[0].CandidateRevisionHash != input.SpanCandidateRevisionHash ||
			value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("scene fact input does not match its frozen spans")
		}
	case "resolve_identities":
		var input IdentityResolutionInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 1 || value.UpstreamCandidates[0].Validate() != nil ||
			value.UpstreamCandidates[0].StageKey != "extract_scene_facts" ||
			source.VersionID != input.SourceVersionID || source.ContentHash != input.SourceHash ||
			value.UpstreamCandidates[0].CandidateRevisionID != input.SceneFactCandidateRevisionID ||
			value.UpstreamCandidates[0].CandidateRevisionHash != input.SceneFactCandidateRevisionHash ||
			value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("identity input does not match its frozen SceneFacts")
		}
	case "review_candidate":
		var input StructureIdentityReviewInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 3 || source.VersionID != input.SourceVersionID ||
			source.ContentHash != input.SourceHash || value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("review input does not match its frozen structure and identity")
		}
		expected := map[string][2]string{
			"propose_script_spans": {input.SpanCandidateRevisionID, input.SpanCandidateRevisionHash},
			"extract_scene_facts":  {input.SceneFactCandidateRevisionID, input.SceneFactCandidateRevisionHash},
			"resolve_identities":   {input.IdentityCandidateRevisionID, input.IdentityCandidateRevisionHash},
		}
		for _, upstream := range value.UpstreamCandidates {
			identity, exists := expected[upstream.StageKey]
			if !exists || upstream.Validate() != nil || identity != [2]string{
				upstream.CandidateRevisionID, upstream.CandidateRevisionHash,
			} {
				return errors.New("review upstream Candidate identity drifted")
			}
			delete(expected, upstream.StageKey)
		}
		if len(expected) != 0 {
			return errors.New("review upstream Candidate set is incomplete")
		}
	case "derive_production_entities":
		var input ProductionEntityDerivationInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 1 || value.UpstreamCandidates[0].Validate() != nil ||
			value.UpstreamCandidates[0].StageKey != "extract_scene_facts" ||
			source.VersionID != input.SourceVersionID || source.ContentHash != input.SourceHash ||
			value.Scope.WorkspaceID != input.StructureIdentitySet.WorkspaceID ||
			value.Scope.ProjectID != input.StructureIdentitySet.ProjectID ||
			value.UpstreamCandidates[0].CandidateRevisionID != input.SceneFactCandidateRevisionID ||
			value.UpstreamCandidates[0].CandidateRevisionHash != input.SceneFactCandidateRevisionHash ||
			value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("production entity input does not match its frozen formal identities")
		}
	case "bind_scene_occurrences":
		var input SceneOccurrenceBindingInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 2 || source.VersionID != input.SourceVersionID ||
			source.ContentHash != input.SourceHash || value.Scope.WorkspaceID != input.StructureIdentitySet.WorkspaceID ||
			value.Scope.ProjectID != input.StructureIdentitySet.ProjectID || value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("Scene binding input does not match its frozen production graph")
		}
		expected := map[string][2]string{
			"extract_scene_facts":        {input.SceneFactCandidateRevisionID, input.SceneFactCandidateRevisionHash},
			"derive_production_entities": {input.ProductionEntityCandidateRevisionID, input.ProductionEntityCandidateRevisionHash},
		}
		for _, upstream := range value.UpstreamCandidates {
			identity, exists := expected[upstream.StageKey]
			if !exists || upstream.Validate() != nil || identity != [2]string{
				upstream.CandidateRevisionID, upstream.CandidateRevisionHash,
			} {
				return errors.New("Scene binding upstream Candidate identity drifted")
			}
			delete(expected, upstream.StageKey)
		}
		if len(expected) != 0 {
			return errors.New("Scene binding upstream Candidate set is incomplete")
		}
	case "reconcile_interaction_continuity":
		var input InteractionContinuityInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 3 || source.VersionID != input.SourceVersionID ||
			source.ContentHash != input.SourceHash ||
			value.Scope.WorkspaceID != input.StructureIdentitySet.WorkspaceID ||
			value.Scope.ProjectID != input.StructureIdentitySet.ProjectID ||
			value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != utf8.RuneCountInString(input.NormalizedText) {
			return errors.New("Interaction/Continuity input does not match its frozen production graph")
		}
		expected := map[string][2]string{
			"extract_scene_facts":        {input.SceneFactCandidateRevisionID, input.SceneFactCandidateRevisionHash},
			"derive_production_entities": {input.ProductionEntityCandidateRevisionID, input.ProductionEntityCandidateRevisionHash},
			"bind_scene_occurrences":     {input.SceneBindingCandidateRevisionID, input.SceneBindingCandidateRevisionHash},
		}
		for _, upstream := range value.UpstreamCandidates {
			identity, exists := expected[upstream.StageKey]
			if !exists || upstream.Validate() != nil || identity != [2]string{
				upstream.CandidateRevisionID, upstream.CandidateRevisionHash,
			} {
				return errors.New("Interaction/Continuity upstream Candidate identity drifted")
			}
			delete(expected, upstream.StageKey)
		}
		if len(expected) != 0 {
			return errors.New("Interaction/Continuity upstream Candidate set is incomplete")
		}
	default:
		return errors.New("unsupported Scene Analysis stage")
	}
	return nil
}

type SceneAnalysisInvocation struct {
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Budget            SceneAnalysisExecutionBudget `json:"budget"`
	Payload           SceneAnalysisPayload         `json:"payload"`
	InputHash         string                       `json:"input_hash"`
}

func NewSceneAnalysisInvocation(
	invocationID, attemptID string,
	release SceneAnalysisReleaseIdentity,
	control SceneAnalysisControlProof,
	budget SceneAnalysisExecutionBudget,
	payload SceneAnalysisPayload,
) (SceneAnalysisInvocation, error) {
	value := SceneAnalysisInvocation{
		InvocationID: invocationID, AttemptID: attemptID, Kind: "storygraph_stage",
		WireSchemaVersion: SceneAnalysisWireSchemaVersion, StageRelease: release,
		Control: control, Budget: budget, Payload: payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return SceneAnalysisInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return SceneAnalysisInvocation{}, err
	}
	return value, nil
}

func DecodeSceneAnalysisInvocation(raw []byte) (SceneAnalysisInvocation, error) {
	var value SceneAnalysisInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return SceneAnalysisInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return SceneAnalysisInvocation{}, err
	}
	return value, nil
}

func (value SceneAnalysisInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Scene Analysis invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.Budget.Validate() != nil || value.Payload.Validate() != nil ||
		!hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Scene Analysis invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("scene Analysis input hash mismatch")
	}
	return nil
}

func (value SceneAnalysisInvocation) ComputeInputHash() (string, error) {
	payload := value.Payload
	payload.SourceRefs = make([]ScriptSourceVersionIdentity, len(value.Payload.SourceRefs))
	copy(payload.SourceRefs, value.Payload.SourceRefs)
	payload.UpstreamCandidates = make(
		[]SceneAnalysisCandidateRevisionIdentity,
		len(value.Payload.UpstreamCandidates),
	)
	copy(payload.UpstreamCandidates, value.Payload.UpstreamCandidates)
	sort.Slice(payload.SourceRefs, func(i, j int) bool {
		left, right := payload.SourceRefs[i], payload.SourceRefs[j]
		return fmt.Sprintf("%s|%s|%s|%020d|%s", left.OwnerKind, left.LogicalID, left.VersionID, left.Revision, left.ContentHash) <
			fmt.Sprintf("%s|%s|%s|%020d|%s", right.OwnerKind, right.LogicalID, right.VersionID, right.Revision, right.ContentHash)
	})
	sort.Slice(payload.UpstreamCandidates, func(i, j int) bool {
		left, right := payload.UpstreamCandidates[i], payload.UpstreamCandidates[j]
		return fmt.Sprintf("%s|%s|%s|%s", left.StageKey, left.ShardKey, left.CandidateRevisionID, left.CandidateRevisionHash) <
			fmt.Sprintf("%s|%s|%s|%s", right.StageKey, right.ShardKey, right.CandidateRevisionID, right.CandidateRevisionHash)
	})
	material := struct {
		WireSchemaVersion string                       `json:"wire_schema_version"`
		StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
		Control           SceneAnalysisControlProof    `json:"control"`
		Budget            SceneAnalysisExecutionBudget `json:"budget"`
		Payload           SceneAnalysisPayload         `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(encoded)
}

func (value SceneAnalysisInvocation) StageInstanceKey() string {
	root := struct {
		IdentityContractID string                    `json:"identity_contract_id"`
		VariantKey         SceneAnalysisStageVariant `json:"variant_key"`
		Scope              SceneAnalysisScope        `json:"scope"`
		ShardManifestHash  string                    `json:"shard_manifest_hash"`
		ShardKey           string                    `json:"shard_key"`
		InputHash          string                    `json:"input_hash"`
	}{
		IdentityContractID: "storygraph-stage-instance-production",
		VariantKey:         value.Payload.Variant,
		Scope:              value.Payload.Scope,
		ShardManifestHash:  value.Payload.Shard.ManifestHash,
		ShardKey:           value.Payload.Shard.ShardKey,
		InputHash:          value.InputHash,
	}
	encoded, _ := json.Marshal(root)
	hash, _ := ProductionCanonicalHash(encoded)
	return hash
}

type SceneAnalysisDispatchAuthorization struct {
	Value        string
	Hash         string
	ClaimVersion int64
	ExpiresAt    time.Time
}

func (value SceneAnalysisDispatchAuthorization) Validate() error {
	digest := sha256.Sum256([]byte(value.Value))
	if strings.TrimSpace(value.Value) == "" || !hashPattern.MatchString(value.Hash) ||
		hex.EncodeToString(digest[:]) != value.Hash || value.ClaimVersion < 1 || value.ExpiresAt.IsZero() {
		return errors.New("invalid Scene Analysis dispatch authorization")
	}
	return nil
}

type SceneAnalysisDispatchAuthorizationClaims struct {
	InvocationID      string `json:"invocation_id"`
	AttemptID         string `json:"attempt_id"`
	InputHash         string `json:"input_hash"`
	SkillReleaseID    string `json:"skill_release_id"`
	SkillReleaseHash  string `json:"skill_release_hash"`
	StageReleaseHash  string `json:"stage_release_hash"`
	BundleContentHash string `json:"bundle_content_hash"`
	ControlHash       string `json:"control_hash"`
	ReleaseFence      int64  `json:"release_fence"`
	ClaimVersion      int64  `json:"claim_version"`
	AgentImageDigest  string `json:"agent_image_digest"`
	ExpiresAt         int64  `json:"expires_at"`
}

func (value SceneAnalysisDispatchAuthorizationClaims) ValidateFor(
	invocation SceneAnalysisInvocation,
	claimVersion, nowUnix int64,
) error {
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.InputHash != invocation.InputHash || value.SkillReleaseID != invocation.StageRelease.SkillReleaseID ||
		value.SkillReleaseHash != invocation.StageRelease.SkillReleaseHash ||
		value.StageReleaseHash != invocation.StageRelease.StageReleaseHash ||
		value.BundleContentHash != invocation.StageRelease.BundleContentHash ||
		value.ControlHash != invocation.Control.ControlHash ||
		value.ReleaseFence != invocation.Control.ReleaseFence || value.ClaimVersion != claimVersion ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid Scene Analysis dispatch authorization claims")
	}
	return nil
}

type SceneAnalysisDiagnostic struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

type SceneAnalysisResultError struct {
	Code        string `json:"code"`
	SafeSummary string `json:"safe_summary"`
	RetryClass  string `json:"retry_class"`
}

type SceneAnalysisExecutor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type SceneAnalysisAttemptResult struct {
	InvocationID              string                       `json:"invocation_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Kind                      string                       `json:"kind"`
	WireSchemaVersion         string                       `json:"wire_schema_version"`
	Variant                   SceneAnalysisStageVariant    `json:"variant"`
	StageRelease              SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control                   SceneAnalysisControlProof    `json:"control"`
	ClaimVersion              int64                        `json:"claim_version"`
	DispatchAuthorizationHash string                       `json:"dispatch_authorization_hash"`
	Status                    string                       `json:"status"`
	CandidateType             string                       `json:"candidate_type"`
	Candidate                 json.RawMessage              `json:"candidate"`
	InputHash                 string                       `json:"input_hash"`
	OutputHash                *string                      `json:"output_hash"`
	Diagnostics               []SceneAnalysisDiagnostic    `json:"diagnostics"`
	DiagnosticHash            string                       `json:"diagnostic_hash"`
	CompletedAt               time.Time                    `json:"completed_at"`
	Executor                  SceneAnalysisExecutor        `json:"executor"`
	Error                     *SceneAnalysisResultError    `json:"error"`
	ResultHash                string                       `json:"result_hash"`
}

func DecodeSceneAnalysisAttemptResult(raw []byte) (SceneAnalysisAttemptResult, error) {
	var value SceneAnalysisAttemptResult
	if err := decodeStrict(raw, &value); err != nil {
		return SceneAnalysisAttemptResult{}, err
	}
	return value, nil
}

func (value SceneAnalysisAttemptResult) ValidateFor(
	invocation SceneAnalysisInvocation,
	claimVersion int64,
	dispatchAuthorizationHash string,
) error {
	expectedCandidateType := map[string]string{
		"propose_script_spans":             "script_span_candidate",
		"extract_scene_facts":              "scene_fact_candidate",
		"resolve_identities":               "identity_resolution_candidate",
		"review_candidate":                 "structure_identity_review_candidate",
		"derive_production_entities":       "production_entity_fragment_candidate",
		"bind_scene_occurrences":           "scene_binding_fragment_candidate",
		"reconcile_interaction_continuity": "continuity_fragment_candidate",
	}
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.Variant != invocation.Payload.Variant || value.StageRelease != invocation.StageRelease ||
		value.Control != invocation.Control || value.InputHash != invocation.InputHash ||
		value.ClaimVersion != claimVersion || value.DispatchAuthorizationHash != dispatchAuthorizationHash ||
		claimVersion < 1 || !hashPattern.MatchString(dispatchAuthorizationHash) ||
		value.CandidateType != expectedCandidateType[invocation.Payload.Variant.StageKey] ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "text" ||
		value.Executor.RuntimeImageDigest != invocation.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "scene-analysis-harness" || strings.TrimSpace(value.Executor.Model) == "" {
		return errors.New("scene Analysis result identity does not match invocation")
	}
	computedResultHash, err := value.ComputeResultHash()
	if err != nil || !hashPattern.MatchString(value.ResultHash) || computedResultHash != value.ResultHash {
		return errors.New("scene Analysis result hash mismatch")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := ProductionCanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("scene Analysis diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Scene Analysis result is incomplete")
		}
		outputHash, hashErr := ProductionCanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("scene Analysis output hash mismatch")
		}
		switch invocation.Payload.Variant.StageKey {
		case "propose_script_spans":
			var input ScriptSpanProposalInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil ||
				ValidateScriptSpanCandidate(value.Candidate, input.NormalizedText) != nil {
				return errors.New("invalid accepted ScriptSpan candidate")
			}
			var candidate ScriptSpanCandidate
			if decodeStrict(value.Candidate, &candidate) != nil ||
				candidate.SourceVersionID != input.SourceVersionID {
				return errors.New("ScriptSpan candidate source identity drifted")
			}
		case "extract_scene_facts":
			var input SceneFactExtractionInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil ||
				ValidateSceneFactCandidate(value.Candidate, input.NormalizedText, input.SpanCandidate) != nil {
				return errors.New("invalid accepted SceneFact candidate")
			}
			var candidate SceneFactCandidate
			if decodeStrict(value.Candidate, &candidate) != nil ||
				candidate.SourceVersionID != input.SourceVersionID ||
				candidate.SpanCandidateRevisionID != input.SpanCandidateRevisionID ||
				candidate.SpanCandidateRevisionHash != input.SpanCandidateRevisionHash {
				return errors.New("SceneFact candidate source identity drifted")
			}
		case "resolve_identities":
			var input IdentityResolutionInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil || input.Validate() != nil {
				return errors.New("invalid IdentityResolution input")
			}
			allowedReuseIdentityKeys := make(map[string]struct{}, len(input.AllowedReuseIdentityKeys))
			for _, key := range input.AllowedReuseIdentityKeys {
				allowedReuseIdentityKeys[key] = struct{}{}
			}
			if ValidateIdentityResolutionCandidate(
				value.Candidate,
				input.SceneFactCandidate,
				allowedReuseIdentityKeys,
			) != nil {
				return errors.New("invalid accepted IdentityResolution candidate")
			}
			var candidate IdentityResolutionCandidate
			if decodeStrict(value.Candidate, &candidate) != nil ||
				candidate.SourceVersionID != input.SourceVersionID ||
				candidate.SceneFactCandidateRevisionID != input.SceneFactCandidateRevisionID ||
				candidate.SceneFactCandidateRevisionHash != input.SceneFactCandidateRevisionHash {
				return errors.New("IdentityResolution candidate source identity drifted")
			}
		case "review_candidate":
			var input StructureIdentityReviewInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil || input.Validate() != nil ||
				ValidateStructureIdentityReviewCandidate(value.Candidate, input) != nil {
				return errors.New("invalid accepted structure identity review candidate")
			}
		case "derive_production_entities":
			var input ProductionEntityDerivationInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil || input.Validate() != nil ||
				ValidateProductionEntityFragmentCandidate(value.Candidate, input) != nil {
				return errors.New("invalid accepted Production Entity candidate")
			}
		case "bind_scene_occurrences":
			var input SceneOccurrenceBindingInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil || input.Validate() != nil ||
				ValidateSceneBindingFragmentCandidate(value.Candidate, input) != nil {
				return errors.New("invalid accepted Scene binding candidate")
			}
		case "reconcile_interaction_continuity":
			var input InteractionContinuityInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil || input.Validate() != nil ||
				ValidateInteractionContinuityCandidate(value.Candidate, input) != nil {
				return errors.New("invalid accepted Interaction/Continuity candidate")
			}
		}
		if invocation.Payload.ProductionRepair != nil && ValidateProductionWorldRepairCandidate(
			*invocation.Payload.ProductionRepair,
			invocation.Payload.Variant.StageKey,
			value.Candidate,
		) != nil {
			return errors.New("accepted Production World repair changed content outside its authorized closure")
		}
	case "rejected", "outcome_unknown":
		expectedRetry := "never"
		if value.Status == "outcome_unknown" {
			expectedRetry = "same_release"
		}
		if value.OutputHash != nil || len(value.Candidate) != 0 && string(value.Candidate) != "null" ||
			value.Error == nil || value.Error.RetryClass != expectedRetry ||
			strings.TrimSpace(value.Error.Code) == "" || strings.TrimSpace(value.Error.SafeSummary) == "" {
			return errors.New("failed Scene Analysis result has invalid semantics")
		}
	default:
		return errors.New("invalid Scene Analysis result status")
	}
	return nil
}

func (value SceneAnalysisAttemptResult) ComputeResultHash() (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &root); err != nil {
		return "", err
	}
	delete(root, "result_hash")
	material, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(material)
}

type SourceEvidenceSpan struct {
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
	TextHash    string `json:"text_hash"`
	ExactAnchor string `json:"exact_anchor"`
}

func (value SourceEvidenceSpan) Validate(text []rune) error {
	if value.SourceStart < 0 || value.SourceEnd <= value.SourceStart || value.SourceEnd > len(text) ||
		!hashPattern.MatchString(value.TextHash) {
		return errors.New("invalid Scene Analysis Evidence range")
	}
	anchor := string(text[value.SourceStart:value.SourceEnd])
	if anchor != value.ExactAnchor || hashUTF8(anchor) != value.TextHash {
		return errors.New("scene Analysis Evidence does not match source")
	}
	return nil
}

type CandidateReviewIssue struct {
	IssueKey string               `json:"issue_key"`
	Code     string               `json:"code"`
	Severity string               `json:"severity"`
	Scope    string               `json:"scope"`
	Summary  string               `json:"summary"`
	Evidence []SourceEvidenceSpan `json:"evidence"`
}

type ScriptEpisodeSpan struct {
	TemporaryEpisodeID string              `json:"temporary_episode_id"`
	Position           int                 `json:"position"`
	CodepointStart     int                 `json:"codepoint_start"`
	CodepointEnd       int                 `json:"codepoint_end"`
	Heading            *string             `json:"heading"`
	Evidence           *SourceEvidenceSpan `json:"evidence"`
	SceneSpanIDs       []string            `json:"scene_span_ids"`
}

type ScriptSceneSpan struct {
	TemporarySpanID string             `json:"temporary_span_id"`
	EpisodeSpanID   string             `json:"episode_span_id"`
	Kind            string             `json:"kind"`
	CodepointStart  int                `json:"codepoint_start"`
	CodepointEnd    int                `json:"codepoint_end"`
	Heading         string             `json:"heading"`
	Evidence        SourceEvidenceSpan `json:"evidence"`
}

type ScriptSpanCoverageProof struct {
	SourceHash        string `json:"source_hash"`
	CodepointStart    int    `json:"codepoint_start"`
	CodepointEnd      int    `json:"codepoint_end"`
	CoveredCodepoints int    `json:"covered_codepoints"`
}

type ScriptSpanCandidate struct {
	SourceVersionID string                  `json:"source_version_id"`
	SourceHash      string                  `json:"source_hash"`
	CodepointCount  int                     `json:"codepoint_count"`
	Coverage        ScriptSpanCoverageProof `json:"coverage"`
	Episodes        []ScriptEpisodeSpan     `json:"episodes"`
	Spans           []ScriptSceneSpan       `json:"spans"`
	ReviewIssues    []CandidateReviewIssue  `json:"review_issues"`
}

func ValidateScriptSpanCandidate(raw json.RawMessage, text string) error {
	var value ScriptSpanCandidate
	if decodeStrict(raw, &value) != nil || value.SourceHash != hashUTF8(text) ||
		value.CodepointCount != utf8.RuneCountInString(text) || len(value.Episodes) == 0 || len(value.Spans) == 0 ||
		value.Coverage.SourceHash != value.SourceHash || value.Coverage.CodepointStart != 0 ||
		value.Coverage.CodepointEnd != value.CodepointCount ||
		value.Coverage.CoveredCodepoints != value.CodepointCount {
		return errors.New("invalid Scene Analysis ScriptSpan candidate")
	}
	if _, err := uuid.Parse(value.SourceVersionID); err != nil {
		return errors.New("invalid Scene Analysis ScriptSpan source identity")
	}
	runes := []rune(text)
	previousEpisodeEnd := 0
	episodeBounds := make(map[string][2]int, len(value.Episodes))
	expectedSceneKeys := make(map[string][]string, len(value.Episodes))
	for index, episode := range value.Episodes {
		if !strings.HasPrefix(episode.TemporaryEpisodeID, "episode_") || episode.Position != index+1 ||
			episode.CodepointStart != previousEpisodeEnd || episode.CodepointEnd <= episode.CodepointStart ||
			episode.CodepointEnd > len(runes) || len(episode.SceneSpanIDs) == 0 ||
			(episode.Heading == nil) != (episode.Evidence == nil) {
			return errors.New("EpisodeSpan coverage is invalid")
		}
		if _, duplicate := episodeBounds[episode.TemporaryEpisodeID]; duplicate {
			return errors.New("EpisodeSpan key is duplicated")
		}
		if episode.Heading != nil && (strings.TrimSpace(*episode.Heading) == "" ||
			episode.Evidence.SourceStart < episode.CodepointStart ||
			episode.Evidence.SourceEnd > episode.CodepointEnd ||
			episode.Evidence.Validate(runes) != nil || *episode.Heading != episode.Evidence.ExactAnchor) {
			return errors.New("EpisodeSpan heading evidence is invalid")
		}
		seenSceneKeys := make(map[string]struct{}, len(episode.SceneSpanIDs))
		for _, key := range episode.SceneSpanIDs {
			if !strings.HasPrefix(key, "span_") {
				return errors.New("EpisodeSpan scene key is invalid")
			}
			if _, duplicate := seenSceneKeys[key]; duplicate {
				return errors.New("EpisodeSpan scene key is duplicated")
			}
			seenSceneKeys[key] = struct{}{}
		}
		episodeBounds[episode.TemporaryEpisodeID] = [2]int{episode.CodepointStart, episode.CodepointEnd}
		expectedSceneKeys[episode.TemporaryEpisodeID] = episode.SceneSpanIDs
		previousEpisodeEnd = episode.CodepointEnd
	}
	if previousEpisodeEnd != len(runes) {
		return errors.New("EpisodeSpan source coverage is incomplete")
	}
	previousEnd := 0
	keys := map[string]struct{}{}
	suppliedSceneKeys := make(map[string][]string, len(expectedSceneKeys))
	for _, span := range value.Spans {
		bounds, episodeExists := episodeBounds[span.EpisodeSpanID]
		if strings.TrimSpace(span.TemporarySpanID) == "" || span.Kind != "scene" || !episodeExists ||
			span.CodepointStart != previousEnd || span.CodepointEnd <= span.CodepointStart ||
			span.CodepointEnd > len(runes) || strings.TrimSpace(span.Heading) == "" ||
			span.CodepointStart < bounds[0] || span.CodepointEnd > bounds[1] ||
			span.Evidence.SourceStart < span.CodepointStart ||
			span.Evidence.SourceEnd > span.CodepointEnd || span.Evidence.Validate(runes) != nil {
			return errors.New("ScriptSpan coverage is invalid")
		}
		if _, exists := keys[span.TemporarySpanID]; exists {
			return errors.New("ScriptSpan key is duplicated")
		}
		keys[span.TemporarySpanID] = struct{}{}
		suppliedSceneKeys[span.EpisodeSpanID] = append(suppliedSceneKeys[span.EpisodeSpanID], span.TemporarySpanID)
		previousEnd = span.CodepointEnd
	}
	if previousEnd != len(runes) {
		return errors.New("ScriptSpan source coverage is incomplete")
	}
	for episodeKey, expected := range expectedSceneKeys {
		if !slices.Equal(suppliedSceneKeys[episodeKey], expected) {
			return errors.New("ScriptSpan episode membership is incomplete")
		}
	}
	return nil
}

type GroundedAction struct {
	Text     string             `json:"text"`
	Evidence SourceEvidenceSpan `json:"evidence"`
}

type GroundedDialogue struct {
	SpeakerMention string             `json:"speaker_mention"`
	Text           string             `json:"text"`
	Evidence       SourceEvidenceSpan `json:"evidence"`
}

type RawEntityMention struct {
	Text           string             `json:"text"`
	OccurrenceRole string             `json:"occurrence_role"`
	Evidence       SourceEvidenceSpan `json:"evidence"`
}

type GroundedSceneAttribute struct {
	Text     string             `json:"text"`
	Evidence SourceEvidenceSpan `json:"evidence"`
}

type SceneFact struct {
	TemporarySceneID     string                  `json:"temporary_scene_id"`
	SpanID               string                  `json:"span_id"`
	SourceStart          int                     `json:"source_start"`
	SourceEnd            int                     `json:"source_end"`
	Location             *GroundedSceneAttribute `json:"location"`
	Time                 *GroundedSceneAttribute `json:"time"`
	Actions              []GroundedAction        `json:"actions"`
	Dialogues            []GroundedDialogue      `json:"dialogues"`
	RawCharacterMentions []RawEntityMention      `json:"raw_character_mentions"`
	RawPropMentions      []RawEntityMention      `json:"raw_prop_mentions"`
}

type SceneFactCandidate struct {
	SourceVersionID           string                 `json:"source_version_id"`
	SourceHash                string                 `json:"source_hash"`
	SpanCandidateRevisionID   string                 `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash string                 `json:"span_candidate_revision_hash"`
	Scenes                    []SceneFact            `json:"scenes"`
	ReviewIssues              []CandidateReviewIssue `json:"review_issues"`
}

func ValidateSceneFactCandidate(raw json.RawMessage, text string, spanRaw json.RawMessage) error {
	var value SceneFactCandidate
	var spans ScriptSpanCandidate
	if decodeStrict(raw, &value) != nil || decodeStrict(spanRaw, &spans) != nil ||
		ValidateScriptSpanCandidate(spanRaw, text) != nil || value.SourceHash != hashUTF8(text) ||
		!hashPattern.MatchString(value.SpanCandidateRevisionHash) || len(value.Scenes) != len(spans.Spans) {
		return errors.New("invalid Scene Analysis SceneFact candidate")
	}
	for _, identifier := range []string{value.SourceVersionID, value.SpanCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Scene Analysis SceneFact identity")
		}
	}
	expected := make(map[string][2]int, len(spans.Spans))
	for _, span := range spans.Spans {
		expected[span.TemporarySpanID] = [2]int{span.CodepointStart, span.CodepointEnd}
	}
	runes := []rune(text)
	sceneKeys := map[string]struct{}{}
	for _, scene := range value.Scenes {
		bounds, exists := expected[scene.SpanID]
		if !exists || bounds != [2]int{scene.SourceStart, scene.SourceEnd} ||
			strings.TrimSpace(scene.TemporarySceneID) == "" {
			return errors.New("SceneFact does not map exactly to ScriptSpan")
		}
		if _, duplicate := sceneKeys[scene.TemporarySceneID]; duplicate {
			return errors.New("SceneFact key is duplicated")
		}
		sceneKeys[scene.TemporarySceneID] = struct{}{}
		evidence := make([]SourceEvidenceSpan, 0, len(scene.Actions)+len(scene.Dialogues)+len(scene.RawCharacterMentions)+len(scene.RawPropMentions)+2)
		if scene.Location != nil {
			if strings.TrimSpace(scene.Location.Text) == "" {
				return errors.New("SceneFact location is empty")
			}
			evidence = append(evidence, scene.Location.Evidence)
		}
		if scene.Time != nil {
			if strings.TrimSpace(scene.Time.Text) == "" {
				return errors.New("SceneFact time is empty")
			}
			evidence = append(evidence, scene.Time.Evidence)
		}
		for _, item := range scene.Actions {
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.Dialogues {
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.RawCharacterMentions {
			if strings.TrimSpace(item.Text) == "" || !validOccurrenceRole(item.OccurrenceRole) {
				return errors.New("invalid SceneFact character mention")
			}
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.RawPropMentions {
			if strings.TrimSpace(item.Text) == "" || !validOccurrenceRole(item.OccurrenceRole) {
				return errors.New("invalid SceneFact prop mention")
			}
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range evidence {
			if item.SourceStart < scene.SourceStart || item.SourceEnd > scene.SourceEnd ||
				item.Validate(runes) != nil {
				return errors.New("SceneFact evidence is invalid")
			}
		}
		delete(expected, scene.SpanID)
	}
	if len(expected) != 0 {
		return errors.New("SceneFact span coverage is incomplete")
	}
	return nil
}

var temporaryIdentityKeyPattern = regexp.MustCompile(`^identity_(character|location|prop)_[a-z0-9_]{1,80}$`)

type IdentityMentionRef struct {
	Kind             string `json:"kind"`
	OccurrenceRole   string `json:"occurrence_role"`
	TemporarySceneID string `json:"temporary_scene_id"`
	SourceStart      int    `json:"source_start"`
	SourceEnd        int    `json:"source_end"`
	TextHash         string `json:"text_hash"`
	ExactAnchor      string `json:"exact_anchor"`
}

type IdentityCluster struct {
	TemporaryIdentityKey  string               `json:"temporary_identity_key"`
	Kind                  string               `json:"kind"`
	Resolution            string               `json:"resolution"`
	ReuseIdentityKey      *string              `json:"reuse_identity_key"`
	CanonicalName         string               `json:"canonical_name"`
	Aliases               []string             `json:"aliases"`
	MentionRefs           []IdentityMentionRef `json:"mention_refs"`
	SupportingEvidence    []SourceEvidenceSpan `json:"supporting_evidence"`
	ContradictingEvidence []SourceEvidenceSpan `json:"contradicting_evidence"`
	ConfidenceBasisPoints int                  `json:"confidence_basis_points"`
	Rationale             string               `json:"rationale"`
}

type AmbiguousIdentityMention struct {
	MentionRef            IdentityMentionRef `json:"mention_ref"`
	CandidateIdentityKeys []string           `json:"candidate_identity_keys"`
	ConfidenceBasisPoints int                `json:"confidence_basis_points"`
	Rationale             string             `json:"rationale"`
}

type RejectedIdentityMention struct {
	MentionRef IdentityMentionRef `json:"mention_ref"`
	Rationale  string             `json:"rationale"`
}

type IdentityResolutionCoverage struct {
	MentionCount        int    `json:"mention_count"`
	ResolvedCount       int    `json:"resolved_count"`
	AmbiguousCount      int    `json:"ambiguous_count"`
	RejectedCount       int    `json:"rejected_count"`
	MentionUniverseHash string `json:"mention_universe_hash"`
}

type IdentityResolutionCandidate struct {
	SourceVersionID                string                     `json:"source_version_id"`
	SourceHash                     string                     `json:"source_hash"`
	SceneFactCandidateRevisionID   string                     `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash string                     `json:"scene_fact_candidate_revision_hash"`
	ResolvedClusters               []IdentityCluster          `json:"resolved_clusters"`
	AmbiguousMentions              []AmbiguousIdentityMention `json:"ambiguous_mentions"`
	RejectedMentions               []RejectedIdentityMention  `json:"rejected_mentions"`
	Coverage                       IdentityResolutionCoverage `json:"coverage"`
	ReviewIssues                   []CandidateReviewIssue     `json:"review_issues"`
}

func ValidateIdentityResolutionCandidate(
	raw json.RawMessage,
	sceneFactRaw json.RawMessage,
	allowedReuseIdentityKeys map[string]struct{},
) error {
	var value IdentityResolutionCandidate
	var sceneFacts SceneFactCandidate
	if decodeStrict(raw, &value) != nil || decodeStrict(sceneFactRaw, &sceneFacts) != nil ||
		value.SourceVersionID != sceneFacts.SourceVersionID || value.SourceHash != sceneFacts.SourceHash ||
		!hashPattern.MatchString(value.SceneFactCandidateRevisionHash) || value.ResolvedClusters == nil ||
		value.AmbiguousMentions == nil || value.RejectedMentions == nil || value.ReviewIssues == nil {
		return errors.New("invalid IdentityResolution candidate")
	}
	for _, identifier := range []string{value.SourceVersionID, value.SceneFactCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid IdentityResolution source identity")
		}
	}

	expected := make(map[string]IdentityMentionRef)
	sourceEvidence := make(map[string]struct{})
	for _, scene := range sceneFacts.Scenes {
		grounded := make([]SourceEvidenceSpan, 0, len(scene.Actions)+len(scene.Dialogues)+len(scene.RawCharacterMentions)+len(scene.RawPropMentions)+2)
		if scene.Location != nil {
			grounded = append(grounded, scene.Location.Evidence)
		}
		if scene.Time != nil {
			grounded = append(grounded, scene.Time.Evidence)
		}
		for _, action := range scene.Actions {
			grounded = append(grounded, action.Evidence)
		}
		for _, dialogue := range scene.Dialogues {
			grounded = append(grounded, dialogue.Evidence)
		}
		for _, mention := range scene.RawCharacterMentions {
			grounded = append(grounded, mention.Evidence)
		}
		for _, mention := range scene.RawPropMentions {
			grounded = append(grounded, mention.Evidence)
		}
		for _, evidence := range grounded {
			sourceEvidence[sourceEvidenceKey(evidence)] = struct{}{}
		}
		if scene.Location != nil {
			ref := IdentityMentionRef{
				Kind: "location", OccurrenceRole: "actual", TemporarySceneID: scene.TemporarySceneID,
				SourceStart: scene.Location.Evidence.SourceStart, SourceEnd: scene.Location.Evidence.SourceEnd,
				TextHash: scene.Location.Evidence.TextHash, ExactAnchor: scene.Location.Evidence.ExactAnchor,
			}
			key := identityMentionKey(ref)
			if _, duplicate := expected[key]; duplicate {
				return errors.New("SceneFact raw mention is duplicated")
			}
			expected[key] = ref
		}
		for _, group := range []struct {
			kind     string
			mentions []RawEntityMention
		}{{"character", scene.RawCharacterMentions}, {"prop", scene.RawPropMentions}} {
			for _, mention := range group.mentions {
				ref := IdentityMentionRef{
					Kind: group.kind, OccurrenceRole: mention.OccurrenceRole, TemporarySceneID: scene.TemporarySceneID,
					SourceStart: mention.Evidence.SourceStart, SourceEnd: mention.Evidence.SourceEnd,
					TextHash: mention.Evidence.TextHash, ExactAnchor: mention.Evidence.ExactAnchor,
				}
				key := identityMentionKey(ref)
				if _, duplicate := expected[key]; duplicate {
					return errors.New("SceneFact raw mention is duplicated")
				}
				expected[key] = ref
			}
		}
	}

	supplied := make([]IdentityMentionRef, 0, len(expected))
	identityKinds := make(map[string]string, len(value.ResolvedClusters))
	resolvedCount := 0
	for _, cluster := range value.ResolvedClusters {
		if !temporaryIdentityKeyPattern.MatchString(cluster.TemporaryIdentityKey) ||
			!strings.HasPrefix(cluster.TemporaryIdentityKey, "identity_"+cluster.Kind+"_") ||
			(cluster.Kind != "character" && cluster.Kind != "location" && cluster.Kind != "prop") ||
			len(cluster.SupportingEvidence) == 0 || cluster.ContradictingEvidence == nil ||
			cluster.ConfidenceBasisPoints < 0 || cluster.ConfidenceBasisPoints > 10_000 || strings.TrimSpace(cluster.Rationale) == "" ||
			strings.TrimSpace(cluster.CanonicalName) == "" || len(cluster.Aliases) == 0 {
			return errors.New("invalid identity cluster")
		}
		if _, duplicate := identityKinds[cluster.TemporaryIdentityKey]; duplicate {
			return errors.New("identity cluster key is duplicated")
		}
		identityKinds[cluster.TemporaryIdentityKey] = cluster.Kind
		aliasSet := make(map[string]struct{}, len(cluster.Aliases))
		canonicalAlias := false
		for _, alias := range cluster.Aliases {
			if strings.TrimSpace(alias) == "" {
				return errors.New("identity alias is empty")
			}
			if _, duplicate := aliasSet[alias]; duplicate {
				return errors.New("identity alias is duplicated")
			}
			aliasSet[alias] = struct{}{}
			canonicalAlias = canonicalAlias || alias == cluster.CanonicalName
		}
		if !canonicalAlias {
			return errors.New("identity aliases omit the canonical name")
		}
		switch cluster.Resolution {
		case "new":
			if cluster.ReuseIdentityKey != nil {
				return errors.New("new identity carries a reuse key")
			}
		case "reuse":
			if cluster.ReuseIdentityKey == nil || strings.TrimSpace(*cluster.ReuseIdentityKey) == "" {
				return errors.New("reused identity omits its key")
			}
			if _, allowed := allowedReuseIdentityKeys[*cluster.ReuseIdentityKey]; !allowed {
				return errors.New("identity reuse key is outside the input allowlist")
			}
		default:
			return errors.New("invalid identity resolution")
		}
		for _, evidence := range append(cluster.SupportingEvidence, cluster.ContradictingEvidence...) {
			if _, exists := sourceEvidence[sourceEvidenceKey(evidence)]; !exists {
				return errors.New("identity evidence is not present in the frozen SceneFacts")
			}
		}
		for _, ref := range cluster.MentionRefs {
			if ref.Kind != cluster.Kind {
				return errors.New("identity cluster mixes mention kinds")
			}
			supplied = append(supplied, ref)
		}
		resolvedCount += len(cluster.MentionRefs)
	}
	for _, ambiguous := range value.AmbiguousMentions {
		if len(ambiguous.CandidateIdentityKeys) == 0 || ambiguous.ConfidenceBasisPoints < 0 ||
			ambiguous.ConfidenceBasisPoints > 10_000 || strings.TrimSpace(ambiguous.Rationale) == "" {
			return errors.New("invalid ambiguous identity mention")
		}
		seenCandidates := make(map[string]struct{}, len(ambiguous.CandidateIdentityKeys))
		for _, key := range ambiguous.CandidateIdentityKeys {
			kind, exists := identityKinds[key]
			if !exists {
				return errors.New("ambiguous mention references an unknown identity cluster")
			}
			if kind != ambiguous.MentionRef.Kind {
				return errors.New("ambiguous mention references a different identity kind")
			}
			if _, duplicate := seenCandidates[key]; duplicate {
				return errors.New("ambiguous identity candidate is duplicated")
			}
			seenCandidates[key] = struct{}{}
		}
		supplied = append(supplied, ambiguous.MentionRef)
	}
	for _, rejected := range value.RejectedMentions {
		if strings.TrimSpace(rejected.Rationale) == "" {
			return errors.New("invalid rejected identity mention")
		}
		supplied = append(supplied, rejected.MentionRef)
	}

	seenMentions := make(map[string]struct{}, len(supplied))
	for _, ref := range supplied {
		key := identityMentionKey(ref)
		expectedRef, exists := expected[key]
		if !exists || expectedRef != ref {
			return errors.New("identity mention drifted from its SceneFact evidence")
		}
		if _, duplicate := seenMentions[key]; duplicate {
			return errors.New("identity mention belongs to more than one partition")
		}
		seenMentions[key] = struct{}{}
	}
	if len(seenMentions) != len(expected) {
		return errors.New("identity candidate does not partition every raw mention")
	}

	orderedKeys := make([]string, 0, len(expected))
	for key := range expected {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)
	orderedUniverse := make([]IdentityMentionRef, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		orderedUniverse = append(orderedUniverse, expected[key])
	}
	encodedUniverse, err := json.Marshal(orderedUniverse)
	if err != nil {
		return err
	}
	universeHash, err := ProductionCanonicalHash(encodedUniverse)
	if err != nil {
		return err
	}
	coverage := value.Coverage
	if coverage.MentionCount != len(expected) || coverage.ResolvedCount != resolvedCount ||
		coverage.AmbiguousCount != len(value.AmbiguousMentions) ||
		coverage.RejectedCount != len(value.RejectedMentions) ||
		coverage.ResolvedCount+coverage.AmbiguousCount+coverage.RejectedCount != coverage.MentionCount ||
		coverage.MentionUniverseHash != universeHash {
		return errors.New("identity mention coverage proof is invalid")
	}
	return nil
}

type StructureIdentityReviewSuggestion struct {
	IssueKey   string   `json:"issue_key"`
	Action     string   `json:"action"`
	TargetKeys []string `json:"target_keys"`
	Rationale  string   `json:"rationale"`
}

type StructureIdentityReviewCandidate struct {
	ProfileKey                     string                              `json:"profile_key"`
	SourceVersionID                string                              `json:"source_version_id"`
	SourceHash                     string                              `json:"source_hash"`
	SpanCandidateRevisionID        string                              `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash      string                              `json:"span_candidate_revision_hash"`
	SceneFactCandidateRevisionID   string                              `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash string                              `json:"scene_fact_candidate_revision_hash"`
	IdentityCandidateRevisionID    string                              `json:"identity_candidate_revision_id"`
	IdentityCandidateRevisionHash  string                              `json:"identity_candidate_revision_hash"`
	ReviewIssues                   []CandidateReviewIssue              `json:"review_issues"`
	Suggestions                    []StructureIdentityReviewSuggestion `json:"suggestions"`
}

func ValidateStructureIdentityReviewCandidate(
	raw json.RawMessage,
	input StructureIdentityReviewInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	var value StructureIdentityReviewCandidate
	if decodeStrict(raw, &value) != nil || value.ProfileKey != "structure_identity" ||
		value.ReviewIssues == nil || value.Suggestions == nil ||
		value.SourceVersionID != input.SourceVersionID || value.SourceHash != input.SourceHash ||
		value.SpanCandidateRevisionID != input.SpanCandidateRevisionID ||
		value.SpanCandidateRevisionHash != input.SpanCandidateRevisionHash ||
		value.SceneFactCandidateRevisionID != input.SceneFactCandidateRevisionID ||
		value.SceneFactCandidateRevisionHash != input.SceneFactCandidateRevisionHash ||
		value.IdentityCandidateRevisionID != input.IdentityCandidateRevisionID ||
		value.IdentityCandidateRevisionHash != input.IdentityCandidateRevisionHash {
		return errors.New("structure identity review candidate does not match its frozen target")
	}
	for _, identifier := range []string{
		value.SourceVersionID, value.SpanCandidateRevisionID,
		value.SceneFactCandidateRevisionID, value.IdentityCandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity review candidate identity")
		}
	}
	deterministic := make(map[string]CandidateReviewIssue, len(input.DeterministicIssues))
	for _, issue := range input.DeterministicIssues {
		deterministic[issue.IssueKey] = issue
	}
	issues := make(map[string]struct{}, len(value.ReviewIssues))
	previous := ""
	text := []rune(input.NormalizedText)
	for _, issue := range value.ReviewIssues {
		frozen, isDeterministic := deterministic[issue.IssueKey]
		if issue.IssueKey <= previous || validateCandidateReviewIssue(issue, text, isDeterministic) != nil {
			return errors.New("structure identity review issues must be unique and sorted")
		}
		if isDeterministic {
			if !reflect.DeepEqual(frozen, issue) {
				return errors.New("structure identity review changed a deterministic issue")
			}
			delete(deterministic, issue.IssueKey)
		} else if len(issue.Evidence) == 0 {
			return errors.New("semantic review issue must carry source evidence")
		}
		issues[issue.IssueKey] = struct{}{}
		previous = issue.IssueKey
	}
	if len(deterministic) != 0 {
		return errors.New("structure identity review omitted a deterministic issue")
	}
	allowedActions := map[string]struct{}{
		"inspect_source": {}, "adjust_episode_boundary": {}, "adjust_scene_boundary": {},
		"separate_identity": {}, "merge_identity": {}, "resolve_mention": {}, "reject_mention": {},
	}
	previous = ""
	for _, suggestion := range value.Suggestions {
		if suggestion.IssueKey <= previous || strings.TrimSpace(suggestion.Rationale) == "" ||
			len(suggestion.TargetKeys) == 0 || !slices.IsSorted(suggestion.TargetKeys) {
			return errors.New("structure identity review suggestions must be unique and sorted")
		}
		if _, exists := issues[suggestion.IssueKey]; !exists {
			return errors.New("review suggestion references an unknown issue")
		}
		if _, allowed := allowedActions[suggestion.Action]; !allowed {
			return errors.New("review suggestion action is invalid")
		}
		previousTarget := ""
		for _, target := range suggestion.TargetKeys {
			if strings.TrimSpace(target) == "" || target == previousTarget {
				return errors.New("review suggestion targets must be sorted and unique")
			}
			previousTarget = target
		}
		previous = suggestion.IssueKey
	}
	return nil
}

var candidateReviewIssueKeyPattern = regexp.MustCompile(`^issue_[a-z0-9_]{1,80}$`)
var candidateReviewCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,80}$`)

func validateCandidateReviewIssue(
	value CandidateReviewIssue,
	text []rune,
	allowEmptyEvidence bool,
) error {
	if !candidateReviewIssueKeyPattern.MatchString(value.IssueKey) ||
		!candidateReviewCodePattern.MatchString(value.Code) ||
		(value.Severity != "warning" && value.Severity != "blocking") ||
		strings.TrimSpace(value.Scope) == "" || strings.TrimSpace(value.Summary) == "" ||
		value.Evidence == nil || (!allowEmptyEvidence && len(value.Evidence) == 0) {
		return errors.New("invalid candidate review issue")
	}
	for _, evidence := range value.Evidence {
		if evidence.Validate(text) != nil {
			return errors.New("invalid candidate review issue Evidence")
		}
	}
	return nil
}

func identityMentionKey(value IdentityMentionRef) string {
	return fmt.Sprintf(
		"%s\x00%s\x00%s\x00%020d\x00%020d\x00%s\x00%s",
		value.Kind, value.OccurrenceRole, value.TemporarySceneID, value.SourceStart, value.SourceEnd,
		value.TextHash, value.ExactAnchor,
	)
}

func validOccurrenceRole(value string) bool {
	return value == "actual" || value == "mentioned_only"
}

func sourceEvidenceKey(value SourceEvidenceSpan) string {
	return fmt.Sprintf(
		"%020d\x00%020d\x00%s\x00%s",
		value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor,
	)
}

func hashUTF8(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
