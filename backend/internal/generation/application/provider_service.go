package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

const (
	submitProviderOperation    = "generation.provider.submit"
	reconcileProviderOperation = "generation.provider.reconcile"

	ProviderOutcomeAccepted  = "accepted"
	ProviderOutcomeSucceeded = "succeeded"
	ProviderOutcomeFailed    = "failed"
	ProviderOutcomeUnknown   = "unknown"
)

var (
	ErrGenerationRequestNotFound     = errors.New("generation request not found")
	ErrProviderJobNotFound           = errors.New("generation Provider job not found")
	ErrProviderResultReceiptNotFound = errors.New("generation Provider result receipt not found")
	providerIdentifierPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,119}$`)
	providerFailurePattern           = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,119}$`)
	providerOutputKeyPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

type ProviderOutput = domain.ProviderOutput

type ProviderSubmission struct {
	WorkspaceID, ProjectID, ProviderJobID string
	RequestID, RequestKey, IntentID       string
	ProviderKey, ExternalModelID          string
	ConnectionVersionID                   string
	CredentialVersionID                   string
	ModelProfileVersionID                 string
	TargetHash                            string
	Units                                 int64
	Target                                domain.GenerationTarget
}

type ProviderOutcome struct {
	Status, ProviderJobKey, ProviderEventID string
	FailureCode                             string
	ActualUnits                             int64
	Outputs                                 []ProviderOutput
	OccurredAt                              time.Time
}

type ProviderGateway interface {
	Submit(context.Context, ProviderSubmission) (ProviderOutcome, error)
	Query(context.Context, ProviderSubmission) (ProviderOutcome, error)
}

type CostProviderOwner interface {
	GetReservation(context.Context, costapp.Actor, string) (costapp.ReservationView, error)
	SettleReservation(context.Context, costapp.Actor, costapp.SettleReservationCommand) (costapp.ReservationResult, error)
	ReleaseReservation(context.Context, costapp.Actor, costapp.ReleaseReservationCommand) (costapp.ReservationResult, error)
}

type QuotaProviderOwner interface {
	GetReservation(context.Context, quotaapp.Actor, string) (quotadomain.Reservation, error)
	Consume(context.Context, quotaapp.Actor, quotaapp.TransitionCommand) (quotaapp.ReservationResult, error)
	Release(context.Context, quotaapp.Actor, quotaapp.TransitionCommand) (quotaapp.ReservationResult, error)
}

type ProviderRepository interface {
	PreparationRepository
	AuthorizeProviderProject(context.Context, Actor, string) (ProviderProjectScope, error)
	LockProviderWorkspace(context.Context, string) error
	LatestProjectProviderBindingForUpdate(context.Context, string, string, string) (domain.ProjectProviderBindingVersion, error)
	FindProjectProviderBinding(context.Context, string) (domain.ProjectProviderBindingVersion, error)
	LatestProviderConnectionForUpdate(context.Context, string, string) (domain.ProviderConnectionVersion, error)
	FindProviderModelProfile(context.Context, string) (domain.ProviderModelProfileVersion, error)
	LatestProviderModelProfileForUpdate(context.Context, string, string) (domain.ProviderModelProfileVersion, error)
	FindRequestByIntent(context.Context, string) (domain.GenerationRequest, error)
	FindGenerationRequest(context.Context, string) (domain.GenerationRequest, error)
	EnsureRequestAndJob(context.Context, domain.GenerationRequest, domain.ProviderJob) (domain.GenerationRequest, domain.ProviderJob, error)
	FindProviderJobByIntent(context.Context, string) (domain.ProviderJob, error)
	GetProviderJobForUpdate(context.Context, string) (domain.ProviderJob, error)
	UpdateProviderJob(context.Context, domain.ProviderJob, int64) (domain.ProviderJob, error)
	FindProviderResultReceiptByJob(context.Context, string) (domain.ProviderResultReceipt, error)
	EnsureProviderResultReceipt(context.Context, domain.ProviderResultReceipt) (domain.ProviderResultReceipt, error)
}

type ProviderTransactionManager interface {
	WithinProviderTransaction(
		context.Context,
		func(ProviderRepository, CostProviderOwner, QuotaProviderOwner) error,
	) error
}

type ProviderConfig struct {
	Now      func() time.Time
	NewID    func() string
	Bindings ProjectProviderBindingResolver
}

type ProjectProviderBindingResolver interface {
	ResolveProjectBinding(context.Context, Actor, string, string) (ResolvedProjectProviderBinding, error)
}

type ProviderProjectScope struct {
	WorkspaceID, ProjectID string
}

type ProviderService struct {
	transactions ProviderTransactionManager
	gateway      ProviderGateway
	config       ProviderConfig
}

type SubmitImageRequestCommand struct {
	IntentID, IdempotencyKey string
}

type ReconcileProviderJobCommand struct {
	ProviderJobID, IdempotencyKey string
}

type ProviderExecutionResult struct {
	Intent          domain.Intent
	Request         domain.GenerationRequest
	Job             domain.ProviderJob
	ProviderReceipt domain.ProviderResultReceipt
	Receipt         platformcommand.Receipt
}

type providerCommandReceipt struct {
	RequestID, JobID string
}

type generationRequestHashInput struct {
	WorkspaceID, ProjectID, IntentID, TargetID string
	BindingID                                  string
	BindingRevision                            int64
	Purpose, ProviderKey, ExternalModelID      string
	ConnectionVersionID, CredentialVersionID   string
	ModelProfileVersionID                      string
	RequestKey, TargetHash                     string
	Units                                      int64
}

type providerJobHashInput struct {
	WorkspaceID, ProjectID, IntentID, RequestID string
	ProviderKey, RequestKey, ProviderJobKey     string
	Status, ProviderReceiptID                   string
	Revision                                    int64
}

