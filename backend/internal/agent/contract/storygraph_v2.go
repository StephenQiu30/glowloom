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
	StoryGraphV2WireSchemaVersion = "storygraph-stage-wire-v2"
	StoryGraphV2SkillBundleHash   = "13f294e3fc9a241d07af80547a792de04d5357270622f3a2361bab84580e5de6"
)

type StageVariantKeyV2 struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value StageVariantKeyV2) Validate() error {
	expected := map[string]string{
		"propose_script_spans": "script-span-candidate-v1",
		"extract_scene_facts":  "scene-fact-candidate-v1",
	}
	if value.ProfileKey != "default" || value.LaneKey != "primary" ||
		expected[value.StageKey] == "" || value.OutputSchemaVersion != expected[value.StageKey] {
		return errors.New("invalid StoryGraph v2 StageVariantKey")
	}
	return nil
}

type OwnerVersionIdentityV1 struct {
	OwnerKind   string    `json:"owner_kind"`
	LogicalID   string    `json:"logical_id"`
	VersionID   string    `json:"version_id"`
	Revision    int64     `json:"revision"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

func (value OwnerVersionIdentityV1) Validate() error {
	if value.OwnerKind != "production/script-source" || strings.TrimSpace(value.LogicalID) == "" ||
		value.Revision < 1 || !hashPattern.MatchString(value.ContentHash) || value.CreatedAt.IsZero() {
		return errors.New("invalid StoryGraph v2 OwnerVersion identity")
	}
	if _, err := uuid.Parse(value.VersionID); err != nil {
		return errors.New("invalid StoryGraph v2 OwnerVersion identity")
	}
	return nil
}

type UpstreamCandidateIdentityV2 struct {
	StageKey              string `json:"stage_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
}

func (value UpstreamCandidateIdentityV2) Validate() error {
	if value.StageKey != "propose_script_spans" || strings.TrimSpace(value.ShardKey) == "" ||
		!hashPattern.MatchString(value.CandidateRevisionHash) ||
		!hashPattern.MatchString(value.SourceResultHash) {
		return errors.New("invalid StoryGraph v2 upstream Candidate identity")
	}
	for _, identifier := range []string{value.CandidateRevisionID, value.SourceInvocationID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid StoryGraph v2 upstream Candidate identity")
		}
	}
	return nil
}

type StageReleaseIdentityV2 struct {
	ReleaseID        string `json:"release_id"`
	DefinitionHash   string `json:"definition_hash"`
	BundleHash       string `json:"bundle_hash"`
	AgentImageDigest string `json:"agent_image_digest"`
}

func (value StageReleaseIdentityV2) Validate() error {
	if _, err := uuid.Parse(value.ReleaseID); err != nil {
		return errors.New("invalid StoryGraph v2 StageRelease identity")
	}
	if !hashPattern.MatchString(value.DefinitionHash) ||
		value.BundleHash != StoryGraphV2SkillBundleHash ||
		!strings.HasPrefix(value.AgentImageDigest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(value.AgentImageDigest, "sha256:")) {
		return errors.New("invalid StoryGraph v2 StageRelease identity")
	}
	return nil
}

type StageControlProofV2 struct {
	RecordID    string `json:"record_id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash"`
}

func (value StageControlProofV2) Validate() error {
	if _, err := uuid.Parse(value.RecordID); err != nil {
		return errors.New("invalid StoryGraph v2 Control proof")
	}
	if value.Revision < 1 || value.Status != "approved" || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid StoryGraph v2 Control proof")
	}
	return nil
}

type V2ExecutionBudget struct {
	MaxModelCalls       int `json:"max_model_calls"`
	MaxExecutionSeconds int `json:"max_execution_seconds"`
	MaxOutputBytes      int `json:"max_output_bytes"`
}

func (value V2ExecutionBudget) Validate() error {
	if value.MaxModelCalls < 1 || value.MaxModelCalls > 2 ||
		value.MaxExecutionSeconds < 1 || value.MaxExecutionSeconds > 600 ||
		value.MaxOutputBytes < 1024 || value.MaxOutputBytes > 1_048_576 {
		return errors.New("invalid StoryGraph v2 execution budget")
	}
	return nil
}

