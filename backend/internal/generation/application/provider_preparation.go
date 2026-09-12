package application

import (
	"context"
	"errors"
	"strings"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

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
	if err != nil || ValidateProviderModelProfileVersion(profile) != nil || profile.WorkspaceID != intent.WorkspaceID ||
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
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, errors.New("generation Provider identifiers are invalid")
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
			return domain.GenerationRequest{}, domain.ProviderJob{}, nil, errors.New("generation Provider call identifier is invalid")
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
	if err != nil || ValidateProviderModelProfileVersion(profile) != nil || profile.WorkspaceID != request.WorkspaceID ||
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
