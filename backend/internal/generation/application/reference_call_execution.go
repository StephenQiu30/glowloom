package application

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

type ReferenceCallExecutionService struct {
	dispatch *ReferenceCallDispatchService
	secrets  ProviderRuntimeSecrets
}

// Execution transactions must own the physical commit; a nested savepoint cannot
// authorize external IO. Dispatch-only storage tests may use a different port.
type ReferenceCallExecutionTransactions interface {
	WithinReferenceCallExecution(context.Context, func(ReferenceCallDispatchRepository) error) error
}

type standaloneReferenceDispatch struct {
	ReferenceCallExecutionTransactions
}

func (tx standaloneReferenceDispatch) WithinReferenceCallDispatch(ctx context.Context, operation func(ReferenceCallDispatchRepository) error) error {
	return tx.WithinReferenceCallExecution(ctx, operation)
}

func NewReferenceCallExecutionService(transactions ReferenceCallExecutionTransactions, registry *MediaFactoryRegistry, secrets ProviderRuntimeSecrets, now func() time.Time, newID func() string) (*ReferenceCallExecutionService, error) {
	if secrets == nil {
		return nil, errors.New("Reference execution secret store is required")
	}
	if transactions == nil {
		return nil, errors.New("Reference execution standalone transactions are required")
	}
	dispatch, err := NewReferenceCallDispatchService(standaloneReferenceDispatch{transactions}, registry, now, newID)
	if err != nil {
		return nil, err
	}
	return &ReferenceCallExecutionService{dispatch: dispatch, secrets: secrets}, nil
}

// Execute owns preflight, the committing Claim, one Submit, and receipt recording.
// A stored dispatch flag/token is never an input or a retry instruction.
func (service *ReferenceCallExecutionService) Execute(ctx context.Context, actor Actor, command ClaimReferenceCallCommand) (domain.ReferenceCallState, error) {
	if !validReferenceCallScope(actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey) || command.ExpectedRevision != 1 {
		return domain.ReferenceCallState{}, invalid("Invalid Reference execution command")
	}
	var state domain.ReferenceCallState
	var inputs referenceExecutionInputs
	var submission ReferenceImageSubmission
	err := service.dispatch.transactions.WithinReferenceCallDispatch(ctx, func(repo ReferenceCallDispatchRepository) error {
		execution, current, err := readReferenceCall(ctx, repo, actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey)
		if err != nil {
			return err
		}
		state = current
		if state.Status != domain.ProviderCallPending {
			return nil
		}
		inputs, err = readReferenceDispatchInputs(ctx, repo, service.dispatch.registry, actor, execution)
		if err != nil {
			return err
		}
		_, calls, err := domain.BuildReferenceProviderJob(command.ExecutionRef, inputs.calls)
		if err != nil {
			return err
		}
		for _, call := range calls {
			if call.CallKey != command.CallKey {
				continue
			}
			for _, request := range inputs.compiled.Requests {
				if request.BundleIndex != call.BundleIndex || request.SlotKey != call.SlotKey {
					continue
				}
				for _, slot := range inputs.target.OutputContract.Slots {
					if slot.SlotKey == call.SlotKey {
						submission = ReferenceImageSubmission{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Call: call, Request: request, Slot: slot}
						return nil
					}
				}
			}
		}
		return conflict("Reference call is missing its frozen request or output slot")
	})
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	if state.Status != domain.ProviderCallPending {
		return state, nil
	}
	factory, err := service.dispatch.registry.Resolve(inputs.provider.connection.ProviderKey, inputs.provider.profile.Modality, inputs.provider.connection.AdapterContractVersion)
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	executable, ok := factory.(ReferenceImageExecutableFactory)
	if !ok {
		return domain.ReferenceCallState{}, errors.New("Reference image adapter is not executable")
	}
	credential := inputs.provider.credential
	plaintext, err := service.secrets.Decrypt(ctx, domain.ProviderSecretContext{WorkspaceID: credential.WorkspaceID, ProviderKey: credential.ProviderKey, CredentialID: credential.ID, Revision: credential.Revision, KeyID: credential.KeyID}, domain.EncryptedProviderSecret{CipherSuite: credential.CipherSuite, KeyID: credential.KeyID, Nonce: credential.Nonce, Ciphertext: credential.Ciphertext, Fingerprint: credential.SecretFingerprint})
	defer wipeProviderSecret(plaintext)
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	runtime, err := executable.NewReferenceImageRuntime(ProviderRuntimeConfig{Connection: inputs.provider.connection, Profile: inputs.provider.profile, Credentials: plaintext})
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	if runtime == nil {
		return domain.ReferenceCallState{}, errors.New("Reference image runtime is unavailable")
	}
	if err = runtime.Preflight(ctx, submission); err != nil {
		return domain.ReferenceCallState{}, err
	}
	// Revalidate after preflight/secret IO. Frozen read-set equality prevents the
	// reconstructed body above from becoming detached from the committed claim.
	claim, err := service.dispatch.Claim(ctx, actor, command)
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	if !claim.ShouldDispatch {
		return claim.State, nil
	}
	observation, submitErr := runtime.Submit(ctx, submission, *claim.State.Dispatch)
	if submitErr == nil && observation.Status != ReferenceImageStaged && observation.Status != ReferenceImageOutputRejected && observation.Status != ReferenceImageOutcomeUnknown {
		return domain.ReferenceCallState{}, conflict("Reference adapter returned an unsupported observation")
	}
	if submitErr != nil {
		// The runtime contract permits an error only before HTTP was attempted.
		// Do not persist arbitrary adapter errors or leak their raw response text.
		observation = ReferenceImageObservation{CallKey: command.CallKey, SubmissionToken: claim.State.Dispatch.SubmissionToken, ObservedAt: service.dispatch.now(), Status: "not_sent", ReasonCode: "submit_not_attempted"}
	}
	if observation.CallKey != command.CallKey || observation.SubmissionToken != claim.State.Dispatch.SubmissionToken {
		return domain.ReferenceCallState{}, conflict("Reference adapter observation has a foreign dispatch identity")
	}
	receipt, err := domain.BuildReferenceCallReceipt(domain.ReferenceCallReceiptInput{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Call: submission.Call, SubmissionToken: observation.SubmissionToken, Slot: submission.Slot, ObservedAt: observation.ObservedAt, Disposition: observation.Status, ReasonCode: observation.ReasonCode, Output: observation.Output, Usage: observation.Usage})
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	return service.record(ctx, actor, receipt)
}

