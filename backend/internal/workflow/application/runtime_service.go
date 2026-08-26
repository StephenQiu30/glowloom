package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type RuntimeRepository interface {
	LoadExecutionPlan(context.Context, domain.StartRequest) (domain.ExecutionPlan, error)
}

type NodeRuntimeRepository interface {
	ClaimNode(context.Context, domain.NodeActivityCommand, string, time.Time) (domain.NodeExecutionClaim, error)
	CompleteNode(context.Context, domain.NodeExecutionClaim, domain.NodeActivityResult, time.Time) error
	RetryNode(context.Context, domain.NodeExecutionClaim, time.Time) error
}

type NodeCacheRuntimeRepository interface {
	CompleteNodeFromCache(context.Context, domain.NodeExecutionClaim, time.Time) (domain.NodeActivityResult, bool, error)
	CompleteNodeWithCache(
		context.Context,
		domain.NodeExecutionClaim,
		domain.NodeActivityResult,
		domain.NodeCacheEntry,
		time.Time,
	) error
}

type NodeCacheRepository interface {
	FindNodeCache(context.Context, string, string) (domain.NodeCacheEntry, error)
	EnsureNodeCache(context.Context, domain.NodeCacheEntry) (domain.NodeCacheEntry, error)
}

type RunCompletionRepository interface {
	CompleteRun(context.Context, domain.CompleteRunCommand, time.Time) error
}

type RunFailureRepository interface {
	FailRun(context.Context, domain.FailRunCommand, time.Time) error
}

type HumanGateRepository interface {
	PrepareHumanGate(context.Context, domain.NodeActivityCommand, time.Time) (domain.HumanGateBinding, error)
}

type HumanGateApplyRepository interface {
	ApplyHumanGate(context.Context, domain.ApplyHumanGateCommand, time.Time) error
}

type HumanTaskOpener interface {
	OpenHumanTask(context.Context, domain.HumanGateBinding) error
}

type NodeExecutor interface {
	Execute(context.Context, domain.NodeExecutorCommand) (domain.NodeExecutorResult, error)
}

type RuntimeConfig struct {
	Now        func() time.Time
	NewID      func() string
	Executor   NodeExecutor
	HumanTasks HumanTaskOpener
}

func (service *RuntimeService) OpenHumanGate(ctx context.Context, command domain.NodeActivityCommand) error {
	if service == nil || service.config.Now == nil || service.config.HumanTasks == nil ||
		strings.TrimSpace(command.WorkflowRunID) == "" || strings.TrimSpace(command.NodeRunID) == "" ||
		strings.TrimSpace(command.NodeID) == "" || strings.TrimSpace(command.Executor) == "" || command.Attempt < 1 {
		return invalid("Invalid human gate input")
	}
	repository, supported := service.repository.(HumanGateRepository)
	if !supported {
		return errors.New("human gate repository is unavailable")
	}
	binding, err := repository.PrepareHumanGate(ctx, command, service.config.Now().UTC())
	if err != nil {
		return normalizeError(err)
	}
	return service.config.HumanTasks.OpenHumanTask(ctx, binding)
}

func (service *RuntimeService) ApplyHumanGate(ctx context.Context, command domain.ApplyHumanGateCommand) error {
	if service == nil || service.config.Now == nil || strings.TrimSpace(command.WorkflowRunID) == "" ||
		strings.TrimSpace(command.NodeRunID) == "" || strings.TrimSpace(command.NodeID) == "" ||
		strings.TrimSpace(command.SignalIntentID) == "" {
		return invalid("Invalid human gate application input")
	}
	switch command.Decision {
	case "APPROVED", "REJECTED", "CHANGES_REQUESTED", "SELECTED":
	default:
		return invalid("Invalid human gate application input")
	}
	if command.Decision == "APPROVED" || command.Decision == "SELECTED" {
		normalized, _, outputHash, outputErr := domain.BuildNodeOutput(command.Output)
		if outputErr != nil || strings.TrimSpace(command.OwnerReceiptID) == "" || command.OutputHash != outputHash {
			return invalid("Invalid human gate application input")
		}
		command.OwnerReceiptID, command.Output = strings.TrimSpace(command.OwnerReceiptID), normalized
	} else if command.OwnerReceiptID != "" || command.OutputHash != "" || command.Output.SchemaVersion != "" || len(command.Output.Bindings) != 0 {
		return invalid("Invalid human gate application input")
	}
	repository, supported := service.repository.(HumanGateApplyRepository)
	if !supported {
		return errors.New("human gate apply repository is unavailable")
	}
	return normalizeError(repository.ApplyHumanGate(ctx, command, service.config.Now().UTC()))
}

type RuntimeService struct {
	repository RuntimeRepository
	config     RuntimeConfig
}

