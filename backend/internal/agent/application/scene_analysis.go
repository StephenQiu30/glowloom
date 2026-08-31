package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

var ErrNotFound = errors.New("Scene Analysis record not found")

type Error struct {
	Code    string
	Message string
}

func (value *Error) Error() string { return value.Message }

func ErrorCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

type SourceInput struct {
	WorkspaceID          string
	ProjectID            string
	OwnerKind            string
	LogicalID            string
	VersionID            string
	Revision             int64
	ContentHash          string
	CreatedAt            time.Time
	NormalizedText       string
	NewlineNormalization string
	CodepointIndexRule   string
}

type Candidate struct {
	ID                    string          `json:"id"`
	WorkspaceID           string          `json:"workspace_id"`
	ProjectID             string          `json:"project_id"`
	StageKey              string          `json:"stage_key"`
	ProfileKey            string          `json:"profile_key"`
	StageInstanceKey      string          `json:"stage_instance_key"`
	Revision              int64           `json:"revision"`
	CandidateType         string          `json:"candidate_type"`
	Candidate             json.RawMessage `json:"candidate"`
	CandidateContentHash  string          `json:"candidate_content_hash"`
	CandidateRevisionHash string          `json:"candidate_revision_hash"`
	SourceInvocationID    string          `json:"source_invocation_id"`
	SourceResultID        string          `json:"source_result_id"`
	SourceResultHash      string          `json:"source_result_hash"`
	CreatedAt             time.Time       `json:"created_at"`
}

type ExecuteCommand struct {
	WorkflowRunID string
	NodeRunID     string
	StageKey      string
	Source        SourceInput
	Upstream      *Candidate
}

type ReleaseRecord struct {
	ID              string
	Identity        contract.SceneAnalysisReleaseIdentity
	Variant         contract.SceneAnalysisStageVariant
	LoadedResources []string
	CreatedAt       time.Time
	InitialControl  contract.SceneAnalysisControlProof
}

type ManifestRecord struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	StageKey, RootInputHash                   string
	Shards                                    json.RawMessage
	CoverageHash, ManifestHash                string
	CreatedAt                                 time.Time
}

type InvocationRecord struct {
	Invocation                                                  contract.SceneAnalysisInvocation
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID, ReleaseID string
	SourceVersionID, SourceHash                                 string
	UpstreamCandidateRevisionID, UpstreamCandidateRevisionHash  *string
	Manifest                                                    ManifestRecord
	CreatedAt                                                   time.Time
}

type AttemptRecord struct {
	ID, InvocationID, ControlHash, AgentImageDigest string
	ClaimVersion, ReleaseFence                      int64
	DispatchedAt                                    time.Time
}

type DispatchAuthorizationRecord struct {
	AttemptID, AuthorizationHash string
	ExpiresAt, IssuedAt          time.Time
}

type ResultAcceptance struct {
	ResultID, CandidateID string
	Invocation            contract.SceneAnalysisInvocation
	Result                contract.SceneAnalysisAttemptResult
	AcceptedAt            time.Time
}

type Repository interface {
	EnsureRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureManifest(context.Context, ManifestRecord) error
	FindInvocation(context.Context, string, string, string, string) (InvocationRecord, error)
	CreateInvocation(context.Context, InvocationRecord) error
	FindCandidateByInvocation(context.Context, string) (Candidate, error)
	CountAttempts(context.Context, string) (int64, error)
	CreateAttempt(context.Context, AttemptRecord) error
	CreateDispatchAuthorization(context.Context, DispatchAuthorizationRecord) error
	AcceptResult(context.Context, ResultAcceptance) (Candidate, error)
	RecordFailedResult(context.Context, ResultAcceptance) error
	GetCandidate(context.Context, string, string) (Candidate, error)
}

type Transactions interface {
	WithinSceneAnalysisTransaction(context.Context, func(Repository) error) error
}

type Runtime interface {
	InvokeSceneAnalysis(
		context.Context,
		contract.SceneAnalysisInvocation,
		contract.SceneAnalysisDispatchAuthorization,
	) (contract.SceneAnalysisAttemptResult, error)
}

type DispatchAuthorizer interface {
	IssueSceneAnalysisDispatchAuthorization(
		contract.SceneAnalysisInvocation,
		int64,
	) (contract.SceneAnalysisDispatchAuthorization, error)
}

