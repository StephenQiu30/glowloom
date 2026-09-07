package application

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

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
		switch call.Status {
		case domain.ProviderCallDispatching:
			if invocation.action != providerActionSubmit && invocation.action != providerActionRecover {
				return conflict("Generation Provider dispatch fence has drifted")
			}
		case domain.ProviderCallSubmitted, domain.ProviderCallRunning:
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
		default:
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
