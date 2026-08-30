package application

import (
	"context"
	"errors"
	"regexp"
	"sort"
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
	terminalProviderOperation  = "generation.provider.terminal"

	ProviderOutcomeAccepted  = "accepted"
	ProviderOutcomeRunning   = "running"
	ProviderOutcomeSucceeded = "succeeded"
	ProviderOutcomeFailed    = "failed"
	ProviderOutcomeUnknown   = "unknown"

	providerActionNone      = "none"
	providerActionSubmit    = "submit"
	providerActionQuery     = "query"
	providerActionRecover   = "recover"
	providerActionExpire    = "expire"
	providerActionAbandon   = "abandon"
	providerPreflightFailed = "provider.preflight_failed"

	ProviderQueryFailureRetryable             = "retryable"
	ProviderQueryFailureIdentityUnrecoverable = "identity_unrecoverable"
	ProviderSubmitFailureIdentityRecoverable  = "identity_recoverable"
)

var (
	ErrGenerationRequestNotFound     = errors.New("generation request not found")
	ErrProviderJobNotFound           = errors.New("generation Provider job not found")
	ErrProviderCallNotFound          = errors.New("generation Provider call not found")
	ErrProviderResultReceiptNotFound = errors.New("generation Provider result receipt not found")
	providerIdentifierPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,179}$`)
	providerFailurePattern           = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,119}$`)
	providerOutputKeyPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

type ProviderOutput = domain.ProviderOutput

type ProviderSubmission struct {
	WorkspaceID, ProjectID, ProviderJobID string
	ProviderCallID, CallKey               string
	CallRequestHash                       string
	CandidateIndex, RequestedOutputCount  int
	RequestID, RequestKey, IntentID       string
	ProviderKey, ExternalModelID          string
	ConnectionVersionID                   string
	CredentialVersionID                   string
	BindingID, BindingContentHash         string
	BindingRevision                       int64
	ModelProfileVersionID                 string
	ModelProfileRevision                  int64
	ModelProfileContentHash               string
	PriceQuoteID, PriceQuoteContentHash   string
	PriceQuoteRevision                    int64
	BillingMetric                         string
	EstimatedUnits                        int64
	RemoteRequestID, RemoteJobID          string
	QueryDeadlineAt                       *time.Time
	RemoteExpiresAt                       *time.Time
	TargetHash                            string
	Target                                domain.GenerationTarget
}

type ProviderOutcome struct {
	Status                       string
	RemoteRequestID, RemoteJobID string
	ProviderEventID, FailureCode string
	Output                       *ProviderOutput
	ProviderUsageObservation     domain.ProviderUsageObservation
	OccurredAt                   time.Time
	QueryDeadlineAt              time.Time
	RemoteExpiresAt              time.Time
}

type ProviderGateway interface {
	Preflight(context.Context, ProviderSubmission) error
	Submit(context.Context, ProviderSubmission) (ProviderOutcome, error)
	Query(context.Context, ProviderSubmission) (ProviderOutcome, error)
}

type ProviderLocalFailure interface {
	ProviderFailureCode() string
}

type ProviderQueryFailure interface {
	ProviderQueryFailureKind() string
}

// ProviderSubmitFailure is implemented only when a Submit error still carries
// an official, queryable remote task identity. Generic Submit errors remain
// outcome-unknown because they cannot prove that the returned identity is safe
// to reconcile.
type ProviderSubmitFailure interface {
	ProviderSubmitFailureKind() string
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
	LockProviderWorkspace(context.Context, string) error
	LatestProviderConnectionForUpdate(context.Context, string, string) (domain.ProviderConnectionVersion, error)
	LatestProviderModelProfileForUpdate(context.Context, string, string) (domain.ProviderModelProfileVersion, error)
	FindProjectProviderBinding(context.Context, string) (domain.ProjectProviderBindingVersion, error)
	FindProviderModelProfile(context.Context, string) (domain.ProviderModelProfileVersion, error)
	FindRequestByIntent(context.Context, string) (domain.GenerationRequest, error)
	FindGenerationRequest(context.Context, string) (domain.GenerationRequest, error)
	EnsureRequestJobAndCalls(
		context.Context,
		domain.GenerationRequest,
		domain.ProviderJob,
		[]domain.ProviderCall,
	) (domain.GenerationRequest, domain.ProviderJob, []domain.ProviderCall, error)
	FindProviderJobByIntent(context.Context, string) (domain.ProviderJob, error)
	GetIntentForProviderJobUpdate(context.Context, string) (domain.Intent, error)
	GetProviderJobForUpdate(context.Context, string) (domain.ProviderJob, error)
	UpdateProviderJob(context.Context, domain.ProviderJob, int64) (domain.ProviderJob, error)
	ListProviderCalls(context.Context, string) ([]domain.ProviderCall, error)
	GetProviderCallForUpdate(context.Context, string) (domain.ProviderCall, error)
	UpdateProviderCall(context.Context, domain.ProviderCall, int64) (domain.ProviderCall, error)
	ListProviderResultReceipts(context.Context, string) ([]domain.ProviderResultReceipt, error)
	FindProviderResultReceiptByCall(context.Context, string) (domain.ProviderResultReceipt, error)
	EnsureProviderResultReceipt(context.Context, domain.ProviderResultReceipt) (domain.ProviderResultReceipt, error)
}

type ProviderTransactionManager interface {
	WithinProviderTransaction(
		context.Context,
		func(ProviderRepository, CostProviderOwner, QuotaProviderOwner) error,
	) error
}

type ProviderConfig struct {
	Now   func() time.Time
	NewID func() string
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
	Intent   domain.Intent
	Target   domain.GenerationTarget
	Request  domain.GenerationRequest
	Job      domain.ProviderJob
	Calls    []domain.ProviderCall
	Receipts []domain.ProviderResultReceipt
	Receipt  platformcommand.Receipt
}

type providerCommandReceipt struct {
	RequestID, JobID string
}

type providerTerminalReceipt struct {
	RequestID, JobID, Status, CallSetHash string
	Revision                              int64
	CallCount, DispatchedCallCount        int
	SucceededCallCount, FailedCallCount   int
}

type generationRequestHashInput struct {
	WorkspaceID, ProjectID, IntentID, TargetID string
	BindingID, BindingContentHash              string
	BindingRevision                            int64
	Purpose, ProviderKey, ExternalModelID      string
	ConnectionVersionID, CredentialVersionID   string
	ModelProfileVersionID                      string
	ModelProfileRevision                       int64
	ModelProfileContentHash                    string
	PriceQuoteID, PriceQuoteContentHash        string
	PriceQuoteRevision                         int64
	BillingMetric, RequestKey, TargetHash      string
	EstimatedUnits                             int64
}

type providerJobHashInput struct {
	WorkspaceID, ProjectID, IntentID, RequestID  string
	ProviderKey, RequestKey, Status, CallSetHash string
	CallCount, DispatchedCallCount               int
	SucceededCallCount, FailedCallCount          int
	Revision                                     int64
}

type providerCallRequestHashInput struct {
	RequestID, RequestContentHash string
	CandidateIndex                int
	RequestedOutputCount          int
}

type providerCallHashInput struct {
	WorkspaceID, ProjectID, JobID, CallKey, RequestHash string
	CandidateIndex, RequestedOutputCount                int
	Status, LocalFailureCode                            string
	RemoteRequestID, RemoteJobID                        string
	DispatchBoundaryEnteredAt                           string
	QueryDeadlineAt                                     string
	RemoteExpiresAt                                     string
	Revision                                            int64
}

type providerCallSetHashItem struct {
	ID, CallKey, RequestHash, CallContentHash string
	ReceiptID, ReceiptContentHash             string
	CandidateIndex, RequestedOutputCount      int
}

type providerReceiptHashInput struct {
	WorkspaceID, ProjectID, CallID       string
	ProviderEventID, Status, FailureCode string
	OutputCount                          int
	ProviderUsageObservation             domain.ProviderUsageObservation
	ProviderUsageHash                    string
	Output                               *ProviderOutput
	OccurredAt                           string
}

type providerInvocation struct {
	intent    domain.Intent
	request   domain.GenerationRequest
	job       domain.ProviderJob
	calls     []domain.ProviderCall
	target    domain.GenerationTarget
	call      domain.ProviderCall
	result    ProviderExecutionResult
	action    string
	operation string
	key       string
	inputHash string
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

func (service *ProviderService) prepareSubmission(
	ctx context.Context,
	authorization domain.ExecutionAuthorization,
	command SubmitImageRequestCommand,
	inputHash string,
) (providerInvocation, error) {
	now := service.now()
	invocation := providerInvocation{
		action: providerActionNone, operation: submitProviderOperation,
		key: command.IdempotencyKey, inputHash: inputHash,
	}
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
		if loadErr = validateAuthorizationBinding(intent, authorization, false); loadErr != nil {
			return loadErr
		}
		request, requestErr := repo.FindRequestByIntent(ctx, intent.ID)
		if errors.Is(requestErr, ErrGenerationRequestNotFound) {
			if intent.Status != domain.IntentClaimed || intent.Revision != authorization.IntentRevision {
				return conflict("Generation Provider authorization is no longer claimable")
			}
			if !service.config.Now().UTC().Before(authorization.ExpiresAt) {
				return authorizationExpired()
			}
			request, invocation.job, invocation.calls, loadErr = service.createProviderExecution(
				ctx, repo, costs, quotas, intent, now,
			)
			if loadErr != nil {
				return loadErr
			}
			intent, loadErr = repo.GetIntentForUpdate(ctx, intent.ID)
			if loadErr != nil {
				return loadErr
			}
		} else if requestErr != nil {
			return requestErr
		} else {
			invocation.job, loadErr = repo.GetProviderJobForUpdate(ctx, intent.ProviderJobID)
			if loadErr != nil {
				return loadErr
			}
			invocation.calls, loadErr = repo.ListProviderCalls(ctx, invocation.job.ID)
			if loadErr != nil {
				return loadErr
			}
		}
		invocation.intent, invocation.request = intent, request
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, invocation.job, invocation.calls); loadErr != nil {
			return loadErr
		}
		invocation.target, loadErr = repo.FindGenerationTarget(ctx, request.TargetID)
		if loadErr != nil {
			return loadErr
		}
		invocation.action, invocation.call = selectProviderAction(invocation.job, invocation.calls, false, now)
		if invocation.action != providerActionNone {
			invocation.result, loadErr = service.loadProviderExecutionResult(
				ctx, repo, costs, quotas, intent, request, invocation.job,
			)
			return loadErr
		}
		invocation.result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, invocation.job)
		if loadErr != nil {
			return loadErr
		}
		for _, call := range invocation.calls {
			if call.Status == domain.ProviderCallDispatching {
				return nil
			}
		}
		receipt, loadErr := service.storeProviderCommandReceipt(ctx, repo, actor, invocation, invocation.result, now)
		if loadErr != nil {
			return loadErr
		}
		invocation.result.Receipt = receipt
		return nil
	})
	return invocation, err
}