type SceneAnalysisConfig struct {
	Now              func() time.Time
	NewID            func() string
	AgentImageDigest string
	Budget           contract.SceneAnalysisExecutionBudget
}

type SceneAnalysisService struct {
	transactions Transactions
	runtime      Runtime
	authorizer   DispatchAuthorizer
	config       SceneAnalysisConfig
}

func NewSceneAnalysisService(
	transactions Transactions,
	runtime Runtime,
	authorizer DispatchAuthorizer,
	config SceneAnalysisConfig,
) (*SceneAnalysisService, error) {
	if transactions == nil || runtime == nil || authorizer == nil ||
		!strings.HasPrefix(config.AgentImageDigest, "sha256:") {
		return nil, errors.New("Scene Analysis dependencies are required")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	if config.Budget == (contract.SceneAnalysisExecutionBudget{}) {
		config.Budget = contract.SceneAnalysisExecutionBudget{
			MaxAttempts: 2, MaxModelCalls: 1, MaxExecutionSeconds: 120, MaxOutputBytes: 131072,
		}
	}
	if config.Budget.Validate() != nil {
		return nil, errors.New("Scene Analysis execution budget is invalid")
	}
	return &SceneAnalysisService{
		transactions: transactions, runtime: runtime, authorizer: authorizer, config: config,
	}, nil
}

func (service *SceneAnalysisService) Execute(ctx context.Context, command ExecuteCommand) (Candidate, error) {
	if err := validateExecuteCommand(command); err != nil {
		return Candidate{}, err
	}
	now := service.config.Now().UTC()
	release, err := service.release(command.StageKey, now)
	if err != nil {
		return Candidate{}, err
	}
	manifest, err := buildManifest(command, now)
	if err != nil {
		return Candidate{}, err
	}

	var invocation contract.SceneAnalysisInvocation
	var claimVersion int64
	var authorization contract.SceneAnalysisDispatchAuthorization
	var completed Candidate
	err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		control, ensureErr := repo.EnsureRelease(ctx, release)
		if ensureErr != nil {
			return ensureErr
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Scene Analysis release is not approved"}
		}
		if ensureErr = repo.EnsureManifest(ctx, manifest); ensureErr != nil {
			return ensureErr
		}
		proposedInvocationID, proposedAttemptID := service.config.NewID(), service.config.NewID()
		proposed, buildErr := buildInvocation(command, release, control, manifest, proposedInvocationID, proposedAttemptID, service.config.Budget)
		if buildErr != nil {
			return buildErr
		}
		existing, findErr := repo.FindInvocation(ctx, command.WorkflowRunID, command.NodeRunID, command.StageKey, proposed.InputHash)
		invocationID := proposedInvocationID
		if findErr == nil {
			invocationID = existing.Invocation.InvocationID
			if existing.Invocation.StageRelease != release.Identity || existing.Invocation.Control != control ||
				existing.Invocation.InputHash != proposed.InputHash {
				return &Error{Code: "invocation_fence_drift", Message: "Scene Analysis invocation fence drifted"}
			}
			candidate, candidateErr := repo.FindCandidateByInvocation(ctx, invocationID)
			if candidateErr == nil {
				completed = candidate
				return nil
			}
			if !errors.Is(candidateErr, ErrNotFound) {
				return candidateErr
			}
			if existing.Invocation.InputHash != proposed.InputHash {
				return &Error{Code: "invocation_input_drift", Message: "Scene Analysis invocation input drifted"}
			}
		} else if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		attempts, countErr := repo.CountAttempts(ctx, invocationID)
		if countErr != nil && !errors.Is(countErr, ErrNotFound) {
			return countErr
		}
		if attempts >= int64(service.config.Budget.MaxAttempts) {
			return &Error{Code: "attempt_budget_exhausted", Message: "Scene Analysis attempt budget is exhausted"}
		}
		claimVersion = attempts + 1
		attemptID := service.config.NewID()
		invocation, buildErr = buildInvocation(command, release, control, manifest, invocationID, attemptID, service.config.Budget)
		if buildErr != nil {
			return buildErr
		}
		if errors.Is(findErr, ErrNotFound) {
			upstreamID, upstreamHash := upstreamPointers(command.Upstream)
			if createErr := repo.CreateInvocation(ctx, InvocationRecord{
				Invocation: invocation, WorkspaceID: command.Source.WorkspaceID, ProjectID: command.Source.ProjectID,
				WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID, ReleaseID: release.ID,
				SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
				UpstreamCandidateRevisionID: upstreamID, UpstreamCandidateRevisionHash: upstreamHash,
				Manifest: manifest, CreatedAt: now,
			}); createErr != nil {
				return createErr
			}
		}
		if createErr := repo.CreateAttempt(ctx, AttemptRecord{
			ID: attemptID, InvocationID: invocationID, ClaimVersion: claimVersion,
			ControlHash: control.ControlHash, ReleaseFence: control.ReleaseFence,
			AgentImageDigest: release.Identity.AgentImageDigest, DispatchedAt: now,
		}); createErr != nil {
			return createErr
		}
		authorization, buildErr = service.authorizer.IssueSceneAnalysisDispatchAuthorization(
			invocation,
			claimVersion,
		)
		if buildErr != nil {
			return buildErr
		}
		return repo.CreateDispatchAuthorization(ctx, DispatchAuthorizationRecord{
			AttemptID: attemptID, AuthorizationHash: authorization.Hash,
			ExpiresAt: authorization.ExpiresAt, IssuedAt: now,
		})
	})
	if err != nil || completed.ID != "" {
		return completed, err
	}

	result, invokeErr := service.runtime.InvokeSceneAnalysis(ctx, invocation, authorization)
	if invokeErr != nil || result.ValidateFor(invocation, claimVersion, authorization.Hash) != nil {
		resultErrorCode := "agent_outcome_unknown"
		resultSummary := "Scene Analysis outcome could not be confirmed"
		if errors.Is(invokeErr, contract.ErrSkillBundleUnavailable) {
			resultErrorCode = "skill_bundle_unavailable"
			resultSummary = "Frozen Scene Analysis skill bundle is unavailable"
		}
		result, err = outcomeUnknownResult(
			invocation,
			authorization,
			service.config.Now().UTC(),
			resultErrorCode,
			resultSummary,
		)
		if err != nil {
			return Candidate{}, err
		}
		acceptance := ResultAcceptance{
			ResultID: service.config.NewID(), CandidateID: service.config.NewID(), Invocation: invocation,
			Result: result, AcceptedAt: result.CompletedAt,
		}
		err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
			return repo.RecordFailedResult(ctx, acceptance)
		})
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, &Error{Code: resultErrorCode, Message: resultSummary}
	}
	acceptance := ResultAcceptance{
		ResultID: service.config.NewID(), CandidateID: service.config.NewID(), Invocation: invocation,
		Result: result, AcceptedAt: service.config.Now().UTC(),
	}
	if result.Status != "accepted" {
		recordErr := service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
			return repo.RecordFailedResult(ctx, acceptance)
		})
		if recordErr != nil {
			return Candidate{}, recordErr
		}
		return Candidate{}, &Error{Code: result.Error.Code, Message: result.Error.SafeSummary}
	}
	err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		var acceptErr error
		completed, acceptErr = repo.AcceptResult(ctx, acceptance)
		return acceptErr
	})
	return completed, err
}

