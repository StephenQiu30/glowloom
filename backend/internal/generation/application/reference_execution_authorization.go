package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const AuthorizeInitialReferenceExecutionOperation = "generation.reference.authorize_execution_initial"

type AuthorizeInitialReferenceExecutionCommand struct {
	WorkspaceID                string                       `json:"workspace_id"`
	ProjectID                  string                       `json:"project_id"`
	TargetRef                  domain.GenerationRevisionRef `json:"generation_target_ref"`
	SelectedProviderBindingRef domain.GenerationRevisionRef `json:"selected_project_provider_binding_version_ref"`
	IdempotencyKey             string                       `json:"idempotency_key"`
}

type ReferenceExecutionAuthorizationRepository interface {
	ReferenceGenerationTargetReadRepository
	referenceExecutionProviderRepository
	LockProviderWorkspace(context.Context, string) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
}

type ReferenceExecutionAuthorizationTransactions interface {
	WithinReferenceExecutionAuthorization(context.Context, func(ReferenceExecutionAuthorizationRepository) error) error
}

type ReferenceExecutionAuthorizationService struct {
	transactions ReferenceExecutionAuthorizationTransactions
	now          func() time.Time
	newID        func() string
}

func NewReferenceExecutionAuthorizationService(transactions ReferenceExecutionAuthorizationTransactions, now func() time.Time, newID func() string) (*ReferenceExecutionAuthorizationService, error) {
	if transactions == nil || now == nil || newID == nil {
		return nil, errors.New("Reference execution authorization dependencies are required")
	}
	return &ReferenceExecutionAuthorizationService{transactions, now, newID}, nil
}

func (service *ReferenceExecutionAuthorizationService) AuthorizeInitial(ctx context.Context, actor Actor, command AuthorizeInitialReferenceExecutionCommand) (domain.ReferenceExecutionAuthorization, error) {
	for _, value := range []string{actor.UserID, command.WorkspaceID, command.ProjectID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return domain.ReferenceExecutionAuthorization{}, invalid("Invalid Reference execution authorization scope")
		}
	}
	if actor.TokenVersion < 1 || !command.TargetRef.Valid() || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return domain.ReferenceExecutionAuthorization{}, invalid("Invalid initial Reference execution authorization command")
	}
	if command.SelectedProviderBindingRef == (domain.GenerationRevisionRef{}) {
		return domain.ReferenceExecutionAuthorization{}, referenceProviderConfigurationRequired(nil)
	}
	if !command.SelectedProviderBindingRef.Valid() {
		return domain.ReferenceExecutionAuthorization{}, invalid("Invalid selected Provider binding identity")
	}
	var result domain.ReferenceExecutionAuthorization
	err := service.transactions.WithinReferenceExecutionAuthorization(ctx, func(repo ReferenceExecutionAuthorizationRepository) error {
		// Configuration writers lock Workspace before Provider versions. Acquire
		// that lock before Target's SHARE locks, avoiding lock upgrades.
		if err := repo.LockProviderWorkspace(ctx, command.WorkspaceID); err != nil {
			return err
		}
		target, err := ReadCurrentReferenceGenerationTarget(ctx, repo, actor, ReadReferenceGenerationTargetQuery{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, TargetRef: command.TargetRef})
		if err != nil {
			return err
		}
		provider, err := readReferenceExecutionProvider(ctx, repo, command.WorkspaceID, command.ProjectID, command.SelectedProviderBindingRef)
		if err != nil {
			return err
		}
		inputHash, err := referenceExecutionAuthorizationInputHash(actor, command, target.TargetReadSetRoot, provider)
		if err != nil {
			return err
		}
		expected := domain.InitialReferenceExecutionAuthorizationInput{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, GenerationTargetRef: command.TargetRef, SelectedProjectProviderBindingVersionRef: command.SelectedProviderBindingRef, MembershipTokenVersion: actor.TokenVersion, AuthorizedBy: actor.UserID}
		receipt, err := repo.FindReceipt(ctx, command.WorkspaceID, AuthorizeInitialReferenceExecutionOperation, command.IdempotencyKey)
		if err == nil {
			result, err = replayReferenceExecutionAuthorization(receipt, inputHash, command, expected)
			return err
		}
		if !errors.Is(err, platformcommand.ErrReceiptNotFound) {
			return err
		}
		expected.HumanActionRef, expected.AuthorizedAt = service.newID(), service.now().UTC().Truncate(time.Microsecond)
		result, err = domain.BuildInitialReferenceExecutionAuthorization(expected)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		receipt, err = repo.EnsureReceipt(ctx, platformcommand.Receipt{ID: result.HumanActionRef, WorkspaceID: command.WorkspaceID, Operation: AuthorizeInitialReferenceExecutionOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: target.ID, Result: raw, CreatedBy: actor.UserID, CreatedAt: result.AuthorizedAt})
		if err != nil {
			return err
		}
		result, err = replayReferenceExecutionAuthorization(receipt, inputHash, command, expected)
		return err
	})
	if err != nil {
		return domain.ReferenceExecutionAuthorization{}, err
	}
	return result, nil
}

func replayReferenceExecutionAuthorization(receipt platformcommand.Receipt, inputHash string, command AuthorizeInitialReferenceExecutionCommand, expected domain.InitialReferenceExecutionAuthorizationInput) (domain.ReferenceExecutionAuthorization, error) {
	if receipt.InputHash != inputHash {
		return domain.ReferenceExecutionAuthorization{}, platformcommand.ErrInputMismatch
	}
	value, err := domain.DecodeReferenceExecutionAuthorization(receipt.Result)
	if err != nil {
		return domain.ReferenceExecutionAuthorization{}, err
	}
	expected.HumanActionRef, expected.AuthorizedAt = receipt.ID, receipt.CreatedAt
	if receipt.WorkspaceID != command.WorkspaceID || receipt.Operation != AuthorizeInitialReferenceExecutionOperation || receipt.IdempotencyKey != command.IdempotencyKey || receipt.ResourceID != command.TargetRef.ID || receipt.CreatedBy != expected.AuthorizedBy || !reflect.DeepEqual(value.InitialReferenceExecutionAuthorizationInput, expected) {
		return domain.ReferenceExecutionAuthorization{}, conflict("Reference execution authorization receipt has drifted")
	}
	return value, nil
}

func referenceExecutionAuthorizationInputHash(actor Actor, command AuthorizeInitialReferenceExecutionCommand, targetReadSetRoot string, provider referenceExecutionProviderFacts) (string, error) {
	return platformcommand.InputHash(struct {
		Actor                 Actor
		Command               AuthorizeInitialReferenceExecutionCommand
		TargetReadSetRoot     string
		ConnectionContentHash string
		ProfileContentHash    string
		CredentialFingerprint string
	}{actor, command, targetReadSetRoot, provider.connection.ContentHash, provider.profile.ContentHash, provider.credential.SecretFingerprint})
}
