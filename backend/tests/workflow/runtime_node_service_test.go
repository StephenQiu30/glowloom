package workflow_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestRuntimeNodeExecutionRetriesWithStableBusinessIdentityAndFencing(t *testing.T) {
	repository := &runtimeNodeRepository{status: "QUEUED"}
	executor := &scriptedNodeExecutor{failures: 1}
	now := time.Date(2026, time.August, 25, 23, 0, 0, 0, time.UTC)
	tokenNo := 0
	service := workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			tokenNo++
			return "claim-token-" + string(rune('0'+tokenNo))
		},
		Executor: executor,
	})
	command := workflow.NodeActivityCommand{
		WorkflowRunID: "00000000-0000-0000-0000-000000000111",
		NodeRunID:     "00000000-0000-0000-0000-000000000222", NodeID: "bible",
		Executor: "activity.production_bible", Attempt: 1,
	}

	if _, err := service.ExecuteNode(context.Background(), command); err == nil {
		t.Fatal("first executor failure was reported as success")
	}
	if repository.status != "RETRYING" || repository.attempt != 1 || repository.claimToken != "" {
		t.Fatalf("failed activity projection = status %s attempt %d token %q", repository.status, repository.attempt, repository.claimToken)
	}
	result, err := service.ExecuteNode(context.Background(), command)
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("retry node execution: result=%#v err=%v", result, err)
	}
	if repository.status != "SUCCEEDED" || repository.attempt != 2 || repository.claimToken != "" {
		t.Fatalf("completed activity projection = status %s attempt %d token %q", repository.status, repository.attempt, repository.claimToken)
	}
	replayed, err := service.ExecuteNode(context.Background(), command)
	if err != nil || replayed.Status != "SUCCEEDED" {
		t.Fatalf("replay completed node: result=%#v err=%v", replayed, err)
	}
	if executor.CallCount() != 2 {
		t.Fatalf("executor call count = %d, want 2", executor.CallCount())
	}
	keys := executor.IdempotencyKeys()
	if len(keys) != 2 || keys[0] != keys[1] || keys[0] == "" {
		t.Fatalf("executor idempotency keys = %v", keys)
	}
	if len(repository.claims) != 2 || repository.claims[0] == repository.claims[1] {
		t.Fatalf("claim fencing tokens = %v", repository.claims)
	}
}

type runtimeNodeRepository struct {
	mu         sync.Mutex
	status     string
	attempt    int
	revision   int
	claimToken string
	claims     []string
}

func (repo *runtimeNodeRepository) LoadExecutionPlan(context.Context, workflow.StartRequest) (workflow.ExecutionPlan, error) {
	return workflow.ExecutionPlan{}, nil
}

func (repo *runtimeNodeRepository) ClaimNode(
	_ context.Context,
	command workflow.NodeActivityCommand,
	claimToken string,
	_ time.Time,
) (workflow.NodeExecutionClaim, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.status == "SUCCEEDED" {
		return workflow.NodeExecutionClaim{Command: command, Status: repo.status, Replay: true}, nil
	}
	repo.status = "RUNNING"
	repo.attempt++
	repo.revision++
	repo.claimToken = claimToken
	repo.claims = append(repo.claims, claimToken)
	return workflow.NodeExecutionClaim{
		Command: command, ClaimToken: claimToken, Status: repo.status,
		Attempt: repo.attempt, Revision: repo.revision,
	}, nil
}

func (repo *runtimeNodeRepository) CompleteNode(
	_ context.Context,
	claim workflow.NodeExecutionClaim,
	status string,
	_ time.Time,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimToken != claim.ClaimToken {
		return errors.New("stale claim")
	}
	repo.status = status
	repo.claimToken = ""
	repo.revision++
	return nil
}

func (repo *runtimeNodeRepository) RetryNode(
	_ context.Context,
	claim workflow.NodeExecutionClaim,
	_ time.Time,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimToken != claim.ClaimToken {
		return errors.New("stale claim")
	}
	repo.status = "RETRYING"
	repo.claimToken = ""
	repo.revision++
	return nil
}

type scriptedNodeExecutor struct {
	mu       sync.Mutex
	failures int
	keys     []string
}

func (executor *scriptedNodeExecutor) Execute(
	_ context.Context,
	command workflow.NodeExecutorCommand,
) (workflow.NodeActivityResult, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.keys = append(executor.keys, command.IdempotencyKey)
	if executor.failures > 0 {
		executor.failures--
		return workflow.NodeActivityResult{}, errors.New("transient executor failure")
	}
	return workflow.NodeActivityResult{Status: "SUCCEEDED"}, nil
}

func (executor *scriptedNodeExecutor) CallCount() int {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return len(executor.keys)
}

func (executor *scriptedNodeExecutor) IdempotencyKeys() []string {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return append([]string(nil), executor.keys...)
}