type V2InvocationScope struct {
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   string  `json:"project_id"`
	EpisodeID   *string `json:"episode_id"`
}

func (value V2InvocationScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid StoryGraph v2 scope")
		}
	}
	if value.EpisodeID != nil {
		if _, err := uuid.Parse(*value.EpisodeID); err != nil {
			return errors.New("invalid StoryGraph v2 scope")
		}
	}
	return nil
}

type V2InvocationShard struct {
	ManifestID     string `json:"manifest_id"`
	ManifestHash   string `json:"manifest_hash"`
	ShardKey       string `json:"shard_key"`
	CodepointStart int    `json:"codepoint_start"`
	CodepointEnd   int    `json:"codepoint_end"`
}

func (value V2InvocationShard) Validate() error {
	if _, err := uuid.Parse(value.ManifestID); err != nil {
		return errors.New("invalid StoryGraph v2 shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) || strings.TrimSpace(value.ShardKey) == "" ||
		value.CodepointStart < 0 || value.CodepointEnd <= value.CodepointStart {
		return errors.New("invalid StoryGraph v2 shard")
	}
	return nil
}

type ProposeScriptSpansInput struct {
	SourceVersionID      string `json:"source_version_id"`
	SourceHash           string `json:"source_hash"`
	NormalizedText       string `json:"normalized_text"`
	CodepointCount       int    `json:"codepoint_count"`
	NewlineNormalization string `json:"newline_normalization"`
}

func (value ProposeScriptSpansInput) Validate() error {
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

type ExtractSceneFactsInput struct {
	SourceVersionID           string          `json:"source_version_id"`
	SourceHash                string          `json:"source_hash"`
	NormalizedText            string          `json:"normalized_text"`
	SpanCandidateRevisionID   string          `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash string          `json:"span_candidate_revision_hash"`
	SpanCandidate             json.RawMessage `json:"span_candidate"`
}

func (value ExtractSceneFactsInput) Validate() error {
	for _, identifier := range []string{value.SourceVersionID, value.SpanCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid scene fact source identity")
		}
	}
	if value.NormalizedText == "" || hashUTF8(value.NormalizedText) != value.SourceHash ||
		!hashPattern.MatchString(value.SpanCandidateRevisionHash) ||
		ValidateV2ScriptSpanCandidate(value.SpanCandidate, value.NormalizedText) != nil {
		return errors.New("invalid scene fact source")
	}
	return nil
}

type V2StagePayload struct {
	Variant            StageVariantKeyV2             `json:"variant"`
	Scope              V2InvocationScope             `json:"scope"`
	SourceRefs         []OwnerVersionIdentityV1      `json:"source_refs"`
	UpstreamCandidates []UpstreamCandidateIdentityV2 `json:"upstream_candidates"`
	Shard              V2InvocationShard             `json:"shard"`
	StageInput         json.RawMessage               `json:"stage_input"`
}

func (value V2StagePayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil || value.Shard.Validate() != nil ||
		len(value.SourceRefs) != 1 || value.SourceRefs[0].Validate() != nil || !jsonObject(value.StageInput) {
		return errors.New("invalid StoryGraph v2 payload")
	}
	source := value.SourceRefs[0]
	switch value.Variant.StageKey {
	case "propose_script_spans":
		var input ProposeScriptSpansInput
		if decodeStrict(value.StageInput, &input) != nil || input.Validate() != nil ||
			len(value.UpstreamCandidates) != 0 || source.VersionID != input.SourceVersionID ||
			source.ContentHash != input.SourceHash || value.Shard.CodepointStart != 0 ||
			value.Shard.CodepointEnd != input.CodepointCount {
			return errors.New("script span input does not match its frozen source")
		}
	case "extract_scene_facts":
		var input ExtractSceneFactsInput
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
		return errors.New("unsupported StoryGraph v2 stage")
	}
	return nil
}

type V2StageInvocation struct {
	InvocationID      string                 `json:"invocation_id"`
	AttemptID         string                 `json:"attempt_id"`
	Kind              string                 `json:"kind"`
	WireSchemaVersion string                 `json:"wire_schema_version"`
	StageRelease      StageReleaseIdentityV2 `json:"stage_release"`
	Control           StageControlProofV2    `json:"control"`
	Budget            V2ExecutionBudget      `json:"budget"`
	Payload           V2StagePayload         `json:"payload"`
	InputHash         string                 `json:"input_hash"`
}

func NewV2StageInvocation(
	invocationID, attemptID string,
	release StageReleaseIdentityV2,
	control StageControlProofV2,
	budget V2ExecutionBudget,
	payload V2StagePayload,
) (V2StageInvocation, error) {
	value := V2StageInvocation{
		InvocationID: invocationID, AttemptID: attemptID, Kind: "storygraph_stage",
		WireSchemaVersion: StoryGraphV2WireSchemaVersion, StageRelease: release,
		Control: control, Budget: budget, Payload: payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return V2StageInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return V2StageInvocation{}, err
	}
	return value, nil
}

func DecodeV2StageInvocation(raw []byte) (V2StageInvocation, error) {
	var value V2StageInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return V2StageInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return V2StageInvocation{}, err
	}
	return value, nil
}

func (value V2StageInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid StoryGraph v2 invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != StoryGraphV2WireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.Budget.Validate() != nil || value.Payload.Validate() != nil ||
		!hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid StoryGraph v2 invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("StoryGraph v2 input hash mismatch")
	}
	return nil
}

func (value V2StageInvocation) ComputeInputHash() (string, error) {
	payload := value.Payload
	payload.SourceRefs = make([]OwnerVersionIdentityV1, len(value.Payload.SourceRefs))
	copy(payload.SourceRefs, value.Payload.SourceRefs)
	payload.UpstreamCandidates = make(
		[]UpstreamCandidateIdentityV2,
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
		WireSchemaVersion string                 `json:"wire_schema_version"`
		StageRelease      StageReleaseIdentityV2 `json:"stage_release"`
		Control           StageControlProofV2    `json:"control"`
		Budget            V2ExecutionBudget      `json:"budget"`
		Payload           V2StagePayload         `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return CanonicalHash(encoded)
}

func (value V2StageInvocation) StageInstanceKey() string {
	material := "storygraph-stage-v2" + value.Payload.Variant.StageKey +
		value.Payload.Variant.ProfileKey + value.Payload.Variant.LaneKey +
		value.Payload.Variant.OutputSchemaVersion + value.Payload.Shard.ShardKey +
		value.Payload.Shard.ManifestHash + value.InputHash
	hash := sha256.Sum256([]byte(material))
	return hex.EncodeToString(hash[:])
}

type V2ExecutionGrantClaims struct {
	InvocationID     string `json:"invocation_id"`
	AttemptID        string `json:"attempt_id"`
	InputHash        string `json:"input_hash"`
	StageReleaseID   string `json:"stage_release_id"`
	AgentImageDigest string `json:"agent_image_digest"`
	ExpiresAt        int64  `json:"expires_at"`
}

func (value V2ExecutionGrantClaims) ValidateFor(invocation V2StageInvocation, nowUnix int64) error {
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.InputHash != invocation.InputHash || value.StageReleaseID != invocation.StageRelease.ReleaseID ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid StoryGraph v2 execution grant claims")
	}
	return nil
}

type V2Diagnostic struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

type V2ResultError struct {
	Code        string `json:"code"`
	SafeSummary string `json:"safe_summary"`
	RetryClass  string `json:"retry_class"`
}

type V2Executor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type V2AttemptResult struct {
	InvocationID      string                 `json:"invocation_id"`
	AttemptID         string                 `json:"attempt_id"`
	Kind              string                 `json:"kind"`
	WireSchemaVersion string                 `json:"wire_schema_version"`
	Variant           StageVariantKeyV2      `json:"variant"`
	StageRelease      StageReleaseIdentityV2 `json:"stage_release"`
	Control           StageControlProofV2    `json:"control"`
	Status            string                 `json:"status"`
	CandidateType     string                 `json:"candidate_type"`
	Candidate         json.RawMessage        `json:"candidate"`
	InputHash         string                 `json:"input_hash"`
	OutputHash        *string                `json:"output_hash"`
	Diagnostics       []V2Diagnostic         `json:"diagnostics"`
	DiagnosticHash    string                 `json:"diagnostic_hash"`
	CompletedAt       time.Time              `json:"completed_at"`
	Executor          V2Executor             `json:"executor"`
	Error             *V2ResultError         `json:"error"`
}

func DecodeV2AttemptResult(raw []byte) (V2AttemptResult, error) {
	var value V2AttemptResult
	if err := decodeStrict(raw, &value); err != nil {
		return V2AttemptResult{}, err
	}
	return value, nil
}

func (value V2AttemptResult) ValidateFor(invocation V2StageInvocation) error {
	expectedCandidateType := map[string]string{
		"propose_script_spans": "script_span_candidate_v2",
		"extract_scene_facts":  "scene_fact_candidate_v2",
	}
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.Kind != "storygraph_stage" || value.WireSchemaVersion != StoryGraphV2WireSchemaVersion ||
		value.Variant != invocation.Payload.Variant || value.StageRelease != invocation.StageRelease ||
		value.Control != invocation.Control || value.InputHash != invocation.InputHash ||
		value.CandidateType != expectedCandidateType[invocation.Payload.Variant.StageKey] ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "text" ||
		value.Executor.RuntimeImageDigest != invocation.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "storygraph-stage-harness-v2" || strings.TrimSpace(value.Executor.Model) == "" {
		return errors.New("StoryGraph v2 result identity does not match invocation")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := CanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("StoryGraph v2 diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted StoryGraph v2 result is incomplete")
		}
		outputHash, hashErr := CanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("StoryGraph v2 output hash mismatch")
		}
		switch invocation.Payload.Variant.StageKey {
		case "propose_script_spans":
			var input ProposeScriptSpansInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil ||
				ValidateV2ScriptSpanCandidate(value.Candidate, input.NormalizedText) != nil {
				return errors.New("invalid accepted ScriptSpan candidate")
			}
			var candidate ScriptSpanCandidateV2
			if decodeStrict(value.Candidate, &candidate) != nil ||
				candidate.SourceVersionID != input.SourceVersionID {
				return errors.New("ScriptSpan candidate source identity drifted")
			}
		case "extract_scene_facts":
			var input ExtractSceneFactsInput
			if decodeStrict(invocation.Payload.StageInput, &input) != nil ||
				ValidateV2SceneFactCandidate(value.Candidate, input.NormalizedText, input.SpanCandidate) != nil {
				return errors.New("invalid accepted SceneFact candidate")
			}
			var candidate SceneFactCandidateV2
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
			return errors.New("failed StoryGraph v2 result has invalid semantics")
		}
	default:
		return errors.New("invalid StoryGraph v2 result status")
	}
	return nil
}