type providerReceiptHashInput struct {
	WorkspaceID, ProjectID, JobID, RequestID     string
	ProviderKey, ProviderJobKey, ProviderEventID string
	Status                                       string
	ActualUnits                                  int64
	Outputs                                      []ProviderOutput
	FailureCode                                  string
	OccurredAt                                   string
}

type providerInvocation struct {
	intent     domain.Intent
	request    domain.GenerationRequest
	job        domain.ProviderJob
	target     domain.GenerationTarget
	result     ProviderExecutionResult
	receipt    platformcommand.Receipt
	callRemote bool
	submit     bool
}

type providerBindingCandidate struct {
	resolved   ResolvedProjectProviderBinding
	resolveErr error
	required   bool
}

func NewProviderService(
	transactions ProviderTransactionManager,
	gateway ProviderGateway,
	config ProviderConfig,
) *ProviderService {
	return &ProviderService{transactions: transactions, gateway: gateway, config: config}
}

func (service *ProviderService) RequireProjectProviderBinding(
	ctx context.Context,
	actor Actor,
	projectID string,
	purpose string,
) (domain.ProjectProviderBindingVersion, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	projectID = strings.TrimSpace(projectID)
	purpose = strings.TrimSpace(purpose)
	if !service.readValid() || service.config.Bindings == nil || !validPreparationActor(actor) ||
		!validUUID(projectID) || !validProviderPurpose(purpose) {
		return domain.ProjectProviderBindingVersion{}, invalid("Invalid Project Media Provider binding request")
	}
	resolved, err := service.config.Bindings.ResolveProjectBinding(ctx, actor, projectID, purpose)
	return resolved.Binding, normalizeProviderError(err)
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
	bindingCandidate, err := service.prepareProviderBindingCandidate(ctx, authorization)
	if err != nil {
		return ProviderExecutionResult{}, normalizeProviderError(err)
	}
	invocation, err := service.prepareProviderSubmission(ctx, authorization, command, inputHash, bindingCandidate)
	if err != nil || !invocation.callRemote {
		return invocation.result, normalizeProviderError(err)
	}
	submission := providerSubmission(invocation.request, invocation.job, invocation.target)
	var outcome ProviderOutcome
	if invocation.submit {
		outcome, err = service.gateway.Submit(ctx, submission)
	} else {
		outcome, err = service.gateway.Query(ctx, submission)
	}
	if err != nil {
		outcome = ProviderOutcome{Status: ProviderOutcomeUnknown, ProviderJobKey: invocation.job.ProviderJobKey}
	}
	result, applyErr := service.applyProviderOutcome(
		ctx, invocation.job.ID, submitProviderOperation, command.IdempotencyKey, inputHash, outcome,
	)
	return result, normalizeProviderError(applyErr)
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
	invocation, err := service.prepareProviderReconcile(ctx, command, inputHash)
	if err != nil || !invocation.callRemote {
		return invocation.result, normalizeProviderError(err)
	}
	outcome, queryErr := service.gateway.Query(ctx, providerSubmission(invocation.request, invocation.job, invocation.target))
	if queryErr != nil {
		outcome = ProviderOutcome{Status: ProviderOutcomeUnknown, ProviderJobKey: invocation.job.ProviderJobKey}
	}
	result, applyErr := service.applyProviderOutcome(
		ctx, invocation.job.ID, reconcileProviderOperation, command.IdempotencyKey, inputHash, outcome,
	)
	return result, normalizeProviderError(applyErr)
}

func (service *ProviderService) RequireSucceededProviderResult(
	ctx context.Context,
	actor Actor,
	providerJobID string,
) (ProviderExecutionResult, error) {
	providerJobID = strings.TrimSpace(providerJobID)
	if !service.readValid() || !validUUID(providerJobID) || !validUUID(actor.UserID) || actor.TokenVersion < 1 {
		return ProviderExecutionResult{}, invalid("Invalid Generation Provider result request")
	}
	var result ProviderExecutionResult
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		job, loadErr := repo.GetProviderJobForUpdate(ctx, providerJobID)
		if loadErr != nil {
			return loadErr
		}
		request, loadErr := repo.FindGenerationRequest(ctx, job.RequestID)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr := repo.GetIntentForUpdate(ctx, job.IntentID)
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
		if result.Job.Status != domain.ProviderJobSucceeded ||
			result.ProviderReceipt.Status != domain.ProviderResultSucceeded {
			return conflict("Generation Provider result is not successful")
		}
		return nil
	})
	return result, normalizeProviderError(err)
}

func (service *ProviderService) prepareProviderBindingCandidate(
	ctx context.Context,
	authorization domain.ExecutionAuthorization,
) (providerBindingCandidate, error) {
	var candidate providerBindingCandidate
	var actor Actor
	var projectID string
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		_ CostProviderOwner,
		_ QuotaProviderOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, authorization.IntentID)
		if loadErr != nil {
			return loadErr
		}
		if _, requestErr := repo.FindRequestByIntent(ctx, intent.ID); requestErr == nil {
			return nil
		} else if !errors.Is(requestErr, ErrGenerationRequestNotFound) {
			return requestErr
		}
		if loadErr = validateAuthorizationBinding(intent, authorization, true); loadErr != nil {
			return loadErr
		}
		if !service.config.Now().UTC().Before(authorization.ExpiresAt) {
			return authorizationExpired()
		}
		candidate.required = true
		actor, projectID = intentActor(intent), intent.ProjectID
		return nil
	})
	if err != nil || !candidate.required {
		return candidate, err
	}
	candidate.resolved, candidate.resolveErr = service.config.Bindings.ResolveProjectBinding(
		ctx,
		actor,
		projectID,
		domain.ProviderPurposeReferenceAsset,
	)
	return candidate, nil
}

