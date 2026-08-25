package workflow_test

import (
	"context"
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
