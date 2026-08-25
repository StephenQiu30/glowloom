package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Compiler interface {
	Compile(context.Context, Actor, CompileCommand) (domain.CompiledFacts, error)
}

type WorkflowStarter interface {
	Start(context.Context, domain.StartRequest) (domain.StartObservation, error)
}

type StartConfig struct {
	Now   func() time.Time
	NewID func() string
}

type StartService struct {
	compiler     Compiler
	transactions TransactionManager
	starter      WorkflowStarter
	config       StartConfig
}

type StartCommand struct {
	AuthoringRevisionID string
	IdempotencyKey      string
}

type temporalStartInput struct {
	WorkflowID            string `json:"workflow_id"`
	WorkflowType          string `json:"workflow_type"`
	WorkflowTypeVersion   string `json:"workflow_type_version"`
	WorkflowRunID         string `json:"workflow_run_id"`
	DefinitionVersionID   string `json:"definition_version_id"`
	RunInputSnapshotID    string `json:"run_input_snapshot_id"`
	DefinitionContentHash string `json:"definition_content_hash"`
	InputSnapshotHash     string `json:"input_snapshot_hash"`
}

func NewStartService(compiler Compiler, transactions TransactionManager, starter WorkflowStarter, config StartConfig) *StartService {
	return &StartService{compiler: compiler, transactions: transactions, starter: starter, config: config}
}

func (service *StartService) Start(ctx context.Context, actor Actor, command StartCommand) (domain.WorkflowRun, error) {
	command.AuthoringRevisionID = strings.TrimSpace(command.AuthoringRevisionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.AuthoringRevisionID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 ||
		service.compiler == nil || service.transactions == nil || service.starter == nil ||
		service.config.Now == nil || service.config.NewID == nil {
		return domain.WorkflowRun{}, invalid("Invalid workflow start request")
	}
	commandInputHash, err := platformcommand.InputHash(struct {
		AuthoringRevisionID string `json:"authoring_revision_id"`
	}{AuthoringRevisionID: command.AuthoringRevisionID})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	compiled, err := service.compiler.Compile(ctx, actor, CompileCommand{
		AuthoringRevisionID: command.AuthoringRevisionID, IdempotencyKey: "start-compile-" + commandInputHash,
	})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	now := service.config.Now().UTC()
	desired, request, err := prepareStart(command, commandInputHash, compiled, actor.UserID, now)
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	var prepared domain.StartPreparation
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var prepareErr error
		prepared, prepareErr = repo.PrepareStart(ctx, desired)
		return prepareErr
	})
	if err != nil {
		return domain.WorkflowRun{}, normalizeError(err)
	}
	if prepared.Intent.CommandInputHash != commandInputHash || prepared.Intent.TemporalInputHash != request.InputHash ||
		prepared.Run.AuthoringRevisionID != command.AuthoringRevisionID {
		return domain.WorkflowRun{}, conflict("Idempotency key was already used with different workflow input")
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Run, nil
	}
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var beginErr error
		prepared, beginErr = repo.BeginStartAttempt(ctx, prepared.Intent.ID, service.config.Now().UTC())
		return beginErr
	})
	if err != nil {
		return domain.WorkflowRun{}, normalizeError(err)
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Run, nil
	}
	observation, startErr := service.starter.Start(ctx, request)
	updatedRun, updatedIntent, receipt := finalizeStart(
		prepared, observation, startErr, service.config.NewID(), service.config.Now().UTC(),
	)
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		return repo.FinalizeStartAttempt(
			ctx, updatedRun, updatedIntent, receipt, prepared.Run.Revision, prepared.Intent.Revision,
		)
	})
	if err != nil {
		return domain.WorkflowRun{}, normalizeError(err)
	}
	return updatedRun, nil
}

