package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ReferencePlanStageKey = "plan_reference_assets"

type ReferencePlanStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value ReferencePlanStageVariant) Validate() error {
	if value.StageKey != ReferencePlanStageKey || value.ProfileKey != "default" ||
		value.LaneKey != "primary" || value.OutputSchemaVersion != ReferencePlanCandidateContractID {
		return errors.New("invalid Reference Plan stage variant")
	}
	return nil
}

type ReferencePlanScope struct {
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
}

func (value ReferencePlanScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Plan scope")
		}
	}
	return nil
}

type ReferencePlanShard struct {
	ManifestID        string `json:"manifest_id"`
	ManifestHash      string `json:"manifest_hash"`
	ShardKey          string `json:"shard_key"`
	ImpactClosureHash string `json:"impact_closure_hash"`
}

func (value ReferencePlanShard) Validate(projectID string) error {
	if _, err := uuid.Parse(value.ManifestID); err != nil {
		return errors.New("invalid Reference Plan shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) ||
		!hashPattern.MatchString(value.ImpactClosureHash) || value.ShardKey != "project:"+projectID {
		return errors.New("invalid Reference Plan shard")
	}
	return nil
}

type ReferencePlanPayload struct {
	Variant    ReferencePlanStageVariant `json:"variant"`
	Scope      ReferencePlanScope        `json:"scope"`
	Shard      ReferencePlanShard        `json:"shard"`
	StageInput ReferencePlanInput        `json:"stage_input"`
}

func (value ReferencePlanPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil ||
		value.Shard.Validate(value.Scope.ProjectID) != nil || value.StageInput.Validate() != nil ||
		value.Scope.WorkspaceID != value.StageInput.WorkspaceID ||
		value.Scope.ProjectID != value.StageInput.ProjectID {
		return errors.New("invalid Reference Plan payload")
	}
	return nil
}

type ReferencePlanInvocation struct {
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Budget            SceneAnalysisExecutionBudget `json:"budget"`
	Payload           ReferencePlanPayload         `json:"payload"`
	InputHash         string                       `json:"input_hash"`
}

func NewReferencePlanInvocation(
	invocationID, attemptID string,
	release SceneAnalysisReleaseIdentity,
	control SceneAnalysisControlProof,
	budget SceneAnalysisExecutionBudget,
	payload ReferencePlanPayload,
) (ReferencePlanInvocation, error) {
	value := ReferencePlanInvocation{
		InvocationID: invocationID, AttemptID: attemptID, Kind: "storygraph_stage",
		WireSchemaVersion: SceneAnalysisWireSchemaVersion, StageRelease: release,
		Control: control, Budget: budget, Payload: payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return ReferencePlanInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return ReferencePlanInvocation{}, err
	}
	return value, nil
}

func DecodeReferencePlanInvocation(raw []byte) (ReferencePlanInvocation, error) {
	var value ReferencePlanInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return ReferencePlanInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return ReferencePlanInvocation{}, err
	}
	return value, nil
}

func (value ReferencePlanInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Plan invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.StageRelease.BundleContentHash != StoryGraphSkillBundleHash ||
		value.Control.Validate() != nil || value.Budget.Validate() != nil || value.Budget.MaxModelCalls != 1 ||
		value.Budget.MaxExecutionSeconds > 120 || value.Budget.MaxOutputBytes > 131072 ||
		value.Payload.Validate() != nil || !hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Reference Plan invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("Reference Plan input hash mismatch")
	}
	return nil
}

func (value ReferencePlanInvocation) ComputeInputHash() (string, error) {
	material := struct {
		WireSchemaVersion string                       `json:"wire_schema_version"`
		StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
		Control           SceneAnalysisControlProof    `json:"control"`
		Budget            SceneAnalysisExecutionBudget `json:"budget"`
		Payload           ReferencePlanPayload         `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, value.Payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(encoded)
}

func (value ReferencePlanInvocation) StageInstanceKey() string {
	root := struct {
		IdentityContractID string                    `json:"identity_contract_id"`
		VariantKey         ReferencePlanStageVariant `json:"variant_key"`
		Scope              ReferencePlanScope        `json:"scope"`
		ShardManifestHash  string                    `json:"shard_manifest_hash"`
		ShardKey           string                    `json:"shard_key"`
		InputHash          string                    `json:"input_hash"`
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

func (value SceneAnalysisDispatchAuthorizationClaims) ValidateForReferencePlan(
	invocation ReferencePlanInvocation,
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
		return errors.New("invalid Reference Plan dispatch authorization claims")
	}
	return nil
}

type ReferencePlanExecutor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type ReferencePlanAttemptResult struct {
	InvocationID              string                       `json:"invocation_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Kind                      string                       `json:"kind"`
	WireSchemaVersion         string                       `json:"wire_schema_version"`
	Variant                   ReferencePlanStageVariant    `json:"variant"`
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
	Executor                  ReferencePlanExecutor        `json:"executor"`
	Error                     *SceneAnalysisResultError    `json:"error"`
	ResultHash                string                       `json:"result_hash"`
}

func DecodeReferencePlanAttemptResult(raw []byte) (ReferencePlanAttemptResult, error) {
	var value ReferencePlanAttemptResult
	if err := decodeStrict(raw, &value); err != nil || value.validateShape() != nil {
		return ReferencePlanAttemptResult{}, errors.New("invalid Reference Plan Attempt Result")
	}
	return value, nil
}

func (value ReferencePlanAttemptResult) ValidateFor(
	invocation ReferencePlanInvocation,
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
		return errors.New("Reference Plan result identity does not match invocation")
	}
	if value.Status == "accepted" {
		candidate, _, err := DecodeReferencePlanCandidate(value.Candidate)
		if err != nil || candidate.ValidateFor(invocation.Payload.StageInput) != nil {
			return errors.New("invalid accepted Reference Plan Candidate")
		}
	}
	return nil
}

func (value ReferencePlanAttemptResult) validateShape() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Reference Plan result identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.Variant.Validate() != nil || value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.ClaimVersion < 1 || !hashPattern.MatchString(value.DispatchAuthorizationHash) ||
		value.CandidateType != "reference_plan_candidate" || !hashPattern.MatchString(value.InputHash) ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "text" ||
		value.Executor.RuntimeImageDigest != value.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "reference-plan-harness" ||
		strings.TrimSpace(value.Executor.Model) == "" || len(value.Executor.Model) > 200 {
		return errors.New("invalid Reference Plan result")
	}
	for _, diagnostic := range value.Diagnostics {
		if !candidateReviewCodePattern.MatchString(diagnostic.Code) || !validVisualText(diagnostic.Summary, 800) {
			return errors.New("invalid Reference Plan diagnostic")
		}
	}
	computedResultHash, err := value.ComputeResultHash()
	if err != nil || !hashPattern.MatchString(value.ResultHash) || computedResultHash != value.ResultHash {
		return errors.New("Reference Plan result hash mismatch")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := ProductionCanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("Reference Plan diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Reference Plan result is incomplete")
		}
		outputHash, hashErr := ProductionCanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("Reference Plan output hash mismatch")
		}
		if _, _, decodeErr := DecodeReferencePlanCandidate(value.Candidate); decodeErr != nil {
			return errors.New("invalid Reference Plan Candidate")
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
			return errors.New("failed Reference Plan result has invalid semantics")
		}
	default:
		return errors.New("invalid Reference Plan result status")
	}
	return nil
}

func (value ReferencePlanAttemptResult) ComputeResultHash() (string, error) {
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
