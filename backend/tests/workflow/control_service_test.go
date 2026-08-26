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

func TestCancelControlReusesStableIdentityUntilUnknownIsReconciled(t *testing.T) {
	now := time.Date(2026, time.August, 26, 4, 0, 0, 0, time.UTC)
	repository := newControlRepository(now)
	controller := &scriptedController{outcomes: []workflow.ControlObservation{
		{Outcome: workflow.ControlOutcomeUnknown},
		{Outcome: workflow.ControlOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
	}}
	generated := 0
	service := workflowapp.NewControlService(repository, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			generated++
			return "receipt-" + string(rune('0'+generated))
		},
	})
	actor := workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}
	command := workflowapp.CancelCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 7,
		IdempotencyKey: "cancel-run-1",
	}

	unknown, err := service.Cancel(context.Background(), actor, command)
	if err != nil || unknown.Status != "unknown" || unknown.AttemptNo != 1 || repository.run.Status != "NEEDS_ATTENTION" {
		t.Fatalf("record unknown cancellation: intent=%#v run=%#v err=%v", unknown, repository.run, err)
	}
	completed, err := service.Cancel(context.Background(), actor, command)
	if err != nil || completed.Status != "completed" || completed.AttemptNo != 2 || repository.run.Status != "CANCELLED" {
		t.Fatalf("reconcile cancellation: intent=%#v run=%#v err=%v", completed, repository.run, err)
	}
	requests := controller.Requests()
	if len(requests) != 2 || requests[0].ControlID != requests[1].ControlID ||
		requests[0].InputHash != requests[1].InputHash || requests[0].InputHash == "" || len(repository.receipts) != 2 {
		t.Fatalf("cancel retry identities = requests %#v receipts %#v", requests, repository.receipts)
	}
	if _, err = service.Cancel(context.Background(), actor, workflowapp.CancelCommand{
		WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		ExpectedRevision: command.ExpectedRevision + 1, IdempotencyKey: command.IdempotencyKey,
	}); err == nil {
		t.Fatal("cancel idempotency key accepted drifted command input")
	}
}

func TestPauseResumeControlCreatesARevisionScopedIdentityForEveryCycle(t *testing.T) {
	now := time.Date(2026, time.August, 26, 6, 0, 0, 0, time.UTC)
	repository := newControlCycleRepository(now)
	controller := &scriptedController{outcomes: []workflow.ControlObservation{
		{Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request"},
		{Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request"},
		{Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request"},
	}}
	receiptNo := 0
	service := workflowapp.NewControlService(repository, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			receiptNo++
			return "receipt-" + string(rune('0'+receiptNo))
		},
	})
	actor := workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}

	firstPause, err := service.Pause(context.Background(), actor, workflowapp.PauseCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 7,
		IdempotencyKey: "pause-run-1-cycle-1",
	})
	if err != nil || firstPause.Status != "completed" || repository.run.Status != "PAUSED" ||
		repository.run.Revision != 8 {
		t.Fatalf("first pause: intent=%#v run=%#v err=%v", firstPause, repository.run, err)
	}
	replayed, err := service.Pause(context.Background(), actor, workflowapp.PauseCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 7,
		IdempotencyKey: "pause-run-1-cycle-1",
	})
	if err != nil || replayed.ID != firstPause.ID || len(controller.Requests()) != 1 {
		t.Fatalf("replay first pause: intent=%#v requests=%#v err=%v", replayed, controller.Requests(), err)
	}
	resumed, err := service.Resume(context.Background(), actor, workflowapp.ResumeCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 8,
		IdempotencyKey: "resume-run-1-cycle-1",
	})
	if err != nil || resumed.Status != "completed" || repository.run.Status != "RUNNING" ||
		repository.run.Revision != 9 {
		t.Fatalf("resume first pause: intent=%#v run=%#v err=%v", resumed, repository.run, err)
	}
	secondPause, err := service.Pause(context.Background(), actor, workflowapp.PauseCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 9,
		IdempotencyKey: "pause-run-1-cycle-2",
	})
	if err != nil || secondPause.Status != "completed" || repository.run.Status != "PAUSED" ||
		repository.run.Revision != 10 {
		t.Fatalf("second pause: intent=%#v run=%#v err=%v", secondPause, repository.run, err)
	}
	if firstPause.ID == secondPause.ID || firstPause.ControlID == secondPause.ControlID {
		t.Fatalf("pause cycles reused identity: first=%#v second=%#v", firstPause, secondPause)
	}
	requests := controller.Requests()
	if len(requests) != 3 || requests[0].Action != workflow.ControlActionPause ||
		requests[1].Action != workflow.ControlActionResume || requests[2].Action != workflow.ControlActionPause {
		t.Fatalf("pause/resume request sequence = %#v", requests)
	}
}