func prepareStart(
	command StartCommand,
	commandInputHash string,
	compiled domain.CompiledFacts,
	createdBy string,
	now time.Time,
) (domain.StartPreparation, domain.StartRequest, error) {
	runID := stableID("workflow-run", compiled.Definition.WorkspaceID, command.IdempotencyKey)
	intentID := stableID("workflow-start-intent", runID)
	workflowID := "lanverse:" + compiled.Definition.WorkspaceID + ":" + compiled.Definition.ProjectID + ":" + runID
	input := temporalStartInput{
		WorkflowID: workflowID, WorkflowType: compiled.Definition.WorkflowType,
		WorkflowTypeVersion: compiled.Definition.WorkflowTypeVersion, WorkflowRunID: runID,
		DefinitionVersionID: compiled.DefinitionID, RunInputSnapshotID: compiled.RunInputSnapshotID,
		DefinitionContentHash: compiled.Definition.ContentHash, InputSnapshotHash: compiled.RunInputSnapshot.ContentHash,
	}
	temporalInputHash, err := platformcommand.InputHash(input)
	if err != nil {
		return domain.StartPreparation{}, domain.StartRequest{}, err
	}
	request := domain.StartRequest{
		WorkflowID: input.WorkflowID, WorkflowType: input.WorkflowType, WorkflowTypeVersion: input.WorkflowTypeVersion,
		WorkflowRunID: input.WorkflowRunID, DefinitionVersionID: input.DefinitionVersionID,
		RunInputSnapshotID: input.RunInputSnapshotID, DefinitionContentHash: input.DefinitionContentHash,
		InputSnapshotHash: input.InputSnapshotHash, InputHash: temporalInputHash,
	}
	run := domain.WorkflowRun{
		ID: runID, WorkspaceID: compiled.Definition.WorkspaceID, ProjectID: compiled.Definition.ProjectID,
		AuthoringRevisionID: compiled.Definition.AuthoringRevisionID, DefinitionVersionID: compiled.DefinitionID,
		RunInputSnapshotID: compiled.RunInputSnapshotID, TemporalWorkflowID: workflowID, StartInputHash: temporalInputHash,
		Status: "QUEUED", ProgressStage: "start_pending", Revision: 1, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now,
	}
	nodes := make([]domain.NodeRunProjection, 0, len(compiled.Definition.NodeExecutions))
	for _, node := range compiled.Definition.NodeExecutions {
		nodes = append(nodes, domain.NodeRunProjection{
			ID: stableID("workflow-node-run", runID, node.NodeID), WorkspaceID: run.WorkspaceID, WorkflowRunID: runID,
			NodeID: node.NodeID, DefinitionKey: node.DefinitionKey, DefinitionVersion: node.DefinitionVersion,
			Status: "QUEUED", Attempt: 0, Revision: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	intent := domain.StartIntent{
		ID: intentID, WorkspaceID: run.WorkspaceID, WorkflowRunID: runID, IdempotencyKey: command.IdempotencyKey,
		CommandInputHash: commandInputHash, TemporalInputHash: temporalInputHash, Status: "pending",
		AttemptNo: 0, Revision: 1, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now,
	}
	return domain.StartPreparation{Run: run, Nodes: nodes, Intent: intent}, request, nil
}

func finalizeStart(
	prepared domain.StartPreparation,
	observation domain.StartObservation,
	startErr error,
	receiptID string,
	now time.Time,
) (domain.WorkflowRun, domain.StartIntent, domain.StartReceipt) {
	run, intent := prepared.Run, prepared.Intent
	outcome := observation.Outcome
	var observed *string
	if len(observation.ObservedInputHash) == 64 {
		value := observation.ObservedInputHash
		observed = &value
	}
	switch {
	case startErr != nil || outcome == domain.StartOutcomeUnknown:
		outcome = domain.StartOutcomeUnknown
		run.Status, run.ProgressStage = "NEEDS_ATTENTION", "start_unknown"
		run.NextAction = stringPointer("reconcile_start")
		run.Error = errorPayload("start_outcome_unknown")
		intent.Status = "unknown"
	case (outcome == domain.StartOutcomeStarted || outcome == domain.StartOutcomeAlreadyStarted) &&
		observation.ObservedInputHash == intent.TemporalInputHash:
		run.Status, run.ProgressStage = "RUNNING", "running"
		run.NextAction, run.Error = nil, nil
		intent.Status = "completed"
	default:
		outcome = domain.StartOutcomeConflict
		run.Status, run.ProgressStage = "NEEDS_ATTENTION", "start_conflict"
		run.NextAction = stringPointer("inspect_workflow_identity")
		run.Error = errorPayload("workflow_identity_conflict")
		intent.Status = "conflict"
	}
	run.Revision++
	run.UpdatedAt = now
	intent.Revision++
	intent.UpdatedAt = now
	receipt := domain.StartReceipt{
		ID: receiptID, WorkspaceID: run.WorkspaceID, StartIntentID: intent.ID, WorkflowRunID: run.ID,
		AttemptNo: intent.AttemptNo, Outcome: outcome, TemporalWorkflowID: run.TemporalWorkflowID,
		ExpectedInputHash: intent.TemporalInputHash, ObservedInputHash: observed, CreatedAt: now,
	}
	return run, intent, receipt
}

func stableID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, ":"))).String()
}

func stringPointer(value string) *string { return &value }

func errorPayload(code string) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{"code": code})
	return encoded
}
