package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ExecuteReferenceBriefCommand struct {
	WorkflowRunID string
	NodeRunID     string
	Input         contract.ReferenceBriefInput
}

type ReferenceBriefInvocationRecord struct {
	Invocation                                       contract.ReferenceBriefInvocation
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	ReleaseID                                        string
	Manifest                                         ManifestRecord
	CreatedAt                                        time.Time
}

type ReferenceBriefResultAcceptance struct {
	ResultID, CandidateID string
	Invocation            ReferenceBriefInvocationRecord
	Result                contract.ReferenceBriefAttemptResult
	AcceptedAt            time.Time
}

type ReferenceBriefRepository interface {
	ValidateReferenceBriefInput(context.Context, contract.ReferenceBriefInput) error
	EnsureReferenceBriefRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureReferenceBriefManifest(context.Context, ManifestRecord) error
	FindReferenceBriefInvocation(context.Context, string, string, string) (ReferenceBriefInvocationRecord, error)
	CreateReferenceBriefInvocation(context.Context, ReferenceBriefInvocationRecord) error
	FindReferenceBriefCandidateByInvocation(context.Context, string) (Candidate, error)
	CountReferenceBriefAttempts(context.Context, string) (int64, error)
	CreateReferenceBriefAttempt(context.Context, AttemptRecord) error
	CreateReferenceBriefDispatchAuthorization(context.Context, DispatchAuthorizationRecord) error
	AcceptReferenceBriefResult(context.Context, ReferenceBriefResultAcceptance) (Candidate, error)
	RecordFailedReferenceBriefResult(context.Context, ReferenceBriefResultAcceptance) error
}

type ReferenceBriefTransactions interface {
	WithinReferenceBriefTransaction(context.Context, func(ReferenceBriefRepository) error) error
}

type ReferenceBriefInvoker interface {
	InvokeReferenceBrief(
		context.Context,
		contract.ReferenceBriefInvocation,
		contract.SceneAnalysisDispatchAuthorization,
	) (contract.ReferenceBriefAttemptResult, error)
}

type ReferenceBriefDispatchAuthorizer interface {
	IssueReferenceBriefDispatchAuthorization(
		contract.ReferenceBriefInvocation,
		int64,
	) (contract.SceneAnalysisDispatchAuthorization, error)
}

type ReferenceBriefExecutionConfig struct {
	Now              func() time.Time
	NewID            func() string
	AgentImageDigest string
	Budget           contract.SceneAnalysisExecutionBudget
}

type ReferenceBriefExecutionService struct {
	transactions ReferenceBriefTransactions
	invoker      ReferenceBriefInvoker
	authorizer   ReferenceBriefDispatchAuthorizer
	config       ReferenceBriefExecutionConfig
}