func (service *ProviderService) prepareProviderSubmission(
	ctx context.Context,
	authorization domain.ExecutionAuthorization,
	command SubmitImageRequestCommand,
	inputHash string,
	bindingCandidate providerBindingCandidate,
) (providerInvocation, error) {
	var invocation providerInvocation
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, command.IntentID)
		if loadErr != nil {
			return loadErr
		}
		actor := intentActor(intent)
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, submitProviderOperation, command.IdempotencyKey); findErr == nil {
			result, replayErr := service.replayProviderCommand(ctx, repo, costs, quotas, receipt, inputHash)
			invocation.result = result
			return replayErr
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		request, requestErr := repo.FindRequestByIntent(ctx, intent.ID)
		if requestErr == nil {
			job, jobErr := repo.FindProviderJobByIntent(ctx, intent.ID)
			if jobErr != nil {
				return jobErr
			}
			if bindingErr := validateAuthorizationBinding(intent, authorization, false); bindingErr != nil {
				return bindingErr
			}
			if factsErr := service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job); factsErr != nil {
				return factsErr
			}
			target, targetErr := repo.FindGenerationTarget(ctx, request.TargetID)
			if targetErr != nil || validateProviderTargetBinding(target, intent, request) != nil {
				return conflict("Generation Provider Target snapshot has drifted")
			}
			if providerJobTerminal(job.Status) {
				result, resultErr := service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
				if resultErr != nil {
					return resultErr
				}
				receipt, receiptErr := service.storeProviderCommandReceipt(
					ctx, repo, actor, submitProviderOperation, command.IdempotencyKey, inputHash, result, now,
				)
				if receiptErr != nil {
					return receiptErr
				}
				result.Receipt = receipt
				invocation.result = result
				return nil
			}
			invocation.intent, invocation.request, invocation.job, invocation.target = intent, request, job, target
			invocation.callRemote = true
			return nil
		}
		if !errors.Is(requestErr, ErrGenerationRequestNotFound) {
			return requestErr
		}
		if loadErr = validateAuthorizationBinding(intent, authorization, true); loadErr != nil {
			return loadErr
		}
		if !service.config.Now().UTC().Before(authorization.ExpiresAt) {
			return authorizationExpired()
		}
		if loadErr = service.validateProviderOwners(ctx, costs, quotas, intent); loadErr != nil {
			return loadErr
		}
		target, targetErr := repo.FindGenerationTarget(ctx, intent.TargetID)
		if targetErr != nil || validateIntentTargetBinding(target, intent) != nil {
			return conflict("Generation Provider Target snapshot has drifted")
		}
		if !bindingCandidate.required {
			return conflict("Project Media Provider binding candidate is missing")
		}
		if bindingCandidate.resolveErr != nil {
			return bindingCandidate.resolveErr
		}
		resolved := bindingCandidate.resolved
		binding, connection, profile := resolved.Binding, resolved.Connection, resolved.Profile
		if binding.WorkspaceID != intent.WorkspaceID || binding.ProjectID != intent.ProjectID ||
			binding.Purpose != domain.ProviderPurposeReferenceAsset ||
			connection.ID != binding.ConnectionVersionID || profile.ID != binding.ModelProfileVersionID ||
			validateProviderConnectionVersion(connection) != nil || validateProviderModelProfileVersion(profile) != nil {
			return conflict("Project Media Provider binding has drifted")
		}
		if lockErr := repo.LockProviderWorkspace(ctx, intent.WorkspaceID); lockErr != nil {
			return lockErr
		}
		latestBinding, bindingErr := repo.LatestProjectProviderBindingForUpdate(
			ctx, intent.WorkspaceID, intent.ProjectID, domain.ProviderPurposeReferenceAsset,
		)
		if bindingErr != nil || latestBinding.ID != binding.ID || latestBinding.ContentHash != binding.ContentHash {
			return conflict("Project Media Provider binding has changed before request creation")
		}
		latestConnection, bindingErr := repo.LatestProviderConnectionForUpdate(
			ctx,
			intent.WorkspaceID,
			connection.ConnectionKey,
		)
		if bindingErr != nil || latestConnection.ID != connection.ID ||
			latestConnection.ContentHash != connection.ContentHash || latestConnection.State != domain.ProviderStateEnabled {
			return conflict("Media Provider connection has changed before request creation")
		}
		latestProfile, bindingErr := repo.LatestProviderModelProfileForUpdate(
			ctx,
			intent.WorkspaceID,
			profile.ProfileKey,
		)
		if bindingErr != nil || latestProfile.ID != profile.ID || latestProfile.ContentHash != profile.ContentHash ||
			latestProfile.State != domain.ProviderStateEnabled {
			return conflict("Media Provider model profile has changed before request creation")
		}
		requestID, jobID := strings.TrimSpace(service.config.NewID()), strings.TrimSpace(service.config.NewID())
		if !validUUID(requestID) || !validUUID(jobID) {
			return errors.New("Generation Provider request identifiers are invalid")
		}
		request = domain.GenerationRequest{
			ID: requestID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
			TargetID:  intent.TargetID,
			BindingID: binding.ID, BindingRevision: binding.Revision, Purpose: binding.Purpose,
			ProviderKey: binding.ProviderKey, ExternalModelID: profile.ExternalModelID,
			ConnectionVersionID: binding.ConnectionVersionID, CredentialVersionID: binding.CredentialVersionID,
			ModelProfileVersionID: binding.ModelProfileVersionID,
			RequestKey:            "generation-request:" + requestID, TargetHash: intent.TargetHash, Units: intent.Units,
			CreatedBy: intent.CreatedBy, CreatedAt: now,
		}
		request.ContentHash, loadErr = generationRequestContentHash(request)
		if loadErr != nil {
			return loadErr
		}
		job := domain.ProviderJob{
			ID: jobID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
			RequestID: request.ID, ProviderKey: request.ProviderKey, RequestKey: request.RequestKey,
			Status: domain.ProviderJobDispatching, Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		job.ContentHash, loadErr = providerJobContentHash(job)
		if loadErr != nil {
			return loadErr
		}
		request, job, loadErr = repo.EnsureRequestAndJob(ctx, request, job)
		if loadErr != nil {
			return loadErr
		}
		intent.GenerationRequestID, intent.ProviderJobID = request.ID, job.ID
		intent.Status, intent.Revision, intent.UpdatedAt = domain.IntentDispatching, intent.Revision+1, now
		intent.ContentHash, loadErr = intentContentHash(intent)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr = repo.UpdateIntent(ctx, intent, intent.Revision-1)
		if loadErr != nil {
			return loadErr
		}
		invocation.intent, invocation.request, invocation.job, invocation.target = intent, request, job, target
		invocation.callRemote, invocation.submit = true, true
		return nil
	})
	return invocation, err
}

