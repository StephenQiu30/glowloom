package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type StoryAnalysisInvocationRepository interface {
	ClaimNextStoryAnalysis(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	ValidateStoryAnalysisInvocation(context.Context, string, int, time.Time) error
	CompleteStoryAnalysisInvocation(context.Context, string, int, contract.StageResult, time.Time) (bool, error)
	FailStoryAnalysisInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type StoryAnalysisWorker struct {
	repository StoryAnalysisInvocationRepository
	resharder  *StoryAnalysisService
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewStoryAnalysisWorker(
	repository StoryAnalysisInvocationRepository,
	resharder *StoryAnalysisService,
	agent AgentClient,
	now func() time.Time,
	interval time.Duration,
	lease time.Duration,
	logger *slog.Logger,
) *StoryAnalysisWorker {
	return &StoryAnalysisWorker{
		repository: repository, resharder: resharder, agent: agent, now: now, interval: interval, lease: lease, logger: logger,
	}
}

func (worker *StoryAnalysisWorker) Run(ctx context.Context) {
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

func (worker *StoryAnalysisWorker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNextStoryAnalysis(ctx, claimTime, claimTime.Add(worker.lease))
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim Story analysis invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := stageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailStoryAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid",
			"Story analysis stage invocation is invalid", false, worker.now().UTC(),
		)
		return true
	}
	if err = worker.repository.ValidateStoryAnalysisInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, worker.now().UTC(),
	); err != nil {
		code := "invocation_policy_invalid"
		if errors.Is(err, ErrStoryAnalysisUpstreamStale) {
			code = "upstream_candidate_stale"
		} else if errors.Is(err, ErrStoryAnalysisManifestStale) {
			code = "manifest_superseded"
		}
		_, _ = worker.repository.FailStoryAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", code, err.Error(), false, worker.now().UTC(),
		)
		return true
	}
	result, invokeErr := worker.agent.Invoke(ctx, request, invocation.Attempts, int64(invocation.ClaimVersion))
	if invokeErr != nil {
		_, _ = worker.repository.FailStoryAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			invokeErr.Error(), true, worker.now().UTC(),
		)
		return true
	}
	if err = result.ValidateFor(request); err != nil {
		_, _ = worker.repository.FailStoryAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			"Agent returned an invalid Story analysis result", true, worker.now().UTC(),
		)
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailStoryAnalysisInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
				"Agent returned an incomplete Story analysis result", true, worker.now().UTC(),
			)
			return true
		}
		if result.Error.Code == "execution_budget_exceeded" && worker.resharder != nil {
			if _, err = worker.resharder.ReshardBudgetExceeded(
				ctx, invocation.ID, invocation.ClaimVersion, result.Error.Summary,
			); err == nil {
				return true
			} else if !errors.Is(err, domain.ErrStoryCandidateCannotSplit) {
				if ctx.Err() == nil {
					worker.logger.Error("reshard Story analysis invocation failed", "invocation_id", invocation.ID, "error", err)
				}
				return true
			}
		}
		_, _ = worker.repository.FailStoryAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code,
			result.Error.Summary, result.Error.Retryable, worker.now().UTC(),
		)
		return true
	}
	if _, err = worker.repository.CompleteStoryAnalysisInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC(),
	); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist Story analysis result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
