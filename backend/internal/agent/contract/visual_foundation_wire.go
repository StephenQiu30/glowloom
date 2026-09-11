package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	VisualFoundationStageKey               = "resolve_visual_foundation"
	VisualFoundationCandidateSchemaVersion = "visual-foundation-candidate-production"
	MaxVisualFoundationImages              = 8
	MaxVisualFoundationImageBytes          = 10 * 1024 * 1024
	MaxVisualFoundationTotalImageBytes     = 32 * 1024 * 1024
)

type VisualFoundationStageVariant struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	LaneKey             string `json:"lane_key"`
	OutputSchemaVersion string `json:"output_schema_version"`
}

func (value VisualFoundationStageVariant) Validate() error {
	if value.StageKey != VisualFoundationStageKey || value.ProfileKey != "default" ||
		value.LaneKey != "primary" || value.OutputSchemaVersion != VisualFoundationCandidateSchemaVersion {
		return errors.New("invalid Visual Foundation stage variant")
	}
	return nil
}

type VisualFoundationScope struct {
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
}

func (value VisualFoundationScope) Validate() error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Visual Foundation scope")
		}
	}
	return nil
}

type VisualFoundationShard struct {
	ManifestID        string `json:"manifest_id"`
	ManifestHash      string `json:"manifest_hash"`
	ShardKey          string `json:"shard_key"`
	ImpactClosureHash string `json:"impact_closure_hash"`
}

func (value VisualFoundationShard) Validate(projectID string) error {
	if _, err := uuid.Parse(value.ManifestID); err != nil {
		return errors.New("invalid Visual Foundation shard")
	}
	if !hashPattern.MatchString(value.ManifestHash) || !hashPattern.MatchString(value.ImpactClosureHash) ||
		value.ShardKey != "project:"+projectID {
		return errors.New("invalid Visual Foundation shard")
	}
	return nil
}

type VisualFoundationMediaAttachment struct {
	AttachmentID   string `json:"attachment_id"`
	MediaObjectID  string `json:"media_object_id"`
	VersionNo      int    `json:"version_no"`
	Purpose        string `json:"purpose"`
	ObjectKey      string `json:"object_key"`
	ContentHash    string `json:"content_hash"`
	MediaType      string `json:"media_type"`
	ByteLength     int64  `json:"byte_length"`
	PixelWidth     int    `json:"pixel_width"`
	PixelHeight    int    `json:"pixel_height"`
	PageCount      int    `json:"page_count"`
	FrameCount     int    `json:"frame_count"`
	RightsBasis    string `json:"rights_basis"`
	RightsRefHash  string `json:"rights_ref_hash"`
	LineageRefHash string `json:"lineage_ref_hash"`
}

func (value VisualFoundationMediaAttachment) Validate() error {
	for _, identifier := range []string{value.AttachmentID, value.MediaObjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Visual Foundation media attachment")
		}
	}
	if value.VersionNo < 1 || value.Purpose != "style_reference" || strings.TrimSpace(value.ObjectKey) == "" ||
		len(value.ObjectKey) > 512 || !hashPattern.MatchString(value.ContentHash) ||
		(value.MediaType != "image/jpeg" && value.MediaType != "image/png" && value.MediaType != "image/webp") ||
		value.ByteLength < 1 || value.ByteLength > MaxVisualFoundationImageBytes ||
		value.PixelWidth < 1 || value.PixelWidth > 16384 || value.PixelHeight < 1 || value.PixelHeight > 16384 ||
		value.PageCount != 1 || value.FrameCount != 1 ||
		(value.RightsBasis != "licensed" && value.RightsBasis != "owned" && value.RightsBasis != "public_domain") ||
		!hashPattern.MatchString(value.RightsRefHash) || !hashPattern.MatchString(value.LineageRefHash) {
		return errors.New("invalid Visual Foundation media attachment")
	}
	return nil
}