func (service *ProviderService) prepareProviderReconcile(
	ctx context.Context,
	command ReconcileProviderJobCommand,
	inputHash string,
) (providerInvocation, error) {
	var invocation providerInvocation
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		job, loadErr := repo.GetProviderJobForUpdate(ctx, command.ProviderJobID)
		if loadErr != nil {
			return loadErr
		}
		request, loadErr := repo.FindGenerationRequest(ctx, job.RequestID)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr := repo.GetIntentForUpdate(ctx, job.IntentID)
		if loadErr != nil {
			return loadErr
		}
		actor := intentActor(intent)
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, reconcileProviderOperation, command.IdempotencyKey); findErr == nil {
			result, replayErr := service.replayProviderCommand(ctx, repo, costs, quotas, receipt, inputHash)
			invocation.result = result
			return replayErr
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job); loadErr != nil {
			return loadErr
		}
		target, targetErr := repo.FindGenerationTarget(ctx, request.TargetID)
		if targetErr != nil || validateProviderTargetBinding(target, intent, request) != nil {
			return conflict("Generation Provider Target snapshot has drifted")
		}
		if providerJobTerminal(job.Status) {
			result, resultErr := service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
			if resultErr != nil {
				return resultErr
			}
			receipt, receiptErr := service.storeProviderCommandReceipt(
				ctx, repo, actor, reconcileProviderOperation, command.IdempotencyKey, inputHash, result, now,
			)
			if receiptErr != nil {
				return receiptErr
			}
			result.Receipt = receipt
			invocation.result = result
			return nil
		}
		invocation.intent, invocation.request, invocation.job, invocation.target = intent, request, job, target
		invocation.callRemote = true
		return nil
	})
	return invocation, err
}

func (service *ProviderService) applyProviderOutcome(
	ctx context.Context,
	jobID, operation, key, inputHash string,
	outcome ProviderOutcome,
) (ProviderExecutionResult, error) {
	var result ProviderExecutionResult
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	outcome = normalizedProviderOutcome(outcome, now)
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		job, loadErr := repo.GetProviderJobForUpdate(ctx, jobID)
		if loadErr != nil {
			return loadErr
		}
		request, loadErr := repo.FindGenerationRequest(ctx, job.RequestID)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr := repo.GetIntentForUpdate(ctx, job.IntentID)
		if loadErr != nil {
			return loadErr
		}
		actor := intentActor(intent)
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, operation, key); findErr == nil {
			result, loadErr = service.replayProviderCommand(ctx, repo, costs, quotas, receipt, inputHash)
			return loadErr
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job); loadErr != nil {
			return loadErr
		}
		if providerJobTerminal(job.Status) {
			result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
			if loadErr != nil {
				return loadErr
			}
			receipt, receiptErr := service.storeProviderCommandReceipt(
				ctx, repo, actor, operation, key, inputHash, result, now,
			)
			if receiptErr != nil {
				return receiptErr
			}
			result.Receipt = receipt
			return nil
		}
		previousJobRevision, previousIntentRevision := job.Revision, intent.Revision
		if job.ProviderJobKey != "" && outcome.ProviderJobKey != "" && job.ProviderJobKey != outcome.ProviderJobKey {
			return conflict("Generation Provider external job binding has drifted")
		}
		if outcome.ProviderJobKey != "" {
			job.ProviderJobKey = outcome.ProviderJobKey
		}
		outcome.ProviderJobKey = job.ProviderJobKey
		if outcome.Status == ProviderOutcomeSucceeded &&
			!providerOutputsUseJobStaging(outcome.Outputs, request.WorkspaceID, job.ID) {
			outcome = ProviderOutcome{
				Status: ProviderOutcomeUnknown, ProviderJobKey: job.ProviderJobKey, OccurredAt: outcome.OccurredAt,
			}
		}
		switch outcome.Status {
		case ProviderOutcomeAccepted:
			job.Status = domain.ProviderJobRunning
			intent.Status = domain.IntentSubmitted
		case ProviderOutcomeUnknown:
			job.Status = domain.ProviderJobUnknown
			intent.Status = domain.IntentOutcomeUnknown
		case ProviderOutcomeSucceeded, ProviderOutcomeFailed:
			providerReceipt, receiptErr := service.persistProviderTerminal(
				ctx, repo, costs, quotas, actor, &intent, request, job, outcome, now,
			)
			if receiptErr != nil {
				return receiptErr
			}
			job.ProviderReceiptID = providerReceipt.ID
			intent.ProviderReceiptID = providerReceipt.ID
			result.ProviderReceipt = providerReceipt
			if outcome.Status == ProviderOutcomeSucceeded {
				job.Status, intent.Status = domain.ProviderJobSucceeded, domain.IntentSucceeded
			} else {
				job.Status, intent.Status = domain.ProviderJobFailed, domain.IntentFailed
			}
		default:
			return errors.New("normalized Provider outcome is invalid")
		}
		job.Revision, job.UpdatedAt = job.Revision+1, now
		job.ContentHash, loadErr = providerJobContentHash(job)
		if loadErr != nil {
			return loadErr
		}
		job, loadErr = repo.UpdateProviderJob(ctx, job, previousJobRevision)
		if loadErr != nil {
			return loadErr
		}
		intent.Revision, intent.UpdatedAt = intent.Revision+1, now
		intent.ContentHash, loadErr = intentContentHash(intent)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr = repo.UpdateIntent(ctx, intent, previousIntentRevision)
		if loadErr != nil {
			return loadErr
		}
		result.Intent, result.Request, result.Job = intent, request, job
		receipt, receiptErr := service.storeProviderCommandReceipt(
			ctx, repo, actor, operation, key, inputHash, result, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result.Receipt = receipt
		return nil
	})
	return result, err
}

