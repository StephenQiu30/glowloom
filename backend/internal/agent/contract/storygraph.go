package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const StoryGraphWireSchemaVersion = "storygraph-stage-wire-v1"

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
	if payload.Stage != "extract_source_evidence" {
		return nil
	}
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
