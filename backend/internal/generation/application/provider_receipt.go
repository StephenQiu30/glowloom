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

type providerCommandReceipt struct {
	RequestID, JobID string
}

type providerTerminalReceipt struct {
	RequestID, JobID, Status, CallSetHash string
	Revision                              int64
	CallCount, DispatchedCallCount        int
	SucceededCallCount, FailedCallCount   int
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
		return domain.ProviderResultReceipt{}, errors.New("generation Provider result receipt identifier is invalid")
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
		return platformcommand.Receipt{}, errors.New("generation Provider command receipt identifier is invalid")
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
		return platformcommand.Receipt{}, errors.New("generation Provider terminal receipt identifier is invalid")
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