type VisualFoundationPayload struct {
	Variant          VisualFoundationStageVariant      `json:"variant"`
	Scope            VisualFoundationScope             `json:"scope"`
	Shard            VisualFoundationShard             `json:"shard"`
	MediaAttachments []VisualFoundationMediaAttachment `json:"media_attachments"`
	StageInput       VisualFoundationInput             `json:"stage_input"`
}

func (value VisualFoundationPayload) Validate() error {
	if value.Variant.Validate() != nil || value.Scope.Validate() != nil ||
		value.Shard.Validate(value.Scope.ProjectID) != nil || value.StageInput.Validate() != nil ||
		value.Scope.WorkspaceID != value.StageInput.WorkspaceID || value.Scope.ProjectID != value.StageInput.ProjectID ||
		len(value.MediaAttachments) > MaxVisualFoundationImages ||
		len(value.MediaAttachments) != len(value.StageInput.ReferenceAttachments) {
		return errors.New("invalid Visual Foundation payload")
	}
	seenAttachments := make(map[string]struct{}, len(value.MediaAttachments))
	seenMediaObjects := make(map[string]struct{}, len(value.MediaAttachments))
	var totalBytes int64
	for index, media := range value.MediaAttachments {
		if media.Validate() != nil {
			return errors.New("invalid Visual Foundation payload")
		}
		if _, exists := seenAttachments[media.AttachmentID]; exists {
			return errors.New("Visual Foundation media identities must be unique")
		}
		if _, exists := seenMediaObjects[media.MediaObjectID]; exists {
			return errors.New("Visual Foundation media identities must be unique")
		}
		seenAttachments[media.AttachmentID] = struct{}{}
		seenMediaObjects[media.MediaObjectID] = struct{}{}
		totalBytes += media.ByteLength
		reference := value.StageInput.ReferenceAttachments[index]
		if media.AttachmentID != reference.AttachmentID || media.ObjectKey != reference.ObjectKey ||
			media.ContentHash != reference.ContentHash || media.MediaType != reference.MediaType ||
			media.RightsBasis != reference.RightsBasis || media.RightsRefHash != reference.RightsRefHash {
			return errors.New("Visual Foundation media manifest drifted from stage input")
		}
	}
	if totalBytes > MaxVisualFoundationTotalImageBytes {
		return errors.New("Visual Foundation media budget exceeded")
	}
	return nil
}

type VisualFoundationInvocation struct {
	InvocationID      string                       `json:"invocation_id"`
	AttemptID         string                       `json:"attempt_id"`
	Kind              string                       `json:"kind"`
	WireSchemaVersion string                       `json:"wire_schema_version"`
	StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
	Control           SceneAnalysisControlProof    `json:"control"`
	Budget            SceneAnalysisExecutionBudget `json:"budget"`
	Payload           VisualFoundationPayload      `json:"payload"`
	InputHash         string                       `json:"input_hash"`
}

func NewVisualFoundationInvocation(
	invocationID, attemptID string,
	release SceneAnalysisReleaseIdentity,
	control SceneAnalysisControlProof,
	budget SceneAnalysisExecutionBudget,
	payload VisualFoundationPayload,
) (VisualFoundationInvocation, error) {
	value := VisualFoundationInvocation{
		InvocationID:      invocationID,
		AttemptID:         attemptID,
		Kind:              "storygraph_stage",
		WireSchemaVersion: SceneAnalysisWireSchemaVersion,
		StageRelease:      release,
		Control:           control,
		Budget:            budget,
		Payload:           payload,
	}
	hash, err := value.ComputeInputHash()
	if err != nil {
		return VisualFoundationInvocation{}, err
	}
	value.InputHash = hash
	if err = value.Validate(); err != nil {
		return VisualFoundationInvocation{}, err
	}
	return value, nil
}

