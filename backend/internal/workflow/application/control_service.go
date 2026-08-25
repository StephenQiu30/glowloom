package application

import (
	"context"
	"strconv"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type ControlRepository interface {
	PrepareControl(context.Context, domain.ControlPreparation) (domain.ControlPreparation, error)
	BeginControlAttempt(context.Context, string, time.Time) (domain.ControlPreparation, error)
	FinalizeControlAttempt(context.Context, domain.ControlFinalization) error
}

type WorkflowController interface {
	Control(context.Context, domain.ControlRequest) (domain.ControlObservation, error)
}

type ControlConfig struct {
	Now   func() time.Time
	NewID func() string
}

type ControlService struct {
	repository ControlRepository
	controller WorkflowController
	config     ControlConfig
}

type ControlCommand struct {
	WorkspaceID, WorkflowRunID string
	ExpectedRevision           int
	IdempotencyKey             string
}

type CancelCommand = ControlCommand
type PauseCommand = ControlCommand
type ResumeCommand = ControlCommand

func NewControlService(repository ControlRepository, controller WorkflowController, config ControlConfig) *ControlService {
	return &ControlService{repository: repository, controller: controller, config: config}
}

func (service *ControlService) Cancel(
	ctx context.Context,
	actor Actor,
	command CancelCommand,
) (domain.ControlIntent, error) {
	return service.control(ctx, actor, command, domain.ControlActionCancel)
}

func (service *ControlService) Pause(
	ctx context.Context,
	actor Actor,
	command PauseCommand,
) (domain.ControlIntent, error) {
	return service.control(ctx, actor, command, domain.ControlActionPause)
}

func (service *ControlService) Resume(
	ctx context.Context,
	actor Actor,
	command ResumeCommand,
) (domain.ControlIntent, error) {
	return service.control(ctx, actor, command, domain.ControlActionResume)
}

func (service *ControlService) control(
	ctx context.Context,
	actor Actor,
	command ControlCommand,
	action string,
) (domain.ControlIntent, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if service == nil || service.repository == nil || service.controller == nil || service.config.Now == nil ||
		service.config.NewID == nil || actor.UserID == "" || command.WorkspaceID == "" || command.WorkflowRunID == "" ||
		command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || !validControlAction(action) {
		return domain.ControlIntent{}, invalid("Invalid workflow control input")
	}
	commandInputHash, err := platformcommand.InputHash(struct {
		WorkspaceID, WorkflowRunID string
		ExpectedRevision           int
		Action                     string
	}{
		WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		ExpectedRevision: command.ExpectedRevision, Action: action,
	})
	if err != nil {
		return domain.ControlIntent{}, err
	}
	now := service.config.Now().UTC()
	intentID := stableID(
		"workflow-control-intent", command.WorkspaceID, command.WorkflowRunID, action,
		strconv.Itoa(command.ExpectedRevision),
	)
	desired := domain.ControlPreparation{Intent: domain.ControlIntent{
		ID: intentID, WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		IdempotencyKey: command.IdempotencyKey, CommandInputHash: commandInputHash,
		ControlID: stableID("workflow-control", intentID), Action: action,
		ExpectedRunRevision: command.ExpectedRevision, Status: "pending", Revision: 1,
		CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
	}}
	prepared, err := service.repository.PrepareControl(ctx, desired)
	if err != nil {
		return domain.ControlIntent{}, normalizeError(err)
	}
	request, err := NewControlRequest(prepared.Intent)
	if err != nil {
		return domain.ControlIntent{}, err
	}
	if prepared.Intent.CommandInputHash != commandInputHash || prepared.Intent.InputHash != request.InputHash ||
		prepared.Intent.WorkflowRunID != command.WorkflowRunID || prepared.Intent.Action != action ||
		prepared.Intent.ExpectedRunRevision != command.ExpectedRevision {
		return domain.ControlIntent{}, conflict("Idempotency key was already used with different workflow control input")
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Intent, nil
	}
	prepared, err = service.repository.BeginControlAttempt(ctx, prepared.Intent.ID, service.config.Now().UTC())
	if err != nil {
		return domain.ControlIntent{}, normalizeError(err)
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Intent, nil
	}
	observation, controlErr := service.controller.Control(ctx, request)
	finalization := finalizeControl(
		prepared, observation, controlErr, service.config.NewID(), service.config.Now().UTC(),
	)
	if err = service.repository.FinalizeControlAttempt(ctx, finalization); err != nil {
		return domain.ControlIntent{}, normalizeError(err)
	}
	return finalization.Intent, nil
}

func validControlAction(action string) bool {
	switch action {
	case domain.ControlActionCancel, domain.ControlActionPause, domain.ControlActionResume:
		return true
	default:
		return false
	}
}

func NewControlRequest(intent domain.ControlIntent) (domain.ControlRequest, error) {
	request := domain.ControlRequest{
		TemporalWorkflowID: intent.TemporalWorkflowID, ControlID: intent.ControlID,
		WorkflowRunID: intent.WorkflowRunID, Action: intent.Action,
	}
	inputHash, err := platformcommand.InputHash(request)
	if err != nil {
		return domain.ControlRequest{}, err
	}
	request.InputHash = inputHash
	return request, nil
}

func finalizeControl(
	prepared domain.ControlPreparation,
	observation domain.ControlObservation,
	controlErr error,
	receiptID string,
	now time.Time,
) domain.ControlFinalization {
	run, intent := prepared.Run, prepared.Intent
	outcome := observation.Outcome
	var observed *string
	if len(observation.ObservedInputHash) == 64 {
		value := observation.ObservedInputHash
		observed = &value
	}
	cancelNodes := false
	applied := (outcome == domain.ControlOutcomeApplied || outcome == domain.ControlOutcomeAlreadyApplied) &&
		observation.ObservedInputHash == intent.InputHash
	switch {
	case controlErr != nil || outcome == domain.ControlOutcomeUnknown:
		outcome, intent.Status = domain.ControlOutcomeUnknown, "unknown"
		if intent.Action == domain.ControlActionPause {
			rememberPauseSource(&run)
		}
		run.Status, run.ProgressStage = "NEEDS_ATTENTION", intent.Action+"_unknown"
		run.NextAction = stringPointer("reconcile_" + intent.Action)
		run.Error = errorPayload(intent.Action + "_outcome_unknown")
	case intent.Action == domain.ControlActionCancel && outcome == domain.ControlOutcomeRequested &&
		observation.ObservedInputHash == intent.InputHash:
		intent.Status = "pending"
		run.Status, run.ProgressStage = "PAUSED", "cancel_requested"
		run.NextAction, run.Error = stringPointer("reconcile_cancel"), nil
	case applied && intent.Action == domain.ControlActionCancel:
		intent.Status = "completed"
		run.Status, run.ProgressStage = "CANCELLED", "cancelled"
		run.NextAction, run.Error = nil, nil
		run.PausedFromStatus, run.PausedFromProgressStage = nil, nil
		cancelNodes = true
	case applied && intent.Action == domain.ControlActionPause:
		rememberPauseSource(&run)
		if run.PausedFromStatus == nil ||
			(*run.PausedFromStatus != "RUNNING" && *run.PausedFromStatus != "RETRYING") {
			outcome, intent.Status = domain.ControlOutcomeConflict, "conflict"
			run.Status, run.ProgressStage = "NEEDS_ATTENTION", "pause_conflict"
			run.NextAction = stringPointer("inspect_workflow_control")
			run.Error = errorPayload("workflow_control_conflict")
			break
		}
		intent.Status = "completed"
		run.Status, run.ProgressStage = "PAUSED", "paused"
		run.NextAction, run.Error = stringPointer("resume_workflow"), nil
	case applied && intent.Action == domain.ControlActionResume && validPauseSource(run):
		intent.Status = "completed"
		run.Status = *run.PausedFromStatus
		run.ProgressStage = *run.PausedFromProgressStage
		run.NextAction, run.Error = nil, nil
		run.PausedFromStatus, run.PausedFromProgressStage = nil, nil
	default:
		outcome, intent.Status = domain.ControlOutcomeConflict, "conflict"
		if run.Status != "SUCCEEDED" && run.Status != "FAILED" && run.Status != "CANCELLED" {
			run.Status, run.ProgressStage = "NEEDS_ATTENTION", intent.Action+"_conflict"
			run.NextAction, run.Error = stringPointer("inspect_workflow_control"), errorPayload("workflow_control_conflict")
		}
	}
	run.Revision++
	run.UpdatedAt = now
	intent.Revision++
	intent.UpdatedAt = now
	return domain.ControlFinalization{
		Run: run, Intent: intent,
		Receipt: domain.ControlReceipt{
			ID: receiptID, WorkspaceID: intent.WorkspaceID, ControlIntentID: intent.ID,
			WorkflowRunID: intent.WorkflowRunID, AttemptNo: intent.AttemptNo,
			Outcome: outcome, ControlID: intent.ControlID, ExpectedInputHash: intent.InputHash,
			ObservedInputHash: observed, CreatedAt: now,
		},
		ExpectedRunRevision: prepared.Run.Revision, ExpectedIntentRevision: prepared.Intent.Revision,
		CancelNonTerminalNodeRuns: cancelNodes,
	}
}

func rememberPauseSource(run *domain.WorkflowRun) {
	if run.PausedFromStatus != nil || run.PausedFromProgressStage != nil {
		return
	}
	status, stage := run.Status, run.ProgressStage
	run.PausedFromStatus = &status
	run.PausedFromProgressStage = &stage
}

func validPauseSource(run domain.WorkflowRun) bool {
	return run.PausedFromStatus != nil && run.PausedFromProgressStage != nil &&
		(*run.PausedFromStatus == "RUNNING" || *run.PausedFromStatus == "RETRYING")
}
