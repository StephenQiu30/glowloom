package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const PrepareInitialReferenceExecutionOperation = "generation.reference.prepare_execution_initial"

type PrepareInitialReferenceExecutionCommand struct {
	WorkspaceID          string                       `json:"workspace_id"`
	ProjectID            string                       `json:"project_id"`
	TargetRef            domain.GenerationRevisionRef `json:"generation_target_ref"`
	AuthorizationRef     domain.GenerationActionRef   `json:"execution_authorization_ref"`
	ExpectedHeadRevision int64                        `json:"expected_execution_head_revision"`
	IdempotencyKey       string                       `json:"idempotency_key"`
}

type ReferenceExecutionRepository interface {
	ReferenceExecutionAuthorizationRepository
	PublishInitialReferenceExecution(context.Context, domain.ReferenceExecution) error
	FindReferenceExecution(context.Context, string, string, string) (domain.ReferenceExecution, error)
	FindReferenceExecutionReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	ValidateReferenceExecutionHead(context.Context, domain.ReferenceExecution) error
	PublishReferenceProviderJob(context.Context, string, string, domain.ReferenceProviderJob, []domain.ReferenceProviderCall) error
	FindReferenceProviderJob(context.Context, string, string, string) (domain.ReferenceProviderJob, []domain.ReferenceProviderCall, error)
}

type ReferenceExecutionTransactions interface {
	WithinReferenceExecution(context.Context, func(ReferenceExecutionRepository) error) error
}

type ReferenceExecutionPreparationService struct {
	transactions ReferenceExecutionTransactions
	registry     *MediaFactoryRegistry
	now          func() time.Time
	newID        func() string
}

func NewReferenceExecutionPreparationService(transactions ReferenceExecutionTransactions, registry *MediaFactoryRegistry, now func() time.Time, newID func() string) (*ReferenceExecutionPreparationService, error) {
	if transactions == nil || registry == nil || now == nil || newID == nil {
		return nil, errors.New("Reference execution preparation dependencies are required")
	}
	return &ReferenceExecutionPreparationService{transactions, registry, now, newID}, nil
}

// PrepareInitial does not dispatch and must not be used to recover an existing
// remote call. Recovery must retain that call's frozen Provider identity.
func (service *ReferenceExecutionPreparationService) PrepareInitial(ctx context.Context, actor Actor, command PrepareInitialReferenceExecutionCommand) (domain.ReferenceExecution, error) {
	if !validUUID(actor.UserID) || !validUUID(command.WorkspaceID) || !validUUID(command.ProjectID) || actor.TokenVersion < 1 || !command.TargetRef.Valid() || !command.AuthorizationRef.Valid() || command.ExpectedHeadRevision != 0 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return domain.ReferenceExecution{}, invalid("Invalid initial Reference execution command")
	}
	var result domain.ReferenceExecution
	err := service.transactions.WithinReferenceExecution(ctx, func(repo ReferenceExecutionRepository) error {
		if err := repo.LockProviderWorkspace(ctx, command.WorkspaceID); err != nil {
			return err
		}
		inputs, err := readReferenceExecutionInputs(ctx, repo, service.registry, actor, command, readReferenceExecutionProvider)
		if err != nil {
			return err
		}
		readSet := inputs.readSet
		inputHash, err := referenceExecutionPreparationInputHash(actor, command, readSet)
		if err != nil {
			return err
		}
		receipt, err := repo.FindReceipt(ctx, command.WorkspaceID, PrepareInitialReferenceExecutionOperation, command.IdempotencyKey)
		if err == nil {
			if receipt.InputHash != inputHash {
				return platformcommand.ErrInputMismatch
			}
			persisted, readErr := repo.FindReferenceExecution(ctx, command.WorkspaceID, command.ProjectID, receipt.ResourceID)
			if readErr != nil {
				return readErr
			}
			result, err = validateReferenceExecutionPublication(ctx, repo, actor, command, inputs, receipt, persisted)
			return err
		}
		if !errors.Is(err, platformcommand.ErrReceiptNotFound) {
			return err
		}
		result, err = domain.BuildInitialReferenceExecution(domain.InitialReferenceExecutionInput{ID: service.newID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ReadSet: readSet, CreatedBy: actor.UserID, MembershipTokenVersion: actor.TokenVersion, CreatedAt: service.now()})
		if err != nil {
			return err
		}
		if err = repo.PublishInitialReferenceExecution(ctx, result); err != nil {
			return err
		}
		job, calls, err := domain.BuildReferenceProviderJob(domain.GenerationRevisionRef{ID: result.ID, Revision: result.Revision, ContentHash: result.ContentHash}, inputs.calls)
		if err != nil {
			return err
		}
		if err = repo.PublishReferenceProviderJob(ctx, command.WorkspaceID, command.ProjectID, job, calls); err != nil {
			return err
		}
		raw, err := json.Marshal(domain.GenerationRevisionRef{ID: result.ID, Revision: result.Revision, ContentHash: result.ContentHash})
		if err != nil {
			return err
		}
		receipt, err = repo.EnsureReceipt(ctx, platformcommand.Receipt{ID: service.newID(), WorkspaceID: command.WorkspaceID, Operation: PrepareInitialReferenceExecutionOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: result.ID, Result: raw, CreatedBy: actor.UserID, CreatedAt: result.CreatedAt})
		if err != nil {
			return err
		}
		if receipt.InputHash != inputHash {
			return platformcommand.ErrInputMismatch
		}
		result, err = validateReferenceExecutionPublication(ctx, repo, actor, command, inputs, receipt, result)
		return err
	})
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	return result, nil
}

