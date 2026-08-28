package generation

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	candidateSetInputExecutor  = "workflow.input.generation_candidate_set"
	referenceAssetNodeExecutor = "activity.reference_asset_generation"
)

var candidateSetHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type CandidateSetSource interface {
	RequireCandidateSet(context.Context, generationapp.Actor, string) (generationdomain.CandidateSet, error)
}

type ReferenceTargetBuilder interface {
	BuildReferenceTargets(
		context.Context,
		generationapp.Actor,
		generationapp.BuildReferenceTargetsCommand,
	) (generationapp.BuildReferenceTargetsResult, error)
}

type ImagePreparation interface {
	PrepareImageGeneration(
		context.Context,
		generationapp.Actor,
		generationapp.PrepareImageGenerationCommand,
	) (generationapp.PreparationResult, error)
}

type ExecutionClaimOwner interface {
	AcquireExecutionClaim(
		context.Context,
		generationapp.AcquireExecutionClaimCommand,
	) (generationapp.ExecutionClaimResult, error)
}

type ReferencePreparation interface {
	ImagePreparation
	ExecutionClaimOwner
}

type ImageProvider interface {
	SubmitImageRequest(
		context.Context,
		generationdomain.ExecutionAuthorization,
		generationapp.SubmitImageRequestCommand,
	) (generationapp.ProviderExecutionResult, error)
	ReconcileProviderJob(
		context.Context,
		generationapp.ReconcileProviderJobCommand,
	) (generationapp.ProviderExecutionResult, error)
}

type NodeExecutor struct {
	candidateSets    CandidateSetSource
	referenceTargets ReferenceTargetBuilder
	preparations     ImagePreparation
	claims           ExecutionClaimOwner
	providers        ImageProvider
}

func NewNodeExecutor(
	candidateSets CandidateSetSource,
	referenceTargets ReferenceTargetBuilder,
	preparations ImagePreparation,
	claims ExecutionClaimOwner,
	providers ImageProvider,
) *NodeExecutor {
	return &NodeExecutor{
		candidateSets: candidateSets, referenceTargets: referenceTargets, preparations: preparations,
		claims: claims, providers: providers,
	}
}

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	switch command.Executor {
	case candidateSetInputExecutor:
		return executor.executeCandidateSet(ctx, command)
	case referenceAssetNodeExecutor:
		return executor.executeReferenceAsset(ctx, command)
	default:
		return domain.NodeExecutorResult{}, errors.New("unsupported Generation workflow node execution")
	}
}

