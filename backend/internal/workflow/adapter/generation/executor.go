package generation

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	candidateSetInputExecutor  = "workflow.input.generation_candidate_set"
	referenceAssetNodeExecutor = "activity.reference_asset_generation"
)

var (
	candidateSetHashPattern            = regexp.MustCompile(`^[0-9a-f]{64}$`)
	referenceProviderIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,179}$`)
)

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

type ProviderOutputMaterializer interface {
	MaterializeSucceededOutputs(
		context.Context,
		generationapp.Actor,
		generationapp.MaterializeProviderOutputsCommand,
	) (generationapp.OutputMaterializationResult, error)
}

type NodeExecutor struct {
	candidateSets    CandidateSetSource
	referenceTargets ReferenceTargetBuilder
	preparations     ImagePreparation
	claims           ExecutionClaimOwner
	providers        ImageProvider
	materializer     ProviderOutputMaterializer
}

func NewNodeExecutor(
	candidateSets CandidateSetSource,
	referenceTargets ReferenceTargetBuilder,
	preparations ImagePreparation,
	claims ExecutionClaimOwner,
	providers ImageProvider,
	materializer ProviderOutputMaterializer,
) *NodeExecutor {
	return &NodeExecutor{
		candidateSets: candidateSets, referenceTargets: referenceTargets, preparations: preparations,
		claims: claims, providers: providers, materializer: materializer,
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
	if set.ID != providerJobID {
		return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet source has drifted")
	}
	output, err := candidateSetNodeOutput(set, command.WorkspaceID, command.ProjectID)
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
		executor.claims == nil || executor.providers == nil || executor.materializer == nil ||
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
		TargetID: selected.ID, TargetHash: selected.TargetHash,
		IdempotencyKey: "workflow-node:" + command.NodeRunID + ":generation-prepare",
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	intent := prepared.Intent
	if !validReferenceAssetIntent(intent, *selected, command) {
		return domain.NodeExecutorResult{}, errors.New("reference asset preparation returned drifted facts")
	}
	switch intent.Status {
	case generationdomain.IntentCancelled, generationdomain.IntentFailed:
		return domain.NodeExecutorResult{}, errors.New("reference asset generation is terminal without candidates")
	case generationdomain.IntentSucceeded, generationdomain.IntentPartialSucceeded:
		return executor.materializeReferenceAsset(ctx, actor, command, intent)
	case generationdomain.IntentOutcomeUnknown:
		return providerOutcomeUnknownResult(), nil
	case generationdomain.IntentPrepared, generationdomain.IntentClaimed, generationdomain.IntentExecuting:
	default:
		return domain.NodeExecutorResult{}, errors.New("reference asset preparation returned an invalid state")
	}
	claim, err := executor.claims.AcquireExecutionClaim(ctx, generationapp.AcquireExecutionClaimCommand{
		IntentID: intent.ID, Claimant: "workflow-node:" + command.NodeRunID,
		IdempotencyKey: "workflow-node:" + command.NodeRunID + ":generation-claim",
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if !validReferenceAssetClaim(claim, intent, *selected, command) {
		return domain.NodeExecutorResult{}, errors.New("reference asset execution claim returned drifted facts")
	}
	var provider generationapp.ProviderExecutionResult
	switch claim.Intent.Status {
	case generationdomain.IntentClaimed:
		provider, err = executor.providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID: intent.ID, IdempotencyKey: "workflow-node:" + command.NodeRunID + ":provider-submit",
		})
	case generationdomain.IntentExecuting:
		provider, err = executor.providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
			ProviderJobID:  claim.Intent.ProviderJobID,
			IdempotencyKey: "workflow-node:" + command.NodeRunID + ":provider-reconcile:" + strconv.FormatInt(claim.Intent.Revision, 10),
		})
	case generationdomain.IntentOutcomeUnknown:
		return providerOutcomeUnknownResult(), nil
	case generationdomain.IntentSucceeded, generationdomain.IntentPartialSucceeded:
		return executor.materializeReferenceAsset(ctx, actor, command, claim.Intent)
	case generationdomain.IntentFailed:
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider job failed")
	default:
		return domain.NodeExecutorResult{}, errors.New("reference asset execution claim returned an invalid state")
	}
	if err != nil {
		if generationapp.IsCode(err, "provider_query_temporarily_unavailable") {
			return domain.NodeExecutorResult{Status: "RETRYING"}, nil
		}
		return domain.NodeExecutorResult{}, err
	}
	if !validReferenceAssetProviderResult(provider, claim.Intent, *selected, command) {
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider result returned drifted facts")
	}
	if provider.Job.Status == generationdomain.ProviderJobOutcomeUnknown {
		return providerOutcomeUnknownResult(), nil
	}
	if provider.Job.Status == generationdomain.ProviderJobFailed {
		return domain.NodeExecutorResult{}, errors.New("reference asset Provider job failed")
	}
	if provider.Job.Status == generationdomain.ProviderJobSucceeded ||
		provider.Job.Status == generationdomain.ProviderJobPartialSucceeded {
		return executor.materializeReferenceAsset(ctx, actor, command, provider.Intent)
	}
	return domain.NodeExecutorResult{Status: "RETRYING"}, nil
}

func providerOutcomeUnknownResult() domain.NodeExecutorResult {
	return domain.NodeExecutorResult{
		Status:     domain.NodeActivityNeedsAttention,
		ErrorCode:  domain.ProviderOutcomeUnknownErrorCode,
		NextAction: domain.ManualProviderReconciliationNextAction,
	}
}

func (executor *NodeExecutor) materializeReferenceAsset(
	ctx context.Context,
	actor generationapp.Actor,
	command domain.NodeExecutorCommand,
	intent generationdomain.Intent,
) (domain.NodeExecutorResult, error) {
	if (intent.Status != generationdomain.IntentSucceeded && intent.Status != generationdomain.IntentPartialSucceeded) ||
		!validCandidateSetUUID(intent.GenerationRequestID) || !validCandidateSetUUID(intent.ProviderJobID) ||
		!candidateSetHashPattern.MatchString(intent.ProviderCallSetHash) {
		return domain.NodeExecutorResult{}, errors.New("reference asset terminal Provider facts have drifted")
	}
	materialized, err := executor.materializer.MaterializeSucceededOutputs(ctx, actor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: intent.ProviderJobID,
	})
	if err != nil {
		if generationapp.IsCode(err, "dependency_unavailable") {
			return domain.NodeExecutorResult{Status: "RETRYING"}, nil
		}
		return domain.NodeExecutorResult{}, err
	}
	if !candidateSetHashPattern.MatchString(materialized.ProviderReceiptSetHash) ||
		materialized.CandidateSet.ID != intent.ProviderJobID ||
		materialized.CandidateSet.ProviderReceiptSetHash != materialized.ProviderReceiptSetHash {
		return domain.NodeExecutorResult{}, errors.New("reference asset materialization returned drifted facts")
	}
	output, err := candidateSetNodeOutput(materialized.CandidateSet, command.WorkspaceID, command.ProjectID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func candidateSetNodeOutput(
	set generationdomain.CandidateSet,
	workspaceID string,
	projectID string,
) (domain.NodeOutputSnapshot, error) {
	if set.WorkspaceID != workspaceID || set.ProjectID != projectID || set.ID == "" ||
		set.Revision != 1 || !validCandidateSetUUID(set.ID) ||
		!candidateSetHashPattern.MatchString(set.ProviderReceiptSetHash) ||
		!candidateSetHashPattern.MatchString(set.ContentHash) || len(set.Candidates) == 0 || len(set.Candidates) > 100 {
		return domain.NodeOutputSnapshot{}, errors.New("Generation CandidateSet source has drifted")
	}
	seenCandidates := make(map[string]struct{}, len(set.Candidates))
	for _, candidate := range set.Candidates {
		if !validCandidateSetUUID(candidate.ID) || !validCandidateSetUUID(candidate.ArtifactID) || !validCandidateSetUUID(candidate.QCReportID) ||
			candidate.Revision < 1 || candidate.ArtifactRevision < 1 ||
			!candidateSetHashPattern.MatchString(candidate.ArtifactSHA256) ||
			!candidateSetHashPattern.MatchString(candidate.QCReportHash) {
			return domain.NodeOutputSnapshot{}, errors.New("Generation CandidateSet contains an invalid candidate")
		}
		if _, exists := seenCandidates[candidate.ID]; exists {
			return domain.NodeOutputSnapshot{}, errors.New("Generation CandidateSet contains duplicate candidates")
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
	return output, err
}

func validReferenceAssetClaim(
	claim generationapp.ExecutionClaimResult,
	prepared generationdomain.Intent,
	target generationdomain.GenerationTarget,
	command domain.NodeExecutorCommand,
) bool {
	intent, authorization := claim.Intent, claim.Authorization
	if !validReferenceAssetIntent(intent, target, command) ||
		!sameReferenceAssetPreparedFacts(intent, prepared) || intent.ID != prepared.ID ||
		intent.TargetID != target.ID || intent.TargetHash != target.TargetHash || intent.Revision < 3 ||
		intent.Revision < prepared.Revision || intent.UpdatedAt.Before(prepared.UpdatedAt) ||
		(prepared.Status != generationdomain.IntentPrepared && !sameReferenceAssetClaimFacts(intent, prepared)) ||
		intent.Claimant == nil || *intent.Claimant != "workflow-node:"+intent.NodeRunID ||
		intent.ClaimToken == nil || intent.ClaimExpiresAt == nil ||
		authorization.IntentID != intent.ID || authorization.ClaimToken != *intent.ClaimToken ||
		authorization.TargetID != target.ID || authorization.TargetHash != target.TargetHash ||
		authorization.BindingVersionID != intent.BindingVersionID ||
		authorization.BindingRevision != intent.BindingRevision ||
		authorization.BindingContentHash != intent.BindingContentHash ||
		authorization.ConnectionVersionID != intent.ConnectionVersionID ||
		authorization.CredentialVersionID != intent.CredentialVersionID ||
		authorization.ModelProfileVersionID != intent.ModelProfileVersionID ||
		authorization.ModelProfileRevision != intent.ModelProfileRevision ||
		authorization.ModelProfileContentHash != intent.ModelProfileContentHash ||
		authorization.PriceQuoteID != intent.PriceQuoteID ||
		authorization.PriceQuoteRevision != intent.PriceQuoteRevision ||
		authorization.PriceQuoteContentHash != intent.PriceQuoteContentHash ||
		authorization.BillingMetric != intent.BillingMetric ||
		authorization.CostReservationID != intent.CostReservationID ||
		authorization.QuotaReservationID != intent.QuotaReservationID ||
		authorization.ClaimFencingVersion != intent.ClaimFencingVersion || authorization.IntentRevision != 3 ||
		authorization.EstimatedUnits != intent.EstimatedUnits || !authorization.ExpiresAt.Equal(*intent.ClaimExpiresAt) {
		return false
	}
	switch intent.Status {
	case generationdomain.IntentClaimed, generationdomain.IntentExecuting, generationdomain.IntentOutcomeUnknown,
		generationdomain.IntentSucceeded, generationdomain.IntentPartialSucceeded, generationdomain.IntentFailed:
		return true
	default:
		return false
	}
}

func validReferenceAssetProviderResult(
	result generationapp.ProviderExecutionResult,
	claimed generationdomain.Intent,
	target generationdomain.GenerationTarget,
	command domain.NodeExecutorCommand,
) bool {
	intent, request, job := result.Intent, result.Request, result.Job
	if !validReferenceAssetIntent(intent, target, command) ||
		!sameReferenceAssetPreparedFacts(intent, claimed) || !sameReferenceAssetClaimFacts(intent, claimed) ||
		intent.Revision < claimed.Revision || intent.UpdatedAt.Before(claimed.UpdatedAt) ||
		!validReferenceAssetRequest(request, intent, target) || !validReferenceAssetJob(job, intent, request) {
		return false
	}
	switch job.Status {
	case generationdomain.ProviderJobPending, generationdomain.ProviderJobRunning:
		return intent.Status == generationdomain.IntentExecuting
	case generationdomain.ProviderJobOutcomeUnknown:
		return intent.Status == generationdomain.IntentOutcomeUnknown
	case generationdomain.ProviderJobSucceeded:
		return intent.Status == generationdomain.IntentSucceeded
	case generationdomain.ProviderJobPartialSucceeded:
		return intent.Status == generationdomain.IntentPartialSucceeded
	case generationdomain.ProviderJobFailed:
		return intent.Status == generationdomain.IntentFailed
	default:
		return false
	}
}

func validReferenceAssetIntent(
	intent generationdomain.Intent,
	target generationdomain.GenerationTarget,
	command domain.NodeExecutorCommand,
) bool {
	if target.ReferenceAsset == nil {
		return false
	}
	for _, identifier := range []string{
		intent.ID, intent.WorkspaceID, intent.ProjectID, intent.WorkflowRunID, intent.NodeRunID,
		intent.TargetID, intent.BindingVersionID, intent.ConnectionVersionID, intent.CredentialVersionID,
		intent.ModelProfileVersionID, intent.PriceQuoteID, intent.CostEstimateID,
		intent.CostReservationID, intent.QuotaReservationID, intent.CostEstimateReceiptID,
		intent.CostReservationReceiptID, intent.QuotaReservationReceiptID, intent.CreatedBy,
	} {
		if !validCandidateSetUUID(identifier) {
			return false
		}
	}
	if intent.WorkspaceID != command.WorkspaceID || intent.ProjectID != command.ProjectID ||
		intent.WorkflowRunID != command.WorkflowRunID || intent.NodeRunID != command.NodeRunID ||
		intent.CreatedBy != command.InitiatorUserID || intent.InitiatorTokenVersion != command.InitiatorTokenVersion ||
		intent.InitiatorTokenVersion < 1 ||
		intent.TargetID != target.ID || intent.TargetHash != target.TargetHash ||
		intent.EstimatedUnits != int64(target.ReferenceAsset.NumberResults) || intent.BindingRevision < 1 ||
		intent.ModelProfileRevision < 1 || intent.PriceQuoteRevision < 1 ||
		intent.BillingMetric != costdomain.MetricGenerationImageCall ||
		intent.CreatedAt.IsZero() || intent.UpdatedAt.Before(intent.CreatedAt) {
		return false
	}
	for _, hash := range []string{
		intent.TargetHash, intent.BindingContentHash, intent.ModelProfileContentHash,
		intent.PriceQuoteContentHash, intent.ContentHash,
	} {
		if !candidateSetHashPattern.MatchString(hash) {
			return false
		}
	}
	providerFactsValid := validCandidateSetUUID(intent.GenerationRequestID) &&
		validCandidateSetUUID(intent.ProviderJobID) && candidateSetHashPattern.MatchString(intent.ProviderCallSetHash)
	claimValid := intent.Claimant != nil && *intent.Claimant == "workflow-node:"+command.NodeRunID &&
		intent.ClaimToken != nil && validCandidateSetUUID(*intent.ClaimToken) && intent.ClaimExpiresAt != nil &&
		intent.ClaimFencingVersion == 1 && intent.CancelledAt == nil
	ownerTerminalFactsEmpty := intent.CostReleaseReceiptID == "" && intent.QuotaReleaseReceiptID == "" &&
		intent.CostSettlementReceiptID == "" && intent.QuotaConsumptionReceiptID == ""
	switch intent.Status {
	case generationdomain.IntentPrepared:
		return intent.Revision == 2 && !providerFactsValid && intent.GenerationRequestID == "" &&
			intent.ProviderJobID == "" && intent.ProviderCallSetHash == "" && intent.Claimant == nil &&
			intent.ClaimToken == nil && intent.ClaimExpiresAt == nil && intent.ClaimFencingVersion == 0 &&
			intent.CancelledAt == nil && ownerTerminalFactsEmpty
	case generationdomain.IntentClaimed:
		return intent.Revision == 3 && !providerFactsValid && intent.GenerationRequestID == "" &&
			intent.ProviderJobID == "" && intent.ProviderCallSetHash == "" && claimValid &&
			intent.ClaimExpiresAt.After(intent.UpdatedAt) && ownerTerminalFactsEmpty
	case generationdomain.IntentExecuting, generationdomain.IntentOutcomeUnknown:
		return intent.Revision >= 4 && providerFactsValid && claimValid && ownerTerminalFactsEmpty
	case generationdomain.IntentSucceeded, generationdomain.IntentPartialSucceeded:
		return intent.Revision >= 5 && providerFactsValid && claimValid &&
			validCandidateSetUUID(intent.CostSettlementReceiptID) &&
			validCandidateSetUUID(intent.QuotaConsumptionReceiptID) &&
			intent.CostReleaseReceiptID == "" && intent.QuotaReleaseReceiptID == ""
	case generationdomain.IntentFailed:
		settled := validCandidateSetUUID(intent.CostSettlementReceiptID) &&
			validCandidateSetUUID(intent.QuotaConsumptionReceiptID) &&
			intent.CostReleaseReceiptID == "" && intent.QuotaReleaseReceiptID == ""
		released := validCandidateSetUUID(intent.CostReleaseReceiptID) &&
			validCandidateSetUUID(intent.QuotaReleaseReceiptID) &&
			intent.CostSettlementReceiptID == "" && intent.QuotaConsumptionReceiptID == ""
		return intent.Revision >= 5 && providerFactsValid && claimValid && (settled || released)
	case generationdomain.IntentCancelled:
		return intent.Revision == 3 && !providerFactsValid && intent.GenerationRequestID == "" &&
			intent.ProviderJobID == "" && intent.ProviderCallSetHash == "" && intent.Claimant == nil &&
			intent.ClaimToken == nil && intent.ClaimExpiresAt == nil && intent.ClaimFencingVersion == 0 &&
			validCandidateSetUUID(intent.CostReleaseReceiptID) &&
			validCandidateSetUUID(intent.QuotaReleaseReceiptID) &&
			intent.CostSettlementReceiptID == "" && intent.QuotaConsumptionReceiptID == "" &&
			intent.CancelledAt != nil && intent.CancelledAt.Equal(intent.UpdatedAt)
	default:
		return false
	}
}

func sameReferenceAssetPreparedFacts(left, right generationdomain.Intent) bool {
	return left.ID == right.ID && generationdomain.SameIntentBinding(left, right) &&
		left.CostEstimateID == right.CostEstimateID && left.CostReservationID == right.CostReservationID &&
		left.QuotaReservationID == right.QuotaReservationID &&
		left.CostEstimateReceiptID == right.CostEstimateReceiptID &&
		left.CostReservationReceiptID == right.CostReservationReceiptID &&
		left.QuotaReservationReceiptID == right.QuotaReservationReceiptID && left.CreatedAt.Equal(right.CreatedAt)
}

func sameReferenceAssetClaimFacts(left, right generationdomain.Intent) bool {
	return sameOptionalString(left.Claimant, right.Claimant) && sameOptionalString(left.ClaimToken, right.ClaimToken) &&
		sameOptionalTime(left.ClaimExpiresAt, right.ClaimExpiresAt) &&
		left.ClaimFencingVersion == right.ClaimFencingVersion
}

func validReferenceAssetRequest(
	request generationdomain.GenerationRequest,
	intent generationdomain.Intent,
	target generationdomain.GenerationTarget,
) bool {
	return validCandidateSetUUID(request.ID) && request.ID == intent.GenerationRequestID &&
		request.WorkspaceID == intent.WorkspaceID && request.ProjectID == intent.ProjectID &&
		request.IntentID == intent.ID && request.TargetID == target.ID && request.TargetHash == target.TargetHash &&
		request.BindingID == intent.BindingVersionID && request.BindingRevision == intent.BindingRevision &&
		request.BindingContentHash == intent.BindingContentHash &&
		request.Purpose == generationdomain.ProviderPurposeReferenceAsset &&
		referenceProviderIdentifierPattern.MatchString(request.ProviderKey) &&
		referenceProviderIdentifierPattern.MatchString(request.ExternalModelID) &&
		request.ConnectionVersionID == intent.ConnectionVersionID &&
		request.CredentialVersionID == intent.CredentialVersionID &&
		request.ModelProfileVersionID == intent.ModelProfileVersionID &&
		request.ModelProfileRevision == intent.ModelProfileRevision &&
		request.ModelProfileContentHash == intent.ModelProfileContentHash && request.PriceQuoteID == intent.PriceQuoteID &&
		request.PriceQuoteRevision == intent.PriceQuoteRevision && request.PriceQuoteContentHash == intent.PriceQuoteContentHash &&
		request.BillingMetric == intent.BillingMetric && request.EstimatedUnits == intent.EstimatedUnits &&
		request.RequestKey == "generation-request:"+request.ID && request.CreatedBy == intent.CreatedBy &&
		candidateSetHashPattern.MatchString(request.ContentHash) && !request.CreatedAt.IsZero()
}

func validReferenceAssetJob(
	job generationdomain.ProviderJob,
	intent generationdomain.Intent,
	request generationdomain.GenerationRequest,
) bool {
	if !validCandidateSetUUID(job.ID) || job.ID != intent.ProviderJobID || job.WorkspaceID != intent.WorkspaceID ||
		job.ProjectID != intent.ProjectID || job.IntentID != intent.ID || job.RequestID != request.ID ||
		job.ProviderKey != request.ProviderKey || job.RequestKey != request.RequestKey ||
		job.CallSetHash != intent.ProviderCallSetHash || !candidateSetHashPattern.MatchString(job.CallSetHash) ||
		job.CallCount != int(request.EstimatedUnits) || job.DispatchedCallCount < 0 || job.SucceededCallCount < 0 ||
		job.FailedCallCount < 0 || job.DispatchedCallCount > job.CallCount ||
		job.SucceededCallCount+job.FailedCallCount > job.CallCount || job.Revision < 1 ||
		!candidateSetHashPattern.MatchString(job.ContentHash) || job.CreatedAt.IsZero() || job.UpdatedAt.Before(job.CreatedAt) {
		return false
	}
	switch job.Status {
	case generationdomain.ProviderJobPending:
		return job.DispatchedCallCount == 0 && job.SucceededCallCount == 0 && job.FailedCallCount == 0
	case generationdomain.ProviderJobRunning:
		return job.SucceededCallCount+job.FailedCallCount < job.CallCount
	case generationdomain.ProviderJobOutcomeUnknown:
		return job.DispatchedCallCount > 0 && job.SucceededCallCount+job.FailedCallCount < job.CallCount
	case generationdomain.ProviderJobSucceeded:
		return job.SucceededCallCount == job.CallCount && job.FailedCallCount == 0
	case generationdomain.ProviderJobPartialSucceeded:
		return job.SucceededCallCount > 0 && job.FailedCallCount > 0 &&
			job.SucceededCallCount+job.FailedCallCount == job.CallCount
	case generationdomain.ProviderJobFailed:
		return job.SucceededCallCount == 0 && job.FailedCallCount == job.CallCount
	default:
		return false
	}
}

func sameOptionalString(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameOptionalTime(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
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