func (service *ReferenceCallExecutionService) record(ctx context.Context, actor Actor, receipt domain.ReferenceCallReceipt) (domain.ReferenceCallState, error) {
	var result domain.ReferenceCallState
	err := service.dispatch.transactions.WithinReferenceCallDispatch(ctx, func(repo ReferenceCallDispatchRepository) error {
		execution, before, err := readReferenceCall(ctx, repo, actor, receipt.WorkspaceID, receipt.ProjectID, receipt.Call.ExecutionRef, receipt.Call.CallKey)
		if err != nil {
			return err
		}
		// Historical receipt recording must not depend on a still-current Head,
		// latest provider, or recompiling business sources after remote execution.
		target, err := repo.FindReferenceGenerationTarget(ctx, receipt.WorkspaceID, receipt.ProjectID, execution.ReadSet.TargetRef.ID)
		if err != nil {
			return err
		}
		if referenceGenerationTargetRef(target) != execution.ReadSet.TargetRef {
			return conflict("Reference receipt Target identity has drifted")
		}
		matched := false
		for _, slot := range target.OutputContract.Slots {
			if reflect.DeepEqual(slot, receipt.Slot) {
				matched = true
				break
			}
		}
		if !matched {
			return conflict("Reference receipt output policy differs from frozen Target")
		}
		after, changed, err := domain.RecordReferenceCallReceipt(before, receipt)
		if err != nil {
			return err
		}
		if changed {
			if err := repo.UpdateReferenceCallState(ctx, receipt.WorkspaceID, receipt.ProjectID, execution.ID, before, after); err != nil {
				return err
			}
		}
		result = after
		return nil
	})
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	return result, nil
}

func readReferenceDispatchInputs(ctx context.Context, repo ReferenceExecutionRepository, registry *MediaFactoryRegistry, actor Actor, execution domain.ReferenceExecution) (referenceExecutionInputs, error) {
	receipt, err := repo.FindReferenceExecutionReceipt(ctx, execution.WorkspaceID, execution.ID)
	if err != nil {
		return referenceExecutionInputs{}, err
	}
	prepared := PrepareInitialReferenceExecutionCommand{WorkspaceID: execution.WorkspaceID, ProjectID: execution.ProjectID, TargetRef: execution.ReadSet.TargetRef, AuthorizationRef: execution.ReadSet.AuthorizationRef, IdempotencyKey: receipt.IdempotencyKey}
	inputs, err := readReferenceExecutionInputs(ctx, repo, registry, actor, prepared, readFrozenReferenceExecutionProvider)
	if err != nil {
		return referenceExecutionInputs{}, err
	}
	preparer := Actor{UserID: execution.CreatedBy, TokenVersion: execution.MembershipTokenVersion}
	hash, err := referenceExecutionPreparationInputHash(preparer, prepared, inputs.readSet)
	if err != nil {
		return referenceExecutionInputs{}, err
	}
	if hash != receipt.InputHash {
		return referenceExecutionInputs{}, platformcommand.ErrInputMismatch
	}
	if _, err = validateReferenceExecutionPublication(ctx, repo, preparer, prepared, inputs, receipt, execution); err != nil {
		return referenceExecutionInputs{}, err
	}
	return inputs, nil
}
