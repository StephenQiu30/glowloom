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
	CompleteInvocation(context.Context, string, int, contract.StageResult, time.Time) (bool, error)
	FailInvocation(context.Context, string, int, string, string, string, bool, time.Time) (bool, error)
}

type AgentClient interface {
	Invoke(context.Context, contract.StageInvocation, int, int64) (contract.StageResult, error)
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
	request, err := stageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid", "Agent stage invocation is invalid", false, worker.now().UTC())
		return true
	}
	result, invokeErr := worker.agent.Invoke(ctx, request, invocation.Attempts, int64(invocation.ClaimVersion))
	if invokeErr != nil {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown", invokeErr.Error(), true, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown", "Agent returned an incomplete result", true, worker.now().UTC())
			return true
		}
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code, result.Error.Summary, result.Error.Retryable, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	normalizedText, err := worker.repository.InvocationSource(ctx, invocation.ID)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "evidence_invalid", err.Error(), false, worker.now().UTC())
		return true
	}
	if _, err = domain.DecodeAndValidateCandidate(result.Candidate, normalizedText); err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "evidence_invalid", err.Error(), false, worker.now().UTC())
		return true
	}
	if _, err = worker.repository.CompleteInvocation(ctx, invocation.ID, invocation.ClaimVersion, result, worker.now().UTC()); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist agent result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}

func stageRequest(invocation domain.Invocation) (contract.StageInvocation, error) {
	var policy contract.StageExecutionPolicy
	if err := json.Unmarshal(invocation.ExecutionPolicy, &policy); err != nil {
		return contract.StageInvocation{}, err
	}
	var payload contract.StageInvocationPayload
	if err := json.Unmarshal(invocation.Payload, &payload); err != nil {
		return contract.StageInvocation{}, err
	}
	request := contract.StageInvocation{
		InvocationID: invocation.ID, Kind: invocation.Kind,
		WireSchemaVersion: contract.StoryGraphWireSchemaVersion, InputHash: invocation.InputHash,
		ExecutionPolicy: policy, Payload: payload,
	}
	return request, request.Validate()
}
