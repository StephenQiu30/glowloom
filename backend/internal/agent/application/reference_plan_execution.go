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

type ExecuteReferencePlanCommand struct {
	WorkflowRunID string
	NodeRunID     string
	Input         contract.ReferencePlanInput
}

type ReferencePlanInvocationRecord struct {
	Invocation                                       contract.ReferencePlanInvocation
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	ReleaseID                                        string
	Manifest                                         ManifestRecord
	CreatedAt                                        time.Time
}

type ReferencePlanResultAcceptance struct {
	ResultID, CandidateID string
	Invocation            ReferencePlanInvocationRecord
	Result                contract.ReferencePlanAttemptResult
	AcceptedAt            time.Time
}

type ReferencePlanRepository interface {
	ValidateReferencePlanInput(context.Context, contract.ReferencePlanInput) error
	EnsureReferencePlanRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureReferencePlanManifest(context.Context, ManifestRecord) error
	FindReferencePlanInvocation(context.Context, string, string, string) (ReferencePlanInvocationRecord, error)
	CreateReferencePlanInvocation(context.Context, ReferencePlanInvocationRecord) error
	FindReferencePlanCandidateByInvocation(context.Context, string) (Candidate, error)
	CountReferencePlanAttempts(context.Context, string) (int64, error)
	CreateReferencePlanAttempt(context.Context, AttemptRecord) error
	CreateReferencePlanDispatchAuthorization(context.Context, DispatchAuthorizationRecord) error
	AcceptReferencePlanResult(context.Context, ReferencePlanResultAcceptance) (Candidate, error)
	RecordFailedReferencePlanResult(context.Context, ReferencePlanResultAcceptance) error
}

type ReferencePlanTransactions interface {
	WithinReferencePlanTransaction(context.Context, func(ReferencePlanRepository) error) error
}

type ReferencePlanInvoker interface {
	InvokeReferencePlan(
		context.Context,
		contract.ReferencePlanInvocation,
		contract.SceneAnalysisDispatchAuthorization,
	) (contract.ReferencePlanAttemptResult, error)
}

type ReferencePlanDispatchAuthorizer interface {
	IssueReferencePlanDispatchAuthorization(
		contract.ReferencePlanInvocation,
		int64,
	) (contract.SceneAnalysisDispatchAuthorization, error)
}

type ReferencePlanExecutionConfig struct {
	Now               func() time.Time
	NewID             func() string
	AgentImageDigest  string
	Budget            contract.SceneAnalysisExecutionBudget
	ValidateCandidate func(contract.ReferencePlanInput, json.RawMessage) error
}

type ReferencePlanExecutionService struct {
	transactions ReferencePlanTransactions
	invoker      ReferencePlanInvoker
	authorizer   ReferencePlanDispatchAuthorizer
	config       ReferencePlanExecutionConfig
}

func NewReferencePlanExecutionService(
	transactions ReferencePlanTransactions,
	invoker ReferencePlanInvoker,
	authorizer ReferencePlanDispatchAuthorizer,
	config ReferencePlanExecutionConfig,
) (*ReferencePlanExecutionService, error) {
	if transactions == nil || invoker == nil || authorizer == nil ||
		!strings.HasPrefix(config.AgentImageDigest, "sha256:") {
		return nil, errors.New("Reference Plan execution dependencies are required")
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
		return nil, errors.New("Reference Plan execution budget is invalid")
	}
	if config.ValidateCandidate == nil {
		config.ValidateCandidate = func(input contract.ReferencePlanInput, candidateBytes json.RawMessage) error {
			candidate, _, decodeErr := contract.DecodeReferencePlanCandidate(candidateBytes)
			if decodeErr != nil {
				return decodeErr
			}
			return candidate.ValidateFor(input)
		}
	}
	return &ReferencePlanExecutionService{
		transactions: transactions, invoker: invoker, authorizer: authorizer, config: config,
	}, nil
}

