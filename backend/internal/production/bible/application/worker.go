package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type InvocationRepository interface {
	ClaimNext(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	InvocationSource(context.Context, string) (string, error)
	CompleteInvocation(context.Context, string, int, contract.Result, time.Time) (bool, error)
	FailInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type AgentClient interface {
	Invoke(context.Context, contract.Invocation) (contract.Result, error)
}

type Worker struct {
	repository InvocationRepository
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	lease      time.Duration
	logger     *slog.Logger
}

func NewWorker(repository InvocationRepository, agent AgentClient, now func() time.Time, interval, lease time.Duration, logger *slog.Logger) *Worker {
	return &Worker{repository: repository, agent: agent, now: now, interval: interval, lease: lease, logger: logger}
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		worked := worker.runOnce(ctx)
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (worker *Worker) runOnce(ctx context.Context) bool {
	claimTime := worker.now().UTC()
	invocation, found, err := worker.repository.ClaimNext(ctx, claimTime, claimTime.Add(worker.lease))
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim agent invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	var executionPolicy contract.ExecutionPolicy
	if err = json.Unmarshal(invocation.ExecutionPolicy, &executionPolicy); err != nil || executionPolicy.ValidateFor(invocation.Kind) != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "invalid_execution_policy", "Agent execution policy is invalid", false, worker.now().UTC())
		return true
	}
	request := contract.Invocation{InvocationID: invocation.ID, Kind: invocation.Kind, InputHash: invocation.InputHash, SchemaVersion: contract.SchemaVersion, ExecutionPolicy: executionPolicy, Payload: invocation.Payload}
	result, invokeErr := worker.agent.Invoke(ctx, request)
	if invokeErr != nil {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_outcome_unknown", invokeErr.Error(), true, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if result.Status != "succeeded" {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code, result.Error.Summary, result.Error.Retryable, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	normalizedText, err := worker.repository.InvocationSource(ctx, invocation.ID)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "source_unavailable", err.Error(), false, worker.now().UTC())
		return true
	}
	_, err = domain.DecodeAndValidateCandidate(json.RawMessage(result.Candidate), normalizedText)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "candidate_validation_failed", err.Error(), false, worker.now().UTC())
		return true
	}
	if _, err = worker.repository.CompleteInvocation(ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC()); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist agent result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