func TestPauseResumeControlReconcilesUnknownWithoutLosingThePausedSource(t *testing.T) {
	now := time.Date(2026, time.August, 26, 6, 30, 0, 0, time.UTC)
	repository := newControlCycleRepository(now)
	controller := &scriptedController{outcomes: []workflow.ControlObservation{
		{Outcome: workflow.ControlOutcomeUnknown},
		{Outcome: workflow.ControlOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
		{Outcome: workflow.ControlOutcomeUnknown},
		{Outcome: workflow.ControlOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
	}}
	service := workflowapp.NewControlService(repository, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string { return "receipt-" + now.String() },
	})
	actor := workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}
	pauseCommand := workflowapp.PauseCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: 7,
		IdempotencyKey: "pause-run-1-unknown",
	}
	unknownPause, err := service.Pause(context.Background(), actor, pauseCommand)
	if err != nil || unknownPause.Status != "unknown" || repository.run.Status != "NEEDS_ATTENTION" ||
		repository.run.PausedFromStatus == nil || *repository.run.PausedFromStatus != "RUNNING" {
		t.Fatalf("record unknown pause: intent=%#v run=%#v err=%v", unknownPause, repository.run, err)
	}
	completedPause, err := service.Pause(context.Background(), actor, pauseCommand)
	if err != nil || completedPause.Status != "completed" || repository.run.Status != "PAUSED" ||
		repository.run.PausedFromStatus == nil || *repository.run.PausedFromStatus != "RUNNING" {
		t.Fatalf("reconcile unknown pause: intent=%#v run=%#v err=%v", completedPause, repository.run, err)
	}
	resumeCommand := workflowapp.ResumeCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", ExpectedRevision: repository.run.Revision,
		IdempotencyKey: "resume-run-1-unknown",
	}
	unknownResume, err := service.Resume(context.Background(), actor, resumeCommand)
	if err != nil || unknownResume.Status != "unknown" || repository.run.Status != "NEEDS_ATTENTION" ||
		repository.run.PausedFromStatus == nil || *repository.run.PausedFromStatus != "RUNNING" {
		t.Fatalf("record unknown resume: intent=%#v run=%#v err=%v", unknownResume, repository.run, err)
	}
	completedResume, err := service.Resume(context.Background(), actor, resumeCommand)
	if err != nil || completedResume.Status != "completed" || repository.run.Status != "RUNNING" ||
		repository.run.PausedFromStatus != nil || repository.run.PausedFromProgressStage != nil {
		t.Fatalf("reconcile unknown resume: intent=%#v run=%#v err=%v", completedResume, repository.run, err)
	}
}

type controlCycleRepository struct {
	mu          sync.Mutex
	run         workflow.WorkflowRun
	byKey       map[string]workflow.ControlPreparation
	keyByIntent map[string]string
	receipts    []workflow.ControlReceipt
}

func newControlCycleRepository(now time.Time) *controlCycleRepository {
	return &controlCycleRepository{
		run: workflow.WorkflowRun{
			ID: "run-1", WorkspaceID: "workspace-1", TemporalWorkflowID: "temporal:run-1",
			Status: "RUNNING", ProgressStage: "node:storyboard", Revision: 7,
			CreatedBy: "reviewer-1", CreatedAt: now, UpdatedAt: now,
		},
		byKey: make(map[string]workflow.ControlPreparation), keyByIntent: make(map[string]string),
	}
}

func (repo *controlCycleRepository) PrepareControl(
	_ context.Context,
	_ workflowapp.Actor,
	desired workflow.ControlPreparation,
) (workflow.ControlPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if prepared, exists := repo.byKey[desired.Intent.IdempotencyKey]; exists {
		prepared.Run = repo.run
		return prepared, nil
	}
	if desired.Intent.ExpectedRunRevision != repo.run.Revision {
		return workflow.ControlPreparation{}, errors.New("stale workflow revision")
	}
	switch desired.Intent.Action {
	case workflow.ControlActionPause:
		if repo.run.Status != "RUNNING" && repo.run.Status != "RETRYING" {
			return workflow.ControlPreparation{}, errors.New("workflow is not pausable")
		}
	case workflow.ControlActionResume:
		if repo.run.Status != "PAUSED" {
			return workflow.ControlPreparation{}, errors.New("workflow is not resumable")
		}
	default:
		return workflow.ControlPreparation{}, errors.New("unsupported workflow action")
	}
	desired.Run = repo.run
	desired.Intent.TemporalWorkflowID = repo.run.TemporalWorkflowID
	request, err := workflowapp.NewControlRequest(desired.Intent)
	if err != nil {
		return workflow.ControlPreparation{}, err
	}
	desired.Intent.InputHash = request.InputHash
	repo.byKey[desired.Intent.IdempotencyKey] = desired
	repo.keyByIntent[desired.Intent.ID] = desired.Intent.IdempotencyKey
	return desired, nil
}