func (service *ProviderService) prepareReconciliation(
	ctx context.Context,
	command ReconcileProviderJobCommand,
	inputHash string,
) (providerInvocation, error) {
	now := service.now()
	invocation := providerInvocation{
		action: providerActionNone, operation: reconcileProviderOperation,
		key: command.IdempotencyKey, inputHash: inputHash,
	}
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		intent, loadErr := repo.GetIntentForProviderJobUpdate(ctx, command.ProviderJobID)
		if loadErr != nil {
			return loadErr
		}
		job, loadErr := repo.GetProviderJobForUpdate(ctx, command.ProviderJobID)
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
		actor := intentActor(intent)
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, reconcileProviderOperation, command.IdempotencyKey); findErr == nil {
			result, replayErr := service.replayProviderCommand(ctx, repo, costs, quotas, receipt, inputHash)
			invocation.result = result
			return replayErr
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		calls, loadErr := repo.ListProviderCalls(ctx, job.ID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job, calls); loadErr != nil {
			return loadErr
		}
		target, loadErr := repo.FindGenerationTarget(ctx, request.TargetID)
		if loadErr != nil {
			return loadErr
		}
		invocation.intent, invocation.request, invocation.job = intent, request, job
		invocation.calls, invocation.target = calls, target
		invocation.action, invocation.call = selectProviderAction(job, calls, true, now)
		if invocation.action != providerActionNone {
			invocation.result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
			return loadErr
		}
		invocation.result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
		if loadErr != nil {
			return loadErr
		}
		receipt, loadErr := service.storeProviderCommandReceipt(ctx, repo, actor, invocation, invocation.result, now)
		if loadErr != nil {
			return loadErr
		}
		invocation.result.Receipt = receipt
		return nil
	})
	return invocation, err
}

func (service *ProviderService) createProviderExecution(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	now time.Time,
) (domain.GenerationRequest, domain.ProviderJob, []domain.ProviderCall, error) {
	if err := service.validateProviderOwners(ctx, repo, costs, quotas, intent, nil); err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	target, err := repo.FindGenerationTarget(ctx, intent.TargetID)
	if err != nil || validateIntentTargetBinding(target, intent) != nil || target.ReferenceAsset == nil ||
		int64(target.ReferenceAsset.NumberResults) != intent.EstimatedUnits {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, conflict("GenerationTarget and intent have drifted")
	}
	if err = repo.LockProviderWorkspace(ctx, intent.WorkspaceID); err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	binding, err := repo.FindProjectProviderBinding(ctx, intent.BindingVersionID)
	if err != nil || validateProjectProviderBinding(binding) != nil || binding.WorkspaceID != intent.WorkspaceID ||
		binding.ProjectID != intent.ProjectID || binding.Purpose != domain.ProviderPurposeReferenceAsset ||
		binding.Modality != domain.MediaModalityImage || binding.Revision != intent.BindingRevision ||
		binding.ContentHash != intent.BindingContentHash || binding.ConnectionVersionID != intent.ConnectionVersionID ||
		binding.CredentialVersionID != intent.CredentialVersionID ||
		binding.ModelProfileVersionID != intent.ModelProfileVersionID {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, conflict("Generation Provider binding snapshot has drifted")
	}
	connection, err := repo.FindProviderConnection(ctx, intent.ConnectionVersionID)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, conflict("Generation Provider connection snapshot has drifted")
	}
	credential, err := repo.FindProviderCredential(ctx, intent.CredentialVersionID)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, conflict("Generation Provider credential snapshot has drifted")
	}
	profile, err := repo.FindProviderModelProfile(ctx, intent.ModelProfileVersionID)
	if err != nil || validateProviderModelProfileVersion(profile) != nil || profile.WorkspaceID != intent.WorkspaceID ||
		profile.ProviderKey != binding.ProviderKey || profile.Modality != domain.MediaModalityImage ||
		profile.BillingMetric != costdomain.MetricGenerationImageCall || profile.BillingMetric != intent.BillingMetric ||
		profile.Revision != intent.ModelProfileRevision || profile.ContentHash != intent.ModelProfileContentHash {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, conflict("Generation Provider model profile snapshot has drifted")
	}
	if err = validateResolvedProviderFacts(binding, connection, credential, profile); err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	latestBinding, err := repo.LatestProjectProviderBindingForUpdate(
		ctx,
		intent.WorkspaceID,
		intent.ProjectID,
		domain.ProviderPurposeReferenceAsset,
	)
	if err != nil || latestBinding.ID != binding.ID || latestBinding.Revision != binding.Revision ||
		latestBinding.ContentHash != binding.ContentHash {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil,
			conflict("Generation Provider binding changed before request creation")
	}
	latestConnection, err := repo.LatestProviderConnectionForUpdate(ctx, intent.WorkspaceID, connection.ConnectionKey)
	if err != nil || latestConnection.ID != connection.ID || latestConnection.Revision != connection.Revision ||
		latestConnection.ContentHash != connection.ContentHash || latestConnection.State != domain.ProviderStateEnabled ||
		latestConnection.CredentialVersionID != credential.ID {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil,
			conflict("Generation Provider connection changed before request creation")
	}
	latestProfile, err := repo.LatestProviderModelProfileForUpdate(ctx, intent.WorkspaceID, profile.ProfileKey)
	if err != nil || latestProfile.ID != profile.ID || latestProfile.Revision != profile.Revision ||
		latestProfile.ContentHash != profile.ContentHash || latestProfile.State != domain.ProviderStateEnabled {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil,
			conflict("Generation Provider model profile changed before request creation")
	}
	requestID, jobID := strings.TrimSpace(service.config.NewID()), strings.TrimSpace(service.config.NewID())
	if !validUUID(requestID) || !validUUID(jobID) {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, errors.New("Generation Provider identifiers are invalid")
	}
	request := domain.GenerationRequest{
		ID: requestID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
		TargetID: intent.TargetID, BindingID: intent.BindingVersionID, BindingRevision: intent.BindingRevision,
		BindingContentHash: intent.BindingContentHash, Purpose: binding.Purpose, ProviderKey: binding.ProviderKey,
		ExternalModelID: profile.ExternalModelID, ConnectionVersionID: intent.ConnectionVersionID,
		CredentialVersionID: intent.CredentialVersionID, ModelProfileVersionID: intent.ModelProfileVersionID,
		ModelProfileRevision: intent.ModelProfileRevision, ModelProfileContentHash: intent.ModelProfileContentHash,
		PriceQuoteID: intent.PriceQuoteID, PriceQuoteRevision: intent.PriceQuoteRevision,
		PriceQuoteContentHash: intent.PriceQuoteContentHash, BillingMetric: intent.BillingMetric,
		RequestKey: "generation-request:" + requestID, TargetHash: intent.TargetHash,
		EstimatedUnits: intent.EstimatedUnits, CreatedBy: intent.CreatedBy, CreatedAt: now,
	}
	request.ContentHash, err = generationRequestContentHash(request)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	calls := make([]domain.ProviderCall, 0, intent.EstimatedUnits)
	for candidateIndex := 1; candidateIndex <= int(intent.EstimatedUnits); candidateIndex++ {
		callID := strings.TrimSpace(service.config.NewID())
		if !validUUID(callID) {
			return domain.GenerationRequest{}, domain.ProviderJob{}, nil, errors.New("Generation Provider call identifier is invalid")
		}
		requestHash, hashErr := providerCallRequestHash(request, candidateIndex)
		if hashErr != nil {
			return domain.GenerationRequest{}, domain.ProviderJob{}, nil, hashErr
		}
		call := domain.ProviderCall{
			ID: callID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, JobID: jobID,
			CandidateIndex: candidateIndex, CallKey: "generation-call:" + callID, RequestHash: requestHash,
			RequestedOutputCount: 1, Status: domain.ProviderCallPending, Revision: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		call.ContentHash, hashErr = providerCallContentHash(call)
		if hashErr != nil {
			return domain.GenerationRequest{}, domain.ProviderJob{}, nil, hashErr
		}
		calls = append(calls, call)
	}
	callSetHash, err := providerCallSetContentHash(calls, nil)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	job := domain.ProviderJob{
		ID: jobID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
		RequestID: request.ID, ProviderKey: request.ProviderKey, RequestKey: request.RequestKey,
		Status: domain.ProviderJobPending, CallSetHash: callSetHash, CallCount: len(calls), Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	job.ContentHash, err = providerJobContentHash(job)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	request, job, calls, err = repo.EnsureRequestJobAndCalls(ctx, request, job, calls)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	previousRevision := intent.Revision
	intent.GenerationRequestID, intent.ProviderJobID, intent.ProviderCallSetHash = request.ID, job.ID, callSetHash
	intent.Status, intent.Revision, intent.UpdatedAt = domain.IntentExecuting, previousRevision+1, now
	intent.ContentHash, err = intentContentHash(intent)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	if _, err = repo.UpdateIntent(ctx, intent, previousRevision); err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	return request, job, calls, nil
}

func (service *ProviderService) executeInvocation(
	ctx context.Context,
	invocation providerInvocation,
) (ProviderExecutionResult, error) {
	if invocation.action == providerActionRecover || invocation.action == providerActionExpire ||
		invocation.action == providerActionAbandon {
		result, err := service.applyProviderOutcome(ctx, invocation, ProviderOutcome{Status: ProviderOutcomeUnknown})
		return result, normalizeProviderError(err)
	}
	submission := providerSubmission(invocation.request, invocation.job, invocation.call, invocation.target)
	if invocation.action == providerActionSubmit {
		if err := service.gateway.Preflight(ctx, submission); err != nil {
			result, applyErr := service.applyLocalProviderFailure(ctx, invocation, providerLocalFailureCode(err))
			return result, normalizeProviderError(applyErr)
		}
		claimed, claimErr := service.claimProviderDispatch(ctx, invocation)
		if claimErr != nil || claimed.action == providerActionNone {
			return claimed.result, normalizeProviderError(claimErr)
		}
		invocation = claimed
		submission = providerSubmission(invocation.request, invocation.job, invocation.call, invocation.target)
		outcome, err := service.gateway.Submit(ctx, submission)
		if err != nil {
			if providerSubmitFailureKind(err) == ProviderSubmitFailureIdentityRecoverable {
				outcome.Status = ProviderOutcomeAccepted
				outcome.ProviderEventID, outcome.FailureCode = "", ""
				outcome.Output = nil
				outcome.ProviderUsageObservation = domain.ProviderUsageObservation{}
				outcome.OccurredAt = time.Time{}
			} else {
				outcome = ProviderOutcome{Status: ProviderOutcomeUnknown}
			}
		}
		result, applyErr := service.applyProviderOutcome(ctx, invocation, outcome)
		return result, normalizeProviderError(applyErr)
	}
	outcome, err := service.gateway.Query(ctx, submission)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ProviderExecutionResult{}, err
		}
		if providerQueryFailureKind(err) == ProviderQueryFailureIdentityUnrecoverable {
			invocation.action = providerActionAbandon
			result, applyErr := service.applyProviderOutcome(ctx, invocation, ProviderOutcome{Status: ProviderOutcomeUnknown})
			return result, normalizeProviderError(applyErr)
		}
		return invocation.result, providerQueryTemporarilyUnavailable()
	}
	// Query responses are allowed to omit the already persisted remote task ID.
	// Keep the immutable binding before normalization so a valid RUNNING response
	// cannot be downgraded to OUTCOME_UNKNOWN merely because the provider did not
	// echo its task identifier.
	outcome.RemoteRequestID = firstNonEmpty(outcome.RemoteRequestID, invocation.call.RemoteRequestID)
	outcome.RemoteJobID = firstNonEmpty(outcome.RemoteJobID, invocation.call.RemoteJobID)
	if outcome.QueryDeadlineAt.IsZero() && invocation.call.QueryDeadlineAt != nil {
		outcome.QueryDeadlineAt = *invocation.call.QueryDeadlineAt
	}
	if outcome.RemoteExpiresAt.IsZero() && invocation.call.RemoteExpiresAt != nil {
		outcome.RemoteExpiresAt = *invocation.call.RemoteExpiresAt
	}
	result, applyErr := service.applyProviderOutcome(ctx, invocation, outcome)
	return result, normalizeProviderError(applyErr)
}