func (service *ReferencePlanExecutionService) Execute(
	ctx context.Context,
	command ExecuteReferencePlanCommand,
) (Candidate, error) {
	if err := validateExecuteReferencePlanCommand(command); err != nil {
		return Candidate{}, err
	}
	now := service.config.Now().UTC()
	release, err := BuildStageReleaseRecord(contract.ReferencePlanStageKey, service.config.AgentImageDigest, now)
	if err != nil {
		return Candidate{}, err
	}
	manifest, impactClosureHash, err := buildReferencePlanManifest(command, now)
	if err != nil {
		return Candidate{}, err
	}

	var invocationRecord ReferencePlanInvocationRecord
	var authorization contract.SceneAnalysisDispatchAuthorization
	var claimVersion int64
	var completed Candidate
	err = service.transactions.WithinReferencePlanTransaction(ctx, func(repo ReferencePlanRepository) error {
		if validateErr := repo.ValidateReferencePlanInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		control, ensureErr := repo.EnsureReferencePlanRelease(ctx, release)
		if ensureErr != nil {
			return ensureErr
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Reference Plan release is not approved"}
		}
		if ensureErr = repo.EnsureReferencePlanManifest(ctx, manifest); ensureErr != nil {
			return ensureErr
		}
		proposedInvocationID, proposedAttemptID := service.config.NewID(), service.config.NewID()
		proposed, buildErr := buildReferencePlanInvocation(
			command, release, control, manifest, impactClosureHash,
			proposedInvocationID, proposedAttemptID, service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		existing, findErr := repo.FindReferencePlanInvocation(
			ctx, command.WorkflowRunID, command.NodeRunID, proposed.InputHash,
		)
		invocationID := proposedInvocationID
		if findErr == nil {
			invocationID = existing.Invocation.InvocationID
			if existing.Invocation.StageRelease != release.Identity || existing.Invocation.Control != control ||
				existing.Invocation.InputHash != proposed.InputHash ||
				!reflect.DeepEqual(existing.Invocation.Payload, proposed.Payload) {
				return &Error{Code: "invocation_fence_drift", Message: "Reference Plan invocation fence drifted"}
			}
			candidate, candidateErr := repo.FindReferencePlanCandidateByInvocation(ctx, invocationID)
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
		attempts, countErr := repo.CountReferencePlanAttempts(ctx, invocationID)
		if countErr != nil && !errors.Is(countErr, ErrNotFound) {
			return countErr
		}
		if attempts >= int64(service.config.Budget.MaxAttempts) {
			return &Error{Code: "attempt_budget_exhausted", Message: "Reference Plan attempt budget is exhausted"}
		}
		claimVersion = attempts + 1
		attemptID := proposedAttemptID
		if findErr == nil {
			attemptID = service.config.NewID()
		}
		invocation, buildErr := buildReferencePlanInvocation(
			command, release, control, manifest, impactClosureHash,
			invocationID, attemptID, service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		invocationRecord = ReferencePlanInvocationRecord{
			Invocation: invocation, WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
			WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			ReleaseID: release.ID, Manifest: manifest, CreatedAt: now,
		}
		if errors.Is(findErr, ErrNotFound) {
			if createErr := repo.CreateReferencePlanInvocation(ctx, invocationRecord); createErr != nil {
				return createErr
			}
		}
		if createErr := repo.CreateReferencePlanAttempt(ctx, AttemptRecord{
			ID: attemptID, InvocationID: invocationID, ClaimVersion: claimVersion,
			ControlHash: control.ControlHash, ReleaseFence: control.ReleaseFence,
			AgentImageDigest: release.Identity.AgentImageDigest, DispatchedAt: now,
		}); createErr != nil {
			return createErr
		}
		authorization, buildErr = service.authorizer.IssueReferencePlanDispatchAuthorization(invocation, claimVersion)
		if buildErr != nil {
			return buildErr
		}
		return repo.CreateReferencePlanDispatchAuthorization(ctx, DispatchAuthorizationRecord{
			AttemptID: attemptID, AuthorizationHash: authorization.Hash,
			ExpiresAt: authorization.ExpiresAt, IssuedAt: now,
		})
	})
	if err != nil || completed.ID != "" {
		return completed, err
	}

	result, invokeErr := service.invoker.InvokeReferencePlan(ctx, invocationRecord.Invocation, authorization)
	if invokeErr != nil || result.ValidateFor(invocationRecord.Invocation, claimVersion, authorization.Hash) != nil {
		errorCode := "agent_outcome_unknown"
		summary := "Reference Plan outcome could not be confirmed"
		if errors.Is(invokeErr, contract.ErrSkillBundleUnavailable) {
			errorCode = "skill_bundle_unavailable"
			summary = "Frozen Reference Plan skill bundle is unavailable"
		}
		result, err = referencePlanOutcomeUnknownResult(
			invocationRecord.Invocation, authorization, service.config.Now().UTC(), errorCode, summary,
		)
		if err != nil {
			return Candidate{}, err
		}
		acceptance := ReferencePlanResultAcceptance{
			ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
			Invocation: invocationRecord, Result: result, AcceptedAt: result.CompletedAt,
		}
		err = service.transactions.WithinReferencePlanTransaction(ctx, func(repo ReferencePlanRepository) error {
			return repo.RecordFailedReferencePlanResult(ctx, acceptance)
		})
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, &Error{Code: errorCode, Message: summary}
	}
	acceptance := ReferencePlanResultAcceptance{
		ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
		Invocation: invocationRecord, Result: result, AcceptedAt: service.config.Now().UTC(),
	}
	if result.Status != "accepted" {
		recordErr := service.transactions.WithinReferencePlanTransaction(ctx, func(repo ReferencePlanRepository) error {
			return repo.RecordFailedReferencePlanResult(ctx, acceptance)
		})
		if recordErr != nil {
			return Candidate{}, recordErr
		}
		return Candidate{}, &Error{Code: result.Error.Code, Message: result.Error.SafeSummary}
	}
	if err = service.config.ValidateCandidate(command.Input, result.Candidate); err != nil {
		return Candidate{}, &Error{
			Code: "invalid_reference_plan_projection", Message: "Reference Plan Candidate cannot be projected from frozen Backend facts",
		}
	}
	err = service.transactions.WithinReferencePlanTransaction(ctx, func(repo ReferencePlanRepository) error {
		if validateErr := repo.ValidateReferencePlanInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		var acceptErr error
		completed, acceptErr = repo.AcceptReferencePlanResult(ctx, acceptance)
		return acceptErr
	})
	return completed, err
}

func validateExecuteReferencePlanCommand(command ExecuteReferencePlanCommand) error {
	for _, identifier := range []string{command.WorkflowRunID, command.NodeRunID} {
		parsed, err := uuid.Parse(identifier)
		if err != nil || parsed == uuid.Nil {
			return &Error{Code: "invalid_reference_plan_command", Message: "Reference Plan command has an invalid identity"}
		}
	}
	if command.Input.Validate() != nil {
		return &Error{Code: "invalid_reference_plan_input", Message: "Reference Plan input is invalid"}
	}
	return nil
}

func buildReferencePlanManifest(
	command ExecuteReferencePlanCommand,
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
		ContractID                            string   `json:"contract_id"`
		ProductionWorldOwnerSetHash           string   `json:"production_world_owner_set_hash"`
		P1ScopeKeys                           []string `json:"p1_scope_keys"`
		VisualFoundationCandidateRevisionHash string   `json:"visual_foundation_candidate_revision_hash"`
		ReferenceTargetSeedRoot               string   `json:"reference_target_seed_root"`
	}{
		ContractID:                            "reference-plan-impact-closure-production",
		ProductionWorldOwnerSetHash:           command.Input.ProductionWorldOwnerSetHash,
		P1ScopeKeys:                           append([]string(nil), command.Input.P1ScopeKeys...),
		VisualFoundationCandidateRevisionHash: command.Input.VisualFoundationCandidateRevisionHash,
		ReferenceTargetSeedRoot:               command.Input.ReferenceTargetSeedRoot,
	})
	if err != nil {
		return ManifestRecord{}, "", err
	}
	impactClosureHash, err := platformcanonical.Hash(impactMaterial)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf(
		"lanverse:reference-plan:manifest:%s:%s", command.NodeRunID, rootInputHash,
	))).String()
	shardKey := "project:" + command.Input.ProjectID
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
		"contract_id": "reference-plan-shard-manifest-production", "manifest_id": manifestID,
		"workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": contract.ReferencePlanStageKey, "root_input_hash": rootInputHash,
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
		StageKey: contract.ReferencePlanStageKey, RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now,
	}, impactClosureHash, nil
}