func NewRuntimeService(repository RuntimeRepository, configurations ...RuntimeConfig) *RuntimeService {
	var configuration RuntimeConfig
	if len(configurations) == 1 {
		configuration = configurations[0]
	}
	return &RuntimeService{repository: repository, config: configuration}
}

func (service *RuntimeService) LoadExecutionPlan(ctx context.Context, request domain.StartRequest) (domain.ExecutionPlan, error) {
	if service == nil || service.repository == nil || strings.TrimSpace(request.WorkflowRunID) == "" ||
		strings.TrimSpace(request.DefinitionVersionID) == "" || strings.TrimSpace(request.RunInputSnapshotID) == "" ||
		len(request.DefinitionContentHash) != 64 || len(request.InputSnapshotHash) != 64 || len(request.InputHash) != 64 {
		return domain.ExecutionPlan{}, invalid("Invalid workflow runtime input")
	}
	plan, err := service.repository.LoadExecutionPlan(ctx, request)
	return plan, normalizeError(err)
}

func (service *RuntimeService) ExecuteNode(ctx context.Context, command domain.NodeActivityCommand) (domain.NodeActivityResult, error) {
	if service == nil {
		return domain.NodeActivityResult{}, invalid("Invalid workflow node execution input")
	}
	repository, supported := service.repository.(NodeRuntimeRepository)
	if !supported || service.config.Now == nil || service.config.NewID == nil || service.config.Executor == nil ||
		strings.TrimSpace(command.WorkflowRunID) == "" || strings.TrimSpace(command.NodeRunID) == "" ||
		strings.TrimSpace(command.NodeID) == "" || strings.TrimSpace(command.Executor) == "" || command.Attempt < 1 {
		return domain.NodeActivityResult{}, invalid("Invalid workflow node execution input")
	}
	claimToken := strings.TrimSpace(service.config.NewID())
	if claimToken == "" {
		return domain.NodeActivityResult{}, errors.New("workflow node claim token is empty")
	}
	claim, err := repository.ClaimNode(ctx, command, claimToken, service.config.Now().UTC())
	if err != nil {
		return domain.NodeActivityResult{}, normalizeError(err)
	}
	if claim.Replay {
		normalized, _, outputHash, outputErr := domain.BuildNodeOutput(claim.Result.Output)
		if outputErr != nil || claim.Result.Status != claim.Status || claim.Result.OutputHash != outputHash {
			return domain.NodeActivityResult{}, errors.New("completed workflow node output has drifted")
		}
		claim.Result.Output = normalized
		return claim.Result, nil
	}
	normalizedInput, _, inputHash, inputErr := domain.BuildNodeInput(claim.Input)
	if inputErr != nil || claim.InputHash != inputHash || len(claim.OutputPorts) == 0 ||
		strings.TrimSpace(claim.WorkspaceID) == "" || strings.TrimSpace(claim.ProjectID) == "" ||
		strings.TrimSpace(claim.InitiatorUserID) == "" || claim.InitiatorTokenVersion < 1 {
		retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
		return domain.NodeActivityResult{}, errors.Join(errors.New("workflow node resolved an invalid execution contract"), retryErr)
	}
	cacheRepository, cacheEnabled, cacheErr := runtimeNodeCacheRepository(service.repository, claim)
	if cacheErr != nil {
		retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
		return domain.NodeActivityResult{}, errors.Join(cacheErr, retryErr)
	}
	if cacheEnabled {
		cached, found, findErr := cacheRepository.CompleteNodeFromCache(ctx, claim, service.config.Now().UTC())
		if findErr != nil {
			retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
			return domain.NodeActivityResult{}, errors.Join(findErr, retryErr)
		}
		if found {
			normalized, _, outputHash, outputErr := domain.BuildNodeOutput(cached.Output)
			if outputErr != nil || cached.Status != "CACHED" || cached.OutputHash != outputHash ||
				domain.ValidateNodeOutputPorts(normalized, claim.OutputPorts) != nil {
				return domain.NodeActivityResult{}, errors.New("workflow node cache returned an invalid output")
			}
			cached.Output = normalized
			return cached, nil
		}
	}
	executorResult, executeErr := service.config.Executor.Execute(ctx, domain.NodeExecutorCommand{
		NodeActivityCommand:   command,
		WorkspaceID:           claim.WorkspaceID,
		ProjectID:             claim.ProjectID,
		InitiatorUserID:       claim.InitiatorUserID,
		InitiatorTokenVersion: claim.InitiatorTokenVersion,
		IdempotencyKey:        "workflow-node:" + command.NodeRunID + ":attempt:" + strconv.Itoa(command.Attempt),
		Input:                 normalizedInput,
		InputHash:             inputHash,
		OutputPorts:           append([]authoring.PortDefinition(nil), claim.OutputPorts...),
	})
	if executeErr != nil {
		return domain.NodeActivityResult{}, errors.Join(executeErr, repository.RetryNode(ctx, claim, service.config.Now().UTC()))
	}
	if executorResult.Status == "RETRYING" {
		if executorResult.Output.SchemaVersion != "" || len(executorResult.Output.Bindings) != 0 {
			retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
			return domain.NodeActivityResult{}, errors.Join(errors.New("waiting workflow node executor returned an output"), retryErr)
		}
		if retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC()); retryErr != nil {
			return domain.NodeActivityResult{}, normalizeError(retryErr)
		}
		return domain.NodeActivityResult{Status: "RETRYING"}, nil
	}
	if executorResult.Status != "SUCCEEDED" && executorResult.Status != "SKIPPED" {
		retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
		return domain.NodeActivityResult{}, errors.Join(errors.New("workflow node executor returned an invalid status"), retryErr)
	}
	normalizedOutput, output, outputHash, outputErr := domain.BuildNodeOutput(executorResult.Output)
	if outputErr != nil || domain.ValidateNodeOutputPorts(normalizedOutput, claim.OutputPorts) != nil {
		retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
		return domain.NodeActivityResult{}, errors.Join(errors.New("workflow node executor returned an invalid output"), retryErr)
	}
	result := domain.NodeActivityResult{Status: executorResult.Status, Output: normalizedOutput, OutputHash: outputHash}
	if cacheEnabled {
		cacheID := strings.TrimSpace(service.config.NewID())
		if cacheID == "" {
			retryErr := repository.RetryNode(ctx, claim, service.config.Now().UTC())
			return domain.NodeActivityResult{}, errors.Join(errors.New("workflow node cache identity is empty"), retryErr)
		}
		entry := domain.NodeCacheEntry{
			ID: cacheID, WorkspaceID: claim.WorkspaceID, CacheKey: claim.CacheKey,
			KeyMaterial: claim.CacheMaterial, Output: output, OutputHash: outputHash,
			SourceWorkflowRunID: command.WorkflowRunID, SourceNodeRunID: command.NodeRunID,
			CreatedAt: service.config.Now().UTC(),
		}
		err = cacheRepository.CompleteNodeWithCache(ctx, claim, result, entry, entry.CreatedAt)
	} else {
		err = repository.CompleteNode(ctx, claim, result, service.config.Now().UTC())
	}
	if err != nil {
		return domain.NodeActivityResult{}, normalizeError(err)
	}
	return result, nil
}