type EvidenceSpanV2 struct {
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
	TextHash    string `json:"text_hash"`
	ExactAnchor string `json:"exact_anchor"`
}

func (value EvidenceSpanV2) Validate(text []rune) error {
	if value.SourceStart < 0 || value.SourceEnd <= value.SourceStart || value.SourceEnd > len(text) ||
		!hashPattern.MatchString(value.TextHash) {
		return errors.New("invalid v2 Evidence range")
	}
	anchor := string(text[value.SourceStart:value.SourceEnd])
	if anchor != value.ExactAnchor || hashUTF8(anchor) != value.TextHash {
		return errors.New("v2 Evidence does not match source")
	}
	return nil
}

type CandidateIssueV2 struct {
	IssueKey string           `json:"issue_key"`
	Code     string           `json:"code"`
	Severity string           `json:"severity"`
	Scope    string           `json:"scope"`
	Summary  string           `json:"summary"`
	Evidence []EvidenceSpanV2 `json:"evidence"`
}

type ScriptSpanV2 struct {
	TemporarySpanID string         `json:"temporary_span_id"`
	Kind            string         `json:"kind"`
	CodepointStart  int            `json:"codepoint_start"`
	CodepointEnd    int            `json:"codepoint_end"`
	Heading         string         `json:"heading"`
	Evidence        EvidenceSpanV2 `json:"evidence"`
}

