package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type EpisodeAnalysisInvocationRepository interface {
	ClaimNextEpisodeAnalysis(context.Context, time.Time, time.Time) (bibledomain.Invocation, bool, error)
	ValidateEpisodeAnalysisInvocation(context.Context, string, int, time.Time) error
	CompleteEpisodeAnalysisInvocation(context.Context, string, int, agentcontract.StageResult, time.Time) (bool, error)
	FailEpisodeAnalysisInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type EpisodeAnalysisAgentClient interface {
	Invoke(context.Context, agentcontract.StageInvocation, int, int64) (agentcontract.StageResult, error)
}

type EpisodeAnalysisWorker struct {
	repository EpisodeAnalysisInvocationRepository
	agent      EpisodeAnalysisAgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewEpisodeAnalysisWorker(
	repository EpisodeAnalysisInvocationRepository,
	agent EpisodeAnalysisAgentClient,
	now func() time.Time,
	interval time.Duration,
	lease time.Duration,
	logger *slog.Logger,
) *EpisodeAnalysisWorker {
	return &EpisodeAnalysisWorker{
		repository: repository, agent: agent, now: now,
		interval: interval, lease: lease, logger: logger,
	}
}

func (worker *EpisodeAnalysisWorker) Run(ctx context.Context) {
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

func (worker *EpisodeAnalysisWorker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNextEpisodeAnalysis(
		ctx,
		claimTime,
		claimTime.Add(worker.lease),
	)
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim Episode analysis invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := episodeStageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid",
			"Episode analysis stage invocation is invalid", false, worker.now().UTC(),
		)
		return true
	}
	if err = worker.repository.ValidateEpisodeAnalysisInvocation(
		ctx,
		invocation.ID,
		invocation.ClaimVersion,
		worker.now().UTC(),
	); err != nil {
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "upstream_candidate_stale",
			err.Error(), false, worker.now().UTC(),
		)
		return true
	}
	result, invokeErr := worker.agent.Invoke(
		ctx,
		request,
		invocation.Attempts,
		int64(invocation.ClaimVersion),
	)
	if invokeErr != nil {
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			invokeErr.Error(), true, worker.now().UTC(),
		)
		return true
	}
	if err = result.ValidateFor(request); err != nil {
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			"Agent returned an invalid Episode analysis result", true, worker.now().UTC(),
		)
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailEpisodeAnalysisInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
				"Agent returned an incomplete Episode analysis result", true, worker.now().UTC(),
			)
			return true
		}
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code,
			result.Error.Summary, result.Error.Retryable, worker.now().UTC(),
		)
		return true
	}
	if _, err = worker.repository.CompleteEpisodeAnalysisInvocation(
		ctx,
		invocation.ID,
		invocation.ClaimVersion,
		result,
		worker.now().UTC(),
	); err != nil && ctx.Err() == nil {
		_, _ = worker.repository.FailEpisodeAnalysisInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "candidate_persistence_unknown",
			"Episode analysis Candidate persistence outcome is unknown", true, worker.now().UTC(),
		)
		worker.logger.Error(
			"persist Episode analysis result failed",
			"invocation_id",
			invocation.ID,
			"error",
			err,
		)
	}
	return true
}

func episodeStageRequest(invocation bibledomain.Invocation) (agentcontract.StageInvocation, error) {
	var policy agentcontract.StageExecutionPolicy
	if err := json.Unmarshal(invocation.ExecutionPolicy, &policy); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	var payload agentcontract.StageInvocationPayload
	if err := json.Unmarshal(invocation.Payload, &payload); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	request := agentcontract.StageInvocation{
		InvocationID:      invocation.ID,
		Kind:              invocation.Kind,
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		InputHash:         invocation.InputHash,
		ExecutionPolicy:   policy,
		Payload:           payload,
	}
	return request, request.Validate()
}
