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

type ExecuteVisualFoundationCommand struct {
	WorkflowRunID    string
	NodeRunID        string
	Input            contract.VisualFoundationInput
	MediaAttachments []contract.VisualFoundationMediaAttachment
}

type VisualFoundationInvocationRecord struct {
	Invocation                                       contract.VisualFoundationInvocation
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	ReleaseID                                        string
	Manifest                                         ManifestRecord
	CreatedAt                                        time.Time
}

type VisualFoundationResultAcceptance struct {
	ResultID, CandidateID string
	Invocation            VisualFoundationInvocationRecord
	Result                contract.VisualFoundationAttemptResult
	AcceptedAt            time.Time
}

type VisualFoundationRepository interface {
	ValidateVisualFoundationInput(context.Context, contract.VisualFoundationInput) error
	EnsureVisualFoundationRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureVisualFoundationManifest(context.Context, ManifestRecord) error
	FindVisualFoundationInvocation(context.Context, string, string, string) (VisualFoundationInvocationRecord, error)
	CreateVisualFoundationInvocation(context.Context, VisualFoundationInvocationRecord) error
	FindVisualFoundationCandidateByInvocation(context.Context, string) (Candidate, error)
	CountVisualFoundationAttempts(context.Context, string) (int64, error)
	CreateVisualFoundationAttempt(context.Context, AttemptRecord) error
	CreateVisualFoundationDispatchAuthorization(context.Context, DispatchAuthorizationRecord) error
	AcceptVisualFoundationResult(context.Context, VisualFoundationResultAcceptance) (Candidate, error)
	RecordFailedVisualFoundationResult(context.Context, VisualFoundationResultAcceptance) error
}

type VisualFoundationTransactions interface {
	WithinVisualFoundationTransaction(context.Context, func(VisualFoundationRepository) error) error
}

type VisualFoundationInvoker interface {
	Invoke(
		context.Context,
		contract.VisualFoundationInvocation,
		contract.SceneAnalysisDispatchAuthorization,
	) (contract.VisualFoundationAttemptResult, error)
}

type VisualFoundationDispatchAuthorizer interface {
	IssueVisualFoundationDispatchAuthorization(
		contract.VisualFoundationInvocation,
		int64,
	) (contract.SceneAnalysisDispatchAuthorization, error)
}

type VisualFoundationExecutionConfig struct {
	Now              func() time.Time
	NewID            func() string
	AgentImageDigest string
	Budget           contract.SceneAnalysisExecutionBudget
}

type VisualFoundationExecutionService struct {
	transactions VisualFoundationTransactions
	invoker      VisualFoundationInvoker
	authorizer   VisualFoundationDispatchAuthorizer
	config       VisualFoundationExecutionConfig
}

func NewVisualFoundationExecutionService(
	transactions VisualFoundationTransactions,
	invoker VisualFoundationInvoker,
	authorizer VisualFoundationDispatchAuthorizer,
	config VisualFoundationExecutionConfig,
) (*VisualFoundationExecutionService, error) {
	if transactions == nil || invoker == nil || authorizer == nil ||
		!strings.HasPrefix(config.AgentImageDigest, "sha256:") {
		return nil, errors.New("Visual Foundation execution dependencies are required")
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
		return nil, errors.New("Visual Foundation execution budget is invalid")
	}
	return &VisualFoundationExecutionService{
		transactions: transactions, invoker: invoker, authorizer: authorizer, config: config,
	}, nil
}