type ScriptSpanCandidateV2 struct {
	SourceVersionID string             `json:"source_version_id"`
	SourceHash      string             `json:"source_hash"`
	CodepointCount  int                `json:"codepoint_count"`
	Spans           []ScriptSpanV2     `json:"spans"`
	ReviewIssues    []CandidateIssueV2 `json:"review_issues"`
}

func ValidateV2ScriptSpanCandidate(raw json.RawMessage, text string) error {
	var value ScriptSpanCandidateV2
	if decodeStrict(raw, &value) != nil || value.SourceHash != hashUTF8(text) ||
		value.CodepointCount != utf8.RuneCountInString(text) || len(value.Spans) == 0 {
		return errors.New("invalid v2 ScriptSpan candidate")
	}
	if _, err := uuid.Parse(value.SourceVersionID); err != nil {
		return errors.New("invalid v2 ScriptSpan source identity")
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
			return errors.New("v2 ScriptSpan coverage is invalid")
		}
		if _, exists := keys[span.TemporarySpanID]; exists {
			return errors.New("v2 ScriptSpan key is duplicated")
		}
		keys[span.TemporarySpanID] = struct{}{}
		previousEnd = span.CodepointEnd
	}
	if previousEnd != len(runes) {
		return errors.New("v2 ScriptSpan source coverage is incomplete")
	}
	return nil
}