func (service *ProviderService) claimProviderDispatch(
	ctx context.Context,
	invocation providerInvocation,
) (providerInvocation, error) {
	now := service.now()
	claimed := invocation
	claimed.action = providerActionNone
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, invocation.intent.ID)
		if loadErr != nil {
			return loadErr
		}
		job, loadErr := repo.GetProviderJobForUpdate(ctx, invocation.job.ID)
		if loadErr != nil {
			return loadErr
		}
		if job.IntentID != intent.ID {
			return conflict("Generation Provider job and intent have drifted")
		}
		call, loadErr := repo.GetProviderCallForUpdate(ctx, invocation.call.ID)
		if loadErr != nil {
			return loadErr
		}
		request, loadErr := repo.FindGenerationRequest(ctx, job.RequestID)
		if loadErr != nil {
			return loadErr
		}
		calls, loadErr := repo.ListProviderCalls(ctx, job.ID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job, calls); loadErr != nil {
			return loadErr
		}
		claimed.result = ProviderExecutionResult{Intent: intent, Target: invocation.target, Request: request, Job: job}
		commandReceipt, loadErr := service.storeProviderCommandReceipt(
			ctx,
			repo,
			intentActor(intent),
			invocation,
			claimed.result,
			now,
		)
		if loadErr != nil {
			return loadErr
		}
		claimed.result.Receipt = commandReceipt
		if call.Status != domain.ProviderCallPending || call.Revision != invocation.call.Revision ||
			call.RequestHash != invocation.call.RequestHash || call.CallKey != invocation.call.CallKey {
			claimed.result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
			if loadErr == nil {
				claimed.result.Receipt = commandReceipt
			}
			return loadErr
		}
		previousCallRevision := call.Revision
		call.Status, call.DispatchBoundaryEnteredAt = domain.ProviderCallDispatching, &now
		call.Revision, call.UpdatedAt = call.Revision+1, now
		call.ContentHash, loadErr = providerCallContentHash(call)
		if loadErr != nil {
			return loadErr
		}
		call, loadErr = repo.UpdateProviderCall(ctx, call, previousCallRevision)
		if loadErr != nil {
			return loadErr
		}
		calls = replaceProviderCall(calls, call)
		job, loadErr = service.updateProviderAggregate(ctx, repo, job, calls, now)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr = service.updateIntentForProviderAggregate(ctx, repo, costs, quotas, intent, job, "", now)
		if loadErr != nil {
			return loadErr
		}
		claimed.intent, claimed.request, claimed.job, claimed.calls, claimed.call = intent, request, job, calls, call
		claimed.action = providerActionSubmit
		claimed.target, loadErr = repo.FindGenerationTarget(ctx, request.TargetID)
		return loadErr
	})
	return claimed, err
}

func (service *ProviderService) applyLocalProviderFailure(
	ctx context.Context,
	invocation providerInvocation,
	failureCode string,
) (ProviderExecutionResult, error) {
	now := service.now()
	var result ProviderExecutionResult
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		job, call, request, intent, calls, loadErr := loadProviderCallForTransition(ctx, repo, invocation)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job, calls); loadErr != nil {
			return loadErr
		}
		if call.Status != domain.ProviderCallPending || call.Revision != invocation.call.Revision {
			result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
			return loadErr
		}
		previousCallRevision := call.Revision
		call.Status, call.LocalFailureCode = domain.ProviderCallFailed, failureCode
		call.Revision, call.UpdatedAt = call.Revision+1, now
		call.ContentHash, loadErr = providerCallContentHash(call)
		if loadErr != nil {
			return loadErr
		}
		call, loadErr = repo.UpdateProviderCall(ctx, call, previousCallRevision)
		if loadErr != nil {
			return loadErr
		}
		calls = replaceProviderCall(calls, call)
		job, loadErr = service.updateProviderAggregate(ctx, repo, job, calls, now)
		if loadErr != nil {
			return loadErr
		}
		var terminalReceipt platformcommand.Receipt
		if providerJobTerminal(job.Status) {
			terminalReceipt, loadErr = service.storeProviderTerminalReceipt(
				ctx, repo, intentActor(intent), intent, request, job, now,
			)
			if loadErr != nil {
				return loadErr
			}
		}
		intent, loadErr = service.updateIntentForProviderAggregate(
			ctx, repo, costs, quotas, intent, job, terminalReceipt.ID, now,
		)
		if loadErr != nil {
			return loadErr
		}
		result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
		if loadErr != nil {
			return loadErr
		}
		commandReceipt, loadErr := service.storeProviderCommandReceipt(
			ctx, repo, intentActor(intent), invocation, result, now,
		)
		if loadErr != nil {
			return loadErr
		}
		result.Receipt = commandReceipt
		return nil
	})
	return result, err
}

