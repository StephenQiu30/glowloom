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
	CompleteInvocation(context.Context, string, int, contract.Result, domain.Candidate, time.Time) (bool, error)
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
			worker.logger.Error("claim storyboard invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request := contract.Invocation{InvocationID: invocation.ID, Kind: invocation.Kind, InputHash: invocation.InputHash, SchemaVersion: contract.SchemaVersion, Payload: invocation.Payload}
	result, invokeErr := worker.agent.Invoke(ctx, request)
	if invokeErr != nil {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "unknown", "agent_outcome_unknown", invokeErr.Error(), true, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown storyboard invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if result.Status != "succeeded" {
		if _, err = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, result.Status, result.Error.Code, result.Error.Summary, result.Error.Retryable, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed storyboard invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	candidate, err := domain.DecodeAndValidateCandidate(json.RawMessage(result.Candidate), invocation.Payload)
	if err != nil {
		_, _ = worker.repository.FailInvocation(ctx, invocation.ID, invocation.ClaimVersion, "failed", "candidate_validation_failed", err.Error(), false, worker.now().UTC())
		return true
	}
	if _, err = worker.repository.CompleteInvocation(ctx, invocation.ID, invocation.ClaimVersion, result, candidate, worker.now().UTC()); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist storyboard result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