func (service *ProviderService) persistProviderTerminal(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	actor Actor,
	intent *domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	outcome ProviderOutcome,
	now time.Time,
) (domain.ProviderResultReceipt, error) {
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return domain.ProviderResultReceipt{}, errors.New("Generation Provider result receipt identifier is invalid")
	}
	status := domain.ProviderResultSucceeded
	if outcome.Status == ProviderOutcomeFailed {
		status = domain.ProviderResultFailed
	}
	receipt := domain.ProviderResultReceipt{
		ID: receiptID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID,
		JobID: job.ID, RequestID: request.ID, ProviderKey: request.ProviderKey,
		ProviderJobKey: outcome.ProviderJobKey, ProviderEventID: outcome.ProviderEventID,
		Status: status, ActualUnits: outcome.ActualUnits, Outputs: append([]ProviderOutput(nil), outcome.Outputs...),
		FailureCode: outcome.FailureCode, OccurredAt: outcome.OccurredAt.UTC(), ReceivedAt: now,
	}
	var err error
	receipt.ContentHash, err = providerResultReceiptContentHash(receipt)
	if err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	if err = validateProviderResultReceipt(receipt, request, job); err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	desired := receipt
	receipt, err = repo.EnsureProviderResultReceipt(ctx, receipt)
	if err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	if !domain.SameProviderResultReceipt(receipt, desired) {
		return domain.ProviderResultReceipt{}, conflict("Generation Provider result receipt has drifted")
	}
	costActor := costapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	quotaActor := quotaapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	if outcome.Status == ProviderOutcomeSucceeded {
		costResult, transitionErr := costs.SettleReservation(ctx, costActor, costapp.SettleReservationCommand{
			ReservationID: intent.CostReservationID, UsageReceiptID: receipt.ID,
			SettledUnits: outcome.ActualUnits, IdempotencyKey: preparationOwnerKey(intent.ID, "cost-settle"),
		})
		if transitionErr != nil {
			return domain.ProviderResultReceipt{}, transitionErr
		}
		quotaResult, transitionErr := quotas.Consume(ctx, quotaActor, quotaapp.TransitionCommand{
			ReservationID: intent.QuotaReservationID, IdempotencyKey: preparationOwnerKey(intent.ID, "quota-consume"),
		})
		if transitionErr != nil {
			return domain.ProviderResultReceipt{}, transitionErr
		}
		intent.CostSettlementReceiptID = costResult.Receipt.ID
		intent.QuotaConsumptionReceiptID = quotaResult.Receipt.ID
	} else {
		costResult, transitionErr := costs.ReleaseReservation(ctx, costActor, costapp.ReleaseReservationCommand{
			ReservationID: intent.CostReservationID, IdempotencyKey: preparationOwnerKey(intent.ID, "cost-release"),
		})
		if transitionErr != nil {
			return domain.ProviderResultReceipt{}, transitionErr
		}
		quotaResult, transitionErr := quotas.Release(ctx, quotaActor, quotaapp.TransitionCommand{
			ReservationID: intent.QuotaReservationID, IdempotencyKey: preparationOwnerKey(intent.ID, "quota-release"),
		})
		if transitionErr != nil {
			return domain.ProviderResultReceipt{}, transitionErr
		}
		intent.CostReleaseReceiptID = costResult.Receipt.ID
		intent.QuotaReleaseReceiptID = quotaResult.Receipt.ID
	}
	return receipt, nil
}

func (service *ProviderService) validateProviderFacts(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
) error {
	if validateIntent(intent) != nil || validateGenerationRequest(request) != nil || validateProviderJob(job) != nil ||
		request.IntentID != intent.ID || job.IntentID != intent.ID || job.RequestID != request.ID ||
		request.TargetID != intent.TargetID || request.TargetHash != intent.TargetHash ||
		request.WorkspaceID != intent.WorkspaceID || request.ProjectID != intent.ProjectID ||
		job.WorkspaceID != intent.WorkspaceID || job.ProjectID != intent.ProjectID ||
		request.ProviderKey != job.ProviderKey || request.RequestKey != job.RequestKey ||
		intent.GenerationRequestID != request.ID || intent.ProviderJobID != job.ID ||
		intent.ProviderReceiptID != job.ProviderReceiptID {
		return conflict("Generation Provider facts have drifted")
	}
	target, err := repo.FindGenerationTarget(ctx, request.TargetID)
	if err != nil || validateProviderTargetBinding(target, intent, request) != nil {
		return conflict("Generation Provider Target snapshot has drifted")
	}
	binding, err := repo.FindProjectProviderBinding(ctx, request.BindingID)
	if err != nil || validateProjectProviderBinding(binding) != nil || binding.WorkspaceID != request.WorkspaceID ||
		binding.ProjectID != request.ProjectID || binding.Revision != request.BindingRevision ||
		binding.Purpose != request.Purpose || binding.ProviderKey != request.ProviderKey ||
		binding.ConnectionVersionID != request.ConnectionVersionID ||
		binding.CredentialVersionID != request.CredentialVersionID ||
		binding.ModelProfileVersionID != request.ModelProfileVersionID {
		return conflict("Generation Provider binding snapshot has drifted")
	}
	profile, err := repo.FindProviderModelProfile(ctx, request.ModelProfileVersionID)
	if err != nil || profile.WorkspaceID != request.WorkspaceID || profile.ProviderKey != request.ProviderKey ||
		profile.ExternalModelID != request.ExternalModelID || profile.Modality != binding.Modality {
		return conflict("Generation Provider model profile snapshot has drifted")
	}
	if err = (&PreparationService{}).validateOwnerReceipts(ctx, repo, intent); err != nil {
		return err
	}
	return service.validateProviderOwners(ctx, costs, quotas, intent)
}