func (service *ProviderService) applyProviderOutcome(
	ctx context.Context,
	invocation providerInvocation,
	outcome ProviderOutcome,
) (ProviderExecutionResult, error) {
	now := service.now()
	rawBinding := canonicalProviderOutcomeBinding(outcome)
	occurredAtProvided := !outcome.OccurredAt.IsZero()
	outcome = normalizedProviderOutcome(outcome, now)
	var result ProviderExecutionResult
	err := service.transactions.WithinProviderTransaction(ctx, func(
		repo ProviderRepository,
		costs CostProviderOwner,
		quotas QuotaProviderOwner,
	) error {
		job, call, request, intent, calls, loadErr := loadProviderCallForTransition(ctx, repo, invocation)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job, calls); loadErr != nil {
			return loadErr
		}
		if invalidProviderOutcomeBinding(call, rawBinding, now) || remoteBindingDrifted(call, rawBinding) {
			return conflict("Generation Provider remote task binding has drifted")
		}
		outcome.RemoteRequestID = firstNonEmpty(outcome.RemoteRequestID, call.RemoteRequestID)
		outcome.RemoteJobID = firstNonEmpty(outcome.RemoteJobID, call.RemoteJobID)
		if outcome.QueryDeadlineAt.IsZero() && call.QueryDeadlineAt != nil {
			outcome.QueryDeadlineAt = *call.QueryDeadlineAt
		}
		if outcome.RemoteExpiresAt.IsZero() && call.RemoteExpiresAt != nil {
			outcome.RemoteExpiresAt = *call.RemoteExpiresAt
		}
		if call.Status == domain.ProviderCallOutcomeUnknown {
			return service.finishUnchangedProviderInvocation(
				ctx, repo, costs, quotas, invocation, intent, request, job, now, &result,
			)
		}
		if call.Status == domain.ProviderCallSucceeded || call.Status == domain.ProviderCallFailed {
			if outcome.Status == ProviderOutcomeSucceeded || outcome.Status == ProviderOutcomeFailed {
				persisted, receiptErr := repo.FindProviderResultReceiptByCall(ctx, call.ID)
				if receiptErr != nil || !providerTerminalOutcomeMatchesReceipt(persisted, outcome, occurredAtProvided) {
					return providerOutcomeConflict()
				}
			}
			return service.finishUnchangedProviderInvocation(
				ctx, repo, costs, quotas, invocation, intent, request, job, now, &result,
			)
		}
		if call.Status == domain.ProviderCallPending {
			return conflict("Generation Provider outcome arrived before the dispatch boundary")
		}
		if call.Status == domain.ProviderCallDispatching {
			if invocation.action != providerActionSubmit && invocation.action != providerActionRecover {
				return conflict("Generation Provider dispatch fence has drifted")
			}
		} else if call.Status == domain.ProviderCallSubmitted || call.Status == domain.ProviderCallRunning {
			switch outcome.Status {
			case ProviderOutcomeAccepted:
				return service.finishProviderQueryWithoutReceipt(
					ctx, repo, costs, quotas, intent, request, job, &result,
				)
			case ProviderOutcomeRunning:
				if call.Status == domain.ProviderCallRunning {
					return service.finishProviderQueryWithoutReceipt(
						ctx, repo, costs, quotas, intent, request, job, &result,
					)
				}
			case ProviderOutcomeUnknown:
				if invocation.action != providerActionExpire && invocation.action != providerActionAbandon {
					return service.finishProviderQueryWithoutReceipt(
						ctx, repo, costs, quotas, intent, request, job, &result,
					)
				}
			}
		} else {
			return conflict("Generation Provider Call state has drifted")
		}
		previousCallRevision := call.Revision
		call.RemoteRequestID, call.RemoteJobID = outcome.RemoteRequestID, outcome.RemoteJobID
		if !outcome.QueryDeadlineAt.IsZero() {
			call.QueryDeadlineAt = cloneProviderTime(&outcome.QueryDeadlineAt)
		}
		if !outcome.RemoteExpiresAt.IsZero() {
			call.RemoteExpiresAt = cloneProviderTime(&outcome.RemoteExpiresAt)
		}
		switch outcome.Status {
		case ProviderOutcomeAccepted:
			call.Status = domain.ProviderCallSubmitted
		case ProviderOutcomeRunning:
			call.Status = domain.ProviderCallRunning
		case ProviderOutcomeUnknown:
			call.Status = domain.ProviderCallOutcomeUnknown
		case ProviderOutcomeSucceeded, ProviderOutcomeFailed:
			terminalReceipt, receiptErr := service.persistProviderCallReceipt(ctx, repo, call, outcome, now)
			if receiptErr != nil {
				return receiptErr
			}
			if terminalReceipt.Status == domain.ProviderResultSucceeded {
				call.Status = domain.ProviderCallSucceeded
			} else {
				call.Status = domain.ProviderCallFailed
			}
		default:
			return errors.New("normalized Provider outcome is invalid")
		}
		call.Revision, call.UpdatedAt = call.Revision+1, now
		call.ContentHash, loadErr = providerCallContentHash(call)
		if loadErr != nil {
			return loadErr
		}
		call, loadErr = repo.UpdateProviderCall(ctx, call, previousCallRevision)
		if loadErr != nil {
			return loadErr
		}
		calls = replaceProviderCall(calls, call)
		job, loadErr = service.updateProviderAggregate(ctx, repo, job, calls, now)
		if loadErr != nil {
			return loadErr
		}
		var terminalReceipt platformcommand.Receipt
		if providerJobTerminal(job.Status) {
			terminalReceipt, loadErr = service.storeProviderTerminalReceipt(
				ctx, repo, intentActor(intent), intent, request, job, now,
			)
			if loadErr != nil {
				return loadErr
			}
		}
		intent, loadErr = service.updateIntentForProviderAggregate(
			ctx, repo, costs, quotas, intent, job, terminalReceipt.ID, now,
		)
		if loadErr != nil {
			return loadErr
		}
		result, loadErr = service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
		if loadErr != nil {
			return loadErr
		}
		commandReceipt, loadErr := service.storeProviderCommandReceipt(
			ctx, repo, intentActor(intent), invocation, result, now,
		)
		if loadErr != nil {
			return loadErr
		}
		result.Receipt = commandReceipt
		return nil
	})
	return result, err
}

func (service *ProviderService) finishUnchangedProviderInvocation(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	invocation providerInvocation,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	now time.Time,
	result *ProviderExecutionResult,
) error {
	loaded, err := service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
	if err != nil {
		return err
	}
	receipt, err := service.storeProviderCommandReceipt(ctx, repo, intentActor(intent), invocation, loaded, now)
	if err != nil {
		return err
	}
	loaded.Receipt = receipt
	*result = loaded
	return nil
}

func (service *ProviderService) finishProviderQueryWithoutReceipt(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	result *ProviderExecutionResult,
) error {
	loaded, err := service.loadProviderExecutionResult(ctx, repo, costs, quotas, intent, request, job)
	if err != nil {
		return err
	}
	*result = loaded
	return nil
}

func providerTerminalOutcomeMatchesReceipt(
	receipt domain.ProviderResultReceipt,
	outcome ProviderOutcome,
	occurredAtProvided bool,
) bool {
	expectedStatus, expectedCount := domain.ProviderResultSucceeded, 1
	if outcome.Status == ProviderOutcomeFailed {
		expectedStatus, expectedCount = domain.ProviderResultFailed, 0
	}
	if receipt.Status != expectedStatus || receipt.OutputCount != expectedCount ||
		receipt.ProviderEventID != outcome.ProviderEventID || receipt.FailureCode != outcome.FailureCode ||
		receipt.ProviderUsageObservation != outcome.ProviderUsageObservation ||
		!optionalProviderOutputEqual(receipt.Output, outcome.Output) {
		return false
	}
	return !occurredAtProvided || receipt.OccurredAt.Equal(outcome.OccurredAt)
}

func optionalProviderOutputEqual(left, right *ProviderOutput) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func providerOutcomeConflict() error {
	return &Error{
		Code: "provider_outcome_conflict", Message: "Generation Provider returned a conflicting terminal outcome",
		Status: 409, NextAction: "manual_provider_reconciliation",
	}
}

func providerQueryTemporarilyUnavailable() error {
	return &Error{
		Code: "provider_query_temporarily_unavailable", Message: "Generation Provider query is temporarily unavailable",
		Status: 503, NextAction: "retry_provider_query",
	}
}

func providerQueryFailureKind(err error) string {
	var typed ProviderQueryFailure
	if errors.As(err, &typed) {
		kind := strings.TrimSpace(typed.ProviderQueryFailureKind())
		if kind == ProviderQueryFailureIdentityUnrecoverable || kind == ProviderQueryFailureRetryable {
			return kind
		}
	}
	return ProviderQueryFailureRetryable
}

func providerSubmitFailureKind(err error) string {
	var typed ProviderSubmitFailure
	if errors.As(err, &typed) &&
		strings.TrimSpace(typed.ProviderSubmitFailureKind()) == ProviderSubmitFailureIdentityRecoverable {
		return ProviderSubmitFailureIdentityRecoverable
	}
	return ""
}

func loadProviderCallForTransition(
	ctx context.Context,
	repo ProviderRepository,
	invocation providerInvocation,
) (domain.ProviderJob, domain.ProviderCall, domain.GenerationRequest, domain.Intent, []domain.ProviderCall, error) {
	intent, err := repo.GetIntentForUpdate(ctx, invocation.intent.ID)
	if err != nil {
		return domain.ProviderJob{}, domain.ProviderCall{}, domain.GenerationRequest{}, domain.Intent{}, nil, err
	}
	job, err := repo.GetProviderJobForUpdate(ctx, invocation.job.ID)
	if err != nil {
		return domain.ProviderJob{}, domain.ProviderCall{}, domain.GenerationRequest{}, domain.Intent{}, nil, err
	}
	if job.IntentID != intent.ID {
		return domain.ProviderJob{}, domain.ProviderCall{}, domain.GenerationRequest{}, domain.Intent{}, nil,
			conflict("Generation Provider job and intent have drifted")
	}
	call, err := repo.GetProviderCallForUpdate(ctx, invocation.call.ID)
	if err != nil {
		return domain.ProviderJob{}, domain.ProviderCall{}, domain.GenerationRequest{}, domain.Intent{}, nil, err
	}
	request, err := repo.FindGenerationRequest(ctx, job.RequestID)
	if err != nil {
		return domain.ProviderJob{}, domain.ProviderCall{}, domain.GenerationRequest{}, domain.Intent{}, nil, err
	}
	calls, err := repo.ListProviderCalls(ctx, job.ID)
	return job, call, request, intent, calls, err
}

func (service *ProviderService) persistProviderCallReceipt(
	ctx context.Context,
	repo ProviderRepository,
	call domain.ProviderCall,
	outcome ProviderOutcome,
	now time.Time,
) (domain.ProviderResultReceipt, error) {
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return domain.ProviderResultReceipt{}, errors.New("Generation Provider result receipt identifier is invalid")
	}
	usageHash, err := platformcommand.InputHash(outcome.ProviderUsageObservation)
	if err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	receipt := domain.ProviderResultReceipt{
		ID: receiptID, WorkspaceID: call.WorkspaceID, ProjectID: call.ProjectID, CallID: call.ID,
		ProviderEventID: outcome.ProviderEventID, Status: domain.ProviderResultSucceeded,
		OutputCount: 1, Output: cloneProviderOutput(outcome.Output),
		ProviderUsageObservation: outcome.ProviderUsageObservation, ProviderUsageHash: usageHash,
		OccurredAt: outcome.OccurredAt, ReceivedAt: now,
	}
	if outcome.Status == ProviderOutcomeFailed {
		receipt.Status, receipt.OutputCount, receipt.Output = domain.ProviderResultFailed, 0, nil
		receipt.FailureCode = outcome.FailureCode
	}
	receipt.ContentHash, err = providerResultReceiptContentHash(receipt)
	if err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	if err = validateProviderResultReceipt(receipt, call); err != nil {
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
	return receipt, nil
}