func outcomeUnknownResult(
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	completedAt time.Time,
	errorCode string,
	safeSummary string,
) (contract.SceneAnalysisAttemptResult, error) {
	diagnostics := []contract.SceneAnalysisDiagnostic{{
		Code: errorCode, Summary: safeSummary,
	}}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(encoded)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	result := contract.SceneAnalysisAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID, Kind: "storygraph_stage",
		WireSchemaVersion: invocation.WireSchemaVersion, Variant: invocation.Payload.Variant,
		StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "outcome_unknown",
		CandidateType: map[string]string{
			"propose_script_spans": "script_span_candidate",
			"extract_scene_facts":  "scene_fact_candidate",
		}[invocation.Payload.Variant.StageKey],
		InputHash: invocation.InputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: completedAt,
		Executor: contract.SceneAnalysisExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "scene-analysis-harness", Model: "unconfirmed",
		},
		Error: &contract.SceneAnalysisResultError{
			Code: errorCode, SafeSummary: safeSummary, RetryClass: "same_release",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}

func (service *SceneAnalysisService) GetCandidate(ctx context.Context, projectID, candidateID string) (Candidate, error) {
	var candidate Candidate
	err := service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		var queryErr error
		candidate, queryErr = repo.GetCandidate(ctx, projectID, candidateID)
		return queryErr
	})
	return candidate, err
}