func DecodeVisualFoundationInvocation(raw []byte) (VisualFoundationInvocation, error) {
	var value VisualFoundationInvocation
	if err := decodeStrict(raw, &value); err != nil {
		return VisualFoundationInvocation{}, err
	}
	if err := value.Validate(); err != nil {
		return VisualFoundationInvocation{}, err
	}
	return value, nil
}

func (value VisualFoundationInvocation) Validate() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Visual Foundation invocation identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.StageRelease.Validate() != nil || value.Control.Validate() != nil || value.Budget.Validate() != nil ||
		value.Budget.MaxModelCalls != 1 || value.Budget.MaxExecutionSeconds > 120 || value.Budget.MaxOutputBytes > 131072 ||
		value.Payload.Validate() != nil || !hashPattern.MatchString(value.InputHash) {
		return errors.New("invalid Visual Foundation invocation")
	}
	computed, err := value.ComputeInputHash()
	if err != nil || computed != value.InputHash {
		return errors.New("Visual Foundation input hash mismatch")
	}
	return nil
}

func (value VisualFoundationInvocation) ComputeInputHash() (string, error) {
	material := struct {
		WireSchemaVersion string                       `json:"wire_schema_version"`
		StageRelease      SceneAnalysisReleaseIdentity `json:"stage_release"`
		Control           SceneAnalysisControlProof    `json:"control"`
		Budget            SceneAnalysisExecutionBudget `json:"budget"`
		Payload           VisualFoundationPayload      `json:"payload"`
	}{value.WireSchemaVersion, value.StageRelease, value.Control, value.Budget, value.Payload}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return ProductionCanonicalHash(encoded)
}