type referenceExecutionInputs struct {
	readSet domain.ReferenceExecutionReadSet
	calls   []domain.ReferenceProviderCallInput
}

func readReferenceExecutionInputs(ctx context.Context, repo ReferenceExecutionRepository, registry *MediaFactoryRegistry, actor Actor, command PrepareInitialReferenceExecutionCommand, readProvider func(context.Context, referenceExecutionProviderRepository, string, string, domain.GenerationRevisionRef) (referenceExecutionProviderFacts, error)) (referenceExecutionInputs, error) {
	var result referenceExecutionInputs
	target, err := ReadCurrentReferenceGenerationTarget(ctx, repo, actor, ReadReferenceGenerationTargetQuery{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, TargetRef: command.TargetRef})
	if err != nil {
		return result, err
	}
	receipt, err := repo.FindReferenceAuthorization(ctx, command.AuthorizationRef.ID)
	if err != nil {
		return result, err
	}
	authorization, err := domain.DecodeReferenceExecutionAuthorization(receipt.Result)
	if err != nil {
		return result, err
	}
	if authorization.HumanActionRef != command.AuthorizationRef.ID || authorization.ContentHash != command.AuthorizationRef.ContentHash || authorization.GenerationTargetRef != command.TargetRef || authorization.WorkspaceID != command.WorkspaceID || authorization.ProjectID != command.ProjectID {
		return result, conflict("Reference execution authorization scope has drifted")
	}
	author := Actor{UserID: authorization.AuthorizedBy, TokenVersion: authorization.MembershipTokenVersion}
	if err = repo.AuthorizeReferenceGenerationProject(ctx, author, command.WorkspaceID, command.ProjectID); err != nil {
		return result, err
	}
	provider, err := readProvider(ctx, repo, command.WorkspaceID, command.ProjectID, authorization.SelectedProjectProviderBindingVersionRef)
	if err != nil {
		return result, err
	}
	original := AuthorizeInitialReferenceExecutionCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, TargetRef: command.TargetRef, SelectedProviderBindingRef: authorization.SelectedProjectProviderBindingVersionRef, IdempotencyKey: receipt.IdempotencyKey}
	hash, err := referenceExecutionAuthorizationInputHash(author, original, target.TargetReadSetRoot, provider)
	if err != nil {
		return result, err
	}
	if _, err = replayReferenceExecutionAuthorization(receipt, hash, original, authorization.InitialReferenceExecutionAuthorizationInput); err != nil {
		return result, err
	}
	compiler, descriptor, releaseHash, err := registry.referenceCompiler(provider.connection.ProviderKey, provider.profile.Modality, provider.connection.AdapterContractVersion)
	if err != nil {
		return result, err
	}
	if len(provider.connection.ResolvedConfig) != 0 {
		return result, invalid("Unsupported Reference Provider connection configuration")
	}
	brief, err := repo.ReadReferenceGenerationBrief(ctx, target.WorkspaceID, target.ProjectID, target.ReferenceBriefRevisionRef.ID, target.ReferenceBriefRevisionRef.RevisionHash)
	if err != nil {
		return result, err
	}
	compiled, err := compiler.CompileReferenceImages(target, brief, provider.profile)
	if err != nil {
		return result, err
	}
	limits := domain.DefaultReferenceGenerationLimits()
	if err = validateReferenceCompilationManifest(target, provider.profile, descriptor, limits, compiled); err != nil {
		return result, err
	}
	policyRef, err := limits.Ref()
	if err != nil {
		return result, err
	}
	result.readSet = domain.ReferenceExecutionReadSet{TargetRef: command.TargetRef, TargetReadSetRoot: target.TargetReadSetRoot, ExpectedHeadRevision: 0, BindingRef: authorization.SelectedProjectProviderBindingVersionRef, ConnectionRef: domain.GenerationRevisionRef{ID: provider.connection.ID, Revision: provider.connection.Revision, ContentHash: provider.connection.ContentHash}, CredentialRef: domain.ReferenceCredentialRef{ID: provider.credential.ID, Revision: provider.credential.Revision, Fingerprint: provider.credential.SecretFingerprint}, ProfileRef: domain.GenerationRevisionRef{ID: provider.profile.ID, Revision: provider.profile.Revision, ContentHash: provider.profile.ContentHash}, RegistryReleaseHash: releaseHash, AdapterRef: descriptor.AdapterRef, CompilerRef: descriptor.CompilerRef, CapabilityHash: descriptor.CapabilityHash, ManifestHash: compiled.ManifestHash, OperationalPolicyRef: policyRef, AuthorizationRef: command.AuthorizationRef}
	result.calls = make([]domain.ReferenceProviderCallInput, len(compiled.Requests))
	for i, request := range compiled.Requests {
		result.calls[i] = domain.ReferenceProviderCallInput{BundleIndex: request.BundleIndex, SlotKey: request.SlotKey, CompiledRequestHash: request.ContentHash}
	}
	return result, nil
}