func (service *ProviderService) validateProviderOwners(
	ctx context.Context,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
) error {
	actor := intentActor(intent)
	costView, err := costs.GetReservation(ctx, costapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, intent.CostReservationID)
	if err != nil {
		return err
	}
	quotaReservation, err := quotas.GetReservation(ctx, quotaapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, intent.QuotaReservationID)
	if err != nil {
		return err
	}
	if costView.Reservation.ID != intent.CostReservationID || costView.Reservation.WorkspaceID != intent.WorkspaceID ||
		costView.Reservation.ProjectID != intent.ProjectID || costView.Reservation.SourceType != costdomain.SourceGenerationIntent ||
		costView.Reservation.SourceID != intent.ID || costView.Reservation.EstimatedUnits != intent.Units ||
		quotaReservation.ID != intent.QuotaReservationID || quotaReservation.WorkspaceID != intent.WorkspaceID ||
		quotaReservation.ProjectID != intent.ProjectID || quotaReservation.SourceType != "generation_intent" ||
		quotaReservation.SourceID != intent.ID || quotaReservation.Units != intent.Units {
		return conflict("Generation Provider Owner bindings have drifted")
	}
	switch intent.Status {
	case domain.IntentClaimed, domain.IntentDispatching, domain.IntentSubmitted, domain.IntentOutcomeUnknown:
		if costView.Reservation.Status != costdomain.ReservationReserved || quotaReservation.Status != quotadomain.ReservationReserved {
			return conflict("Generation Provider reservations are not executable")
		}
	case domain.IntentSucceeded:
		if costView.Reservation.Status != costdomain.ReservationSettled || quotaReservation.Status != quotadomain.ReservationConsumed {
			return conflict("Generation Provider success Owner facts have drifted")
		}
	case domain.IntentFailed:
		if costView.Reservation.Status != costdomain.ReservationReleased || quotaReservation.Status != quotadomain.ReservationReleased {
			return conflict("Generation Provider failure Owner facts have drifted")
		}
	default:
		return conflict("Generation Provider intent is not executable")
	}
	return nil
}

func (service *ProviderService) loadProviderExecutionResult(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
) (ProviderExecutionResult, error) {
	if err := service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job); err != nil {
		return ProviderExecutionResult{}, err
	}
	result := ProviderExecutionResult{Intent: intent, Request: request, Job: job}
	if providerJobTerminal(job.Status) {
		receipt, err := repo.FindProviderResultReceiptByJob(ctx, job.ID)
		if err != nil || validateProviderResultReceipt(receipt, request, job) != nil || receipt.ID != job.ProviderReceiptID {
			return ProviderExecutionResult{}, conflict("Generation Provider terminal receipt has drifted")
		}
		result.ProviderReceipt = receipt
	}
	return result, nil
}

func (service *ProviderService) replayProviderCommand(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	receipt platformcommand.Receipt,
	inputHash string,
) (ProviderExecutionResult, error) {
	replayed, err := platformcommand.Replay[providerCommandReceipt](receipt, inputHash)
	if err != nil || receipt.ResourceID != replayed.JobID {
		return ProviderExecutionResult{}, platformcommand.ErrInputMismatch
	}
	request, err := repo.FindGenerationRequest(ctx, replayed.RequestID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	job, err := repo.GetProviderJobForUpdate(ctx, replayed.JobID)
	if err != nil || job.RequestID != request.ID {
		return ProviderExecutionResult{}, platformcommand.ErrInputMismatch
	}
	intent, err := repo.GetIntentForUpdate(ctx, job.IntentID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	result, err := service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	result.Receipt = receipt
	return result, nil
}

func (service *ProviderService) storeProviderCommandReceipt(
	ctx context.Context,
	repo ProviderRepository,
	actor Actor,
	operation, key, inputHash string,
	result ProviderExecutionResult,
	now time.Time,
) (platformcommand.Receipt, error) {
	encoded, err := platformcommand.Result(providerCommandReceipt{RequestID: result.Request.ID, JobID: result.Job.ID})
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	return service.ensureProviderCommandReceipt(
		ctx, repo, actor, result.Intent.WorkspaceID, operation, key, inputHash, result.Job.ID, encoded, now,
	)
}

func (service *ProviderService) ensureProviderCommandReceipt(
	ctx context.Context,
	repo ProviderRepository,
	actor Actor,
	workspaceID, operation, key, inputHash, resourceID string,
	encoded []byte,
	now time.Time,
) (platformcommand.Receipt, error) {
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return platformcommand.Receipt{}, errors.New("Generation Provider command receipt identifier is invalid")
	}
	return repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: workspaceID, Operation: operation, IdempotencyKey: key,
		InputHash: inputHash, ResourceID: resourceID, Result: encoded,
		CreatedBy: actor.UserID, CreatedAt: now,
	})
}