func NewReferenceBriefExecutionService(
	transactions ReferenceBriefTransactions,
	invoker ReferenceBriefInvoker,
	authorizer ReferenceBriefDispatchAuthorizer,
	config ReferenceBriefExecutionConfig,
) (*ReferenceBriefExecutionService, error) {
	if transactions == nil || invoker == nil || authorizer == nil ||
		!strings.HasPrefix(config.AgentImageDigest, "sha256:") {
		return nil, errors.New("Reference Brief execution dependencies are required")
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
	if config.Budget.Validate() != nil || config.Budget.MaxModelCalls != 1 ||
		config.Budget.MaxExecutionSeconds > 120 || config.Budget.MaxOutputBytes > 131072 {
		return nil, errors.New("Reference Brief execution budget is invalid")
	}
	return &ReferenceBriefExecutionService{
		transactions: transactions, invoker: invoker, authorizer: authorizer, config: config,
	}, nil
}

func (service *ReferenceBriefExecutionService) Execute(
	ctx context.Context,
	command ExecuteReferenceBriefCommand,
) (Candidate, error) {
	if err := validateExecuteReferenceBriefCommand(command); err != nil {
		return Candidate{}, err
	}
	now := service.config.Now().UTC()
	release, err := BuildStageReleaseRecord(contract.ReferenceBriefStageKey, service.config.AgentImageDigest, now)
	if err != nil {
		return Candidate{}, err
	}
	if command.Input.StageRelease.StageReleaseHash != release.Identity.StageReleaseHash {
		return Candidate{}, &Error{Code: "invalid_reference_brief_input", Message: "Reference Brief input release is not executable"}
	}
	manifest, impactClosureHash, err := buildReferenceBriefManifest(command, now)
	if err != nil {
		return Candidate{}, err
	}

	var invocationRecord ReferenceBriefInvocationRecord
	var authorization contract.SceneAnalysisDispatchAuthorization
	var claimVersion int64
	var completed Candidate
	err = service.transactions.WithinReferenceBriefTransaction(ctx, func(repo ReferenceBriefRepository) error {
		if validateErr := repo.ValidateReferenceBriefInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		control, ensureErr := repo.EnsureReferenceBriefRelease(ctx, release)
		if ensureErr != nil {
			return ensureErr
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Reference Brief release is not approved"}
		}
		if ensureErr = repo.EnsureReferenceBriefManifest(ctx, manifest); ensureErr != nil {
			return ensureErr
		}
		proposedInvocationID, proposedAttemptID := service.config.NewID(), service.config.NewID()
		proposed, buildErr := buildReferenceBriefInvocation(
			command, release, control, manifest, impactClosureHash,
			proposedInvocationID, proposedAttemptID, service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		existing, findErr := repo.FindReferenceBriefInvocation(
			ctx, command.WorkflowRunID, command.NodeRunID, proposed.InputHash,
		)
		invocationID := proposedInvocationID
		if findErr == nil {
			invocationID = existing.Invocation.InvocationID
			if existing.Invocation.StageRelease != release.Identity || existing.Invocation.Control != control ||
				existing.Invocation.InputHash != proposed.InputHash ||
				!reflect.DeepEqual(existing.Invocation.Payload, proposed.Payload) {
				return &Error{Code: "invocation_fence_drift", Message: "Reference Brief invocation fence drifted"}
			}
			candidate, candidateErr := repo.FindReferenceBriefCandidateByInvocation(ctx, invocationID)
			if candidateErr == nil {
				completed = candidate
				return nil
			}
			if !errors.Is(candidateErr, ErrNotFound) {
				return candidateErr
			}
		} else if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		attempts, countErr := repo.CountReferenceBriefAttempts(ctx, invocationID)
		if countErr != nil && !errors.Is(countErr, ErrNotFound) {
			return countErr
		}
		if attempts >= int64(service.config.Budget.MaxAttempts) {
			return &Error{Code: "attempt_budget_exhausted", Message: "Reference Brief attempt budget is exhausted"}
		}
		claimVersion = attempts + 1
		attemptID := proposedAttemptID
		if findErr == nil {
			attemptID = service.config.NewID()
		}
		invocation, buildErr := buildReferenceBriefInvocation(
			command, release, control, manifest, impactClosureHash,
			invocationID, attemptID, service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		invocationRecord = ReferenceBriefInvocationRecord{
			Invocation: invocation, WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
			WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			ReleaseID: release.ID, Manifest: manifest, CreatedAt: now,
		}
		if errors.Is(findErr, ErrNotFound) {
			if createErr := repo.CreateReferenceBriefInvocation(ctx, invocationRecord); createErr != nil {
				return createErr
			}
		}
		if createErr := repo.CreateReferenceBriefAttempt(ctx, AttemptRecord{
			ID: attemptID, InvocationID: invocationID, ClaimVersion: claimVersion,
			ControlHash: control.ControlHash, ReleaseFence: control.ReleaseFence,
			AgentImageDigest: release.Identity.AgentImageDigest, DispatchedAt: now,
		}); createErr != nil {
			return createErr
		}
		authorization, buildErr = service.authorizer.IssueReferenceBriefDispatchAuthorization(invocation, claimVersion)
		if buildErr != nil {
			return buildErr
		}
		return repo.CreateReferenceBriefDispatchAuthorization(ctx, DispatchAuthorizationRecord{
			AttemptID: attemptID, AuthorizationHash: authorization.Hash,
			ExpiresAt: authorization.ExpiresAt, IssuedAt: now,
		})
	})
	if err != nil || completed.ID != "" {
		return completed, err
	}

	result, invokeErr := service.invoker.InvokeReferenceBrief(ctx, invocationRecord.Invocation, authorization)
	if invokeErr != nil || result.ValidateFor(invocationRecord.Invocation, claimVersion, authorization.Hash) != nil {
		errorCode := "agent_outcome_unknown"
		summary := "Reference Brief outcome could not be confirmed"
		if errors.Is(invokeErr, contract.ErrSkillBundleUnavailable) {
			errorCode = "skill_bundle_unavailable"
			summary = "Frozen Reference Brief skill bundle is unavailable"
		}
		result, err = referenceBriefOutcomeUnknownResult(
			invocationRecord.Invocation, authorization, service.config.Now().UTC(), errorCode, summary,
		)
		if err != nil {
			return Candidate{}, err
		}
		acceptance := ReferenceBriefResultAcceptance{
			ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
			Invocation: invocationRecord, Result: result, AcceptedAt: result.CompletedAt,
		}
		err = service.transactions.WithinReferenceBriefTransaction(ctx, func(repo ReferenceBriefRepository) error {
			return repo.RecordFailedReferenceBriefResult(ctx, acceptance)
		})
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, &Error{Code: errorCode, Message: summary}
	}
	acceptance := ReferenceBriefResultAcceptance{
		ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
		Invocation: invocationRecord, Result: result, AcceptedAt: service.config.Now().UTC(),
	}
	if result.Status != "accepted" {
		recordErr := service.transactions.WithinReferenceBriefTransaction(ctx, func(repo ReferenceBriefRepository) error {
			return repo.RecordFailedReferenceBriefResult(ctx, acceptance)
		})
		if recordErr != nil {
			return Candidate{}, recordErr
		}
		return Candidate{}, &Error{Code: result.Error.Code, Message: result.Error.SafeSummary}
	}
	err = service.transactions.WithinReferenceBriefTransaction(ctx, func(repo ReferenceBriefRepository) error {
		if validateErr := repo.ValidateReferenceBriefInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		var acceptErr error
		completed, acceptErr = repo.AcceptReferenceBriefResult(ctx, acceptance)
		return acceptErr
	})
	return completed, err
}

func validateExecuteReferenceBriefCommand(command ExecuteReferenceBriefCommand) error {
	for _, identifier := range []string{command.WorkflowRunID, command.NodeRunID} {
		parsed, err := uuid.Parse(identifier)
		if err != nil || parsed == uuid.Nil {
			return &Error{Code: "invalid_reference_brief_command", Message: "Reference Brief command has an invalid identity"}
		}
	}
	if command.Input.Validate() != nil {
		return &Error{Code: "invalid_reference_brief_input", Message: "Reference Brief input is invalid"}
	}
	return nil
}

func buildReferenceBriefManifest(
	command ExecuteReferenceBriefCommand,
	now time.Time,
) (ManifestRecord, string, error) {
	inputJSON, err := json.Marshal(command.Input)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	rootInputHash, err := platformcanonical.Hash(inputJSON)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	impactMaterial, err := json.Marshal(struct {
		ContractID              string `json:"contract_id"`
		ApprovedPlanContentHash string `json:"approved_plan_content_hash"`
		TargetBusinessKey       string `json:"target_business_key"`
		TargetContentHash       string `json:"target_content_hash"`
		TypedReadSetRoot        string `json:"typed_read_set_root"`
	}{
		ContractID:              "reference-brief-impact-closure-production",
		ApprovedPlanContentHash: command.Input.ApprovedReferencePlanVersionRef.OwnerContentHash,
		TargetBusinessKey:       command.Input.TargetBusinessKey,
		TargetContentHash:       command.Input.ReferencePlanTargetRef.OwnerContentHash,
		TypedReadSetRoot:        command.Input.TypedReadSetRoot,
	})
	if err != nil {
		return ManifestRecord{}, "", err
	}
	impactClosureHash, err := platformcanonical.Hash(impactMaterial)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf(
		"lanverse:reference-brief:manifest:%s:%s", command.NodeRunID, rootInputHash,
	))).String()
	shardKey := "reference_target:" + command.Input.TargetBusinessKey
	shards, err := json.Marshal([]map[string]any{{
		"shard_key": shardKey, "impact_closure_hash": impactClosureHash,
	}})
	if err != nil {
		return ManifestRecord{}, "", err
	}
	coverageHash, err := platformcanonical.Hash(shards)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	material, err := json.Marshal(map[string]any{
		"contract_id": "reference-brief-shard-manifest-production", "manifest_id": manifestID,
		"workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": contract.ReferenceBriefStageKey, "root_input_hash": rootInputHash,
		"shards": json.RawMessage(shards), "coverage_hash": coverageHash,
	})
	if err != nil {
		return ManifestRecord{}, "", err
	}
	manifestHash, err := platformcanonical.Hash(material)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	return ManifestRecord{
		ID: manifestID, WorkspaceID: command.Input.WorkspaceID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		StageKey: contract.ReferenceBriefStageKey, RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now,
	}, impactClosureHash, nil
}

func buildReferenceBriefInvocation(
	command ExecuteReferenceBriefCommand,
	release ReleaseRecord,
	control contract.SceneAnalysisControlProof,
	manifest ManifestRecord,
	impactClosureHash string,
	invocationID string,
	attemptID string,
	budget contract.SceneAnalysisExecutionBudget,
) (contract.ReferenceBriefInvocation, error) {
	return contract.NewReferenceBriefInvocation(
		invocationID,
		attemptID,
		release.Identity,
		control,
		budget,
		contract.ReferenceBriefPayload{
			Variant: contract.ReferenceBriefStageVariant{
				StageKey: contract.ReferenceBriefStageKey, ProfileKey: "default", LaneKey: "primary",
				OutputSchemaVersion: contract.ReferenceBriefCandidateContractID,
			},
			Scope: contract.ReferenceBriefScope{
				WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
				TargetBusinessKey: command.Input.TargetBusinessKey,
			},
			Shard: contract.ReferenceBriefShard{
				ManifestID: manifest.ID, ManifestHash: manifest.ManifestHash,
				ShardKey:          "reference_target:" + command.Input.TargetBusinessKey,
				ImpactClosureHash: impactClosureHash,
			},
			StageInput: command.Input,
		},
	)
}

func referenceBriefOutcomeUnknownResult(
	invocation contract.ReferenceBriefInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	completedAt time.Time,
	errorCode string,
	safeSummary string,
) (contract.ReferenceBriefAttemptResult, error) {
	diagnostics := []contract.SceneAnalysisDiagnostic{{Code: errorCode, Summary: safeSummary}}
	diagnosticsJSON, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.ReferenceBriefAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticsJSON)
	if err != nil {
		return contract.ReferenceBriefAttemptResult{}, err
	}
	result := contract.ReferenceBriefAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "outcome_unknown", CandidateType: "reference_brief_candidate",
		InputHash: invocation.InputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: completedAt,
		Executor: contract.ReferenceBriefExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-brief-harness", Model: "unconfirmed",
		},
		Error: &contract.SceneAnalysisResultError{
			Code: errorCode, SafeSummary: safeSummary, RetryClass: "same_release",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.ReferenceBriefAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}