type EvidenceFactV2 struct {
	Text     string         `json:"text"`
	Evidence EvidenceSpanV2 `json:"evidence"`
}

type DialogueFactV2 struct {
	SpeakerMention string         `json:"speaker_mention"`
	Text           string         `json:"text"`
	Evidence       EvidenceSpanV2 `json:"evidence"`
}

type RawMentionV2 struct {
	Text     string         `json:"text"`
	Evidence EvidenceSpanV2 `json:"evidence"`
}

type SceneFactV2 struct {
	TemporarySceneID     string           `json:"temporary_scene_id"`
	SpanID               string           `json:"span_id"`
	SourceStart          int              `json:"source_start"`
	SourceEnd            int              `json:"source_end"`
	LocationText         *string          `json:"location_text"`
	TimeText             *string          `json:"time_text"`
	Actions              []EvidenceFactV2 `json:"actions"`
	Dialogues            []DialogueFactV2 `json:"dialogues"`
	RawCharacterMentions []RawMentionV2   `json:"raw_character_mentions"`
	RawPropMentions      []RawMentionV2   `json:"raw_prop_mentions"`
}

type SceneFactCandidateV2 struct {
	SourceVersionID           string             `json:"source_version_id"`
	SourceHash                string             `json:"source_hash"`
	SpanCandidateRevisionID   string             `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash string             `json:"span_candidate_revision_hash"`
	Scenes                    []SceneFactV2      `json:"scenes"`
	ReviewIssues              []CandidateIssueV2 `json:"review_issues"`
}

func ValidateV2SceneFactCandidate(raw json.RawMessage, text string, spanRaw json.RawMessage) error {
	var value SceneFactCandidateV2
	var spans ScriptSpanCandidateV2
	if decodeStrict(raw, &value) != nil || decodeStrict(spanRaw, &spans) != nil ||
		ValidateV2ScriptSpanCandidate(spanRaw, text) != nil || value.SourceHash != hashUTF8(text) ||
		!hashPattern.MatchString(value.SpanCandidateRevisionHash) || len(value.Scenes) != len(spans.Spans) {
		return errors.New("invalid v2 SceneFact candidate")
	}
	for _, identifier := range []string{value.SourceVersionID, value.SpanCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid v2 SceneFact identity")
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
			return errors.New("v2 SceneFact does not map exactly to ScriptSpan")
		}
		if _, duplicate := sceneKeys[scene.TemporarySceneID]; duplicate {
			return errors.New("v2 SceneFact key is duplicated")
		}
		sceneKeys[scene.TemporarySceneID] = struct{}{}
		evidence := make([]EvidenceSpanV2, 0, len(scene.Actions)+len(scene.Dialogues)+len(scene.RawCharacterMentions)+len(scene.RawPropMentions))
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
				return errors.New("v2 SceneFact evidence is invalid")
			}
		}
		delete(expected, scene.SpanID)
	}
	if len(expected) != 0 {
		return errors.New("v2 SceneFact span coverage is incomplete")
	}
	return nil
}

func hashUTF8(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