func validateAuthorizationBinding(intent domain.Intent, authorization domain.ExecutionAuthorization, initial bool) error {
	if validateIntent(intent) != nil || !validUUID(authorization.IntentID) || !validUUID(authorization.ClaimToken) ||
		!validUUID(authorization.TargetID) || !intentHashPattern.MatchString(authorization.TargetHash) ||
		intent.ID != authorization.IntentID || intent.ClaimToken == nil || *intent.ClaimToken != authorization.ClaimToken ||
		intent.TargetID != authorization.TargetID || intent.TargetHash != authorization.TargetHash ||
		intent.CostReservationID != authorization.CostReservationID ||
		intent.QuotaReservationID != authorization.QuotaReservationID ||
		intent.ClaimFencingVersion != authorization.ClaimFencingVersion || authorization.IntentRevision != 3 ||
		intent.Units != authorization.Units || intent.ClaimExpiresAt == nil ||
		!intent.ClaimExpiresAt.Equal(authorization.ExpiresAt) {
		return conflict("Generation Provider authorization has drifted")
	}
	if initial && (intent.Status != domain.IntentClaimed || intent.Revision != authorization.IntentRevision) {
		return conflict("Generation Provider authorization is no longer claimable")
	}
	return nil
}

func validateIntentTargetBinding(target domain.GenerationTarget, intent domain.Intent) error {
	if domain.ValidateGenerationTarget(target) != nil || target.ID != intent.TargetID || target.WorkspaceID != intent.WorkspaceID ||
		target.ProjectID != intent.ProjectID || target.TargetHash != intent.TargetHash ||
		target.CreatedBy != intent.CreatedBy {
		return conflict("GenerationTarget and intent have drifted")
	}
	return nil
}

func validateProviderTargetBinding(
	target domain.GenerationTarget,
	intent domain.Intent,
	request domain.GenerationRequest,
) error {
	if err := validateIntentTargetBinding(target, intent); err != nil || request.TargetID != target.ID ||
		request.TargetHash != target.TargetHash || request.WorkspaceID != target.WorkspaceID ||
		request.ProjectID != target.ProjectID || request.CreatedBy != target.CreatedBy || request.Units != intent.Units {
		return conflict("GenerationTarget, intent and request have drifted")
	}
	return nil
}

func validateProjectProviderBinding(value domain.ProjectProviderBindingVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validProviderPurpose(value.Purpose) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		(value.Modality != domain.MediaModalityImage && value.Modality != domain.MediaModalityVideo) ||
		!validUUID(value.ConnectionVersionID) || !validUUID(value.CredentialVersionID) ||
		!validUUID(value.ModelProfileVersionID) || !providerIdentifierPattern.MatchString(value.AdapterContractVersion) ||
		value.Revision < 1 || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Generation Provider binding facts have drifted")
	}
	hash, err := projectProviderBindingContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider binding facts have drifted")
	}
	return nil
}

func validateGenerationRequest(value domain.GenerationRequest) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.IntentID) || !validUUID(value.TargetID) || !validUUID(value.BindingID) || value.BindingRevision < 1 ||
		!validProviderPurpose(value.Purpose) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		!providerIdentifierPattern.MatchString(value.ExternalModelID) || !validUUID(value.ConnectionVersionID) ||
		!validUUID(value.CredentialVersionID) || !validUUID(value.ModelProfileVersionID) ||
		value.RequestKey != "generation-request:"+value.ID || !intentHashPattern.MatchString(value.TargetHash) ||
		value.Units < 1 || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Generation request facts have drifted")
	}
	hash, err := generationRequestContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation request facts have drifted")
	}
	return nil
}

func validateProviderJob(value domain.ProviderJob) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.IntentID) || !validUUID(value.RequestID) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		value.RequestKey == "" || value.Revision < 1 || value.CreatedAt.IsZero() || value.UpdatedAt.Before(value.CreatedAt) {
		return conflict("Generation Provider job facts have drifted")
	}
	switch value.Status {
	case domain.ProviderJobDispatching, domain.ProviderJobUnknown:
		if value.ProviderReceiptID != "" {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobRunning:
		if value.ProviderJobKey == "" || value.ProviderReceiptID != "" {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobSucceeded, domain.ProviderJobFailed:
		if !validUUID(value.ProviderReceiptID) || (value.Status == domain.ProviderJobSucceeded && value.ProviderJobKey == "") {
			return conflict("Generation Provider job facts have drifted")
		}
	default:
		return conflict("Generation Provider job facts have drifted")
	}
	hash, err := providerJobContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider job facts have drifted")
	}
	return nil
}

func validateProviderResultReceipt(
	value domain.ProviderResultReceipt,
	request domain.GenerationRequest,
	job domain.ProviderJob,
) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.JobID) || !validUUID(value.RequestID) || value.WorkspaceID != request.WorkspaceID ||
		value.ProjectID != request.ProjectID || value.JobID != job.ID || value.RequestID != request.ID ||
		value.ProviderKey != request.ProviderKey || value.ProviderJobKey != job.ProviderJobKey ||
		value.ProviderEventID == "" || len(value.ProviderEventID) > 180 || value.OccurredAt.IsZero() ||
		value.ReceivedAt.Before(value.OccurredAt) {
		return conflict("Generation Provider result receipt has drifted")
	}
	if value.Status == domain.ProviderResultSucceeded {
		if value.ActualUnits < 1 || value.ActualUnits > request.Units || len(value.Outputs) != int(value.ActualUnits) ||
			value.FailureCode != "" || !validProviderOutputs(value.Outputs) ||
			!providerOutputsUseJobStaging(value.Outputs, request.WorkspaceID, job.ID) {
			return conflict("Generation Provider result receipt has drifted")
		}
	} else if value.Status == domain.ProviderResultFailed {
		if value.ActualUnits != 0 || len(value.Outputs) != 0 || !providerFailurePattern.MatchString(value.FailureCode) {
			return conflict("Generation Provider result receipt has drifted")
		}
	} else {
		return conflict("Generation Provider result receipt has drifted")
	}
	hash, err := providerResultReceiptContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider result receipt has drifted")
	}
	return nil
}

