package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type StoryReviewInvocationRepository interface {
	ClaimNextStoryReview(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	ValidateStoryReviewInvocation(context.Context, string, int, time.Time) error
	CompleteStoryReviewInvocation(context.Context, string, int, contract.StageResult, time.Time) (bool, error)
	FailStoryReviewInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type StoryReviewWorker struct {
	repository StoryReviewInvocationRepository
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewStoryReviewWorker(
	repository StoryReviewInvocationRepository,
	agent AgentClient,
	now func() time.Time,
	interval time.Duration,
	lease time.Duration,
	logger *slog.Logger,
) *StoryReviewWorker {
	return &StoryReviewWorker{
		repository: repository, agent: agent, now: now, interval: interval, lease: lease, logger: logger,
	}
}

func (worker *StoryReviewWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		if worker.runOnce(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (worker *StoryReviewWorker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNextStoryReview(ctx, claimTime, claimTime.Add(worker.lease))
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim Story review invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := stageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailStoryReviewInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid",
			"Story review stage invocation is invalid", false, worker.now().UTC(),
		)
		return true
	}
	if err = worker.repository.ValidateStoryReviewInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, worker.now().UTC(),
	); err != nil {
		_, _ = worker.repository.FailStoryReviewInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "review_input_stale", err.Error(), false, worker.now().UTC(),
		)
		return true
	}
	result, invokeErr := worker.agent.Invoke(ctx, request, invocation.Attempts, int64(invocation.ClaimVersion))
	if invokeErr != nil {
		_, _ = worker.repository.FailStoryReviewInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			invokeErr.Error(), true, worker.now().UTC(),
		)
		return true
	}
	if err = result.ValidateFor(request); err != nil {
		_, _ = worker.repository.FailStoryReviewInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			"Agent returned an invalid Story review result", true, worker.now().UTC(),
		)
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailStoryReviewInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
				"Agent returned an incomplete Story review result", true, worker.now().UTC(),
			)
			return true
		}
		_, _ = worker.repository.FailStoryReviewInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code,
			result.Error.Summary, result.Error.Retryable, worker.now().UTC(),
		)
		return true
	}
	if _, err = worker.repository.CompleteStoryReviewInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC(),
	); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist Story review result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