func buildReferencePlanInvocation(
	command ExecuteReferencePlanCommand,
	release ReleaseRecord,
	control contract.SceneAnalysisControlProof,
	manifest ManifestRecord,
	impactClosureHash string,
	invocationID string,
	attemptID string,
	budget contract.SceneAnalysisExecutionBudget,
) (contract.ReferencePlanInvocation, error) {
	return contract.NewReferencePlanInvocation(
		invocationID,
		attemptID,
		release.Identity,
		control,
		budget,
		contract.ReferencePlanPayload{
			Variant: contract.ReferencePlanStageVariant{
				StageKey: contract.ReferencePlanStageKey, ProfileKey: "default", LaneKey: "primary",
				OutputSchemaVersion: contract.ReferencePlanCandidateContractID,
			},
			Scope: contract.ReferencePlanScope{
				WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
			},
			Shard: contract.ReferencePlanShard{
				ManifestID: manifest.ID, ManifestHash: manifest.ManifestHash,
				ShardKey: "project:" + command.Input.ProjectID, ImpactClosureHash: impactClosureHash,
			},
			StageInput: command.Input,
		},
	)
}

func referencePlanOutcomeUnknownResult(
	invocation contract.ReferencePlanInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	completedAt time.Time,
	errorCode string,
	safeSummary string,
) (contract.ReferencePlanAttemptResult, error) {
	diagnostics := []contract.SceneAnalysisDiagnostic{{Code: errorCode, Summary: safeSummary}}
	diagnosticsJSON, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.ReferencePlanAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticsJSON)
	if err != nil {
		return contract.ReferencePlanAttemptResult{}, err
	}
	result := contract.ReferencePlanAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "outcome_unknown", CandidateType: "reference_plan_candidate",
		InputHash: invocation.InputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: completedAt,
		Executor: contract.ReferencePlanExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-plan-harness", Model: "unconfirmed",
		},
		Error: &contract.SceneAnalysisResultError{
			Code: errorCode, SafeSummary: safeSummary, RetryClass: "same_release",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.ReferencePlanAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}