func (executor *NodeExecutor) executeCandidateSet(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || executor.candidateSets == nil ||
		strings.TrimSpace(command.IdempotencyKey) == "" || command.InitiatorTokenVersion < 1 {
		return domain.NodeExecutorResult{}, errors.New("unsupported Generation workflow node execution")
	}
	for _, identifier := range []string{command.WorkspaceID, command.ProjectID, command.InitiatorUserID} {
		if _, err := uuid.Parse(strings.TrimSpace(identifier)); err != nil {
			return domain.NodeExecutorResult{}, errors.New("invalid Generation workflow execution boundary")
		}
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidates" ||
		command.OutputPorts[0].ValueType != "generation_candidate_set" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Generation CandidateSet source contract")
	}
	shot := input.Bindings[0]
	shotRevision, revisionErr := strconv.Atoi(shot.ReferenceVersion)
	if shot.Port != "shot" || shot.ValueType != "production_shot" ||
		shot.SourceKind != domain.NodeInputSourceNodeOutput || strings.TrimSpace(shot.SourceNodeID) == "" ||
		shot.SourcePort != "shot" || revisionErr != nil || shotRevision < 1 ||
		!validCandidateSetUUID(shot.ReferenceID) || !candidateSetHashPattern.MatchString(shot.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet Shot input has drifted")
	}
	var config map[string]json.RawMessage
	var providerJobID string
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["provider_job_id"], &providerJobID) != nil || !validCandidateSetUUID(providerJobID) {
		return domain.NodeExecutorResult{}, errors.New("invalid Generation CandidateSet source config")
	}
	providerJobID = strings.TrimSpace(providerJobID)
	set, err := executor.candidateSets.RequireCandidateSet(ctx, generationapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, providerJobID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if set.ID != providerJobID || set.WorkspaceID != command.WorkspaceID || set.ProjectID != command.ProjectID ||
		set.Revision != 1 || !validCandidateSetUUID(set.ProviderReceiptID) ||
		!candidateSetHashPattern.MatchString(set.ContentHash) || len(set.Candidates) == 0 || len(set.Candidates) > 100 {
		return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet source has drifted")
	}
	seenCandidates := make(map[string]struct{}, len(set.Candidates))
	for _, candidate := range set.Candidates {
		if !validCandidateSetUUID(candidate.ID) || !validCandidateSetUUID(candidate.ArtifactID) || !validCandidateSetUUID(candidate.QCReportID) ||
			candidate.Revision < 1 || candidate.ArtifactRevision < 1 ||
			!candidateSetHashPattern.MatchString(candidate.ArtifactSHA256) ||
			!candidateSetHashPattern.MatchString(candidate.QCReportHash) {
			return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet contains an invalid candidate")
		}
		if _, exists := seenCandidates[candidate.ID]; exists {
			return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet contains duplicate candidates")
		}
		seenCandidates[candidate.ID] = struct{}{}
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidates", ValueType: "generation_candidate_set", ReferenceID: set.ID,
			ReferenceVersion: strconv.Itoa(set.Revision), ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeReferenceAsset(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || executor.referenceTargets == nil || executor.preparations == nil ||
		executor.claims == nil || executor.providers == nil ||
		strings.TrimSpace(command.IdempotencyKey) == "" || command.InitiatorTokenVersion < 1 {
		return domain.NodeExecutorResult{}, errors.New("reference asset workflow owners are unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.InitiatorUserID,
		command.WorkflowRunID, command.NodeRunID,
	} {
		if !validCandidateSetUUID(identifier) {
			return domain.NodeExecutorResult{}, errors.New("invalid reference asset workflow execution boundary")
		}
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidates" ||
		command.OutputPorts[0].ValueType != "generation_candidate_set" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid reference asset workflow node contract")
	}
	approved := input.Bindings[0]
	if approved.Port != "intents" || approved.ValueType != "approved_storyboard_intents" ||
		approved.SourceKind != domain.NodeInputSourceNodeOutput || strings.TrimSpace(approved.SourceNodeID) == "" ||
		approved.SourcePort != "intents" || approved.ReferenceVersion != "1" ||
		!validCandidateSetUUID(approved.ReferenceID) || !candidateSetHashPattern.MatchString(approved.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("approved Storyboard Intent input has drifted")
	}
	assetID, assetStateID, err := referenceAssetSelector(input.Config)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	actor := generationapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	built, err := executor.referenceTargets.BuildReferenceTargets(ctx, actor, generationapp.BuildReferenceTargetsCommand{
		ApprovedIntentSetID: approved.ReferenceID, ExpectedContentHash: approved.ContentHash,
		IdempotencyKey: "workflow-run:" + command.WorkflowRunID + ":reference-targets",
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	var selected *generationdomain.GenerationTarget
	for index := range built.Targets {
		target := &built.Targets[index]
		if generationdomain.ValidateGenerationTarget(*target) != nil || target.WorkspaceID != command.WorkspaceID ||
			target.ProjectID != command.ProjectID || target.CreatedBy != command.InitiatorUserID ||
			target.Kind != generationdomain.GenerationTargetReferenceAsset || target.ReferenceAsset == nil ||
			target.SourceOwnerRef.Owner != "storyboard" || target.SourceOwnerRef.Resource != "approved_storyboard_intents" ||
			target.SourceOwnerRef.ID != approved.ReferenceID || target.SourceOwnerRef.Revision != 1 ||
			target.SourceOwnerRef.ContentHash != approved.ContentHash {
			return domain.NodeExecutorResult{}, errors.New("reference asset Target Builder returned drifted facts")
		}
		if target.ReferenceAsset.AssetID != assetID || target.ReferenceAsset.AssetStateRef.ID != assetStateID {
			continue
		}
		if selected != nil {
			return domain.NodeExecutorResult{}, errors.New("reference asset selector matched multiple Targets")
		}
		selected = target
	}
	if selected == nil {
		return domain.NodeExecutorResult{}, errors.New("reference asset selector did not match an approved Target")
	}
	prepared, err := executor.preparations.PrepareImageGeneration(ctx, actor, generationapp.PrepareImageGenerationCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID, WorkflowInputHash: inputHash,
		TargetID: selected.ID, TargetHash: selected.TargetHash, Units: int64(selected.ReferenceAsset.NumberResults),
		IdempotencyKey: "workflow-node:" + command.NodeRunID + ":generation-prepare",
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	intent := prepared.Intent
	if !validCandidateSetUUID(intent.ID) || intent.WorkspaceID != command.WorkspaceID || intent.ProjectID != command.ProjectID ||
		intent.WorkflowRunID != command.WorkflowRunID || intent.NodeRunID != command.NodeRunID ||
		intent.TargetID != selected.ID || intent.TargetHash != selected.TargetHash ||
		intent.Units != int64(selected.ReferenceAsset.NumberResults) {
		return domain.NodeExecutorResult{}, errors.New("reference asset preparation returned drifted facts")
	}
	if intent.Status == generationdomain.IntentCancelled || intent.Status == generationdomain.IntentFailed {
		return domain.NodeExecutorResult{}, errors.New("reference asset generation is terminal without candidates")
	}
	if intent.Status == generationdomain.IntentSucceeded {
		if !validCandidateSetUUID(intent.GenerationRequestID) || !validCandidateSetUUID(intent.ProviderJobID) ||
			!validCandidateSetUUID(intent.ProviderReceiptID) {
			return domain.NodeExecutorResult{}, errors.New("reference asset terminal Provider facts have drifted")
		}
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	}
	claim, err := executor.claims.AcquireExecutionClaim(ctx, generationapp.AcquireExecutionClaimCommand{
		IntentID: intent.ID, Claimant: "workflow-node:" + command.NodeRunID,
		IdempotencyKey: "workflow-node:" + command.NodeRunID + ":generation-claim",
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if !validReferenceAssetClaim(claim, intent, *selected) {
		return domain.NodeExecutorResult{}, errors.New("reference asset execution claim returned drifted facts")
	}
	var provider generationapp.ProviderExecutionResult
	switch claim.Intent.Status {
	case generationdomain.IntentClaimed, generationdomain.IntentDispatching:
		provider, err = executor.providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID: intent.ID, IdempotencyKey: "workflow-node:" + command.NodeRunID + ":provider-submit",
		})
	case generationdomain.IntentSubmitted, generationdomain.IntentOutcomeUnknown:
		provider, err = executor.providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
			ProviderJobID:  claim.Intent.ProviderJobID,
			IdempotencyKey: "workflow-node:" + command.NodeRunID + ":provider-reconcile:" + strconv.FormatInt(claim.Intent.Revision, 10),
		})
	case generationdomain.IntentSucceeded:
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case generationdomain.IntentFailed:
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider job failed")
	default:
		return domain.NodeExecutorResult{}, errors.New("reference asset execution claim returned an invalid state")
	}
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if !validReferenceAssetProviderResult(provider, claim.Intent, *selected) {
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider result returned drifted facts")
	}
	if provider.Job.Status == generationdomain.ProviderJobFailed {
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider job failed")
	}
	return domain.NodeExecutorResult{Status: "RETRYING"}, nil
}

func validReferenceAssetClaim(
	claim generationapp.ExecutionClaimResult,
	prepared generationdomain.Intent,
	target generationdomain.GenerationTarget,
) bool {
	intent, authorization := claim.Intent, claim.Authorization
	if !generationdomain.SameIntentBinding(intent, prepared) || intent.ID != prepared.ID ||
		intent.TargetID != target.ID || intent.TargetHash != target.TargetHash || intent.Revision < 3 ||
		intent.Claimant == nil || *intent.Claimant != "workflow-node:"+intent.NodeRunID ||
		intent.ClaimToken == nil || intent.ClaimExpiresAt == nil ||
		authorization.IntentID != intent.ID || authorization.ClaimToken != *intent.ClaimToken ||
		authorization.TargetID != target.ID || authorization.TargetHash != target.TargetHash ||
		authorization.CostReservationID != intent.CostReservationID ||
		authorization.QuotaReservationID != intent.QuotaReservationID ||
		authorization.ClaimFencingVersion != intent.ClaimFencingVersion || authorization.IntentRevision != 3 ||
		authorization.Units != intent.Units || !authorization.ExpiresAt.Equal(*intent.ClaimExpiresAt) {
		return false
	}
	switch intent.Status {
	case generationdomain.IntentClaimed, generationdomain.IntentDispatching,
		generationdomain.IntentSubmitted, generationdomain.IntentOutcomeUnknown,
		generationdomain.IntentSucceeded, generationdomain.IntentFailed:
		return true
	default:
		return false
	}
}

func validReferenceAssetProviderResult(
	result generationapp.ProviderExecutionResult,
	claimed generationdomain.Intent,
	target generationdomain.GenerationTarget,
) bool {
	intent, request, job := result.Intent, result.Request, result.Job
	if !generationdomain.SameIntentBinding(intent, claimed) || intent.ID != claimed.ID ||
		intent.TargetID != target.ID || intent.TargetHash != target.TargetHash ||
		!validCandidateSetUUID(request.ID) || request.ID != intent.GenerationRequestID ||
		request.IntentID != intent.ID || request.TargetID != target.ID || request.TargetHash != target.TargetHash ||
		!validCandidateSetUUID(job.ID) || job.ID != intent.ProviderJobID || job.IntentID != intent.ID ||
		job.RequestID != request.ID || job.Revision < 1 {
		return false
	}
	switch job.Status {
	case generationdomain.ProviderJobRunning:
		return intent.Status == generationdomain.IntentSubmitted
	case generationdomain.ProviderJobUnknown:
		return intent.Status == generationdomain.IntentOutcomeUnknown
	case generationdomain.ProviderJobSucceeded:
		return intent.Status == generationdomain.IntentSucceeded
	case generationdomain.ProviderJobFailed:
		return intent.Status == generationdomain.IntentFailed
	default:
		return false
	}
}

func referenceAssetSelector(config json.RawMessage) (string, string, error) {
	var fields map[string]json.RawMessage
	var assetID, assetStateID string
	if json.Unmarshal(config, &fields) != nil || len(fields) != 2 ||
		json.Unmarshal(fields["asset_id"], &assetID) != nil ||
		json.Unmarshal(fields["asset_state_id"], &assetStateID) != nil {
		return "", "", errors.New("invalid reference asset workflow node config")
	}
	assetID, assetStateID = strings.TrimSpace(assetID), strings.TrimSpace(assetStateID)
	if !validCandidateSetUUID(assetID) || !validCandidateSetUUID(assetStateID) {
		return "", "", errors.New("invalid reference asset workflow node selector")
	}
	return assetID, assetStateID, nil
}

func validCandidateSetUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
