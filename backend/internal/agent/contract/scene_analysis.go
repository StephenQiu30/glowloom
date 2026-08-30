package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	SceneAnalysisWireSchema      = "storygraph-scene-analysis-wire"
	SceneAnalysisSkillBundleHash = "b395c382be96a37895a5404fc828bf5dbb163c201271b979c3d9d9a7b4b7c66a"
)

type SceneAnalysisStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value SceneAnalysisStageVariant) Validate() error {
	expected := map[string]string{
		"propose_script_spans": "script-span-candidate",
		"extract_scene_facts":  "scene-fact-candidate",
	}
	if value.ProfileKey != "default" || value.LaneKey != "primary" ||
		expected[value.StageKey] == "" || value.OutputSchemaVersion != expected[value.StageKey] {
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
	if value.OwnerKind != "production/script-source" || strings.TrimSpace(value.LogicalID) == "" ||
		value.Revision < 1 || !hashPattern.MatchString(value.ContentHash) || value.CreatedAt.IsZero() {
		return errors.New("invalid script source version identity")
	}
	if _, err := uuid.Parse(value.VersionID); err != nil {
		return errors.New("invalid script source version identity")
	}
	return nil
}

type ScriptSpanRevisionIdentity struct {
	StageKey              string `json:"stage_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
}

func (value ScriptSpanRevisionIdentity) Validate() error {
	if value.StageKey != "propose_script_spans" || strings.TrimSpace(value.ShardKey) == "" ||
		!hashPattern.MatchString(value.CandidateRevisionHash) ||
		!hashPattern.MatchString(value.SourceResultHash) {
		return errors.New("invalid script span revision identity")
	}
	for _, identifier := range []string{value.CandidateRevisionID, value.SourceInvocationID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid script span revision identity")
		}
	}
	return nil
}

type SceneAnalysisReleaseIdentity struct {
	ReleaseID        string `json:"release_id"`
	DefinitionHash   string `json:"definition_hash"`
	BundleHash       string `json:"bundle_hash"`
	AgentImageDigest string `json:"agent_image_digest"`
}

func (value SceneAnalysisReleaseIdentity) Validate() error {
	if _, err := uuid.Parse(value.ReleaseID); err != nil {
		return errors.New("invalid Scene Analysis release identity")
	}
	if !hashPattern.MatchString(value.DefinitionHash) ||
		value.BundleHash != SceneAnalysisSkillBundleHash ||
		!strings.HasPrefix(value.AgentImageDigest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(value.AgentImageDigest, "sha256:")) {
		return errors.New("invalid Scene Analysis release identity")
	}
	return nil
}

type SceneAnalysisControlProof struct {
	RecordID    string `json:"record_id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash"`
}

func (value SceneAnalysisControlProof) Validate() error {
	if _, err := uuid.Parse(value.RecordID); err != nil {
		return errors.New("invalid Scene Analysis Control proof")
	}
	if value.Revision < 1 || value.Status != "approved" || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid Scene Analysis Control proof")
	}
	return nil
}

type SceneAnalysisExecutionBudget struct {
	MaxModelCalls       int `json:"max_model_calls"`
	MaxExecutionSeconds int `json:"max_execution_seconds"`
	MaxOutputBytes      int `json:"max_output_bytes"`
}

func (value SceneAnalysisExecutionBudget) Validate() error {
	if value.MaxModelCalls < 1 || value.MaxModelCalls > 2 ||
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
}

func (value SceneAnalysisScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Scene Analysis scope")
		}
	}
	if value.EpisodeID != nil {
		if _, err := uuid.Parse(*value.EpisodeID); err != nil {
			return errors.New("invalid Scene Analysis scope")
		}
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
	SourceVersionID      string `json:"source_version_id"`
	SourceHash           string `json:"source_hash"`
	NormalizedText       string `json:"normalized_text"`
	CodepointCount       int    `json:"codepoint_count"`
	NewlineNormalization string `json:"newline_normalization"`
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
	return nil
}

type SceneFactExtractionInput struct {
	SourceVersionID           string          `json:"source_version_id"`
	SourceHash                string          `json:"source_hash"`
	NormalizedText            string          `json:"normalized_text"`
	SpanCandidateRevisionID   string          `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash string          `json:"span_candidate_revision_hash"`
	SpanCandidate             json.RawMessage `json:"span_candidate"`
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
	Variant            SceneAnalysisStageVariant     `json:"variant"`
	Scope              SceneAnalysisScope            `json:"scope"`
	SourceRefs         []ScriptSourceVersionIdentity `json:"source_refs"`
	UpstreamCandidates []ScriptSpanRevisionIdentity  `json:"upstream_candidates"`
	Shard              SceneAnalysisShard            `json:"shard"`
	StageInput         json.RawMessage               `json:"stage_input"`
}

func (value SceneAnalysisPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil || value.Shard.Validate() != nil ||
		len(value.SourceRefs) != 1 || value.SourceRefs[0].Validate() != nil || !jsonObject(value.StageInput) {
		return errors.New("invalid Scene Analysis payload")
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
		WireSchemaVersion: SceneAnalysisWireSchema, StageRelease: release,
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
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchema ||
		value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.Budget.Validate() != nil || value.Payload.Validate() != nil ||
		!hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Scene Analysis invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("Scene Analysis input hash mismatch")
	}
	return nil
}

func (value SceneAnalysisInvocation) ComputeInputHash() (string, error) {
	payload := value.Payload
	payload.SourceRefs = make([]ScriptSourceVersionIdentity, len(value.Payload.SourceRefs))
	copy(payload.SourceRefs, value.Payload.SourceRefs)
	payload.UpstreamCandidates = make(
		[]ScriptSpanRevisionIdentity,
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
	return CanonicalHash(encoded)
}

func (value SceneAnalysisInvocation) StageInstanceKey() string {
	material := "storygraph-scene-analysis-stage" + value.Payload.Variant.StageKey +
		value.Payload.Variant.ProfileKey + value.Payload.Variant.LaneKey +
		value.Payload.Variant.OutputSchemaVersion + value.Payload.Shard.ShardKey +
		value.Payload.Shard.ManifestHash + value.InputHash
	hash := sha256.Sum256([]byte(material))
	return hex.EncodeToString(hash[:])
}

type SceneAnalysisExecutionGrantClaims struct {
	InvocationID     string `json:"invocation_id"`
	AttemptID        string `json:"attempt_id"`
	InputHash        string `json:"input_hash"`
	StageReleaseID   string `json:"stage_release_id"`
	AgentImageDigest string `json:"agent_image_digest"`
	ExpiresAt        int64  `json:"expires_at"`
}

func (value SceneAnalysisExecutionGrantClaims) ValidateFor(invocation SceneAnalysisInvocation, nowUnix int64) error {
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.InputHash != invocation.InputHash || value.StageReleaseID != invocation.StageRelease.ReleaseID ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid Scene Analysis execution grant claims")
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
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	Variant           SceneAnalysisStageVariant    `json:"variant"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Status            string                       `json:"status"`
	CandidateType     string                       `json:"candidate_type"`
	Candidate         json.RawMessage              `json:"candidate"`
	InputHash         string                       `json:"input_hash"`
	OutputHash        *string                      `json:"output_hash"`
	Diagnostics       []SceneAnalysisDiagnostic    `json:"diagnostics"`
	DiagnosticHash    string                       `json:"diagnostic_hash"`
	CompletedAt       time.Time                    `json:"completed_at"`
	Executor          SceneAnalysisExecutor        `json:"executor"`
	Error             *SceneAnalysisResultError    `json:"error"`
}

func DecodeSceneAnalysisAttemptResult(raw []byte) (SceneAnalysisAttemptResult, error) {
	var value SceneAnalysisAttemptResult
	if err := decodeStrict(raw, &value); err != nil {
		return SceneAnalysisAttemptResult{}, err
	}
	return value, nil
}

func (value SceneAnalysisAttemptResult) ValidateFor(invocation SceneAnalysisInvocation) error {
	expectedCandidateType := map[string]string{
		"propose_script_spans": "script_span_candidate",
		"extract_scene_facts":  "scene_fact_candidate",
	}
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchema ||
		value.Variant != invocation.Payload.Variant || value.StageRelease != invocation.StageRelease ||
		value.Control != invocation.Control || value.InputHash != invocation.InputHash ||
		value.CandidateType != expectedCandidateType[invocation.Payload.Variant.StageKey] ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "text" ||
		value.Executor.RuntimeImageDigest != invocation.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "scene-analysis-harness" || strings.TrimSpace(value.Executor.Model) == "" {
		return errors.New("Scene Analysis result identity does not match invocation")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := CanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("Scene Analysis diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Scene Analysis result is incomplete")
		}
		outputHash, hashErr := CanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("Scene Analysis output hash mismatch")
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
		return errors.New("Scene Analysis Evidence does not match source")
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

type ScriptSceneSpan struct {
	TemporarySpanID string             `json:"temporary_span_id"`
	Kind            string             `json:"kind"`
	CodepointStart  int                `json:"codepoint_start"`
	CodepointEnd    int                `json:"codepoint_end"`
	Heading         string             `json:"heading"`
	Evidence        SourceEvidenceSpan `json:"evidence"`
}

type ScriptSpanCandidate struct {
	SourceVersionID string                 `json:"source_version_id"`
	SourceHash      string                 `json:"source_hash"`
	CodepointCount  int                    `json:"codepoint_count"`
	Spans           []ScriptSceneSpan      `json:"spans"`
	ReviewIssues    []CandidateReviewIssue `json:"review_issues"`
}

func ValidateScriptSpanCandidate(raw json.RawMessage, text string) error {
	var value ScriptSpanCandidate
	if decodeStrict(raw, &value) != nil || value.SourceHash != hashUTF8(text) ||
		value.CodepointCount != utf8.RuneCountInString(text) || len(value.Spans) == 0 {
		return errors.New("invalid Scene Analysis ScriptSpan candidate")
	}
	if _, err := uuid.Parse(value.SourceVersionID); err != nil {
		return errors.New("invalid Scene Analysis ScriptSpan source identity")
	}
	runes := []rune(text)
	previousEnd := 0
	keys := map[string]struct{}{}
	for _, span := range value.Spans {
		if strings.TrimSpace(span.TemporarySpanID) == "" || span.Kind != "scene" ||
			span.CodepointStart != previousEnd || span.CodepointEnd <= span.CodepointStart ||
			span.CodepointEnd > len(runes) || strings.TrimSpace(span.Heading) == "" ||
			span.Evidence.SourceStart < span.CodepointStart ||
			span.Evidence.SourceEnd > span.CodepointEnd || span.Evidence.Validate(runes) != nil {
			return errors.New("ScriptSpan coverage is invalid")
		}
		if _, exists := keys[span.TemporarySpanID]; exists {
			return errors.New("ScriptSpan key is duplicated")
		}
		keys[span.TemporarySpanID] = struct{}{}
		previousEnd = span.CodepointEnd
	}
	if previousEnd != len(runes) {
		return errors.New("ScriptSpan source coverage is incomplete")
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
	Text     string             `json:"text"`
	Evidence SourceEvidenceSpan `json:"evidence"`
}

type SceneFact struct {
	TemporarySceneID     string             `json:"temporary_scene_id"`
	SpanID               string             `json:"span_id"`
	SourceStart          int                `json:"source_start"`
	SourceEnd            int                `json:"source_end"`
	LocationText         *string            `json:"location_text"`
	TimeText             *string            `json:"time_text"`
	Actions              []GroundedAction   `json:"actions"`
	Dialogues            []GroundedDialogue `json:"dialogues"`
	RawCharacterMentions []RawEntityMention `json:"raw_character_mentions"`
	RawPropMentions      []RawEntityMention `json:"raw_prop_mentions"`
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
		evidence := make([]SourceEvidenceSpan, 0, len(scene.Actions)+len(scene.Dialogues)+len(scene.RawCharacterMentions)+len(scene.RawPropMentions))
		for _, item := range scene.Actions {
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.Dialogues {
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.RawCharacterMentions {
			evidence = append(evidence, item.Evidence)
		}
		for _, item := range scene.RawPropMentions {
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

func hashUTF8(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
