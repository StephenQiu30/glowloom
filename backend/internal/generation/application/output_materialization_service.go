package application

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

type MaterializableProviderResultOwner interface {
	RequireMaterializableProviderResult(context.Context, Actor, string) (ProviderExecutionResult, error)
}

type ProviderOutputAssetCommand struct {
	WorkspaceID, ProjectID, ProviderJobID string
	ProviderCallID, ProviderReceiptID     string
	Output                                ProviderOutput
	ExpectedWidth, ExpectedHeight         int
	RegisterIdempotencyKey                string
	ValidateIdempotencyKey                string
}

type ProviderOutputAssetOwner interface {
	EnsureProviderOutputReady(context.Context, Actor, ProviderOutputAssetCommand) (ReadyArtifact, error)
}

type ReadyCandidateOwner interface {
	RegisterReadyCandidate(context.Context, Actor, RegisterReadyCandidateCommand) (RegisterCandidateResult, error)
	RequireEvaluatedProviderOutput(context.Context, Actor, string, string, string, string) (domain.CandidateWithReport, error)
}

type OutputMaterializationService struct {
	providers  MaterializableProviderResultOwner
	assets     ProviderOutputAssetOwner
	candidates ReadyCandidateOwner
}

type MaterializeProviderOutputsCommand struct {
	ProviderJobID string
}

type MaterializedProviderOutput struct {
	ProviderCallID, ProviderReceiptID string
	Output                            ProviderOutput
	Artifact                          ReadyArtifact
	Candidate                         domain.Candidate
	Report                            domain.QCReport
}

type OutputMaterializationResult struct {
	ProviderReceiptSetHash string
	Outputs                []MaterializedProviderOutput
	CandidateSet           domain.CandidateSet
}

type materializableProviderOutput struct {
	Call    domain.ProviderCall
	Receipt domain.ProviderResultReceipt
	Output  ProviderOutput
}

type providerReceiptSetHashItem struct {
	CallID, CallContentHash       string
	CallStatus, LocalFailureCode  string
	ReceiptID, ReceiptContentHash string
}

func NewOutputMaterializationService(
	providers MaterializableProviderResultOwner,
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
		!validUUID(command.ProviderJobID) || !validPreparationActor(actor) {
		return OutputMaterializationResult{}, invalid("Invalid Provider output materialization request")
	}
	execution, err := service.providers.RequireMaterializableProviderResult(ctx, actor, command.ProviderJobID)
	if err != nil {
		return OutputMaterializationResult{}, err
	}
	outputs, receiptSetHash, err := materializableProviderOutputs(execution)
	if err != nil {
		return OutputMaterializationResult{}, err
	}
	expectedWidth, expectedHeight, err := materializationTargetDimensions(execution)
	if err != nil {
		return OutputMaterializationResult{}, err
	}
	result := OutputMaterializationResult{
		ProviderReceiptSetHash: receiptSetHash,
		Outputs:                make([]MaterializedProviderOutput, 0, len(outputs)),
	}
	for _, source := range outputs {
		if source.Output.Width != expectedWidth || source.Output.Height != expectedHeight {
			return OutputMaterializationResult{}, conflict("Provider output dimensions do not match the frozen GenerationTarget")
		}
		keySuffix := source.Receipt.ID + ":" + source.Output.OutputKey
		artifact, materializeErr := service.assets.EnsureProviderOutputReady(ctx, actor, ProviderOutputAssetCommand{
			WorkspaceID: execution.Intent.WorkspaceID, ProjectID: execution.Intent.ProjectID,
			ProviderJobID: execution.Job.ID, ProviderCallID: source.Call.ID,
			ProviderReceiptID: source.Receipt.ID, Output: source.Output,
			ExpectedWidth: expectedWidth, ExpectedHeight: expectedHeight,
			RegisterIdempotencyKey: "provider-output-register:" + keySuffix,
			ValidateIdempotencyKey: "provider-output-validate:" + keySuffix,
		})
		if materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		if materializeErr = validateMaterializedArtifact(execution, source, artifact); materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		candidate, materializeErr := service.candidates.RegisterReadyCandidate(ctx, actor, RegisterReadyCandidateCommand{
			ArtifactID: artifact.ID, ProviderJobID: execution.Job.ID, ProviderCallID: source.Call.ID,
			ProviderReceiptID: source.Receipt.ID, OutputKey: source.Output.OutputKey,
			IdempotencyKey: "provider-output-candidate:" + keySuffix,
		})
		if materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		if materializeErr = validateMaterializedCandidate(execution, source, artifact, candidate); materializeErr != nil {
			return OutputMaterializationResult{}, materializeErr
		}
		result.Outputs = append(result.Outputs, MaterializedProviderOutput{
			ProviderCallID: source.Call.ID, ProviderReceiptID: source.Receipt.ID,
			Output: source.Output, Artifact: artifact, Candidate: candidate.Candidate, Report: candidate.Report,
		})
	}
	result.CandidateSet, err = buildCandidateSet(execution, receiptSetHash, candidateBundles(result.Outputs))
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
		!validUUID(providerJobID) || !validPreparationActor(actor) {
		return domain.CandidateSet{}, invalid("Invalid generation CandidateSet request")
	}
	execution, err := service.providers.RequireMaterializableProviderResult(ctx, actor, providerJobID)
	if err != nil {
		return domain.CandidateSet{}, err
	}
	outputs, receiptSetHash, err := materializableProviderOutputs(execution)
	if err != nil {
		return domain.CandidateSet{}, err
	}
	bundles := make([]domain.CandidateWithReport, 0, len(outputs))
	for _, source := range outputs {
		bundle, loadErr := service.candidates.RequireEvaluatedProviderOutput(
			ctx,
			actor,
			execution.Job.ID,
			source.Call.ID,
			source.Receipt.ID,
			source.Output.OutputKey,
		)
		if loadErr != nil {
			return domain.CandidateSet{}, loadErr
		}
		if bundle.Candidate.ProviderJobID != execution.Job.ID || bundle.Candidate.ProviderCallID != source.Call.ID ||
			bundle.Candidate.ProviderReceiptID != source.Receipt.ID || bundle.Candidate.OutputKey != source.Output.OutputKey ||
			bundle.Candidate.ArtifactSHA256 != source.Output.SHA256 || bundle.Candidate.MediaType != source.Output.MediaType ||
			bundle.Candidate.Width != source.Output.Width || bundle.Candidate.Height != source.Output.Height {
			return domain.CandidateSet{}, conflict("Generation CandidateSet Provider output has drifted")
		}
		bundles = append(bundles, bundle)
	}
	return buildCandidateSet(execution, receiptSetHash, bundles)
}

