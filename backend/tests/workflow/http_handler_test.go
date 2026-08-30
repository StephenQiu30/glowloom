package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	workflowhttp "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowHandlerStartsAndReturnsAuthorizedRunProjection(t *testing.T) {
	const revisionID = "00000000-0000-0000-0000-000000000111"
	now := time.Date(2026, time.August, 26, 16, 0, 0, 0, time.UTC)
	mutations := &workflowMutationStub{run: domain.WorkflowRun{ID: "run-1", WorkspaceID: "workspace-1"}}
	queries := &workflowQueryStub{view: application.RunView{
		Run: domain.WorkflowRun{
			ID: "run-1", WorkspaceID: "workspace-1", ProjectID: "project-1", Status: "RUNNING",
			ProgressStage: "running", Error: json.RawMessage(`{"code":"provider_wait"}`),
			Revision: 2, CreatedAt: now, UpdatedAt: now,
		},
		Nodes: []domain.NodeRunProjection{{
			ID: "node-run-1", WorkspaceID: "workspace-1", WorkflowRunID: "run-1", NodeID: "script",
			DefinitionKey: "production.script", DefinitionVersion: "1.0.0", Executor: "workflow.input.script_revision",
			RiskLevel: "low", Status: "QUEUED", Revision: 1, CreatedAt: now, UpdatedAt: now,
		}},
	}}
	controls := &workflowControlStub{}
	handler := workflowhttp.New(mutations, queries, controls, workflowAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/workflow-runs", strings.NewReader(
		`{"authoring_revision_id":"`+revisionID+`","idempotency_key":"start-1"}`,
	))
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"id":"run-1"`) ||
		!strings.Contains(response.Body.String(), `"node_id":"script"`) ||
		!strings.Contains(response.Body.String(), `"error":{"code":"provider_wait"}`) {
		t.Fatalf("start response = %d %s", response.Code, response.Body.String())
	}
	if mutations.start.AuthoringRevisionID != revisionID || mutations.start.IdempotencyKey != "start-1" ||
		queries.runID != "run-1" || mutations.actor.UserID != "user-1" || mutations.actor.TokenVersion != 3 {
		t.Fatalf("start command=%#v actor=%#v query=%q", mutations.start, mutations.actor, queries.runID)
	}
}

func TestWorkflowHandlerControlsAndRerunsWithoutTrustingWorkspaceInput(t *testing.T) {
	mutations := &workflowMutationStub{run: domain.WorkflowRun{ID: "rerun-1"}}
	queries := &workflowQueryStub{view: application.RunView{Run: domain.WorkflowRun{
		ID: "run-1", WorkspaceID: "workspace-1", Status: "PAUSED", Revision: 8,
	}}}
	controls := &workflowControlStub{intent: domain.ControlIntent{ID: "control-1", Action: domain.ControlActionPause, Status: "completed"}}
	handler := workflowhttp.New(mutations, queries, controls, workflowAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)

	controlResponse := httptest.NewRecorder()
	mux.ServeHTTP(controlResponse, httptest.NewRequest(
		http.MethodPost, "/api/workflow-runs/run-1/controls",
		strings.NewReader(`{"action":"pause","expected_revision":7,"idempotency_key":"pause-1"}`),
	))
	if controlResponse.Code != http.StatusAccepted || controls.pause.WorkflowRunID != "run-1" ||
		controls.pause.WorkspaceID != "" || controls.pause.ExpectedRevision != 7 || queries.runID != "run-1" ||
		!strings.Contains(controlResponse.Body.String(), `"action":"pause"`) {
		t.Fatalf("control response=%d %s command=%#v query=%q", controlResponse.Code, controlResponse.Body.String(), controls.pause, queries.runID)
	}

	rerunResponse := httptest.NewRecorder()
	mux.ServeHTTP(rerunResponse, httptest.NewRequest(
		http.MethodPost, "/api/workflow-runs/run-1/reruns",
		strings.NewReader(`{"root_node_id":"storyboard","idempotency_key":"rerun-1"}`),
	))
	if rerunResponse.Code != http.StatusAccepted || mutations.rerun.SourceWorkflowRunID != "run-1" ||
		mutations.rerun.RootNodeID != "storyboard" || queries.runID != "rerun-1" {
		t.Fatalf("rerun response=%d %s command=%#v query=%q", rerunResponse.Code, rerunResponse.Body.String(), mutations.rerun, queries.runID)
	}
}

func TestWorkflowHandlerRejectsInvalidControlAndAuthentication(t *testing.T) {
	mutations := &workflowMutationStub{}
	queries := &workflowQueryStub{}
	controls := &workflowControlStub{}
	mux := http.NewServeMux()
	workflowhttp.New(mutations, queries, controls, workflowAuthenticator{}).Register(mux)
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(
		http.MethodPost, "/api/workflow-runs/run-1/controls",
		strings.NewReader(`{"action":"restart","expected_revision":1,"idempotency_key":"control-1"}`),
	))
	if invalid.Code != http.StatusUnprocessableEntity || controls.calls != 0 {
		t.Fatalf("invalid control response=%d %s calls=%d", invalid.Code, invalid.Body.String(), controls.calls)
	}

	unauthorized := http.NewServeMux()
	workflowhttp.New(mutations, queries, controls, workflowAuthenticator{err: errors.New("invalid token")}).Register(unauthorized)
	response := httptest.NewRecorder()
	unauthorized.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/workflow-runs/run-1", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("unauthorized response=%d %s", response.Code, response.Body.String())
	}
}

type workflowAuthenticator struct{ err error }

func (authenticator workflowAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "user-1", TokenVersion: 3}, authenticator.err
}

type workflowMutationStub struct {
	run   domain.WorkflowRun
	actor application.Actor
	start application.StartCommand
	rerun application.RerunCommand
}

func (stub *workflowMutationStub) Start(
	_ context.Context,
	actor application.Actor,
	command application.StartCommand,
) (domain.WorkflowRun, error) {
	stub.actor, stub.start = actor, command
	return stub.run, nil
}

func (stub *workflowMutationStub) Rerun(
	_ context.Context,
	actor application.Actor,
	command application.RerunCommand,
) (domain.WorkflowRun, error) {
	stub.actor, stub.rerun = actor, command
	return stub.run, nil
}

type workflowQueryStub struct {
	view  application.RunView
	runID string
}

func (stub *workflowQueryStub) GetRun(
	_ context.Context,
	_ application.Actor,
	runID string,
) (application.RunView, error) {
	stub.runID = runID
	view := stub.view
	view.Run.ID = runID
	return view, nil
}

type workflowControlStub struct {
	intent domain.ControlIntent
	pause  application.PauseCommand
	calls  int
}

func (stub *workflowControlStub) Pause(
	_ context.Context,
	_ application.Actor,
	command application.PauseCommand,
) (domain.ControlIntent, error) {
	stub.pause = command
	stub.calls++
	return stub.intent, nil
}

func (stub *workflowControlStub) Resume(
	context.Context,
	application.Actor,
	application.ResumeCommand,
) (domain.ControlIntent, error) {
	stub.calls++
	return stub.intent, nil
}

func (stub *workflowControlStub) Cancel(
	context.Context,
	application.Actor,
	application.CancelCommand,
) (domain.ControlIntent, error) {
	stub.calls++
	return stub.intent, nil
}
