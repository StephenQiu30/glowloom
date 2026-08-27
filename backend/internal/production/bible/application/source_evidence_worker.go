package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type SourceEvidenceInvocationRepository interface {
	ClaimNextSourceEvidence(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	CompleteSourceEvidenceInvocation(context.Context, string, int, contract.StageResult, time.Time) (bool, error)
	FailSourceEvidenceInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type SourceEvidenceWorker struct {
	repository SourceEvidenceInvocationRepository
	resharder  *SourceEvidenceService
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewSourceEvidenceWorker(
	repository SourceEvidenceInvocationRepository,
	resharder *SourceEvidenceService,
	agent AgentClient,
	now func() time.Time,
	interval time.Duration,
	lease time.Duration,
	logger *slog.Logger,
) *SourceEvidenceWorker {
	return &SourceEvidenceWorker{
		repository: repository, resharder: resharder, agent: agent, now: now,
		interval: interval, lease: lease, logger: logger,
	}
}

func (worker *SourceEvidenceWorker) Run(ctx context.Context) {
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

func (worker *SourceEvidenceWorker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNextSourceEvidence(
		ctx, claimTime, claimTime.Add(worker.lease),
	)
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim source Evidence invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := stageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailSourceEvidenceInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid",
			"Source Evidence stage invocation is invalid", false, worker.now().UTC(),
		)
		return true
	}
	result, invokeErr := worker.agent.Invoke(
		ctx, request, invocation.Attempts, int64(invocation.ClaimVersion),
	)
	if invokeErr != nil {
		if _, err = worker.repository.FailSourceEvidenceInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			invokeErr.Error(), true, worker.now().UTC(),
		); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown source Evidence invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if err = result.ValidateFor(request); err != nil {
		_, _ = worker.repository.FailSourceEvidenceInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
			"Agent returned an invalid Source Evidence result", true, worker.now().UTC(),
		)
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailSourceEvidenceInvocation(
				ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown",
				"Agent returned an incomplete Source Evidence result", true, worker.now().UTC(),
			)
			return true
		}
		if result.Error.Code == "execution_budget_exceeded" && worker.resharder != nil {
			if _, err = worker.resharder.ReshardBudgetExceeded(
				ctx, invocation.ID, invocation.ClaimVersion, result.Error.Summary,
			); err == nil {
				return true
			} else if !errors.Is(err, ErrSourceEvidenceShardCannotSplit) {
				if ctx.Err() == nil {
					worker.logger.Error("reshard source Evidence invocation failed", "invocation_id", invocation.ID, "error", err)
				}
				return true
			}
		}
		if _, err = worker.repository.FailSourceEvidenceInvocation(
			ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code,
			result.Error.Summary, result.Error.Retryable, worker.now().UTC(),
		); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed source Evidence invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if _, err = worker.repository.CompleteSourceEvidenceInvocation(
		ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC(),
	); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist source Evidence result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
