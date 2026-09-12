package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ReferenceBriefStageKey = "compile_reference_brief"

type ReferenceBriefStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value ReferenceBriefStageVariant) Validate() error {
	if value.StageKey != ReferenceBriefStageKey || value.ProfileKey != "default" ||
		value.LaneKey != "primary" || value.OutputSchemaVersion != ReferenceBriefCandidateContractID {
		return errors.New("invalid Reference Brief stage variant")
	}
	return nil
}

type ReferenceBriefScope struct {
	WorkspaceID       string `json:"workspace_id"`
	ProjectID         string `json:"project_id"`
	TargetBusinessKey string `json:"target_business_key"`
}

func (value ReferenceBriefScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Brief scope")
		}
	}
	if referencePlanBusinessKeyKind(value.TargetBusinessKey) == "" {
		return errors.New("invalid Reference Brief scope")
	}
	return nil
}

type ReferenceBriefShard struct {
	ManifestID        string `json:"manifest_id"`
	ManifestHash      string `json:"manifest_hash"`
	ShardKey          string `json:"shard_key"`
	ImpactClosureHash string `json:"impact_closure_hash"`
}

func (value ReferenceBriefShard) Validate(targetBusinessKey string) error {
	if _, err := uuid.Parse(value.ManifestID); err != nil {
		return errors.New("invalid Reference Brief shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) || !hashPattern.MatchString(value.ImpactClosureHash) ||
		value.ShardKey != "reference_target:"+targetBusinessKey {
		return errors.New("invalid Reference Brief shard")
	}
	return nil
}

type ReferenceBriefPayload struct {
	Variant    ReferenceBriefStageVariant `json:"variant"`
	Scope      ReferenceBriefScope        `json:"scope"`
	Shard      ReferenceBriefShard        `json:"shard"`
	StageInput ReferenceBriefInput        `json:"stage_input"`
}

func (value ReferenceBriefPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil ||
		value.Shard.Validate(value.Scope.TargetBusinessKey) != nil || value.StageInput.Validate() != nil ||
		value.Scope.WorkspaceID != value.StageInput.WorkspaceID ||
		value.Scope.ProjectID != value.StageInput.ProjectID ||
		value.Scope.TargetBusinessKey != value.StageInput.TargetBusinessKey {
		return errors.New("invalid Reference Brief payload")
	}
	return nil
}

type ReferenceBriefInvocation struct {
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Budget            SceneAnalysisExecutionBudget `json:"budget"`
	Payload           ReferenceBriefPayload        `json:"payload"`
	InputHash         string                       `json:"input_hash"`
}

func NewReferenceBriefInvocation(
	invocationID, attemptID string,
	release SceneAnalysisReleaseIdentity,
	control SceneAnalysisControlProof,
	budget SceneAnalysisExecutionBudget,
	payload ReferenceBriefPayload,
) (ReferenceBriefInvocation, error) {
	value := ReferenceBriefInvocation{
		InvocationID: invocationID, AttemptID: attemptID, Kind: "storygraph_stage",
		WireSchemaVersion: SceneAnalysisWireSchemaVersion, StageRelease: release,
		Control: control, Budget: budget, Payload: payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return ReferenceBriefInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return ReferenceBriefInvocation{}, err
	}
	return value, nil
}

func DecodeReferenceBriefInvocation(raw []byte) (ReferenceBriefInvocation, error) {
	var value ReferenceBriefInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return ReferenceBriefInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return ReferenceBriefInvocation{}, err
	}
	return value, nil
}

func (value ReferenceBriefInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Brief invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.StageRelease.BundleContentHash != StoryGraphSkillBundleHash ||
		value.Control.Validate() != nil || value.Budget.Validate() != nil || value.Budget.MaxModelCalls != 1 ||
		value.Budget.MaxExecutionSeconds > 120 || value.Budget.MaxOutputBytes > 131072 ||
		value.Payload.Validate() != nil ||
		value.Payload.StageInput.StageRelease.StageReleaseHash != value.StageRelease.StageReleaseHash ||
		!hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Reference Brief invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("Reference Brief input hash mismatch")
	}
	return nil
}

