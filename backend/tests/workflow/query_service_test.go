package workflow_test

import (
	"context"
	"testing"
	"time"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowQueryReturnsAuthorizedRunSnapshotWithoutWriting(t *testing.T) {
	now := time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC)
	repository := &workflowQueryRepository{view: workflowapp.RunView{
		Run: workflow.WorkflowRun{
			ID: "run-1", WorkspaceID: "workspace-1", Status: "RUNNING", Revision: 2,
			CreatedAt: now, UpdatedAt: now,
		},
		Nodes: []workflow.NodeRunProjection{{
			ID: "node-run-1", WorkspaceID: "workspace-1", WorkflowRunID: "run-1",
			NodeID: "script", Status: "SUCCEEDED", Revision: 2, CreatedAt: now, UpdatedAt: now,
		}},
	}}
	service := workflowapp.NewQueryService(repository)
	actor := workflowapp.Actor{UserID: "user-1", TokenVersion: 3}

	view, err := service.GetRun(context.Background(), actor, "run-1")
	if err != nil || view.Run.ID != "run-1" || len(view.Nodes) != 1 || repository.calls != 1 {
		t.Fatalf("query view=%#v calls=%d err=%v", view, repository.calls, err)
	}
	if _, err = service.GetRun(context.Background(), workflowapp.Actor{}, "run-1"); err == nil || repository.calls != 1 {
		t.Fatalf("invalid actor reached repository: calls=%d err=%v", repository.calls, err)
	}
}

type workflowQueryRepository struct {
	view  workflowapp.RunView
	calls int
}

func (repo *workflowQueryRepository) GetRun(
	_ context.Context,
	actor workflowapp.Actor,
	runID string,
) (workflowapp.RunView, error) {
	repo.calls++
	if actor.UserID != "user-1" || actor.TokenVersion != 3 || runID != "run-1" {
		type unexpectedQuery struct{}
		return workflowapp.RunView{}, unexpectedQueryError{unexpectedQuery{}}
	}
	return repo.view, nil
}

type unexpectedQueryError struct{ value any }

func (value unexpectedQueryError) Error() string { return "unexpected workflow query" }
