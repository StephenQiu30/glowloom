package workflow_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
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
	if err != nil || result.Status != "SUCCEEDED" || result.OutputHash == "" ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0].Port != "candidate" {
		t.Fatalf("retry node execution: result=%#v err=%v", result, err)
	}
	if repository.status != "SUCCEEDED" || repository.attempt != 2 || repository.claimToken != "" {
		t.Fatalf("completed activity projection = status %s attempt %d token %q", repository.status, repository.attempt, repository.claimToken)
	}
	replayed, err := service.ExecuteNode(context.Background(), command)
	if err != nil || replayed.Status != "SUCCEEDED" || replayed.OutputHash != result.OutputHash ||
		replayed.Output.Bindings[0] != result.Output.Bindings[0] {
		t.Fatalf("replay completed node: result=%#v err=%v", replayed, err)
	}
	if repository.result.OutputHash != result.OutputHash || repository.result.Output.Bindings[0] != result.Output.Bindings[0] {
		t.Fatalf("repository output projection = %#v", repository.result)
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
	commands := executor.Commands()
	if len(commands) != 2 || commands[1].InputHash == "" || commands[1].Input.SchemaVersion != workflow.NodeInputSchemaVersion ||
		len(commands[1].OutputPorts) != 1 || commands[1].OutputPorts[0].Key != "candidate" {
		t.Fatalf("executor commands lost frozen input/output contract: %#v", commands)
	}
}

func TestRuntimeRejectsExecutorOwnedCacheAndInvalidOutputWithoutCommittingIt(t *testing.T) {
	tests := []struct {
		name     string
		executor *scriptedNodeExecutor
	}{
		{name: "executor cannot declare cache hit", executor: &scriptedNodeExecutor{status: "CACHED"}},
		{name: "output binding must be valid", executor: &scriptedNodeExecutor{invalidOutput: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &runtimeNodeRepository{status: "QUEUED"}
			service := workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
				Now:   func() time.Time { return time.Date(2026, time.August, 26, 4, 0, 0, 0, time.UTC) },
				NewID: func() string { return "claim-token" }, Executor: test.executor,
			})
			_, err := service.ExecuteNode(context.Background(), workflow.NodeActivityCommand{
				WorkflowRunID: "00000000-0000-0000-0000-000000000111",
				NodeRunID:     "00000000-0000-0000-0000-000000000222",
				NodeID:        "bible", Executor: "activity.production_bible", Attempt: 1,
			})
			if err == nil || repository.status != "RETRYING" || repository.result.OutputHash != "" {
				t.Fatalf("invalid executor result was committed: status=%s result=%#v err=%v", repository.status, repository.result, err)
			}
		})
	}
}

type runtimeNodeRepository struct {
	mu         sync.Mutex
	status     string
	attempt    int
	revision   int
	claimToken string
	claims     []string
	result     workflow.NodeActivityResult
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
		return workflow.NodeExecutionClaim{Command: command, Status: repo.status, Result: repo.result, Replay: true}, nil
	}
	repo.status = "RUNNING"
	repo.attempt++
	repo.revision++
	repo.claimToken = claimToken
	repo.claims = append(repo.claims, claimToken)
	input, _, inputHash, err := workflow.BuildNodeInput(successfulNodeInput())
	if err != nil {
		return workflow.NodeExecutionClaim{}, err
	}
	return workflow.NodeExecutionClaim{
		Command: command, ClaimToken: claimToken, Status: repo.status,
		Attempt: repo.attempt, Revision: repo.revision, Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "production_bible_candidate", Required: true}},
	}, nil
}

func (repo *runtimeNodeRepository) CompleteNode(
	_ context.Context,
	claim workflow.NodeExecutionClaim,
	result workflow.NodeActivityResult,
	_ time.Time,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimToken != claim.ClaimToken {
		return errors.New("stale claim")
	}
	repo.status = result.Status
	repo.result = result
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
	mu            sync.Mutex
	failures      int
	status        string
	invalidOutput bool
	keys          []string
	commands      []workflow.NodeExecutorCommand
}

func (executor *scriptedNodeExecutor) Execute(
	_ context.Context,
	command workflow.NodeExecutorCommand,
) (workflow.NodeExecutorResult, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.keys = append(executor.keys, command.IdempotencyKey)
	executor.commands = append(executor.commands, command)
	if executor.failures > 0 {
		executor.failures--
		return workflow.NodeExecutorResult{}, errors.New("transient executor failure")
	}
	status := executor.status
	if status == "" {
		status = "SUCCEEDED"
	}
	output := successfulExecutorOutputFor(command.OutputPorts)
	if executor.invalidOutput {
		output.Bindings[0].ContentHash = "invalid"
	}
	return workflow.NodeExecutorResult{Status: status, Output: output}, nil
}

func successfulExecutorOutput() workflow.NodeOutputSnapshot {
	return successfulExecutorOutputFor([]authoring.PortDefinition{{
		Key: "candidate", ValueType: "production_bible_candidate", Required: true,
	}})
}

func successfulExecutorOutputFor(ports []authoring.PortDefinition) workflow.NodeOutputSnapshot {
	bindings := make([]workflow.NodeOutputBinding, 0, len(ports))
	for _, port := range ports {
		if !port.Required {
			continue
		}
		bindings = append(bindings, workflow.NodeOutputBinding{
			Port: port.Key, ValueType: port.ValueType,
			ReferenceID: "00000000-0000-0000-0000-000000000333", ReferenceVersion: "1",
			ContentHash: strings.Repeat("c", 64),
		})
	}
	return workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings:      bindings,
	}
}

func successfulNodeInput() workflow.NodeInputSnapshot {
	return workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: []byte(`{}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: "00000000-0000-0000-0000-000000000444", Version: "1",
			Hash: strings.Repeat("d", 64),
		}},
	}
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

func (executor *scriptedNodeExecutor) Commands() []workflow.NodeExecutorCommand {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return append([]workflow.NodeExecutorCommand(nil), executor.commands...)
}