func (service *VisualFoundationExecutionService) Execute(
	ctx context.Context,
	command ExecuteVisualFoundationCommand,
) (Candidate, error) {
	if err := validateExecuteVisualFoundationCommand(command); err != nil {
		return Candidate{}, err
	}
	now := service.config.Now().UTC()
	release, err := BuildStageReleaseRecord(
		contract.VisualFoundationStageKey,
		service.config.AgentImageDigest,
		now,
	)
	if err != nil {
		return Candidate{}, err
	}
	manifest, impactClosureHash, err := buildVisualFoundationManifest(command, now)
	if err != nil {
		return Candidate{}, err
	}

	var invocationRecord VisualFoundationInvocationRecord
	var authorization contract.SceneAnalysisDispatchAuthorization
	var claimVersion int64
	var completed Candidate
	err = service.transactions.WithinVisualFoundationTransaction(ctx, func(repo VisualFoundationRepository) error {
		if validateErr := repo.ValidateVisualFoundationInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		control, ensureErr := repo.EnsureVisualFoundationRelease(ctx, release)
		if ensureErr != nil {
			return ensureErr
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Visual Foundation release is not approved"}
		}
		if ensureErr = repo.EnsureVisualFoundationManifest(ctx, manifest); ensureErr != nil {
			return ensureErr
		}
		proposedInvocationID := service.config.NewID()
		proposedAttemptID := service.config.NewID()
		proposed, buildErr := buildVisualFoundationInvocation(
			command,
			release,
			control,
			manifest,
			impactClosureHash,
			proposedInvocationID,
			proposedAttemptID,
			service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		existing, findErr := repo.FindVisualFoundationInvocation(
			ctx,
			command.WorkflowRunID,
			command.NodeRunID,
			proposed.InputHash,
		)
		invocationID := proposedInvocationID
		if findErr == nil {
			invocationID = existing.Invocation.InvocationID
			if existing.Invocation.StageRelease != release.Identity || existing.Invocation.Control != control ||
				existing.Invocation.InputHash != proposed.InputHash ||
				!reflect.DeepEqual(existing.Invocation.Payload, proposed.Payload) {
				return &Error{Code: "invocation_fence_drift", Message: "Visual Foundation invocation fence drifted"}
			}
			candidate, candidateErr := repo.FindVisualFoundationCandidateByInvocation(ctx, invocationID)
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
		attempts, countErr := repo.CountVisualFoundationAttempts(ctx, invocationID)
		if countErr != nil && !errors.Is(countErr, ErrNotFound) {
			return countErr
		}
		if attempts >= int64(service.config.Budget.MaxAttempts) {
			return &Error{Code: "attempt_budget_exhausted", Message: "Visual Foundation attempt budget is exhausted"}
		}
		claimVersion = attempts + 1
		attemptID := proposedAttemptID
		if findErr == nil {
			attemptID = service.config.NewID()
		}
		invocation, buildErr := buildVisualFoundationInvocation(
			command,
			release,
			control,
			manifest,
			impactClosureHash,
			invocationID,
			attemptID,
			service.config.Budget,
		)
		if buildErr != nil {
			return buildErr
		}
		invocationRecord = VisualFoundationInvocationRecord{
			Invocation: invocation, WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
			WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			ReleaseID: release.ID, Manifest: manifest, CreatedAt: now,
		}
		if errors.Is(findErr, ErrNotFound) {
			if createErr := repo.CreateVisualFoundationInvocation(ctx, invocationRecord); createErr != nil {
				return createErr
			}
		}
		if createErr := repo.CreateVisualFoundationAttempt(ctx, AttemptRecord{
			ID: attemptID, InvocationID: invocationID, ClaimVersion: claimVersion,
			ControlHash: control.ControlHash, ReleaseFence: control.ReleaseFence,
			AgentImageDigest: release.Identity.AgentImageDigest, DispatchedAt: now,
		}); createErr != nil {
			return createErr
		}
		authorization, buildErr = service.authorizer.IssueVisualFoundationDispatchAuthorization(
			invocation,
			claimVersion,
		)
		if buildErr != nil {
			return buildErr
		}
		return repo.CreateVisualFoundationDispatchAuthorization(ctx, DispatchAuthorizationRecord{
			AttemptID: attemptID, AuthorizationHash: authorization.Hash,
			ExpiresAt: authorization.ExpiresAt, IssuedAt: now,
		})
	})
	if err != nil || completed.ID != "" {
		return completed, err
	}

	result, invokeErr := service.invoker.Invoke(ctx, invocationRecord.Invocation, authorization)
	if invokeErr != nil || result.ValidateFor(invocationRecord.Invocation, claimVersion, authorization.Hash) != nil {
		resultErrorCode := "agent_outcome_unknown"
		resultSummary := "Visual Foundation outcome could not be confirmed"
		if errors.Is(invokeErr, contract.ErrSkillBundleUnavailable) {
			resultErrorCode = "skill_bundle_unavailable"
			resultSummary = "Frozen Visual Foundation skill bundle is unavailable"
		}
		result, err = visualFoundationOutcomeUnknownResult(
			invocationRecord.Invocation,
			authorization,
			service.config.Now().UTC(),
			resultErrorCode,
			resultSummary,
		)
		if err != nil {
			return Candidate{}, err
		}
		acceptance := VisualFoundationResultAcceptance{
			ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
			Invocation: invocationRecord, Result: result, AcceptedAt: result.CompletedAt,
		}
		err = service.transactions.WithinVisualFoundationTransaction(ctx, func(repo VisualFoundationRepository) error {
			return repo.RecordFailedVisualFoundationResult(ctx, acceptance)
		})
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, &Error{Code: resultErrorCode, Message: resultSummary}
	}
	acceptance := VisualFoundationResultAcceptance{
		ResultID: service.config.NewID(), CandidateID: service.config.NewID(),
		Invocation: invocationRecord, Result: result, AcceptedAt: service.config.Now().UTC(),
	}
	if result.Status != "accepted" {
		recordErr := service.transactions.WithinVisualFoundationTransaction(ctx, func(repo VisualFoundationRepository) error {
			return repo.RecordFailedVisualFoundationResult(ctx, acceptance)
		})
		if recordErr != nil {
			return Candidate{}, recordErr
		}
		return Candidate{}, &Error{Code: result.Error.Code, Message: result.Error.SafeSummary}
	}
	err = service.transactions.WithinVisualFoundationTransaction(ctx, func(repo VisualFoundationRepository) error {
		if validateErr := repo.ValidateVisualFoundationInput(ctx, command.Input); validateErr != nil {
			return validateErr
		}
		var acceptErr error
		completed, acceptErr = repo.AcceptVisualFoundationResult(ctx, acceptance)
		return acceptErr
	})
	return completed, err
}

func validateExecuteVisualFoundationCommand(command ExecuteVisualFoundationCommand) error {
	for _, identifier := range []string{command.WorkflowRunID, command.NodeRunID} {
		parsed, err := uuid.Parse(identifier)
		if err != nil || parsed == uuid.Nil {
			return &Error{Code: "invalid_visual_foundation_command", Message: "Visual Foundation command has an invalid identity"}
		}
	}
	if command.Input.Validate() != nil {
		return &Error{Code: "invalid_visual_foundation_input", Message: "Visual Foundation input is invalid"}
	}
	return nil
}

func buildVisualFoundationManifest(
	command ExecuteVisualFoundationCommand,
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
		ContractID                  string                        `json:"contract_id"`
		ProductionWorldOwnerSetHash string                        `json:"production_world_owner_set_hash"`
		ConfirmedWorldRoots         []contract.ConfirmedWorldRoot `json:"confirmed_world_roots"`
	}{
		ContractID:                  "visual-foundation-impact-closure-production",
		ProductionWorldOwnerSetHash: command.Input.ProductionWorldOwnerSetHash,
		ConfirmedWorldRoots:         command.Input.ConfirmedWorldRoots,
	})
	if err != nil {
		return ManifestRecord{}, "", err
	}
	impactClosureHash, err := platformcanonical.Hash(impactMaterial)
	if err != nil {
		return ManifestRecord{}, "", err
	}
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf(
		"lanverse:visual-foundation:manifest:%s:%s", command.NodeRunID, rootInputHash,
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
		"contract_id": "visual-foundation-shard-manifest-production", "manifest_id": manifestID,
		"workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": contract.VisualFoundationStageKey, "root_input_hash": rootInputHash,
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
		StageKey: contract.VisualFoundationStageKey, RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now,
	}, impactClosureHash, nil
}

