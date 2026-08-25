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
		commands[1].WorkspaceID != "00000000-0000-0000-0000-000000000999" ||
		commands[1].ProjectID != "00000000-0000-0000-0000-000000000998" ||
		commands[1].InitiatorUserID != "00000000-0000-0000-0000-000000000997" ||
		commands[1].InitiatorTokenVersion != 3 ||
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

func TestRuntimeNodeCacheHitSkipsExecutorAndCommitsCachedOutput(t *testing.T) {
	cachedOutput := successfulExecutorOutput()
	_, _, cachedHash, err := workflow.BuildNodeOutput(cachedOutput)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeNodeRepository{
		status: "QUEUED", cachePolicy: "by_inputs",
		cachedResult: workflow.NodeActivityResult{Status: "CACHED", Output: cachedOutput, OutputHash: cachedHash},
	}
	executor := &scriptedNodeExecutor{}
	service := workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
		Now:   func() time.Time { return time.Date(2026, time.August, 26, 4, 30, 0, 0, time.UTC) },
		NewID: func() string { return "00000000-0000-0000-0000-000000000555" }, Executor: executor,
	})
	result, err := service.ExecuteNode(context.Background(), workflow.NodeActivityCommand{
		WorkflowRunID: "00000000-0000-0000-0000-000000000111",
		NodeRunID:     "00000000-0000-0000-0000-000000000222",
		NodeID:        "bible", Executor: "activity.production_bible", Attempt: 1,
	})
	if err != nil || result.Status != "CACHED" || result.OutputHash != cachedHash || executor.CallCount() != 0 ||
		repository.status != "CACHED" || repository.result.OutputHash != cachedHash {
		t.Fatalf("runtime cache hit = result %#v calls=%d repo=%#v err=%v", result, executor.CallCount(), repository, err)
	}
}

func TestRuntimeNodeCacheMissCommitsFactWithNodeOutput(t *testing.T) {
	repository := &runtimeNodeRepository{status: "QUEUED", cachePolicy: "by_inputs"}
	executor := &scriptedNodeExecutor{}
	identities := []string{
		"00000000-0000-0000-0000-000000000555",
		"00000000-0000-0000-0000-000000000556",
	}
	service := workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
		Now: func() time.Time { return time.Date(2026, time.August, 26, 4, 45, 0, 0, time.UTC) },
		NewID: func() string {
			identity := identities[0]
			identities = identities[1:]
			return identity
		},
		Executor: executor,
	})
	result, err := service.ExecuteNode(context.Background(), workflow.NodeActivityCommand{
		WorkflowRunID: "00000000-0000-0000-0000-000000000111",
		NodeRunID:     "00000000-0000-0000-0000-000000000222",
		NodeID:        "bible", Executor: "activity.production_bible", Attempt: 1,
	})
	if err != nil || result.Status != "SUCCEEDED" || executor.CallCount() != 1 ||
		repository.cacheEntry.ID != "00000000-0000-0000-0000-000000000556" ||
		len(repository.cacheEntry.CacheKey) != 64 ||
		repository.cacheEntry.OutputHash != result.OutputHash || repository.result.OutputHash != result.OutputHash {
		t.Fatalf("runtime cache miss = result %#v entry=%#v repo=%#v err=%v", result, repository.cacheEntry, repository, err)
	}
}

type runtimeNodeRepository struct {
	mu           sync.Mutex
	status       string
	attempt      int
	revision     int
	claimToken   string
	claims       []string
	result       workflow.NodeActivityResult
	cachePolicy  string
	cachedResult workflow.NodeActivityResult
	cacheEntry   workflow.NodeCacheEntry
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
	cachePolicy := repo.cachePolicy
	if cachePolicy == "" {
		cachePolicy = "never"
	}
	material := workflow.NodeCacheKeyMaterial{
		SchemaVersion: workflow.NodeCacheKeySchemaVersion, NodeDefinitionContentHash: strings.Repeat("1", 64),
		ConfigHash: strings.Repeat("2", 64), NormalizedInputHash: inputHash, RuntimeContractVersion: "1.0.0",
	}
	material, cacheKey, err := workflow.BuildNodeCacheKey(material)
	if err != nil {
		return workflow.NodeExecutionClaim{}, err
	}
	if cachePolicy == "never" {
		cacheKey = ""
	}
	return workflow.NodeExecutionClaim{
		Command: command, ClaimToken: claimToken, Status: repo.status,
		Attempt: repo.attempt, Revision: repo.revision, Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "production_bible_candidate", Required: true}},
		WorkspaceID: "00000000-0000-0000-0000-000000000999",
		ProjectID:   "00000000-0000-0000-0000-000000000998", InitiatorUserID: "00000000-0000-0000-0000-000000000997",
		InitiatorTokenVersion: 3,
		CachePolicy:           cachePolicy,
		CacheMaterial:         material, CacheKey: cacheKey,
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

func (repo *runtimeNodeRepository) CompleteNodeFromCache(
	_ context.Context,
	claim workflow.NodeExecutionClaim,
	_ time.Time,
) (workflow.NodeActivityResult, bool, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.cachedResult.Status == "" {
		return workflow.NodeActivityResult{}, false, nil
	}
	if repo.claimToken != claim.ClaimToken || claim.CachePolicy != "by_inputs" || claim.CacheKey == "" {
		return workflow.NodeActivityResult{}, false, errors.New("invalid cache claim")
	}
	repo.status = "CACHED"
	repo.result = repo.cachedResult
	repo.claimToken = ""
	repo.revision++
	return repo.cachedResult, true, nil
}

func (repo *runtimeNodeRepository) CompleteNodeWithCache(
	ctx context.Context,
	claim workflow.NodeExecutionClaim,
	result workflow.NodeActivityResult,
	entry workflow.NodeCacheEntry,
	now time.Time,
) error {
	repo.cacheEntry = entry
	return repo.CompleteNode(ctx, claim, result, now)
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