func validateExecuteCommand(command ExecuteCommand) error {
	for _, identifier := range []string{
		command.WorkflowRunID, command.NodeRunID, command.Source.WorkspaceID, command.Source.ProjectID,
		command.Source.LogicalID, command.Source.VersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return &Error{Code: "invalid_scene_analysis_command", Message: "Scene Analysis command has an invalid identity"}
		}
	}
	if command.Source.OwnerKind != "production/script" || command.Source.Revision < 1 ||
		command.Source.CreatedAt.IsZero() || command.Source.NewlineNormalization != "lf" ||
		command.Source.CodepointIndexRule != "unicode-code-point" || command.Source.NormalizedText == "" ||
		strings.Contains(command.Source.NormalizedText, "\r") || !utf8.ValidString(command.Source.NormalizedText) ||
		len(command.Source.ContentHash) != 64 {
		return &Error{Code: "invalid_scene_analysis_source", Message: "Scene Analysis source is invalid"}
	}
	if command.StageKey == "propose_script_spans" {
		if command.Upstream != nil {
			return &Error{Code: "unexpected_upstream_candidate", Message: "ScriptSpan stage cannot read an upstream candidate"}
		}
		return nil
	}
	if command.StageKey != "extract_scene_facts" || command.Upstream == nil ||
		command.Upstream.ProjectID != command.Source.ProjectID ||
		command.Upstream.CandidateType != "script_span_candidate" ||
		contract.ValidateScriptSpanCandidate(command.Upstream.Candidate, command.Source.NormalizedText) != nil {
		return &Error{Code: "invalid_upstream_candidate", Message: "SceneFact stage requires one exact ScriptSpan candidate"}
	}
	return nil
}

func (service *SceneAnalysisService) release(stageKey string, now time.Time) (ReleaseRecord, error) {
	outputSchemaVersion := map[string]string{
		"propose_script_spans": contract.ScriptSpanCandidateSchemaVersion,
		"extract_scene_facts":  contract.SceneFactCandidateSchemaVersion,
	}[stageKey]
	variant := contract.SceneAnalysisStageVariant{
		StageKey: stageKey, ProfileKey: "default", LaneKey: "primary",
		OutputSchemaVersion: outputSchemaVersion,
	}
	if variant.Validate() != nil {
		return ReleaseRecord{}, errors.New("unsupported Scene Analysis stage")
	}
	skillMaterial, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-skill-release", "bundle_content_hash": contract.SceneAnalysisSkillBundleHash,
		"agent_image_digest":  service.config.AgentImageDigest,
		"wire_schema_version": contract.SceneAnalysisWireSchemaVersion,
		"lane_key":            "primary",
		"output_schema_versions": []string{
			contract.SceneFactCandidateSchemaVersion,
			contract.ScriptSpanCandidateSchemaVersion,
		},
	})
	skillHash, err := platformcanonical.Hash(skillMaterial)
	if err != nil {
		return ReleaseRecord{}, err
	}
	skillID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:skill:"+skillHash)).String()
	stageMaterial, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-stage-release", "variant": variant,
		"skill_release_id": skillID, "skill_release_hash": skillHash,
		"bundle_content_hash": contract.SceneAnalysisSkillBundleHash, "agent_image_digest": service.config.AgentImageDigest,
	})
	stageHash, err := platformcanonical.Hash(stageMaterial)
	if err != nil {
		return ReleaseRecord{}, err
	}
	releaseID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:stage:"+stageHash)).String()
	controlRecordID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:control:"+stageHash)).String()
	controlMaterial, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-control", "control_record_id": controlRecordID,
		"release_id": releaseID, "stage_release_hash": stageHash, "control_revision": 1,
		"status": "approved", "release_fence": 0,
	})
	controlHash, err := platformcanonical.Hash(controlMaterial)
	if err != nil {
		return ReleaseRecord{}, err
	}
	resource := map[string]string{
		"propose_script_spans": "references/script-spans.md",
		"extract_scene_facts":  "references/scene-facts.md",
	}[stageKey]
	return ReleaseRecord{
		ID: releaseID,
		Identity: contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID: skillID, SkillReleaseHash: skillHash, StageReleaseHash: stageHash,
			BundleContentHash: contract.SceneAnalysisSkillBundleHash, AgentImageDigest: service.config.AgentImageDigest,
		},
		Variant: variant, LoadedResources: []string{"SKILL.md", resource}, CreatedAt: now,
		InitialControl: contract.SceneAnalysisControlProof{
			ControlRecordID: controlRecordID, ControlRevision: 1, Status: "approved",
			ControlHash: controlHash, ReleaseFence: 0,
		},
	}, nil
}

