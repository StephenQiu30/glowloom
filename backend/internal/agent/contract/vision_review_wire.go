package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const VisionReviewStageKey = "review_reference_artifact"

type VisionReviewStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value VisionReviewStageVariant) Validate() error {
	if value.StageKey != VisionReviewStageKey || value.ProfileKey != "default" ||
		value.LaneKey != "primary" || value.OutputSchemaVersion != VisionReviewCandidateContractID {
		return errors.New("invalid Vision Review stage variant")
	}
	return nil
}

type VisionReviewScope struct {
	WorkspaceID   string `json:"workspace_id"`
	ProjectID     string `json:"project_id"`
	BundleInputID string `json:"bundle_input_id"`
}

func (value VisionReviewScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID, value.BundleInputID} {
		if !visionReviewCanonicalIdentity(identifier) {
			return errors.New("invalid Vision Review scope")
		}
	}
	return nil
}

func visionReviewCanonicalIdentity(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

type VisionReviewShard struct {
	ManifestID        string `json:"manifest_id"`
	ManifestHash      string `json:"manifest_hash"`
	ShardKey          string `json:"shard_key"`
	ImpactClosureHash string `json:"impact_closure_hash"`
}

func (value VisionReviewShard) Validate(bundleInputID string) error {
	if !visionReviewCanonicalIdentity(value.ManifestID) {
		return errors.New("invalid Vision Review shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) || !hashPattern.MatchString(value.ImpactClosureHash) ||
		value.ShardKey != "vision_bundle:"+bundleInputID {
		return errors.New("invalid Vision Review shard")
	}
	return nil
}

type VisionReviewPayload struct {
	Variant    VisionReviewStageVariant `json:"variant"`
	Scope      VisionReviewScope        `json:"scope"`
	Shard      VisionReviewShard        `json:"shard"`
	StageInput VisionReviewInput        `json:"stage_input"`
}

func (value VisionReviewPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil ||
		value.Shard.Validate(value.Scope.BundleInputID) != nil || value.StageInput.Validate() != nil ||
		value.Scope.WorkspaceID != value.StageInput.Subject.WorkspaceID ||
		value.Scope.ProjectID != value.StageInput.Subject.ProjectID ||
		value.Scope.BundleInputID != value.StageInput.Subject.BundleInputRef.ID {
		return errors.New("invalid Vision Review payload")
	}
	return nil
}

type VisionReviewInvocation struct {
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Budget            SceneAnalysisExecutionBudget `json:"budget"`
	Payload           VisionReviewPayload          `json:"payload"`
	InputHash         string                       `json:"input_hash"`
}

func NewVisionReviewInvocation(
	invocationID, attemptID string,
	release SceneAnalysisReleaseIdentity,
	control SceneAnalysisControlProof,
	budget SceneAnalysisExecutionBudget,
	payload VisionReviewPayload,
) (VisionReviewInvocation, error) {
	value := VisionReviewInvocation{
		InvocationID: invocationID, AttemptID: attemptID, Kind: "storygraph_stage",
		WireSchemaVersion: SceneAnalysisWireSchemaVersion, StageRelease: release,
		Control: control, Budget: budget, Payload: payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return VisionReviewInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return VisionReviewInvocation{}, err
	}
	return value, nil
}

func DecodeVisionReviewInvocation(raw []byte) (VisionReviewInvocation, error) {
	var value VisionReviewInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return VisionReviewInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return VisionReviewInvocation{}, err
	}
	// The typed decoder alone cannot distinguish missing zero-valued fields.
	if err := validateVisionReviewWireShape(raw, value); err != nil {
		return VisionReviewInvocation{}, err
	}
	return value, nil
}

func (value VisionReviewInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if !visionReviewCanonicalIdentity(identifier) {
			return errors.New("invalid Vision Review invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.StageRelease.BundleContentHash != StoryGraphSkillBundleHash ||
		value.Control.Validate() != nil || value.Budget.Validate() != nil || value.Budget.MaxModelCalls != 1 ||
		value.Budget.MaxExecutionSeconds > 120 || value.Budget.MaxOutputBytes > 131072 ||
		value.Payload.Validate() != nil ||
		value.Payload.StageInput.Subject.StageReleaseHash != value.StageRelease.StageReleaseHash ||
		!hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Vision Review invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("Vision Review input hash mismatch")
	}
	return nil
}

func (value VisionReviewInvocation) ComputeInputHash() (string, error) {
	material := struct {
		WireSchemaVersion string                       `json:"wire_schema_version"`
		StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
		Control           SceneAnalysisControlProof    `json:"control"`
		Budget            SceneAnalysisExecutionBudget `json:"budget"`
		Payload           VisionReviewPayload          `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, value.Payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(encoded)
}

func (value VisionReviewInvocation) StageInstanceKey() string {
	root := struct {
		IdentityContractID string                   `json:"identity_contract_id"`
		VariantKey         VisionReviewStageVariant `json:"variant_key"`
		Scope              VisionReviewScope        `json:"scope"`
		ShardManifestHash  string                   `json:"shard_manifest_hash"`
		ShardKey           string                   `json:"shard_key"`
		InputHash          string                   `json:"input_hash"`
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

func (value SceneAnalysisDispatchAuthorizationClaims) ValidateForVisionReview(
	invocation VisionReviewInvocation,
	claimVersion, nowUnix int64,
) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	if claimVersion < 1 || value.InvocationID != invocation.InvocationID ||
		value.AttemptID != invocation.AttemptID || value.InputHash != invocation.InputHash ||
		value.SkillReleaseID != invocation.StageRelease.SkillReleaseID ||
		value.SkillReleaseHash != invocation.StageRelease.SkillReleaseHash ||
		value.StageReleaseHash != invocation.StageRelease.StageReleaseHash ||
		value.BundleContentHash != invocation.StageRelease.BundleContentHash ||
		value.ControlHash != invocation.Control.ControlHash ||
		value.ReleaseFence != invocation.Control.ReleaseFence || value.ClaimVersion != claimVersion ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid Vision Review dispatch authorization claims")
	}
	return nil
}

type VisionReviewExecutor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type VisionReviewAttemptResult struct {
	InvocationID              string                       `json:"invocation_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Kind                      string                       `json:"kind"`
	WireSchemaVersion         string                       `json:"wire_schema_version"`
	Variant                   VisionReviewStageVariant     `json:"variant"`
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
	Executor                  VisionReviewExecutor         `json:"executor"`
	Error                     *SceneAnalysisResultError    `json:"error"`
	ResultHash                string                       `json:"result_hash"`
}

func DecodeVisionReviewAttemptResult(raw []byte) (VisionReviewAttemptResult, error) {
	var value VisionReviewAttemptResult
	if err := decodeStrict(raw, &value); err != nil || value.validateShape() != nil {
		return VisionReviewAttemptResult{}, errors.New("invalid Vision Review Attempt Result")
	}
	if err := validateVisionReviewWireShape(raw, value); err != nil {
		return VisionReviewAttemptResult{}, err
	}
	return value, nil
}

func validateVisionReviewWireShape(raw []byte, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	want, err := ProductionCanonicalJSON(encoded)
	if err != nil {
		return err
	}
	got, err := ProductionCanonicalJSON(raw)
	if err != nil || !bytes.Equal(got, want) {
		return errors.New("Vision Review wire has missing or noncanonical fields")
	}
	return nil
}

func (value VisionReviewAttemptResult) ValidateFor(
	invocation VisionReviewInvocation,
	claimVersion int64,
	dispatchAuthorizationHash string,
) error {
	if err := value.validateShape(); err != nil {
		return err
	}
	if err := invocation.Validate(); err != nil {
		return err
	}
	if value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.Variant != invocation.Payload.Variant || value.StageRelease != invocation.StageRelease ||
		value.Control != invocation.Control || value.ClaimVersion != claimVersion ||
		value.DispatchAuthorizationHash != dispatchAuthorizationHash || value.InputHash != invocation.InputHash ||
		value.Executor.RuntimeImageDigest != invocation.StageRelease.AgentImageDigest {
		return errors.New("Vision Review result identity does not match invocation")
	}
	if value.Status == "accepted" {
		candidate, _, err := DecodeVisionReviewCandidate(value.Candidate)
		if err != nil || candidate.ValidateFor(invocation.Payload.StageInput.Subject) != nil {
			return errors.New("invalid accepted Vision Review Candidate")
		}
	}
	return nil
}

func (value VisionReviewAttemptResult) validateShape() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if !visionReviewCanonicalIdentity(identifier) {
			return errors.New("invalid Vision Review result identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.Variant.Validate() != nil || value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.ClaimVersion < 1 || !hashPattern.MatchString(value.DispatchAuthorizationHash) ||
		value.CandidateType != "vision_review_candidate" || !hashPattern.MatchString(value.InputHash) ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "vision" ||
		value.Executor.RuntimeImageDigest != value.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "vision-review-harness" ||
		strings.TrimSpace(value.Executor.Model) == "" || len(value.Executor.Model) > 200 {
		return errors.New("invalid Vision Review result")
	}
	for _, diagnostic := range value.Diagnostics {
		if !candidateReviewCodePattern.MatchString(diagnostic.Code) || !validVisualText(diagnostic.Summary, 800) {
			return errors.New("invalid Vision Review diagnostic")
		}
	}
	computedResultHash, err := value.ComputeResultHash()
	if err != nil || !hashPattern.MatchString(value.ResultHash) || computedResultHash != value.ResultHash {
		return errors.New("Vision Review result hash mismatch")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := ProductionCanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("Vision Review diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Vision Review result is incomplete")
		}
		outputHash, hashErr := ProductionCanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("Vision Review output hash mismatch")
		}
		if _, _, decodeErr := DecodeVisionReviewCandidate(value.Candidate); decodeErr != nil {
			return errors.New("invalid Vision Review Candidate")
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
			return errors.New("failed Vision Review result has invalid semantics")
		}
	default:
		return errors.New("invalid Vision Review result status")
	}
	return nil
}

func (value VisionReviewAttemptResult) ComputeResultHash() (string, error) {
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
