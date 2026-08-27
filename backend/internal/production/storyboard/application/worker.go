package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type InvocationRepository interface {
	ClaimNext(context.Context, time.Time, time.Time) (domain.Invocation, bool, error)
	CompleteInvocation(context.Context, string, int, contract.StageResult, domain.Candidate, time.Time) (bool, error)
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
			worker.logger.Error("claim storyboard invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request, err := storyboardStageRequest(invocation)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "invocation_policy_invalid", "Agent stage invocation is invalid", false, worker.now().UTC())
		return true
	}
	result, invokeErr := worker.agent.Invoke(ctx, request, invocation.Attempts, int64(invocation.ClaimVersion))
	if invokeErr != nil {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown", invokeErr.Error(), true, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown storyboard invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if result.Status != "succeeded" {
		if result.Error == nil {
			_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_execution_unknown", "Agent returned an incomplete result", true, worker.now().UTC())
			return true
		}
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code, result.Error.Summary, result.Error.Retryable, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed storyboard invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	candidate, err := domain.DecodeAndValidateCandidate(result.Candidate, request.Payload.StageInput)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "candidate_schema_invalid", err.Error(), false, worker.now().UTC())
		return true
	}
	if _, err = worker.repository.CompleteInvocation(ctx, invocation.ID, invocation.ClaimVersion, result, candidate, worker.now().UTC()); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist storyboard result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}

func storyboardStageRequest(invocation domain.Invocation) (contract.StageInvocation, error) {
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
