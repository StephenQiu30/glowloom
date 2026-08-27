package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type EpisodeSegmentationInvocationRepository interface {
	ClaimNextEpisodeSegmentation(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	ValidateEpisodeSegmentationInvocation(context.Context, string, int, time.Time) error
	CompleteEpisodeSegmentationInvocation(context.Context, string, int, contract.StageResult, time.Time) (bool, error)
	FailEpisodeSegmentationInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type EpisodeSegmentationWorker struct {
	repository EpisodeSegmentationInvocationRepository
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewEpisodeSegmentationWorker(
	repository EpisodeSegmentationInvocationRepository,
	agent AgentClient,
	now func() time.Time,
	interval time.Duration,
	lease time.Duration,
	logger *slog.Logger,
) *EpisodeSegmentationWorker {
	return &EpisodeSegmentationWorker{
		repository: repository, agent: agent, now: now, interval: interval, lease: lease, logger: logger,
	}
}

func (worker *EpisodeSegmentationWorker) Run(ctx context.Context) {
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

func (worker *EpisodeSegmentationWorker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNextEpisodeSegmentation(ctx, claimTime, claimTime.Add(worker.lease))
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim Episode segmentation invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := stageRequest(invocation)
	if err != nil || worker.repository.ValidateEpisodeSegmentationInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, worker.now().UTC(),
	) != nil {
		_, _ = worker.repository.FailEpisodeSegmentationInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid",
			"Episode segmentation stage invocation is invalid", false, worker.now().UTC(),
		)
		return true
	}
	result, invokeErr := worker.agent.Invoke(ctx, request, invocation.Attempts, int64(invocation.ClaimVersion))
	if invokeErr != nil {
		_, _ = worker.repository.FailEpisodeSegmentationInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			invokeErr.Error(), true, worker.now().UTC(),
		)
		return true
	}
	if err = result.ValidateFor(request); err != nil {
		_, _ = worker.repository.FailEpisodeSegmentationInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			"Agent returned an invalid Episode segmentation result", true, worker.now().UTC(),
		)
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailEpisodeSegmentationInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
				"Agent returned an incomplete Episode segmentation result", true, worker.now().UTC(),
			)
			return true
		}
		_, _ = worker.repository.FailEpisodeSegmentationInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code,
			result.Error.Summary, result.Error.Retryable, worker.now().UTC(),
		)
		return true
	}
	if _, err = worker.repository.CompleteEpisodeSegmentationInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC(),
	); err != nil {
		if errors.Is(err, ErrEpisodeSegmentationCandidateInvalid) {
			_, _ = worker.repository.FailEpisodeSegmentationInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "failed", "candidate_validation_failed",
				err.Error(), false, worker.now().UTC(),
			)
		} else if ctx.Err() == nil {
			worker.logger.Error("persist Episode segmentation result failed", "invocation_id", invocation.ID, "error", err)
		}
	}
	return true
}