func buildVisualFoundationInvocation(
	command ExecuteVisualFoundationCommand,
	release ReleaseRecord,
	control contract.SceneAnalysisControlProof,
	manifest ManifestRecord,
	impactClosureHash string,
	invocationID string,
	attemptID string,
	budget contract.SceneAnalysisExecutionBudget,
) (contract.VisualFoundationInvocation, error) {
	return contract.NewVisualFoundationInvocation(
		invocationID,
		attemptID,
		release.Identity,
		control,
		budget,
		contract.VisualFoundationPayload{
			Variant: contract.VisualFoundationStageVariant{
				StageKey: contract.VisualFoundationStageKey, ProfileKey: "default", LaneKey: "primary",
				OutputSchemaVersion: contract.VisualFoundationCandidateSchemaVersion,
			},
			Scope: contract.VisualFoundationScope{
				WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
			},
			Shard: contract.VisualFoundationShard{
				ManifestID: manifest.ID, ManifestHash: manifest.ManifestHash,
				ShardKey: "project:" + command.Input.ProjectID, ImpactClosureHash: impactClosureHash,
			},
			MediaAttachments: append([]contract.VisualFoundationMediaAttachment(nil), command.MediaAttachments...),
			StageInput:       command.Input,
		},
	)
}

func visualFoundationOutcomeUnknownResult(
	invocation contract.VisualFoundationInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	completedAt time.Time,
	errorCode string,
	safeSummary string,
) (contract.VisualFoundationAttemptResult, error) {
	diagnostics := []contract.SceneAnalysisDiagnostic{{Code: errorCode, Summary: safeSummary}}
	diagnosticsJSON, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticsJSON)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	result := contract.VisualFoundationAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "outcome_unknown", CandidateType: "visual_foundation_candidate",
		InputHash: invocation.InputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: completedAt,
		Executor: contract.VisualFoundationExecutor{
			RuntimeClass: "vision", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "visual-foundation-harness", Model: "unconfirmed",
		},
		Error: &contract.SceneAnalysisResultError{
			Code: errorCode, SafeSummary: safeSummary, RetryClass: "same_release",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}
