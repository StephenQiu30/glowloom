package application

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type SucceededProviderResultOwner interface {
	RequireSucceededProviderResult(context.Context, Actor, string) (ProviderExecutionResult, error)
}

type ProviderOutputAssetCommand struct {
	WorkspaceID, ProjectID, ProviderJobID, ProviderReceiptID string
	Output                                                   ProviderOutput
	RegisterIdempotencyKey, ValidateIdempotencyKey           string
}

type ProviderOutputAssetOwner interface {
	EnsureProviderOutputReady(context.Context, Actor, ProviderOutputAssetCommand) (ReadyArtifact, error)
}

type ReadyCandidateOwner interface {
	RegisterReadyCandidate(context.Context, Actor, RegisterReadyCandidateCommand) (RegisterCandidateResult, error)
	RequireEvaluatedProviderOutput(context.Context, Actor, string, string) (domain.CandidateWithReport, error)
}

type OutputMaterializationService struct {
	providers  SucceededProviderResultOwner
	assets     ProviderOutputAssetOwner
	candidates ReadyCandidateOwner
}

type MaterializeProviderOutputsCommand struct {
	ProviderJobID string
}

type MaterializedProviderOutput struct {
	Output    ProviderOutput
	Artifact  ReadyArtifact
	Candidate domain.Candidate
	Report    domain.QCReport
}

type OutputMaterializationResult struct {
	ProviderReceiptID string
	Outputs           []MaterializedProviderOutput
	CandidateSet      domain.CandidateSet
}

func NewOutputMaterializationService(
	providers SucceededProviderResultOwner,
	assets ProviderOutputAssetOwner,
	candidates ReadyCandidateOwner,
) *OutputMaterializationService {
	return &OutputMaterializationService{providers: providers, assets: assets, candidates: candidates}
}

func (service *OutputMaterializationService) MaterializeSucceededOutputs(
	ctx context.Context,
	actor Actor,
	command MaterializeProviderOutputsCommand,
) (OutputMaterializationResult, error) {
	command.ProviderJobID = strings.TrimSpace(command.ProviderJobID)
	if service == nil || service.providers == nil || service.assets == nil || service.candidates == nil ||
		!validUUID(command.ProviderJobID) || !validUUID(actor.UserID) || actor.TokenVersion < 1 {
		return OutputMaterializationResult{}, invalid("Invalid Provider output materialization request")
	}
	execution, err := service.providers.RequireSucceededProviderResult(ctx, actor, command.ProviderJobID)
	if err != nil {
		return OutputMaterializationResult{}, err
	}
	if err = validateMaterializationSource(execution); err != nil {
		return OutputMaterializationResult{}, err
	}
	result := OutputMaterializationResult{
		ProviderReceiptID: execution.ProviderReceipt.ID,
		Outputs:           make([]MaterializedProviderOutput, 0, len(execution.ProviderReceipt.Outputs)),
	}
	for _, output := range execution.ProviderReceipt.Outputs {
		keySuffix := execution.ProviderReceipt.ID + ":" + output.OutputKey
		artifact, materializeErr := service.assets.EnsureProviderOutputReady(ctx, actor, ProviderOutputAssetCommand{
			WorkspaceID: execution.Intent.WorkspaceID, ProjectID: execution.Intent.ProjectID,
			ProviderJobID: execution.Job.ID, ProviderReceiptID: execution.ProviderReceipt.ID, Output: output,
			RegisterIdempotencyKey: "provider-output-register:" + keySuffix,
			ValidateIdempotencyKey: "provider-output-validate:" + keySuffix,
		})
		if materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		if materializeErr = validateMaterializedArtifact(execution, output, artifact); materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		candidate, materializeErr := service.candidates.RegisterReadyCandidate(ctx, actor, RegisterReadyCandidateCommand{
			ArtifactID: artifact.ID, IdempotencyKey: "provider-output-candidate:" + keySuffix,
		})
		if materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		if materializeErr = validateMaterializedCandidate(execution, output, artifact, candidate); materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		result.Outputs = append(result.Outputs, MaterializedProviderOutput{
			Output: output, Artifact: artifact, Candidate: candidate.Candidate, Report: candidate.Report,
		})
	}
	result.CandidateSet, err = buildCandidateSet(execution, candidateBundles(result.Outputs))
	if err != nil {
		return OutputMaterializationResult{}, err
	}
	return result, nil
}

func (service *OutputMaterializationService) RequireCandidateSet(
	ctx context.Context,
	actor Actor,
	providerJobID string,
) (domain.CandidateSet, error) {
	providerJobID = strings.TrimSpace(providerJobID)
	if service == nil || service.providers == nil || service.candidates == nil ||
		!validUUID(providerJobID) || !validUUID(actor.UserID) || actor.TokenVersion < 1 {
		return domain.CandidateSet{}, invalid("Invalid generation CandidateSet request")
	}
	execution, err := service.providers.RequireSucceededProviderResult(ctx, actor, providerJobID)
	if err != nil {
		return domain.CandidateSet{}, err
	}
	if err = validateMaterializationSource(execution); err != nil {
		return domain.CandidateSet{}, err
	}
	bundles := make([]domain.CandidateWithReport, 0, len(execution.ProviderReceipt.Outputs))
	for _, output := range execution.ProviderReceipt.Outputs {
		bundle, loadErr := service.candidates.RequireEvaluatedProviderOutput(ctx, actor, execution.Job.ID, output.OutputKey)
		if loadErr != nil {
			return domain.CandidateSet{}, loadErr
		}
		if bundle.Candidate.ProviderJobID != execution.Job.ID || bundle.Candidate.OutputKey != output.OutputKey ||
			bundle.Candidate.ArtifactSHA256 != output.SHA256 || bundle.Candidate.MediaType != output.MediaType ||
			bundle.Candidate.Width != output.Width || bundle.Candidate.Height != output.Height {
			return domain.CandidateSet{}, conflict("Generation CandidateSet Provider output has drifted")
		}
		bundles = append(bundles, bundle)
	}
	return buildCandidateSet(execution, bundles)
}