func materializableProviderOutputs(
	execution ProviderExecutionResult,
) ([]materializableProviderOutput, string, error) {
	if err := validateMaterializationSource(execution); err != nil {
		return nil, "", err
	}
	receipts := make(map[string]domain.ProviderResultReceipt, len(execution.Receipts))
	for _, receipt := range execution.Receipts {
		receipts[receipt.CallID] = receipt
	}
	outputs := make([]materializableProviderOutput, 0, execution.Job.SucceededCallCount)
	hashItems := make([]providerReceiptSetHashItem, 0, len(execution.Calls))
	for _, call := range execution.Calls {
		receipt := receipts[call.ID]
		hashItems = append(hashItems, providerReceiptSetHashItem{
			CallID: call.ID, CallContentHash: call.ContentHash, CallStatus: call.Status,
			LocalFailureCode: call.LocalFailureCode, ReceiptID: receipt.ID, ReceiptContentHash: receipt.ContentHash,
		})
		if call.Status != domain.ProviderCallSucceeded {
			continue
		}
		receipt, exists := receipts[call.ID]
		if !exists || receipt.Status != domain.ProviderResultSucceeded || receipt.Output == nil {
			return nil, "", conflict("Provider output materialization receipt set has drifted")
		}
		outputs = append(outputs, materializableProviderOutput{Call: call, Receipt: receipt, Output: *receipt.Output})
	}
	if len(outputs) == 0 || len(outputs) != execution.Job.SucceededCallCount {
		return nil, "", conflict("Provider output materialization has no successful Calls")
	}
	receiptSetHash, err := platformcommand.InputHash(hashItems)
	if err != nil {
		return nil, "", err
	}
	return outputs, receiptSetHash, nil
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
	providerReceiptSetHash string,
	bundles []domain.CandidateWithReport,
) (domain.CandidateSet, error) {
	if len(bundles) != execution.Job.SucceededCallCount || !intentHashPattern.MatchString(providerReceiptSetHash) {
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
	contentHash, err := platformcommand.InputHash(struct {
		ProviderReceiptSetHash string
		Candidates             []domain.CandidateReference
	}{ProviderReceiptSetHash: providerReceiptSetHash, Candidates: references})
	if err != nil {
		return domain.CandidateSet{}, err
	}
	return domain.CandidateSet{
		ID: execution.Job.ID, WorkspaceID: execution.Intent.WorkspaceID, ProjectID: execution.Intent.ProjectID,
		ProviderReceiptSetHash: providerReceiptSetHash, Candidates: references, ContentHash: contentHash, Revision: 1,
	}, nil
}

func validateMaterializationSource(value ProviderExecutionResult) error {
	if validateIntent(value.Intent) != nil || validateGenerationRequest(value.Request) != nil ||
		validateProviderJob(value.Job) != nil ||
		(value.Intent.Status != domain.IntentSucceeded && value.Intent.Status != domain.IntentPartialSucceeded) ||
		(value.Job.Status != domain.ProviderJobSucceeded && value.Job.Status != domain.ProviderJobPartialSucceeded) ||
		value.Intent.GenerationRequestID != value.Request.ID || value.Intent.ProviderJobID != value.Job.ID ||
		value.Intent.ProviderCallSetHash != value.Job.CallSetHash ||
		validateProviderTargetBinding(value.Target, value.Intent, value.Request) != nil ||
		validateProviderCalls(value.Calls, value.Request, value.Job) != nil ||
		validateProviderReceiptSet(value.Calls, value.Receipts) != nil {
		return conflict("Provider output materialization source has drifted")
	}
	return nil
}

func validateMaterializedArtifact(
	execution ProviderExecutionResult,
	source materializableProviderOutput,
	artifact ReadyArtifact,
) error {
	for _, identifier := range []string{artifact.ID, artifact.WorkspaceID, artifact.ProjectID, artifact.SourceID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("Asset Owner returned an invalid Provider output identifier")
		}
	}
	if artifact.WorkspaceID != execution.Intent.WorkspaceID || artifact.ProjectID != execution.Intent.ProjectID ||
		artifact.SourceType != "generation_provider_receipt" || artifact.SourceID != source.Receipt.ID ||
		artifact.OutputKey != source.Output.OutputKey || artifact.SHA256 != source.Output.SHA256 ||
		artifact.SizeBytes != source.Output.Bytes || artifact.MediaType != source.Output.MediaType ||
		artifact.Width != source.Output.Width || artifact.Height != source.Output.Height || artifact.Revision < 2 {
		return conflict("Provider output and READY Artifact have drifted")
	}
	return nil
}