func (service *ProviderService) updateProviderAggregate(
	ctx context.Context,
	repo ProviderRepository,
	job domain.ProviderJob,
	calls []domain.ProviderCall,
	now time.Time,
) (domain.ProviderJob, error) {
	previousRevision := job.Revision
	receipts, err := repo.ListProviderResultReceipts(ctx, job.ID)
	if err != nil {
		return domain.ProviderJob{}, err
	}
	if err = validateProviderReceiptSet(calls, receipts); err != nil {
		return domain.ProviderJob{}, err
	}
	job.CallSetHash, err = providerCallSetContentHash(calls, receipts)
	if err != nil {
		return domain.ProviderJob{}, err
	}
	dispatched, succeeded, failed := 0, 0, 0
	hasUnknown := false
	for _, call := range calls {
		if call.DispatchBoundaryEnteredAt != nil {
			dispatched++
		}
		switch call.Status {
		case domain.ProviderCallSucceeded:
			succeeded++
		case domain.ProviderCallFailed:
			failed++
		case domain.ProviderCallOutcomeUnknown:
			hasUnknown = true
		}
	}
	job.DispatchedCallCount, job.SucceededCallCount, job.FailedCallCount = dispatched, succeeded, failed
	switch {
	case hasUnknown:
		job.Status = domain.ProviderJobOutcomeUnknown
	case succeeded+failed == len(calls) && succeeded == len(calls):
		job.Status = domain.ProviderJobSucceeded
	case succeeded+failed == len(calls) && succeeded > 0:
		job.Status = domain.ProviderJobPartialSucceeded
	case succeeded+failed == len(calls):
		job.Status = domain.ProviderJobFailed
	case dispatched == 0 && failed == 0:
		job.Status = domain.ProviderJobPending
	default:
		job.Status = domain.ProviderJobRunning
	}
	job.Revision, job.UpdatedAt = previousRevision+1, now
	job.ContentHash, err = providerJobContentHash(job)
	if err != nil {
		return domain.ProviderJob{}, err
	}
	return repo.UpdateProviderJob(ctx, job, previousRevision)
}

func (service *ProviderService) updateIntentForProviderAggregate(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	job domain.ProviderJob,
	usageReceiptID string,
	now time.Time,
) (domain.Intent, error) {
	previousRevision := intent.Revision
	intent.ProviderCallSetHash = job.CallSetHash
	switch job.Status {
	case domain.ProviderJobPending, domain.ProviderJobRunning:
		intent.Status = domain.IntentExecuting
	case domain.ProviderJobOutcomeUnknown:
		intent.Status = domain.IntentOutcomeUnknown
	case domain.ProviderJobSucceeded, domain.ProviderJobPartialSucceeded, domain.ProviderJobFailed:
		actor := intentActor(intent)
		costActor := costapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
		quotaActor := quotaapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
		if job.DispatchedCallCount == 0 {
			costResult, err := costs.ReleaseReservation(ctx, costActor, costapp.ReleaseReservationCommand{
				ReservationID:  intent.CostReservationID,
				IdempotencyKey: preparationOwnerKey(intent.ID, "cost-release"),
			})
			if err != nil {
				return domain.Intent{}, err
			}
			quotaResult, err := quotas.Release(ctx, quotaActor, quotaapp.TransitionCommand{
				ReservationID:  intent.QuotaReservationID,
				IdempotencyKey: preparationOwnerKey(intent.ID, "quota-release"),
			})
			if err != nil {
				return domain.Intent{}, err
			}
			intent.CostReleaseReceiptID, intent.QuotaReleaseReceiptID = costResult.Receipt.ID, quotaResult.Receipt.ID
		} else {
			if !validUUID(usageReceiptID) {
				return domain.Intent{}, conflict("Generation Provider terminal usage receipt is missing")
			}
			costResult, err := costs.SettleReservation(ctx, costActor, costapp.SettleReservationCommand{
				ReservationID: intent.CostReservationID, UsageReceiptID: usageReceiptID,
				SettledUnits:   int64(job.DispatchedCallCount),
				IdempotencyKey: preparationOwnerKey(intent.ID, "cost-settle"),
			})
			if err != nil {
				return domain.Intent{}, err
			}
			quotaResult, err := quotas.Consume(ctx, quotaActor, quotaapp.TransitionCommand{
				ReservationID:  intent.QuotaReservationID,
				IdempotencyKey: preparationOwnerKey(intent.ID, "quota-consume"),
			})
			if err != nil {
				return domain.Intent{}, err
			}
			intent.CostSettlementReceiptID = costResult.Receipt.ID
			intent.QuotaConsumptionReceiptID = quotaResult.Receipt.ID
		}
		switch job.Status {
		case domain.ProviderJobSucceeded:
			intent.Status = domain.IntentSucceeded
		case domain.ProviderJobPartialSucceeded:
			intent.Status = domain.IntentPartialSucceeded
		default:
			intent.Status = domain.IntentFailed
		}
	default:
		return domain.Intent{}, conflict("Generation Provider aggregate state has drifted")
	}
	intent.Revision, intent.UpdatedAt = previousRevision+1, now
	var err error
	intent.ContentHash, err = intentContentHash(intent)
	if err != nil {
		return domain.Intent{}, err
	}
	return repo.UpdateIntent(ctx, intent, previousRevision)
}

func (service *ProviderService) validateProviderFacts(
	ctx context.Context,
	repo ProviderRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	calls []domain.ProviderCall,
) error {
	if validateIntent(intent) != nil || validateGenerationRequest(request) != nil || validateProviderJob(job) != nil ||
		request.IntentID != intent.ID || job.IntentID != intent.ID || job.RequestID != request.ID ||
		request.TargetID != intent.TargetID || request.TargetHash != intent.TargetHash ||
		request.WorkspaceID != intent.WorkspaceID || request.ProjectID != intent.ProjectID ||
		job.WorkspaceID != intent.WorkspaceID || job.ProjectID != intent.ProjectID ||
		request.ProviderKey != job.ProviderKey || request.RequestKey != job.RequestKey ||
		intent.GenerationRequestID != request.ID || intent.ProviderJobID != job.ID ||
		intent.ProviderCallSetHash != job.CallSetHash || request.BindingID != intent.BindingVersionID ||
		request.BindingRevision != intent.BindingRevision || request.BindingContentHash != intent.BindingContentHash ||
		request.ConnectionVersionID != intent.ConnectionVersionID || request.CredentialVersionID != intent.CredentialVersionID ||
		request.ModelProfileVersionID != intent.ModelProfileVersionID ||
		request.ModelProfileRevision != intent.ModelProfileRevision ||
		request.ModelProfileContentHash != intent.ModelProfileContentHash || request.PriceQuoteID != intent.PriceQuoteID ||
		request.PriceQuoteRevision != intent.PriceQuoteRevision || request.PriceQuoteContentHash != intent.PriceQuoteContentHash ||
		request.BillingMetric != intent.BillingMetric || request.EstimatedUnits != intent.EstimatedUnits ||
		len(calls) != job.CallCount {
		return conflict("Generation Provider facts have drifted")
	}
	target, err := repo.FindGenerationTarget(ctx, request.TargetID)
	if err != nil || validateProviderTargetBinding(target, intent, request) != nil {
		return conflict("Generation Provider Target snapshot has drifted")
	}
	binding, err := repo.FindProjectProviderBinding(ctx, request.BindingID)
	if err != nil || validateProjectProviderBinding(binding) != nil || binding.WorkspaceID != request.WorkspaceID ||
		binding.ProjectID != request.ProjectID || binding.Revision != request.BindingRevision ||
		binding.ContentHash != request.BindingContentHash || binding.Purpose != request.Purpose ||
		binding.ProviderKey != request.ProviderKey || binding.ConnectionVersionID != request.ConnectionVersionID ||
		binding.CredentialVersionID != request.CredentialVersionID ||
		binding.ModelProfileVersionID != request.ModelProfileVersionID {
		return conflict("Generation Provider binding snapshot has drifted")
	}
	profile, err := repo.FindProviderModelProfile(ctx, request.ModelProfileVersionID)
	if err != nil || validateProviderModelProfileVersion(profile) != nil || profile.WorkspaceID != request.WorkspaceID ||
		profile.ProviderKey != request.ProviderKey || profile.ExternalModelID != request.ExternalModelID ||
		profile.Modality != binding.Modality || profile.Revision != request.ModelProfileRevision ||
		profile.ContentHash != request.ModelProfileContentHash || profile.BillingMetric != request.BillingMetric {
		return conflict("Generation Provider model profile snapshot has drifted")
	}
	if err = validateProviderCalls(calls, request, job); err != nil {
		return err
	}
	receipts, err := repo.ListProviderResultReceipts(ctx, job.ID)
	if err != nil || validateProviderReceiptSet(calls, receipts) != nil {
		return conflict("Generation Provider receipt set has drifted")
	}
	callSetHash, err := providerCallSetContentHash(calls, receipts)
	if err != nil || callSetHash != job.CallSetHash {
		return conflict("Generation Provider Call set has drifted")
	}
	if err = (&PreparationService{}).validateOwnerReceipts(ctx, repo, intent); err != nil {
		return err
	}
	return service.validateProviderOwners(ctx, repo, costs, quotas, intent, &job)
}