func candidateBundles(outputs []MaterializedProviderOutput) []domain.CandidateWithReport {
	bundles := make([]domain.CandidateWithReport, len(outputs))
	for index := range outputs {
		bundles[index] = domain.CandidateWithReport{Candidate: outputs[index].Candidate, Report: outputs[index].Report}
	}
	return bundles
}

func buildCandidateSet(
	execution ProviderExecutionResult,
	bundles []domain.CandidateWithReport,
) (domain.CandidateSet, error) {
	if len(bundles) != len(execution.ProviderReceipt.Outputs) {
		return domain.CandidateSet{}, conflict("Generation CandidateSet is not fully materialized")
	}
	references := make([]domain.CandidateReference, 0, len(bundles))
	for _, bundle := range bundles {
		if bundle.Candidate.Status != domain.CandidateQCPassed || bundle.Report.Status != domain.QCPassed {
			continue
		}
		references = append(references, domain.CandidateReference{
			ID: bundle.Candidate.ID, Revision: bundle.Candidate.Revision,
			ArtifactID: bundle.Candidate.ArtifactID, ArtifactRevision: bundle.Candidate.ArtifactRevision,
			ArtifactSHA256: bundle.Candidate.ArtifactSHA256,
			QCReportID:     bundle.Report.ID, QCReportHash: bundle.Report.ReportHash,
		})
	}
	slices.SortFunc(references, func(left, right domain.CandidateReference) int {
		return strings.Compare(left.ID, right.ID)
	})
	if len(references) == 0 {
		return domain.CandidateSet{}, conflict("Generation CandidateSet has no QC-passed candidate")
	}
	contentHash, err := candidateReferencesHash(references)
	if err != nil {
		return domain.CandidateSet{}, err
	}
	return domain.CandidateSet{
		ID: execution.Job.ID, WorkspaceID: execution.Intent.WorkspaceID, ProjectID: execution.Intent.ProjectID,
		ProviderReceiptID: execution.ProviderReceipt.ID, Candidates: references, ContentHash: contentHash, Revision: 1,
	}, nil
}

func validateMaterializationSource(value ProviderExecutionResult) error {
	if validateIntent(value.Intent) != nil || validateGenerationRequest(value.Request) != nil ||
		validateProviderJob(value.Job) != nil ||
		validateProviderResultReceipt(value.ProviderReceipt, value.Request, value.Job) != nil ||
		value.Intent.Status != domain.IntentSucceeded || value.Job.Status != domain.ProviderJobSucceeded ||
		value.ProviderReceipt.Status != domain.ProviderResultSucceeded ||
		value.Intent.ProviderReceiptID != value.ProviderReceipt.ID || value.Job.ProviderReceiptID != value.ProviderReceipt.ID {
		return conflict("Provider output materialization source has drifted")
	}
	return nil
}

func validateMaterializedArtifact(
	execution ProviderExecutionResult,
	output ProviderOutput,
	artifact ReadyArtifact,
) error {
	for _, identifier := range []string{artifact.ID, artifact.WorkspaceID, artifact.ProjectID, artifact.SourceID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("Asset Owner returned an invalid Provider output identifier")
		}
	}
	if artifact.WorkspaceID != execution.Intent.WorkspaceID || artifact.ProjectID != execution.Intent.ProjectID ||
		artifact.SourceType != "generation_provider_job" || artifact.SourceID != execution.Job.ID ||
		artifact.OutputKey != output.OutputKey || artifact.SHA256 != output.SHA256 ||
		artifact.SizeBytes != output.Bytes || artifact.MediaType != output.MediaType ||
		artifact.Width != output.Width || artifact.Height != output.Height || artifact.Revision < 2 {
		return conflict("Provider output and READY Artifact have drifted")
	}
	return nil
}

func validateMaterializedCandidate(
	execution ProviderExecutionResult,
	output ProviderOutput,
	artifact ReadyArtifact,
	value RegisterCandidateResult,
) error {
	if value.Candidate.WorkspaceID != execution.Intent.WorkspaceID ||
		value.Candidate.ProjectID != execution.Intent.ProjectID ||
		value.Candidate.ProviderJobID != execution.Job.ID || value.Candidate.OutputKey != output.OutputKey ||
		value.Candidate.ArtifactID != artifact.ID || value.Candidate.ArtifactRevision != artifact.Revision ||
		value.Candidate.ArtifactSHA256 != artifact.SHA256 || value.Candidate.MediaType != artifact.MediaType ||
		value.Candidate.Width != artifact.Width || value.Candidate.Height != artifact.Height ||
		value.Report.CandidateID != value.Candidate.ID || value.Report.WorkspaceID != value.Candidate.WorkspaceID {
		return conflict("Provider output Candidate facts have drifted")
	}
	return nil
}