func normalizedProviderOutcome(value ProviderOutcome, now time.Time) ProviderOutcome {
	value.Status = strings.TrimSpace(value.Status)
	value.ProviderJobKey = strings.TrimSpace(value.ProviderJobKey)
	value.ProviderEventID = strings.TrimSpace(value.ProviderEventID)
	value.FailureCode = strings.TrimSpace(value.FailureCode)
	if value.OccurredAt.IsZero() {
		value.OccurredAt = now
	} else {
		value.OccurredAt = value.OccurredAt.UTC().Truncate(time.Microsecond)
	}
	if value.OccurredAt.After(now) {
		return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
	}
	validJobKey := value.ProviderJobKey == "" || len(value.ProviderJobKey) <= 180
	switch value.Status {
	case ProviderOutcomeAccepted:
		if validJobKey && value.ProviderJobKey != "" && value.ProviderEventID == "" && value.FailureCode == "" &&
			value.ActualUnits == 0 && len(value.Outputs) == 0 {
			return value
		}
	case ProviderOutcomeSucceeded:
		if validJobKey && value.ProviderJobKey != "" && value.ProviderEventID != "" && len(value.ProviderEventID) <= 180 &&
			value.FailureCode == "" && value.ActualUnits > 0 && len(value.Outputs) == int(value.ActualUnits) &&
			validProviderOutputs(value.Outputs) {
			return value
		}
	case ProviderOutcomeFailed:
		if validJobKey && value.ProviderEventID != "" && len(value.ProviderEventID) <= 180 &&
			providerFailurePattern.MatchString(value.FailureCode) && value.ActualUnits == 0 && len(value.Outputs) == 0 {
			return value
		}
	case ProviderOutcomeUnknown:
		if validJobKey {
			return ProviderOutcome{Status: ProviderOutcomeUnknown, ProviderJobKey: value.ProviderJobKey, OccurredAt: value.OccurredAt}
		}
	}
	return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
}

func validProviderOutputs(outputs []ProviderOutput) bool {
	seen := make(map[string]struct{}, len(outputs))
	for _, output := range outputs {
		if !providerOutputKeyPattern.MatchString(output.OutputKey) || output.StagingObjectKey == "" ||
			len(output.StagingObjectKey) > 512 || !intentHashPattern.MatchString(output.SHA256) || output.Bytes < 1 ||
			(output.MediaType != "image/png" && output.MediaType != "image/jpeg") || output.Width < 1 || output.Height < 1 {
			return false
		}
		if _, exists := seen[output.OutputKey]; exists {
			return false
		}
		seen[output.OutputKey] = struct{}{}
	}
	return len(outputs) > 0
}

func providerOutputsUseJobStaging(outputs []ProviderOutput, workspaceID, providerJobID string) bool {
	prefix := "staging/" + workspaceID + "/" + providerJobID + "/"
	for _, output := range outputs {
		if !strings.HasPrefix(output.StagingObjectKey, prefix) || strings.Contains(output.StagingObjectKey, "..") ||
			strings.HasSuffix(output.StagingObjectKey, "/") {
			return false
		}
	}
	return len(outputs) > 0
}

func providerJobTerminal(status string) bool {
	return status == domain.ProviderJobSucceeded || status == domain.ProviderJobFailed
}

func providerSubmission(
	request domain.GenerationRequest,
	job domain.ProviderJob,
	target domain.GenerationTarget,
) ProviderSubmission {
	return ProviderSubmission{
		WorkspaceID: request.WorkspaceID, ProjectID: request.ProjectID, ProviderJobID: job.ID,
		RequestID: request.ID, RequestKey: request.RequestKey, IntentID: request.IntentID,
		ProviderKey: request.ProviderKey, ExternalModelID: request.ExternalModelID,
		ConnectionVersionID: request.ConnectionVersionID, CredentialVersionID: request.CredentialVersionID,
		ModelProfileVersionID: request.ModelProfileVersionID,
		TargetHash:            request.TargetHash, Units: request.Units, Target: target,
	}
}

func generationRequestContentHash(value domain.GenerationRequest) (string, error) {
	return platformcommand.InputHash(generationRequestHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID, TargetID: value.TargetID,
		BindingID: value.BindingID, BindingRevision: value.BindingRevision, Purpose: value.Purpose,
		ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID,
		ConnectionVersionID: value.ConnectionVersionID, CredentialVersionID: value.CredentialVersionID,
		ModelProfileVersionID: value.ModelProfileVersionID,
		RequestKey:            value.RequestKey, TargetHash: value.TargetHash, Units: value.Units,
	})
}

func providerJobContentHash(value domain.ProviderJob) (string, error) {
	return platformcommand.InputHash(providerJobHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID,
		RequestID: value.RequestID, ProviderKey: value.ProviderKey, RequestKey: value.RequestKey,
		ProviderJobKey: value.ProviderJobKey, Status: value.Status, ProviderReceiptID: value.ProviderReceiptID,
		Revision: value.Revision,
	})
}

func providerResultReceiptContentHash(value domain.ProviderResultReceipt) (string, error) {
	return platformcommand.InputHash(providerReceiptHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, JobID: value.JobID, RequestID: value.RequestID,
		ProviderKey: value.ProviderKey, ProviderJobKey: value.ProviderJobKey, ProviderEventID: value.ProviderEventID,
		Status: value.Status, ActualUnits: value.ActualUnits, Outputs: value.Outputs, FailureCode: value.FailureCode,
		OccurredAt: value.OccurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func (service *ProviderService) valid() bool {
	return service.readValid() && service.config.Bindings != nil && service.gateway != nil
}

func (service *ProviderService) readValid() bool {
	return service != nil && service.transactions != nil && service.config.Now != nil && service.config.NewID != nil
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
		errors.Is(err, ErrProviderResultReceiptNotFound) {
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
