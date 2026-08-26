package workflow_test

import (
	"context"
	"testing"
	"time"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowControlDerivesWorkspaceFromAuthorizedRun(t *testing.T) {
	now := time.Date(2026, time.August, 26, 15, 0, 0, 0, time.UTC)
	repository := &authorizingControlRepository{run: workflow.WorkflowRun{
		ID: "run-1", WorkspaceID: "workspace-1", TemporalWorkflowID: "temporal-run-1",
		Status: "RUNNING", ProgressStage: "node:script", Revision: 4,
	}}
	controller := &scriptedController{outcomes: []workflow.ControlObservation{{
		Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request",
	}}}
	service := workflowapp.NewControlService(repository, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string { return "receipt-1" },
	})
	actor := workflowapp.Actor{UserID: "editor-1", TokenVersion: 7}

	intent, err := service.Pause(context.Background(), actor, workflowapp.PauseCommand{
		WorkflowRunID: "run-1", ExpectedRevision: 4, IdempotencyKey: "pause-run-1",
	})
	if err != nil || intent.WorkspaceID != "workspace-1" || repository.authorizeCalls != 1 ||
		repository.prepareCalls != 1 || repository.observedActor != actor {
		t.Fatalf("control intent=%#v repository=%#v err=%v", intent, repository, err)
	}
}

type authorizingControlRepository struct {
	run                          workflow.WorkflowRun
	intent                       workflow.ControlIntent
	authorizeCalls, prepareCalls int
	observedActor                workflowapp.Actor
}

func (repo *authorizingControlRepository) ResolveRunAccess(
	_ context.Context,
	actor workflowapp.Actor,
	runID string,
	write bool,
) (workflow.WorkflowRun, error) {
	repo.authorizeCalls++
	repo.observedActor = actor
	if runID != repo.run.ID || !write {
		return workflow.WorkflowRun{}, workflowapp.ErrNotFound
	}
	return repo.run, nil
}

func (repo *authorizingControlRepository) PrepareControl(
	_ context.Context,
	actor workflowapp.Actor,
	desired workflow.ControlPreparation,
) (workflow.ControlPreparation, error) {
	repo.prepareCalls++
	repo.observedActor = actor
	desired.Run = repo.run
	desired.Intent.TemporalWorkflowID = repo.run.TemporalWorkflowID
	request, err := workflowapp.NewControlRequest(desired.Intent)
	if err != nil {
		return workflow.ControlPreparation{}, err
	}
	desired.Intent.InputHash = request.InputHash
	repo.intent = desired.Intent
	return desired, nil
}

func (repo *authorizingControlRepository) BeginControlAttempt(
	_ context.Context,
	_ string,
	_ time.Time,
) (workflow.ControlPreparation, error) {
	repo.intent.AttemptNo++
	repo.intent.Revision++
	return workflow.ControlPreparation{Run: repo.run, Intent: repo.intent}, nil
}

func (repo *authorizingControlRepository) FinalizeControlAttempt(
	_ context.Context,
	finalization workflow.ControlFinalization,
) error {
	repo.intent = finalization.Intent
	repo.run = finalization.Run
	return nil
}
