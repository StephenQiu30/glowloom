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
	ClaimNext(context.Context, time.Time) (domain.Invocation, bool, error)
	InvocationSource(context.Context, string) (string, error)
	CompleteInvocation(context.Context, string, contract.Result, time.Time) error
	FailInvocation(context.Context, string, string, string, string, bool, time.Time) error
}

type AgentClient interface {
	Invoke(context.Context, contract.Invocation) (contract.Result, error)
}

type Worker struct {
	repository InvocationRepository
	agent      AgentClient
	now        func() time.Time
	interval   time.Duration
	logger     *slog.Logger
}

func NewWorker(repository InvocationRepository, agent AgentClient, now func() time.Time, interval time.Duration, logger *slog.Logger) *Worker {
	return &Worker{repository: repository, agent: agent, now: now, interval: interval, logger: logger}
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
	invocation, found, err := worker.repository.ClaimNext(ctx, worker.now().UTC())
	if err != nil {
		if ctx.Err() == nil {
			worker.logger.Error("claim agent invocation failed", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	request := contract.Invocation{InvocationID: invocation.ID, Kind: invocation.Kind, InputHash: invocation.InputHash, SchemaVersion: contract.SchemaVersion, Payload: invocation.Payload}
	result, invokeErr := worker.agent.Invoke(ctx, request)
	if invokeErr != nil {
		if err = worker.repository.FailInvocation(ctx, invocation.ID, "unknown", "agent_outcome_unknown", invokeErr.Error(), true, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record unknown agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	if result.Status != "succeeded" {
		if err = worker.repository.FailInvocation(ctx, invocation.ID, result.Status, result.Error.Code, result.Error.Summary, result.Error.Retryable, worker.now().UTC()); err != nil && ctx.Err() == nil {
			worker.logger.Error("record failed agent invocation failed", "invocation_id", invocation.ID, "error", err)
		}
		return true
	}
	normalizedText, err := worker.repository.InvocationSource(ctx, invocation.ID)
	if err != nil {
		_ = worker.repository.FailInvocation(ctx, invocation.ID, "failed", "source_unavailable", err.Error(), false, worker.now().UTC())
		return true
	}
	_, err = domain.DecodeAndValidateCandidate(json.RawMessage(result.Candidate), normalizedText)
	if err != nil {
		_ = worker.repository.FailInvocation(ctx, invocation.ID, "failed", "candidate_validation_failed", err.Error(), false, worker.now().UTC())
		return true
	}
	if err = worker.repository.CompleteInvocation(ctx, invocation.ID, result, worker.now().UTC()); err != nil && ctx.Err() == nil {
		worker.logger.Error("persist agent result failed", "invocation_id", invocation.ID, "error", err)
	}
	return true
}