func buildManifest(command ExecuteCommand, now time.Time) (ManifestRecord, error) {
	rootInputHash := command.Source.ContentHash
	if command.Upstream != nil {
		rootInputHash = command.Upstream.CandidateRevisionHash
	}
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf(
		"lanverse:scene-analysis:manifest:%s:%s:%s", command.NodeRunID, command.StageKey, rootInputHash,
	))).String()
	shardKey := "script:full"
	shards, _ := json.Marshal([]map[string]any{{
		"shard_key": shardKey, "codepoint_start": 0, "codepoint_end": utf8.RuneCountInString(command.Source.NormalizedText),
	}})
	coverageHash, err := platformcanonical.Hash(shards)
	if err != nil {
		return ManifestRecord{}, err
	}
	material, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-shard-manifest", "manifest_id": manifestID,
		"workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": command.StageKey, "root_input_hash": rootInputHash,
		"shards": json.RawMessage(shards), "coverage_hash": coverageHash,
	})
	manifestHash, err := platformcanonical.Hash(material)
	if err != nil {
		return ManifestRecord{}, err
	}
	return ManifestRecord{
		ID: manifestID, WorkspaceID: command.Source.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		NodeRunID: command.NodeRunID, StageKey: command.StageKey, RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now,
	}, nil
}

func buildInvocation(
	command ExecuteCommand,
	release ReleaseRecord,
	control contract.SceneAnalysisControlProof,
	manifest ManifestRecord,
	invocationID, attemptID string,
	budget contract.SceneAnalysisExecutionBudget,
) (contract.SceneAnalysisInvocation, error) {
	text := command.Source.NormalizedText
	payload := contract.SceneAnalysisPayload{
		Variant: release.Variant,
		Scope:   contract.SceneAnalysisScope{WorkspaceID: command.Source.WorkspaceID, ProjectID: command.Source.ProjectID},
		SourceRefs: []contract.ScriptSourceVersionIdentity{{
			OwnerKind: command.Source.OwnerKind, LogicalID: command.Source.LogicalID, VersionID: command.Source.VersionID,
			Revision: command.Source.Revision, ContentHash: command.Source.ContentHash, CreatedAt: command.Source.CreatedAt.UTC(),
		}},
		Shard: contract.SceneAnalysisShard{
			ManifestID: manifest.ID, ManifestHash: manifest.ManifestHash, ShardKey: "script:full",
			CodepointStart: 0, CodepointEnd: utf8.RuneCountInString(text),
		},
	}
	if command.StageKey == "propose_script_spans" {
		payload.UpstreamCandidates = []contract.ScriptSpanRevisionIdentity{}
		payload.StageInput, _ = json.Marshal(contract.ScriptSpanProposalInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText: text, CodepointCount: utf8.RuneCountInString(text), NewlineNormalization: "lf",
		})
	} else {
		payload.UpstreamCandidates = []contract.ScriptSpanRevisionIdentity{{
			StageKey: "propose_script_spans", ShardKey: "script:full",
			CandidateRevisionID: command.Upstream.ID, CandidateRevisionHash: command.Upstream.CandidateRevisionHash,
			SourceInvocationID: command.Upstream.SourceInvocationID, SourceResultHash: command.Upstream.SourceResultHash,
		}}
		payload.StageInput, _ = json.Marshal(contract.SceneFactExtractionInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText: text, SpanCandidateRevisionID: command.Upstream.ID,
			SpanCandidateRevisionHash: command.Upstream.CandidateRevisionHash,
			SpanCandidate:             command.Upstream.Candidate,
		})
	}
	return contract.NewSceneAnalysisInvocation(invocationID, attemptID, release.Identity, control, budget, payload)
}

func upstreamPointers(candidate *Candidate) (*string, *string) {
	if candidate == nil {
		return nil, nil
	}
	id, hash := candidate.ID, candidate.CandidateRevisionHash
	return &id, &hash
}
