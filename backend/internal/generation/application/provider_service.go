package application

import (
	"context"
	"errors"
	"strings"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
)

type ProviderConfig struct {
	Now   func() time.Time
	NewID func() string
}

type ProviderService struct {
	transactions ProviderTransactionManager
	gateway      ProviderGateway
	config       ProviderConfig
}

func NewProviderService(
	transactions ProviderTransactionManager,
	gateway ProviderGateway,
	config ProviderConfig,
) *ProviderService {
	return &ProviderService{transactions: transactions, gateway: gateway, config: config}
}

func (service *ProviderService) SubmitImageRequest(
	ctx context.Context,
	authorization domain.ExecutionAuthorization,
	command SubmitImageRequestCommand,
) (ProviderExecutionResult, error) {
	command.IntentID = strings.TrimSpace(command.IntentID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if !service.valid() || !validUUID(command.IntentID) || command.IntentID != strings.TrimSpace(authorization.IntentID) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderExecutionResult{}, invalid("Invalid Generation Provider submission")
	}
	inputHash, err := platformcommand.InputHash(struct {
		Authorization domain.ExecutionAuthorization
		Command       SubmitImageRequestCommand
	}{Authorization: authorization, Command: command})
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	invocation, err := service.prepareSubmission(ctx, authorization, command, inputHash)
	if err != nil || invocation.action == providerActionNone {
		return invocation.result, normalizeProviderError(err)
	}
	return service.executeInvocation(ctx, invocation)
}

func (service *ProviderService) ReconcileProviderJob(
	ctx context.Context,
	command ReconcileProviderJobCommand,
) (ProviderExecutionResult, error) {
	command.ProviderJobID = strings.TrimSpace(command.ProviderJobID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if !service.valid() || !validUUID(command.ProviderJobID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return ProviderExecutionResult{}, invalid("Invalid Generation Provider reconciliation")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	invocation, err := service.prepareReconciliation(ctx, command, inputHash)
	if err != nil || invocation.action == providerActionNone {
		return invocation.result, normalizeProviderError(err)
	}
	return service.executeInvocation(ctx, invocation)
}

func (service *ProviderService) RequireMaterializableProviderResult(
	ctx context.Context,
	actor Actor,
	providerJobID string,
) (ProviderExecutionResult, error) {
	providerJobID = strings.TrimSpace(providerJobID)
	if !service.readValid() || !validPreparationActor(actor) || !validUUID(providerJobID) {
		return ProviderExecutionResult{}, invalid("Invalid Generation Provider result request")
	}
	var result ProviderExecutionResult
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		intent, loadErr := repo.GetIntentForProviderJobUpdate(ctx, providerJobID)
		if loadErr != nil {
			return loadErr
		}
		job, loadErr := repo.GetProviderJobForUpdate(ctx, providerJobID)
		if loadErr != nil {
			return loadErr
		}
		if job.IntentID != intent.ID {
			return conflict("Generation Provider job and intent have drifted")
		}
		request, loadErr := repo.FindGenerationRequest(ctx, job.RequestID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if actor.UserID != intent.CreatedBy || actor.TokenVersion != intent.InitiatorTokenVersion {
			return conflict("Generation Provider result actor has drifted")
		}
		result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
		if loadErr != nil {
			return loadErr
		}
		if result.Job.Status != domain.ProviderJobSucceeded && result.Job.Status != domain.ProviderJobPartialSucceeded {
			return conflict("Generation Provider result is not materializable")
		}
		return nil
	})
	return result, normalizeProviderError(err)
}

func (service *ProviderService) valid() bool {
	return service.readValid() && service.gateway != nil
}

func (service *ProviderService) readValid() bool {
	return service != nil && service.transactions != nil && service.config.Now != nil && service.config.NewID != nil
}

func (service *ProviderService) now() time.Time {
	return service.config.Now().UTC().Truncate(time.Microsecond)
}

func normalizeProviderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Generation Provider command or facts have drifted")
	}
	if errors.Is(err, ErrProjectProviderBindingNotFound) {
		return notFound("Generation Provider binding is not set")
	}
	if errors.Is(err, ErrGenerationTargetNotFound) {
		return notFound("GenerationTarget not found")
	}
	if errors.Is(err, ErrGenerationRequestNotFound) || errors.Is(err, ErrProviderJobNotFound) ||
		errors.Is(err, ErrProviderCallNotFound) || errors.Is(err, ErrProviderResultReceiptNotFound) {
		return notFound("Generation Provider execution was not found")
	}
	var costError *costapp.Error
	if errors.As(err, &costError) {
		return &Error{Code: costError.Code, Message: costError.Message, NextAction: costError.NextAction, Status: costError.Status}
	}
	var quotaError *quotaapp.Error
	if errors.As(err, &quotaError) {
		return &Error{Code: quotaError.Code, Message: quotaError.Message, NextAction: quotaError.NextAction, Status: quotaError.Status}
	}
	return err
}