func (service *ProviderService) validateProviderOwners(
	ctx context.Context,
	repo PreparationRepository,
	costs CostProviderOwner,
	quotas QuotaProviderOwner,
	intent domain.Intent,
	job *domain.ProviderJob,
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
		costView.Reservation.SourceID != intent.ID || costView.Reservation.EstimatedUnits != intent.EstimatedUnits ||
		costView.Reservation.PriceQuoteID != intent.PriceQuoteID ||
		costView.Reservation.PriceQuoteRevision != intent.PriceQuoteRevision ||
		costView.Reservation.Metric != intent.BillingMetric ||
		quotaReservation.ID != intent.QuotaReservationID || quotaReservation.WorkspaceID != intent.WorkspaceID ||
		quotaReservation.ProjectID != intent.ProjectID || quotaReservation.SourceType != "generation_intent" ||
		quotaReservation.SourceID != intent.ID || quotaReservation.Units != intent.EstimatedUnits ||
		quotaReservation.Metric != intent.BillingMetric {
		return conflict("Generation Provider Owner bindings have drifted")
	}
	switch intent.Status {
	case domain.IntentClaimed, domain.IntentExecuting, domain.IntentOutcomeUnknown:
		if costView.Reservation.Status != costdomain.ReservationReserved || quotaReservation.Status != quotadomain.ReservationReserved {
			return conflict("Generation Provider reservations are not executable")
		}
	case domain.IntentSucceeded, domain.IntentPartialSucceeded, domain.IntentFailed:
		if intent.CostSettlementReceiptID != "" {
			if job == nil || job.ID != intent.ProviderJobID || job.DispatchedCallCount <= 0 ||
				costView.Reservation.Status != costdomain.ReservationSettled ||
				costView.Reservation.SettledUnits != int64(job.DispatchedCallCount) ||
				costView.Reservation.UsageReceiptID == nil ||
				quotaReservation.Status != quotadomain.ReservationConsumed {
				return conflict("Generation Provider settled Owner facts have drifted")
			}
			usageReceipt, receiptErr := repo.FindReceiptByID(ctx, *costView.Reservation.UsageReceiptID)
			if receiptErr != nil || usageReceipt.WorkspaceID != intent.WorkspaceID ||
				usageReceipt.Operation != terminalProviderOperation ||
				usageReceipt.ResourceID != job.ID || usageReceipt.CreatedBy != intent.CreatedBy {
				return conflict("Generation Provider terminal usage receipt has drifted")
			}
			terminal := providerTerminalSnapshot(domain.GenerationRequest{ID: job.RequestID}, *job)
			terminalHash, hashErr := platformcommand.InputHash(terminal)
			replayed, replayErr := platformcommand.Replay[providerTerminalReceipt](usageReceipt, terminalHash)
			if hashErr != nil || replayErr != nil || replayed != terminal {
				return conflict("Generation Provider terminal usage receipt has drifted")
			}
		} else if costView.Reservation.Status != costdomain.ReservationReleased ||
			quotaReservation.Status != quotadomain.ReservationReleased || job == nil ||
			job.ID != intent.ProviderJobID || job.DispatchedCallCount != 0 ||
			costView.Reservation.SettledUnits != 0 || costView.Reservation.UsageReceiptID != nil {
			return conflict("Generation Provider released Owner facts have drifted")
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
	calls, err := repo.ListProviderCalls(ctx, job.ID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	if err = service.validateProviderFacts(ctx, repo, costs, quotas, intent, request, job, calls); err != nil {
		return ProviderExecutionResult{}, err
	}
	target, err := repo.FindGenerationTarget(ctx, request.TargetID)
	if err != nil || validateProviderTargetBinding(target, intent, request) != nil {
		return ProviderExecutionResult{}, conflict("Generation Provider Target snapshot has drifted")
	}
	receipts, err := repo.ListProviderResultReceipts(ctx, job.ID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	if err = validateProviderReceiptSet(calls, receipts); err != nil {
		return ProviderExecutionResult{}, err
	}
	return ProviderExecutionResult{
		Intent: intent, Target: target, Request: request, Job: job,
		Calls:    append([]domain.ProviderCall(nil), calls...),
		Receipts: append([]domain.ProviderResultReceipt(nil), receipts...),
	}, nil
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
	intent, err := repo.GetIntentForProviderJobUpdate(ctx, replayed.JobID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	job, err := repo.GetProviderJobForUpdate(ctx, replayed.JobID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	if job.IntentID != intent.ID {
		return ProviderExecutionResult{}, platformcommand.ErrInputMismatch
	}
	request, err := repo.FindGenerationRequest(ctx, replayed.RequestID)
	if err != nil {
		return ProviderExecutionResult{}, err
	}
	if job.RequestID != request.ID {
		return ProviderExecutionResult{}, platformcommand.ErrInputMismatch
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
	invocation providerInvocation,
	result ProviderExecutionResult,
	now time.Time,
) (platformcommand.Receipt, error) {
	encoded, err := platformcommand.Result(providerCommandReceipt{RequestID: result.Request.ID, JobID: result.Job.ID})
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return platformcommand.Receipt{}, errors.New("Generation Provider command receipt identifier is invalid")
	}
	return repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: result.Intent.WorkspaceID, Operation: invocation.operation,
		IdempotencyKey: invocation.key, InputHash: invocation.inputHash, ResourceID: result.Job.ID,
		Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
}

func (service *ProviderService) storeProviderTerminalReceipt(
	ctx context.Context,
	repo ProviderRepository,
	actor Actor,
	intent domain.Intent,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	now time.Time,
) (platformcommand.Receipt, error) {
	if !providerJobTerminal(job.Status) || request.ID != job.RequestID || intent.ID != job.IntentID {
		return platformcommand.Receipt{}, conflict("Generation Provider terminal aggregate has drifted")
	}
	snapshot := providerTerminalSnapshot(request, job)
	inputHash, err := platformcommand.InputHash(snapshot)
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	encoded, err := platformcommand.Result(snapshot)
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return platformcommand.Receipt{}, errors.New("Generation Provider terminal receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: intent.WorkspaceID, Operation: terminalProviderOperation,
		IdempotencyKey: preparationOwnerKey(intent.ID, "provider-terminal"), InputHash: inputHash,
		ResourceID: job.ID, Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	replayed, err := platformcommand.Replay[providerTerminalReceipt](receipt, inputHash)
	if err != nil || replayed != snapshot || receipt.ResourceID != job.ID || receipt.CreatedBy != actor.UserID {
		return platformcommand.Receipt{}, conflict("Generation Provider terminal receipt has drifted")
	}
	return receipt, nil
}

func providerTerminalSnapshot(
	request domain.GenerationRequest,
	job domain.ProviderJob,
) providerTerminalReceipt {
	return providerTerminalReceipt{
		RequestID: request.ID, JobID: job.ID, Status: job.Status, CallSetHash: job.CallSetHash,
		Revision: job.Revision, CallCount: job.CallCount, DispatchedCallCount: job.DispatchedCallCount,
		SucceededCallCount: job.SucceededCallCount, FailedCallCount: job.FailedCallCount,
	}
}

func selectProviderAction(
	job domain.ProviderJob,
	calls []domain.ProviderCall,
	recoverDispatching bool,
	now time.Time,
) (string, domain.ProviderCall) {
	if providerJobTerminal(job.Status) || job.Status == domain.ProviderJobOutcomeUnknown {
		return providerActionNone, domain.ProviderCall{}
	}
	for _, call := range calls {
		if call.Status == domain.ProviderCallDispatching {
			if recoverDispatching {
				return providerActionRecover, call
			}
			return providerActionNone, domain.ProviderCall{}
		}
	}
	for _, call := range calls {
		if call.Status == domain.ProviderCallSubmitted || call.Status == domain.ProviderCallRunning {
			if call.QueryDeadlineAt != nil && !now.Before(*call.QueryDeadlineAt) {
				return providerActionExpire, call
			}
			return providerActionQuery, call
		}
	}
	for _, call := range calls {
		if call.Status == domain.ProviderCallPending {
			return providerActionSubmit, call
		}
	}
	return providerActionNone, domain.ProviderCall{}
}

func validateAuthorizationBinding(intent domain.Intent, authorization domain.ExecutionAuthorization, _ bool) error {
	if validateIntent(intent) != nil || !validUUID(authorization.IntentID) || !validUUID(authorization.ClaimToken) ||
		!validUUID(authorization.TargetID) || !intentHashPattern.MatchString(authorization.TargetHash) ||
		intent.ID != authorization.IntentID || intent.ClaimToken == nil || *intent.ClaimToken != authorization.ClaimToken ||
		intent.TargetID != authorization.TargetID || intent.TargetHash != authorization.TargetHash ||
		intent.BindingVersionID != authorization.BindingVersionID || intent.BindingRevision != authorization.BindingRevision ||
		intent.BindingContentHash != authorization.BindingContentHash ||
		intent.ConnectionVersionID != authorization.ConnectionVersionID ||
		intent.CredentialVersionID != authorization.CredentialVersionID ||
		intent.ModelProfileVersionID != authorization.ModelProfileVersionID ||
		intent.ModelProfileRevision != authorization.ModelProfileRevision ||
		intent.ModelProfileContentHash != authorization.ModelProfileContentHash ||
		intent.PriceQuoteID != authorization.PriceQuoteID || intent.PriceQuoteRevision != authorization.PriceQuoteRevision ||
		intent.PriceQuoteContentHash != authorization.PriceQuoteContentHash || intent.BillingMetric != authorization.BillingMetric ||
		intent.CostReservationID != authorization.CostReservationID ||
		intent.QuotaReservationID != authorization.QuotaReservationID ||
		intent.ClaimFencingVersion != authorization.ClaimFencingVersion || authorization.IntentRevision != 3 ||
		intent.EstimatedUnits != authorization.EstimatedUnits || intent.ClaimExpiresAt == nil ||
		!intent.ClaimExpiresAt.Equal(authorization.ExpiresAt) {
		return conflict("Generation Provider authorization has drifted")
	}
	return nil
}

func validateIntentTargetBinding(target domain.GenerationTarget, intent domain.Intent) error {
	if domain.ValidateGenerationTarget(target) != nil || target.ID != intent.TargetID || target.WorkspaceID != intent.WorkspaceID ||
		target.ProjectID != intent.ProjectID || target.TargetHash != intent.TargetHash || target.CreatedBy != intent.CreatedBy {
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
		request.ProjectID != target.ProjectID || request.CreatedBy != target.CreatedBy ||
		request.EstimatedUnits != intent.EstimatedUnits {
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
		!intentHashPattern.MatchString(value.BindingContentHash) || !validProviderPurpose(value.Purpose) ||
		!providerIdentifierPattern.MatchString(value.ProviderKey) || !providerIdentifierPattern.MatchString(value.ExternalModelID) ||
		!validUUID(value.ConnectionVersionID) || !validUUID(value.CredentialVersionID) ||
		!validUUID(value.ModelProfileVersionID) || value.ModelProfileRevision < 1 ||
		!intentHashPattern.MatchString(value.ModelProfileContentHash) || !validUUID(value.PriceQuoteID) ||
		value.PriceQuoteRevision < 1 || !intentHashPattern.MatchString(value.PriceQuoteContentHash) ||
		!costdomain.IsBillingMetric(value.BillingMetric) || value.RequestKey != "generation-request:"+value.ID ||
		!intentHashPattern.MatchString(value.TargetHash) || value.EstimatedUnits < 1 ||
		!validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
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
		value.RequestKey == "" || value.CallCount < 1 || value.DispatchedCallCount < 0 ||
		value.SucceededCallCount < 0 || value.FailedCallCount < 0 ||
		value.DispatchedCallCount > value.CallCount || value.SucceededCallCount+value.FailedCallCount > value.CallCount ||
		!intentHashPattern.MatchString(value.CallSetHash) || value.Revision < 1 || value.CreatedAt.IsZero() ||
		value.UpdatedAt.Before(value.CreatedAt) {
		return conflict("Generation Provider job facts have drifted")
	}
	switch value.Status {
	case domain.ProviderJobPending:
		if value.DispatchedCallCount != 0 || value.SucceededCallCount != 0 || value.FailedCallCount != 0 {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobRunning:
		if value.SucceededCallCount+value.FailedCallCount >= value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobOutcomeUnknown:
		if value.DispatchedCallCount == 0 || value.SucceededCallCount+value.FailedCallCount >= value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobSucceeded:
		if value.SucceededCallCount != value.CallCount || value.FailedCallCount != 0 {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobPartialSucceeded:
		if value.SucceededCallCount == 0 || value.FailedCallCount == 0 ||
			value.SucceededCallCount+value.FailedCallCount != value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobFailed:
		if value.SucceededCallCount != 0 || value.FailedCallCount != value.CallCount {
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

func validateProviderCalls(calls []domain.ProviderCall, request domain.GenerationRequest, job domain.ProviderJob) error {
	if len(calls) != job.CallCount {
		return conflict("Generation Provider Call set has drifted")
	}
	for index, call := range calls {
		if !validUUID(call.ID) || call.WorkspaceID != job.WorkspaceID || call.ProjectID != job.ProjectID ||
			call.JobID != job.ID || call.CandidateIndex != index+1 || call.CallKey != "generation-call:"+call.ID ||
			!intentHashPattern.MatchString(call.RequestHash) || call.RequestedOutputCount != 1 || call.Revision < 1 ||
			call.CreatedAt.IsZero() || call.UpdatedAt.Before(call.CreatedAt) || len(call.RemoteRequestID) > 180 ||
			len(call.RemoteJobID) > 180 || (call.LocalFailureCode != "" && !providerFailurePattern.MatchString(call.LocalFailureCode)) {
			return conflict("Generation Provider Call facts have drifted")
		}
		if (call.QueryDeadlineAt == nil) != (call.RemoteExpiresAt == nil) {
			return conflict("Generation Provider Call retention window has drifted")
		}
		if call.QueryDeadlineAt != nil && (call.DispatchBoundaryEnteredAt == nil ||
			!call.QueryDeadlineAt.After(*call.DispatchBoundaryEnteredAt) ||
			!call.RemoteExpiresAt.After(*call.QueryDeadlineAt)) {
			return conflict("Generation Provider Call retention window has drifted")
		}
		expectedRequestHash, err := providerCallRequestHash(request, call.CandidateIndex)
		if err != nil || expectedRequestHash != call.RequestHash {
			return conflict("Generation Provider Call request has drifted")
		}
		switch call.Status {
		case domain.ProviderCallPending:
			if call.DispatchBoundaryEnteredAt != nil || call.LocalFailureCode != "" || call.RemoteRequestID != "" || call.RemoteJobID != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
			if call.QueryDeadlineAt != nil || call.RemoteExpiresAt != nil {
				return conflict("Generation Provider Call retention window has drifted")
			}
		case domain.ProviderCallDispatching:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" || call.RemoteRequestID != "" || call.RemoteJobID != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
			if call.QueryDeadlineAt != nil || call.RemoteExpiresAt != nil {
				return conflict("Generation Provider Call retention window has drifted")
			}
		case domain.ProviderCallSubmitted, domain.ProviderCallRunning:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" ||
				(call.RemoteRequestID == "" && call.RemoteJobID == "") || call.QueryDeadlineAt == nil ||
				call.RemoteExpiresAt == nil {
				return conflict("Generation Provider Call facts have drifted")
			}
		case domain.ProviderCallSucceeded, domain.ProviderCallOutcomeUnknown:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
		case domain.ProviderCallFailed:
			localFailure := call.DispatchBoundaryEnteredAt == nil && call.LocalFailureCode != "" &&
				call.RemoteRequestID == "" && call.RemoteJobID == "" && call.QueryDeadlineAt == nil &&
				call.RemoteExpiresAt == nil
			remoteFailure := call.DispatchBoundaryEnteredAt != nil && call.LocalFailureCode == ""
			if !localFailure && !remoteFailure {
				return conflict("Generation Provider Call facts have drifted")
			}
		default:
			return conflict("Generation Provider Call facts have drifted")
		}
		hash, err := providerCallContentHash(call)
		if err != nil || hash != call.ContentHash {
			return conflict("Generation Provider Call facts have drifted")
		}
	}
	return nil
}

func validateProviderResultReceipt(value domain.ProviderResultReceipt, call domain.ProviderCall) error {
	if !validUUID(value.ID) || value.WorkspaceID != call.WorkspaceID || value.ProjectID != call.ProjectID ||
		value.CallID != call.ID || len(value.ProviderEventID) > 180 || value.OccurredAt.IsZero() ||
		value.ReceivedAt.Before(value.OccurredAt) || !intentHashPattern.MatchString(value.ProviderUsageHash) ||
		!validProviderUsage(value.ProviderUsageObservation) {
		return conflict("Generation Provider result receipt has drifted")
	}
	usageHash, err := platformcommand.InputHash(value.ProviderUsageObservation)
	if err != nil || usageHash != value.ProviderUsageHash {
		return conflict("Generation Provider usage observation has drifted")
	}
	if value.Status == domain.ProviderResultSucceeded {
		if value.OutputCount != 1 || value.Output == nil || value.FailureCode != "" ||
			!validProviderOutput(*value.Output) || !providerOutputUsesCallStaging(*value.Output, call.WorkspaceID, call.JobID, call.ID) {
			return conflict("Generation Provider result receipt has drifted")
		}
	} else if value.Status == domain.ProviderResultFailed {
		if value.OutputCount != 0 || value.Output != nil || !providerFailurePattern.MatchString(value.FailureCode) {
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

func validateProviderReceiptSet(calls []domain.ProviderCall, receipts []domain.ProviderResultReceipt) error {
	byCall := make(map[string]domain.ProviderResultReceipt, len(receipts))
	for _, receipt := range receipts {
		if _, exists := byCall[receipt.CallID]; exists {
			return conflict("Generation Provider result receipt set contains duplicate Calls")
		}
		byCall[receipt.CallID] = receipt
	}
	for _, call := range calls {
		receipt, exists := byCall[call.ID]
		if call.Status == domain.ProviderCallSucceeded ||
			(call.Status == domain.ProviderCallFailed && call.DispatchBoundaryEnteredAt != nil) {
			if !exists || validateProviderResultReceipt(receipt, call) != nil ||
				(call.Status == domain.ProviderCallSucceeded) != (receipt.Status == domain.ProviderResultSucceeded) {
				return conflict("Generation Provider terminal Call receipt has drifted")
			}
			delete(byCall, call.ID)
		} else if exists {
			return conflict("Generation Provider Call has an unexpected receipt")
		}
	}
	if len(byCall) != 0 {
		return conflict("Generation Provider receipt set has unknown Calls")
	}
	return nil
}

func normalizedProviderOutcome(value ProviderOutcome, now time.Time) ProviderOutcome {
	value.Status = strings.TrimSpace(value.Status)
	value.RemoteRequestID = strings.TrimSpace(value.RemoteRequestID)
	value.RemoteJobID = strings.TrimSpace(value.RemoteJobID)
	value.ProviderEventID = strings.TrimSpace(value.ProviderEventID)
	value.FailureCode = strings.TrimSpace(value.FailureCode)
	if value.OccurredAt.IsZero() {
		value.OccurredAt = now
	} else {
		value.OccurredAt = value.OccurredAt.UTC().Truncate(time.Microsecond)
	}
	validRemote := len(value.RemoteRequestID) <= 180 && len(value.RemoteJobID) <= 180
	validEvent := len(value.ProviderEventID) <= 180
	validUsage := validProviderUsage(value.ProviderUsageObservation)
	if !value.QueryDeadlineAt.IsZero() {
		value.QueryDeadlineAt = value.QueryDeadlineAt.UTC().Truncate(time.Microsecond)
	}
	if !value.RemoteExpiresAt.IsZero() {
		value.RemoteExpiresAt = value.RemoteExpiresAt.UTC().Truncate(time.Microsecond)
	}
	if value.OccurredAt.After(now) || !validRemote || !validEvent || !validUsage {
		return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
	}
	switch value.Status {
	case ProviderOutcomeAccepted, ProviderOutcomeRunning:
		validRetentionWindow := value.QueryDeadlineAt.After(now) &&
			value.RemoteExpiresAt.After(value.QueryDeadlineAt) &&
			!value.RemoteExpiresAt.After(now.Add(30*24*time.Hour))
		if (value.RemoteRequestID != "" || value.RemoteJobID != "") && !value.QueryDeadlineAt.IsZero() &&
			!value.RemoteExpiresAt.IsZero() && validRetentionWindow && value.FailureCode == "" && value.Output == nil {
			return value
		}
	case ProviderOutcomeSucceeded:
		if value.FailureCode == "" && value.Output != nil && validProviderOutput(*value.Output) {
			value.Output = cloneProviderOutput(value.Output)
			return value
		}
	case ProviderOutcomeFailed:
		if providerFailurePattern.MatchString(value.FailureCode) && value.Output == nil {
			return value
		}
	case ProviderOutcomeUnknown:
		if value.FailureCode == "" && value.Output == nil {
			return ProviderOutcome{
				Status: ProviderOutcomeUnknown, RemoteRequestID: value.RemoteRequestID, RemoteJobID: value.RemoteJobID,
				OccurredAt: value.OccurredAt, QueryDeadlineAt: value.QueryDeadlineAt,
				RemoteExpiresAt: value.RemoteExpiresAt,
			}
		}
	}
	return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
}

func validProviderUsage(value domain.ProviderUsageObservation) bool {
	const maximum = int64(1_000_000_000_000)
	return value.InputTokens >= 0 && value.InputTokens <= maximum &&
		value.OutputTokens >= 0 && value.OutputTokens <= maximum && value.TotalTokens >= 0 && value.TotalTokens <= maximum &&
		value.ImageCount >= 0 && value.ImageCount <= maximum && value.VideoDurationMS >= 0 && value.VideoDurationMS <= maximum
}

func validProviderOutput(output ProviderOutput) bool {
	return providerOutputKeyPattern.MatchString(output.OutputKey) && output.StagingObjectKey != "" &&
		len(output.StagingObjectKey) <= 512 && intentHashPattern.MatchString(output.SHA256) && output.Bytes > 0 &&
		(output.MediaType == "image/png" || output.MediaType == "image/jpeg") && output.Width > 0 && output.Height > 0
}

func providerOutputUsesCallStaging(output ProviderOutput, workspaceID, providerJobID, providerCallID string) bool {
	prefix := "staging/" + workspaceID + "/" + providerJobID + "/" + providerCallID + "/"
	return strings.HasPrefix(output.StagingObjectKey, prefix) && !strings.Contains(output.StagingObjectKey, "..") &&
		!strings.HasSuffix(output.StagingObjectKey, "/")
}

func providerJobTerminal(status string) bool {
	return status == domain.ProviderJobSucceeded || status == domain.ProviderJobPartialSucceeded ||
		status == domain.ProviderJobFailed
}

func providerCallIrreversible(status string) bool {
	return status == domain.ProviderCallSucceeded || status == domain.ProviderCallFailed ||
		status == domain.ProviderCallOutcomeUnknown
}

func providerSubmission(
	request domain.GenerationRequest,
	job domain.ProviderJob,
	call domain.ProviderCall,
	target domain.GenerationTarget,
) ProviderSubmission {
	return ProviderSubmission{
		WorkspaceID: request.WorkspaceID, ProjectID: request.ProjectID, ProviderJobID: job.ID,
		ProviderCallID: call.ID, CallKey: call.CallKey, CallRequestHash: call.RequestHash, CandidateIndex: call.CandidateIndex,
		RequestedOutputCount: call.RequestedOutputCount, RequestID: request.ID, RequestKey: request.RequestKey,
		IntentID: request.IntentID, ProviderKey: request.ProviderKey, ExternalModelID: request.ExternalModelID,
		ConnectionVersionID: request.ConnectionVersionID, CredentialVersionID: request.CredentialVersionID,
		BindingID: request.BindingID, BindingRevision: request.BindingRevision, BindingContentHash: request.BindingContentHash,
		ModelProfileVersionID: request.ModelProfileVersionID, ModelProfileRevision: request.ModelProfileRevision,
		ModelProfileContentHash: request.ModelProfileContentHash, PriceQuoteID: request.PriceQuoteID,
		PriceQuoteRevision: request.PriceQuoteRevision, PriceQuoteContentHash: request.PriceQuoteContentHash,
		BillingMetric: request.BillingMetric, EstimatedUnits: request.EstimatedUnits,
		RemoteRequestID: call.RemoteRequestID, RemoteJobID: call.RemoteJobID,
		QueryDeadlineAt: cloneProviderTime(call.QueryDeadlineAt),
		RemoteExpiresAt: cloneProviderTime(call.RemoteExpiresAt),
		TargetHash:      request.TargetHash, Target: target,
	}
}

func generationRequestContentHash(value domain.GenerationRequest) (string, error) {
	return platformcommand.InputHash(generationRequestHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID, TargetID: value.TargetID,
		BindingID: value.BindingID, BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		Purpose: value.Purpose, ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID,
		ConnectionVersionID: value.ConnectionVersionID, CredentialVersionID: value.CredentialVersionID,
		ModelProfileVersionID: value.ModelProfileVersionID, ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteID: value.PriceQuoteID,
		PriceQuoteRevision: value.PriceQuoteRevision, PriceQuoteContentHash: value.PriceQuoteContentHash,
		BillingMetric: value.BillingMetric, RequestKey: value.RequestKey, TargetHash: value.TargetHash,
		EstimatedUnits: value.EstimatedUnits,
	})
}

func providerJobContentHash(value domain.ProviderJob) (string, error) {
	return platformcommand.InputHash(providerJobHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID,
		RequestID: value.RequestID, ProviderKey: value.ProviderKey, RequestKey: value.RequestKey,
		Status: value.Status, CallSetHash: value.CallSetHash, CallCount: value.CallCount,
		DispatchedCallCount: value.DispatchedCallCount, SucceededCallCount: value.SucceededCallCount,
		FailedCallCount: value.FailedCallCount, Revision: value.Revision,
	})
}

func providerCallRequestHash(request domain.GenerationRequest, candidateIndex int) (string, error) {
	return platformcommand.InputHash(providerCallRequestHashInput{
		RequestID: request.ID, RequestContentHash: request.ContentHash,
		CandidateIndex: candidateIndex, RequestedOutputCount: 1,
	})
}

func providerCallContentHash(value domain.ProviderCall) (string, error) {
	dispatchAt := ""
	if value.DispatchBoundaryEnteredAt != nil {
		dispatchAt = value.DispatchBoundaryEnteredAt.UTC().Format(time.RFC3339Nano)
	}
	return platformcommand.InputHash(providerCallHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, JobID: value.JobID,
		CallKey: value.CallKey, RequestHash: value.RequestHash, CandidateIndex: value.CandidateIndex,
		RequestedOutputCount: value.RequestedOutputCount, Status: value.Status,
		LocalFailureCode: value.LocalFailureCode, RemoteRequestID: value.RemoteRequestID,
		RemoteJobID: value.RemoteJobID, DispatchBoundaryEnteredAt: dispatchAt,
		QueryDeadlineAt: optionalProviderTimeHash(value.QueryDeadlineAt),
		RemoteExpiresAt: optionalProviderTimeHash(value.RemoteExpiresAt), Revision: value.Revision,
	})
}

func providerCallSetContentHash(
	calls []domain.ProviderCall,
	receipts []domain.ProviderResultReceipt,
) (string, error) {
	ordered := append([]domain.ProviderCall(nil), calls...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].CandidateIndex < ordered[right].CandidateIndex })
	receiptByCall := make(map[string]domain.ProviderResultReceipt, len(receipts))
	for _, receipt := range receipts {
		receiptByCall[receipt.CallID] = receipt
	}
	items := make([]providerCallSetHashItem, 0, len(ordered))
	for _, call := range ordered {
		receipt := receiptByCall[call.ID]
		items = append(items, providerCallSetHashItem{
			ID: call.ID, CallKey: call.CallKey, RequestHash: call.RequestHash, CallContentHash: call.ContentHash,
			ReceiptID: receipt.ID, ReceiptContentHash: receipt.ContentHash,
			CandidateIndex: call.CandidateIndex, RequestedOutputCount: call.RequestedOutputCount,
		})
	}
	return platformcommand.InputHash(items)
}

func providerResultReceiptContentHash(value domain.ProviderResultReceipt) (string, error) {
	return platformcommand.InputHash(providerReceiptHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, CallID: value.CallID,
		ProviderEventID: value.ProviderEventID, Status: value.Status, FailureCode: value.FailureCode,
		OutputCount: value.OutputCount, ProviderUsageObservation: value.ProviderUsageObservation,
		ProviderUsageHash: value.ProviderUsageHash, Output: cloneProviderOutput(value.Output),
		OccurredAt: value.OccurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func providerLocalFailureCode(err error) string {
	var typed ProviderLocalFailure
	if errors.As(err, &typed) {
		value := strings.TrimSpace(typed.ProviderFailureCode())
		if providerFailurePattern.MatchString(value) {
			return value
		}
	}
	return providerPreflightFailed
}

func remoteBindingDrifted(call domain.ProviderCall, outcome ProviderOutcome) bool {
	return (call.RemoteRequestID != "" && outcome.RemoteRequestID != "" && call.RemoteRequestID != outcome.RemoteRequestID) ||
		(call.RemoteJobID != "" && outcome.RemoteJobID != "" && call.RemoteJobID != outcome.RemoteJobID) ||
		(call.QueryDeadlineAt != nil && !outcome.QueryDeadlineAt.IsZero() &&
			!call.QueryDeadlineAt.Equal(outcome.QueryDeadlineAt)) ||
		(call.RemoteExpiresAt != nil && !outcome.RemoteExpiresAt.IsZero() &&
			!call.RemoteExpiresAt.Equal(outcome.RemoteExpiresAt))
}

func canonicalProviderOutcomeBinding(outcome ProviderOutcome) ProviderOutcome {
	outcome.RemoteRequestID = strings.TrimSpace(outcome.RemoteRequestID)
	outcome.RemoteJobID = strings.TrimSpace(outcome.RemoteJobID)
	if !outcome.QueryDeadlineAt.IsZero() {
		outcome.QueryDeadlineAt = outcome.QueryDeadlineAt.UTC().Truncate(time.Microsecond)
	}
	if !outcome.RemoteExpiresAt.IsZero() {
		outcome.RemoteExpiresAt = outcome.RemoteExpiresAt.UTC().Truncate(time.Microsecond)
	}
	return ProviderOutcome{
		RemoteRequestID: outcome.RemoteRequestID,
		RemoteJobID:     outcome.RemoteJobID,
		QueryDeadlineAt: outcome.QueryDeadlineAt,
		RemoteExpiresAt: outcome.RemoteExpiresAt,
	}
}

func invalidProviderOutcomeBinding(call domain.ProviderCall, outcome ProviderOutcome, now time.Time) bool {
	if len(outcome.RemoteRequestID) > 180 || len(outcome.RemoteJobID) > 180 {
		return true
	}
	if outcome.QueryDeadlineAt.IsZero() != outcome.RemoteExpiresAt.IsZero() {
		return true
	}
	if outcome.QueryDeadlineAt.IsZero() {
		return false
	}
	return call.DispatchBoundaryEnteredAt == nil ||
		!outcome.QueryDeadlineAt.After(*call.DispatchBoundaryEnteredAt) ||
		!outcome.RemoteExpiresAt.After(outcome.QueryDeadlineAt) ||
		outcome.RemoteExpiresAt.After(now.Add(30*24*time.Hour))
}

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

func replaceProviderCall(calls []domain.ProviderCall, replacement domain.ProviderCall) []domain.ProviderCall {
	result := append([]domain.ProviderCall(nil), calls...)
	for index := range result {
		if result[index].ID == replacement.ID {
			result[index] = replacement
			return result
		}
	}
	return result
}

func cloneProviderOutput(value *ProviderOutput) *ProviderOutput {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneProviderTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC().Truncate(time.Microsecond)
	return &cloned
}

func optionalProviderTimeHash(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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