func runtimeNodeCacheRepository(
	repository RuntimeRepository,
	claim domain.NodeExecutionClaim,
) (NodeCacheRuntimeRepository, bool, error) {
	switch claim.CachePolicy {
	case "never":
		if claim.CacheKey != "" {
			return nil, false, errors.New("non-cacheable workflow node has a cache key")
		}
		return nil, false, nil
	case "by_inputs":
		_, cacheKey, err := domain.BuildNodeCacheKey(claim.CacheMaterial)
		if err != nil || cacheKey != claim.CacheKey || strings.TrimSpace(claim.WorkspaceID) == "" {
			return nil, false, errors.New("workflow node cache contract has drifted")
		}
		cacheRepository, supported := repository.(NodeCacheRuntimeRepository)
		if !supported {
			return nil, false, errors.New("workflow node cache repository is unavailable")
		}
		return cacheRepository, true, nil
	default:
		return nil, false, errors.New("workflow node cache policy is invalid")
	}
}

func (service *RuntimeService) CompleteRun(ctx context.Context, command domain.CompleteRunCommand) error {
	if service == nil || service.config.Now == nil || strings.TrimSpace(command.WorkflowRunID) == "" {
		return invalid("Invalid workflow completion input")
	}
	repository, supported := service.repository.(RunCompletionRepository)
	if !supported {
		return errors.New("workflow completion repository is unavailable")
	}
	return normalizeError(repository.CompleteRun(ctx, command, service.config.Now().UTC()))
}

func (service *RuntimeService) FailRun(ctx context.Context, command domain.FailRunCommand) error {
	if service == nil || service.config.Now == nil || strings.TrimSpace(command.WorkflowRunID) == "" ||
		strings.TrimSpace(command.NodeRunID) == "" || strings.TrimSpace(command.NodeID) == "" ||
		command.FailureCode != "node_activity_failed" {
		return invalid("Invalid workflow failure input")
	}
	repository, supported := service.repository.(RunFailureRepository)
	if !supported {
		return errors.New("workflow failure repository is unavailable")
	}
	return normalizeError(repository.FailRun(ctx, command, service.config.Now().UTC()))
}