func (value VisualFoundationInvocation) StageInstanceKey() string {
	root := struct {
		IdentityContractID string                       `json:"identity_contract_id"`
		VariantKey         VisualFoundationStageVariant `json:"variant_key"`
		Scope              VisualFoundationScope        `json:"scope"`
		ShardManifestHash  string                       `json:"shard_manifest_hash"`
		ShardKey           string                       `json:"shard_key"`
		InputHash          string                       `json:"input_hash"`
	}{
		IdentityContractID: "storygraph-stage-instance-production",
		VariantKey:         value.Payload.Variant,
		Scope:              value.Payload.Scope,
		ShardManifestHash:  value.Payload.Shard.ManifestHash,
		ShardKey:           value.Payload.Shard.ShardKey,
		InputHash:          value.InputHash,
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

func (value SceneAnalysisDispatchAuthorizationClaims) ValidateForVisualFoundation(
	invocation VisualFoundationInvocation,
	claimVersion, nowUnix int64,
) error {
	if claimVersion < 1 || value.InvocationID != invocation.InvocationID || value.AttemptID != invocation.AttemptID ||
		value.InputHash != invocation.InputHash || value.SkillReleaseID != invocation.StageRelease.SkillReleaseID ||
		value.SkillReleaseHash != invocation.StageRelease.SkillReleaseHash ||
		value.StageReleaseHash != invocation.StageRelease.StageReleaseHash ||
		value.BundleContentHash != invocation.StageRelease.BundleContentHash ||
		value.ControlHash != invocation.Control.ControlHash ||
		value.ReleaseFence != invocation.Control.ReleaseFence || value.ClaimVersion != claimVersion ||
		value.AgentImageDigest != invocation.StageRelease.AgentImageDigest || value.ExpiresAt <= nowUnix {
		return errors.New("invalid Visual Foundation dispatch authorization claims")
	}
	return nil
}

type VisualFoundationExecutor struct {
	RuntimeClass       string `json:"runtime_class"`
	RuntimeImageDigest string `json:"runtime_image_digest"`
	HarnessVersion     string `json:"harness_version"`
	Model              string `json:"model"`
}

type VisualFoundationAttemptResult struct {
	InvocationID              string                       `json:"invocation_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Kind                      string                       `json:"kind"`
	WireSchemaVersion         string                       `json:"wire_schema_version"`
	Variant                   VisualFoundationStageVariant `json:"variant"`
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
	Executor                  VisualFoundationExecutor     `json:"executor"`
	Error                     *SceneAnalysisResultError    `json:"error"`
	ResultHash                string                       `json:"result_hash"`
}

func DecodeVisualFoundationAttemptResult(raw []byte) (VisualFoundationAttemptResult, error) {
	var value VisualFoundationAttemptResult
	if err := decodeStrict(raw, &value); err != nil || value.validateShape() != nil {
		return VisualFoundationAttemptResult{}, errors.New("invalid Visual Foundation Attempt Result")
	}
	return value, nil
}

func (value VisualFoundationAttemptResult) ValidateFor(
	invocation VisualFoundationInvocation,
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
		return errors.New("Visual Foundation result identity does not match invocation")
	}
	if value.Status == "accepted" {
		candidate, _, err := DecodeVisualFoundationCandidate(value.Candidate)
		if err != nil || candidate.ValidateFor(invocation.Payload.StageInput) != nil {
			return errors.New("invalid accepted Visual Foundation Candidate")
		}
	}
	return nil
}

func (value VisualFoundationAttemptResult) validateShape() error {
	for _, identifier := range []string{value.InvocationID, value.AttemptID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Visual Foundation result identity")
		}
	}
	if value.Kind != "storygraph_stage" || value.WireSchemaVersion != SceneAnalysisWireSchemaVersion ||
		value.Variant.Validate() != nil || value.StageRelease.Validate() != nil || value.Control.Validate() != nil ||
		value.ClaimVersion < 1 || !hashPattern.MatchString(value.DispatchAuthorizationHash) ||
		value.CandidateType != "visual_foundation_candidate" || !hashPattern.MatchString(value.InputHash) ||
		value.CompletedAt.IsZero() || value.Diagnostics == nil || !hashPattern.MatchString(value.DiagnosticHash) ||
		value.Executor.RuntimeClass != "vision" ||
		value.Executor.RuntimeImageDigest != value.StageRelease.AgentImageDigest ||
		value.Executor.HarnessVersion != "visual-foundation-harness" ||
		strings.TrimSpace(value.Executor.Model) == "" || len(value.Executor.Model) > 200 {
		return errors.New("invalid Visual Foundation result")
	}
	for _, diagnostic := range value.Diagnostics {
		if !candidateReviewCodePattern.MatchString(diagnostic.Code) || !validVisualText(diagnostic.Summary, 800) {
			return errors.New("invalid Visual Foundation diagnostic")
		}
	}
	computedResultHash, err := value.ComputeResultHash()
	if err != nil || !hashPattern.MatchString(value.ResultHash) || computedResultHash != value.ResultHash {
		return errors.New("Visual Foundation result hash mismatch")
	}
	diagnostics, err := json.Marshal(value.Diagnostics)
	if err != nil {
		return err
	}
	diagnosticHash, err := ProductionCanonicalHash(diagnostics)
	if err != nil || diagnosticHash != value.DiagnosticHash {
		return errors.New("Visual Foundation diagnostic hash mismatch")
	}
	switch value.Status {
	case "accepted":
		if value.OutputHash == nil || !jsonObject(value.Candidate) || value.Error != nil {
			return errors.New("accepted Visual Foundation result is incomplete")
		}
		outputHash, hashErr := ProductionCanonicalHash(value.Candidate)
		if hashErr != nil || outputHash != *value.OutputHash {
			return errors.New("Visual Foundation output hash mismatch")
		}
		if _, _, decodeErr := DecodeVisualFoundationCandidate(value.Candidate); decodeErr != nil {
			return errors.New("invalid Visual Foundation Candidate")
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
			return errors.New("failed Visual Foundation result has invalid semantics")
		}
	default:
		return errors.New("invalid Visual Foundation result status")
	}
	return nil
}

func (value VisualFoundationAttemptResult) ComputeResultHash() (string, error) {
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