func validateReferenceCompilationManifest(target ReferenceGenerationTarget, profile domain.ProviderModelProfileVersion, descriptor ReferenceImageCompilerDescriptor, limits domain.ReferenceGenerationLimits, compiled ReferenceImageCompilation) error {
	count := target.OutputContract.CandidateBundleCount
	slots := target.OutputContract.Slots
	if count > limits.MaxBundles || len(slots) > limits.MaxSlots || count*len(slots) > limits.MaxDailyCalls || len(compiled.Requests) != count*len(slots) || compiled.Descriptor != descriptor || compiled.ContractID != descriptor.CompilerRef.ContractID || compiled.TargetRef != referenceGenerationTargetRef(target) || compiled.BriefRef != target.ReferenceBriefRevisionRef || compiled.ProfileRef != (domain.GenerationRevisionRef{ID: profile.ID, Revision: profile.Revision, ContentHash: profile.ContentHash}) {
		return conflict("Reference compiled request manifest does not match frozen input")
	}
	for i, request := range compiled.Requests {
		slot := slots[i%len(slots)]
		hash, err := canonical.Hash(request.Body)
		if err != nil || len(request.Body) == 0 || int64(len(request.Body)) > limits.MaxInputBytes || slot.MaxBytes > limits.MaxOutputBytes || request.BundleIndex != i/len(slots) || request.SlotKey != slot.SlotKey || hash != request.ContentHash {
			return conflict("Reference compiled request slot identity has drifted")
		}
	}
	expected := compiled.ManifestHash
	compiled.ManifestHash = ""
	raw, err := json.Marshal(compiled)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return err
	}
	if expected != hash {
		return conflict("Reference compiled request manifest hash has drifted")
	}
	return nil
}

func validateReferenceExecutionPublication(ctx context.Context, repo ReferenceExecutionRepository, actor Actor, command PrepareInitialReferenceExecutionCommand, inputs referenceExecutionInputs, receipt platformcommand.Receipt, value domain.ReferenceExecution) (domain.ReferenceExecution, error) {
	expected, err := domain.BuildInitialReferenceExecution(domain.InitialReferenceExecutionInput{ID: value.ID, WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ReadSet: inputs.readSet, CreatedBy: actor.UserID, MembershipTokenVersion: actor.TokenVersion, CreatedAt: receipt.CreatedAt})
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	var ref domain.GenerationRevisionRef
	if canonical.Decode(receipt.Result, &ref) != nil || ref != (domain.GenerationRevisionRef{ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash}) || !reflect.DeepEqual(expected, value) || !validUUID(receipt.ID) || receipt.WorkspaceID != command.WorkspaceID || receipt.Operation != PrepareInitialReferenceExecutionOperation || receipt.IdempotencyKey != command.IdempotencyKey || receipt.ResourceID != value.ID || receipt.CreatedBy != actor.UserID {
		return domain.ReferenceExecution{}, conflict("Reference execution publication receipt has drifted")
	}
	publication, err := repo.FindReferenceExecutionReceipt(ctx, command.WorkspaceID, value.ID)
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	if publication.ID != receipt.ID {
		return domain.ReferenceExecution{}, conflict("Reference execution has an ambiguous publication")
	}
	if err = repo.ValidateReferenceExecutionHead(ctx, value); err != nil {
		return domain.ReferenceExecution{}, err
	}
	expectedJob, expectedCalls, err := domain.BuildReferenceProviderJob(ref, inputs.calls)
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	job, calls, err := repo.FindReferenceProviderJob(ctx, command.WorkspaceID, command.ProjectID, value.ID)
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	if !reflect.DeepEqual(job, expectedJob) || !reflect.DeepEqual(calls, expectedCalls) {
		return domain.ReferenceExecution{}, conflict("Reference Provider call set differs from frozen compilation")
	}
	return value, nil
}

func referenceExecutionPreparationInputHash(actor Actor, command PrepareInitialReferenceExecutionCommand, readSet domain.ReferenceExecutionReadSet) (string, error) {
	return platformcommand.InputHash(struct {
		Actor   Actor
		Command PrepareInitialReferenceExecutionCommand
		ReadSet domain.ReferenceExecutionReadSet
	}{actor, command, readSet})
}