func (value ReferenceBriefInvocation) ComputeInputHash() (string, error) {
	material := struct {
		WireSchemaVersion string                       `json:"wire_schema_version"`
		StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
		Control           SceneAnalysisControlProof    `json:"control"`
		Budget            SceneAnalysisExecutionBudget `json:"budget"`
		Payload           ReferenceBriefPayload        `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, value.Payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(encoded)
}

func (value ReferenceBriefInvocation) StageInstanceKey() string {
	root := struct {
		IdentityContractID string                     `json:"identity_contract_id"`
		VariantKey         ReferenceBriefStageVariant `json:"variant_key"`
		Scope              ReferenceBriefScope        `json:"scope"`
		ShardManifestHash  string                     `json:"shard_manifest_hash"`
		ShardKey           string                     `json:"shard_key"`
		InputHash          string                     `json:"input_hash"`
	}{
		IdentityContractID: "storygraph-stage-instance-production", VariantKey: value.Payload.Variant,
		Scope: value.Payload.Scope, ShardManifestHash: value.Payload.Shard.ManifestHash,
		ShardKey: value.Payload.Shard.ShardKey, InputHash: value.InputHash,
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return ""
	}
	hash, err := ProductionCanonicalHash(encoded)
	if err != nil {
		return ""
	}
	return hash
}

func (value SceneAnalysisDispatchAuthorizationClaims) ValidateForReferenceBrief(
	invocation ReferenceBriefInvocation,
	claimVersion, nowUnix int64,
) error {
	if claimVersion < 1 || value.InvocationID != invocation.InvocationID ||
		value.AttemptID != invocation.AttemptID || value.InputHash != invocation.InputHash ||
		value.SkillReleaseID != invocation.StageRelease.SkillReleaseID ||
		value.SkillReleaseHash != invocation.StageRelease.SkillReleaseHash ||
		value.StageReleaseHash != invocation.StageRelease.StageReleaseHash ||
		value.BundleContentHash != invocation.StageRelease.BundleContentHash ||
		value.ControlHash != invocation.Control.ControlHash ||
		value.ReleaseFence != invocation.Control.ReleaseFence || value.ClaimVersion != claimVersion ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid Reference Brief dispatch authorization claims")
	}
	return nil
}

type ReferenceBriefExecutor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type ReferenceBriefAttemptResult struct {
	InvocationID              string                       `json:"invocation_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Kind                      string                       `json:"kind"`
	WireSchemaVersion         string                       `json:"wire_schema_version"`
	Variant                   ReferenceBriefStageVariant   `json:"variant"`
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
	Executor                  ReferenceBriefExecutor       `json:"executor"`
	Error                     *SceneAnalysisResultError    `json:"error"`
	ResultHash                string                       `json:"result_hash"`
}

func DecodeReferenceBriefAttemptResult(raw []byte) (ReferenceBriefAttemptResult, error) {
	var value ReferenceBriefAttemptResult
	if err := decodeStrict(raw, &value); err != nil || value.validateShape() != nil {
		return ReferenceBriefAttemptResult{}, errors.New("invalid Reference Brief Attempt Result")
	}
	return value, nil
}

func (value ReferenceBriefAttemptResult) ValidateFor(
	invocation ReferenceBriefInvocation,
	claimVersion int64,
	dispatchAuthorizationHash string,
) error {
	if err := value.validateShape(); err != nil {
		return err
	}
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.Variant != invocation.Payload.Variant || value.StageRelease != invocation.StageRelease ||
		value.Control != invocation.Control || value.ClaimVersion != claimVersion ||
		value.DispatchAuthorizationHash != dispatchAuthorizationHash || value.InputHash != invocation.InputHash ||
		value.Executor.RuntimeImageDigest != invocation.StageRelease.AgentImageDigest {
		return errors.New("Reference Brief result identity does not match invocation")
	}
	if value.Status == "accepted" {
		candidate, _, err := DecodeReferenceBriefCandidate(value.Candidate)
		if err != nil || candidate.ValidateFor(invocation.Payload.StageInput) != nil {
			return errors.New("invalid accepted Reference Brief Candidate")
		}
	}
	return nil
}

func (value ReferenceBriefAttemptResult) validateShape() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Brief result identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.Variant.Validate() != nil || value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.ClaimVersion < 1 || !hashPattern.MatchString(value.DispatchAuthorizationHash) ||
		value.CandidateType != "reference_brief_candidate" || !hashPattern.MatchString(value.InputHash) ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "text" ||
		value.Executor.RuntimeImageDigest != value.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "reference-brief-harness" ||
		strings.TrimSpace(value.Executor.Model) == "" || len(value.Executor.Model) > 200 {
		return errors.New("invalid Reference Brief result")
	}
	for _, diagnostic := range value.Diagnostics {
		if !candidateReviewCodePattern.MatchString(diagnostic.Code) || !validVisualText(diagnostic.Summary, 800) {
			return errors.New("invalid Reference Brief diagnostic")
		}
	}
	computedResultHash, err := value.ComputeResultHash()
	if err != nil || !hashPattern.MatchString(value.ResultHash) || computedResultHash != value.ResultHash {
		return errors.New("Reference Brief result hash mismatch")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := ProductionCanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("Reference Brief diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Reference Brief result is incomplete")
		}
		outputHash, hashErr := ProductionCanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("Reference Brief output hash mismatch")
		}
		if _, _, decodeErr := DecodeReferenceBriefCandidate(value.Candidate); decodeErr != nil {
			return errors.New("invalid Reference Brief Candidate")
		}
	case "rejected", "outcome_unknown":
		expectedRetry := "never"
		if value.Status == "outcome_unknown" {
			expectedRetry = "same_release"
		}
		if value.OutputHash != nil || len(value.Candidate) != 0 && string(value.Candidate) != "null" ||
			value.Error == nil || value.Error.RetryClass != expectedRetry ||
			!candidateReviewCodePattern.MatchString(value.Error.Code) ||
			!validVisualText(value.Error.SafeSummary, 800) {
			return errors.New("failed Reference Brief result has invalid semantics")
		}
	default:
		return errors.New("invalid Reference Brief result status")
	}
	return nil
}

func (value ReferenceBriefAttemptResult) ComputeResultHash() (string, error) {
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