func (repo *controlCycleRepository) ResolveRunAccess(
	_ context.Context,
	_ workflowapp.Actor,
	runID string,
	write bool,
) (workflow.WorkflowRun, error) {
	if runID != repo.run.ID || !write {
		return workflow.WorkflowRun{}, workflowapp.ErrNotFound
	}
	return repo.run, nil
}

func (repo *controlCycleRepository) BeginControlAttempt(
	_ context.Context,
	intentID string,
	now time.Time,
) (workflow.ControlPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	key := repo.keyByIntent[intentID]
	prepared := repo.byKey[key]
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		prepared.Run = repo.run
		return prepared, nil
	}
	prepared.Intent.Status = "pending"
	prepared.Intent.AttemptNo++
	prepared.Intent.Revision++
	prepared.Intent.UpdatedAt = now
	prepared.Run = repo.run
	repo.byKey[key] = prepared
	return prepared, nil
}

func (repo *controlCycleRepository) FinalizeControlAttempt(
	_ context.Context,
	finalization workflow.ControlFinalization,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	key := repo.keyByIntent[finalization.Intent.ID]
	repo.run = finalization.Run
	repo.byKey[key] = workflow.ControlPreparation{Run: repo.run, Intent: finalization.Intent}
	repo.receipts = append(repo.receipts, finalization.Receipt)
	return nil
}

type controlRepository struct {
	mu       sync.Mutex
	run      workflow.WorkflowRun
	prepared workflow.ControlPreparation
	receipts []workflow.ControlReceipt
}

func newControlRepository(now time.Time) *controlRepository {
	return &controlRepository{run: workflow.WorkflowRun{
		ID: "run-1", WorkspaceID: "workspace-1", TemporalWorkflowID: "temporal:run-1",
		Status: "RUNNING", ProgressStage: "node:storyboard", Revision: 7,
		CreatedBy: "reviewer-1", CreatedAt: now, UpdatedAt: now,
	}}
}

func (repo *controlRepository) PrepareControl(
	_ context.Context,
	_ workflowapp.Actor,
	desired workflow.ControlPreparation,
) (workflow.ControlPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.prepared.Intent.ID == "" {
		desired.Run = repo.run
		desired.Intent.TemporalWorkflowID = repo.run.TemporalWorkflowID
		request, err := workflowapp.NewControlRequest(desired.Intent)
		if err != nil {
			return workflow.ControlPreparation{}, err
		}
		desired.Intent.InputHash = request.InputHash
		repo.prepared = desired
	}
	repo.prepared.Run = repo.run
	return repo.prepared, nil
}

func (repo *controlRepository) ResolveRunAccess(
	_ context.Context,
	_ workflowapp.Actor,
	runID string,
	write bool,
) (workflow.WorkflowRun, error) {
	if runID != repo.run.ID || !write {
		return workflow.WorkflowRun{}, workflowapp.ErrNotFound
	}
	return repo.run, nil
}

func (repo *controlRepository) BeginControlAttempt(
	_ context.Context,
	intentID string,
	now time.Time,
) (workflow.ControlPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.prepared.Intent.ID != intentID || repo.prepared.Intent.Status == "completed" ||
		repo.prepared.Intent.Status == "conflict" {
		return repo.prepared, nil
	}
	repo.prepared.Intent.Status = "pending"
	repo.prepared.Intent.AttemptNo++
	repo.prepared.Intent.Revision++
	repo.prepared.Intent.UpdatedAt = now
	repo.prepared.Run = repo.run
	return repo.prepared, nil
}

func (repo *controlRepository) FinalizeControlAttempt(
	_ context.Context,
	finalization workflow.ControlFinalization,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.prepared.Intent = finalization.Intent
	repo.run = finalization.Run
	repo.receipts = append(repo.receipts, finalization.Receipt)
	return nil
}

type scriptedController struct {
	mu       sync.Mutex
	outcomes []workflow.ControlObservation
	requests []workflow.ControlRequest
}

func (controller *scriptedController) Control(
	_ context.Context,
	request workflow.ControlRequest,
) (workflow.ControlObservation, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.requests = append(controller.requests, request)
	observation := controller.outcomes[0]
	controller.outcomes = controller.outcomes[1:]
	if observation.ObservedInputHash == "match_request" {
		observation.ObservedInputHash = request.InputHash
	}
	return observation, nil
}

func (controller *scriptedController) Requests() []workflow.ControlRequest {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return append([]workflow.ControlRequest(nil), controller.requests...)
}