func validateMaterializedCandidate(
	execution ProviderExecutionResult,
	source materializableProviderOutput,
	artifact ReadyArtifact,
	value RegisterCandidateResult,
) error {
	if value.Candidate.WorkspaceID != execution.Intent.WorkspaceID ||
		value.Candidate.ProjectID != execution.Intent.ProjectID ||
		value.Candidate.ProviderJobID != execution.Job.ID || value.Candidate.ProviderCallID != source.Call.ID ||
		value.Candidate.ProviderReceiptID != source.Receipt.ID || value.Candidate.OutputKey != source.Output.OutputKey ||
		value.Candidate.ArtifactID != artifact.ID || value.Candidate.ArtifactRevision != artifact.Revision ||
		value.Candidate.ArtifactSHA256 != artifact.SHA256 || value.Candidate.MediaType != artifact.MediaType ||
		value.Candidate.Width != artifact.Width || value.Candidate.Height != artifact.Height ||
		value.Report.CandidateID != value.Candidate.ID || value.Report.WorkspaceID != value.Candidate.WorkspaceID {
		return conflict("Provider output Candidate facts have drifted")
	}
	return nil
}

func materializationTargetDimensions(execution ProviderExecutionResult) (int, int, error) {
	if err := validateProviderTargetBinding(execution.Target, execution.Intent, execution.Request); err != nil {
		return 0, 0, conflict("Provider output GenerationTarget has drifted")
	}
	switch execution.Target.Kind {
	case domain.GenerationTargetReferenceAsset:
		if execution.Target.ReferenceAsset == nil {
			return 0, 0, conflict("Provider output GenerationTarget has drifted")
		}
		return execution.Target.ReferenceAsset.Width, execution.Target.ReferenceAsset.Height, nil
	case domain.GenerationTargetShotFrame:
		if execution.Target.ShotFrame == nil {
			return 0, 0, conflict("Provider output GenerationTarget has drifted")
		}
		return execution.Target.ShotFrame.Width, execution.Target.ShotFrame.Height, nil
	default:
		return 0, 0, conflict("Provider output GenerationTarget has an unsupported modality")
	}
}
