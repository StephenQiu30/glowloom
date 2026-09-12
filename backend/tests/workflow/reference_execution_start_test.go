package workflow_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	adapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"github.com/google/uuid"
)

type referenceStartOwners struct {
	progress gen.ReferenceJobProgress
	failure  string
	stages   []string
	draft    authoringapp.CreateCommand
	publish  authoringapp.PublishCommand
	start    app.StartCommand
}

func (o *referenceStartOwners) Get(context.Context, genapp.Actor, string, string) (gen.ReferenceJobProgress, error) {
	o.stages = append(o.stages, "read")
	if o.failure == "read" {
		return gen.ReferenceJobProgress{}, errors.New("read failure")
	}
	return o.progress, nil
}
func (o *referenceStartOwners) Create(_ context.Context, _ authoringapp.Actor, command authoringapp.CreateCommand) (authoring.Draft, error) {
	o.stages = append(o.stages, "draft")
	o.draft = command
	if o.failure == "draft" {
		return authoring.Draft{}, errors.New("draft failure")
	}
	return authoring.Draft{ID: "889eb5de-4216-4820-b86f-0c498ac6a11e", Revision: 1}, nil
}
func (o *referenceStartOwners) Publish(_ context.Context, _ authoringapp.Actor, command authoringapp.PublishCommand) (authoring.Revision, error) {
	o.stages = append(o.stages, "publish")
	o.publish = command
	if o.failure == "publish" {
		return authoring.Revision{}, errors.New("publish failure")
	}
	return authoring.Revision{ID: "9ba3c5c5-dce0-4d4d-9952-13e90a22cc1a"}, nil
}
func (o *referenceStartOwners) Start(_ context.Context, _ app.Actor, command app.StartCommand) (flow.WorkflowRun, error) {
	o.stages = append(o.stages, "start")
	o.start = command
	if o.failure == "start" {
		return flow.WorkflowRun{ID: uuid.NewString()}, errors.New("start failure")
	}
	return flow.WorkflowRun{ID: "08ad91b3-11f7-46d0-9c52-3a91490b1fe8", Status: "STARTING"}, nil
}

func TestReferenceExecutionStartUsesExistingOwnerStages(t *testing.T) {
	actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	for _, fault := range []string{"", "read", "draft", "publish", "start", "hash", "key", "actor"} {
		t.Run(fault, func(t *testing.T) {
			owners := &referenceStartOwners{progress: referenceExecutionProgressFixture(t), failure: fault}
			service, err := app.NewReferenceExecutionStartService(owners, owners, owners)
			if err != nil {
				t.Fatal(err)
			}
			command := app.StartReferenceExecutionCommand{ProjectID: uuid.NewString(), ExecutionRef: owners.progress.ExecutionRef, IdempotencyKey: "full-reference"}
			caller := actor
			switch fault {
			case "hash":
				command.ExecutionRef.ContentHash = strings.Repeat("f", 64)
			case "key":
				command.IdempotencyKey = " key "
			case "actor":
				caller.TokenVersion = 0
			}
			run, err := service.Start(context.Background(), caller, command)
			if fault != "" {
				if err == nil || !reflect.DeepEqual(run, flow.WorkflowRun{}) {
					t.Fatal("failed owner stage returned a run")
				}
				return
			}
			if err != nil || run.ID == "" || !reflect.DeepEqual(owners.stages, []string{"read", "draft", "publish", "start"}) {
				t.Fatalf("owner order: %v %v", owners.stages, err)
			}
			if len(owners.draft.FrozenInputs) != 1 || owners.draft.FrozenInputs[0].Kind != "reference_execution" || owners.draft.FrozenInputs[0].Hash != command.ExecutionRef.ContentHash {
				t.Fatal("fake script placeholder")
			}
			draft, publish, start := owners.draft, owners.publish, owners.start
			if _, err := service.Start(context.Background(), actor, command); err != nil || !reflect.DeepEqual(draft, owners.draft) || publish != owners.publish || start != owners.start {
				t.Fatal("restart changed owner command identity")
			}
		})
	}
}

type referenceHTTPStarter struct {
	calls   int
	command app.StartReferenceExecutionCommand
	err     error
}

func (s *referenceHTTPStarter) Start(_ context.Context, _ app.Actor, command app.StartReferenceExecutionCommand) (flow.WorkflowRun, error) {
	s.calls++
	s.command = command
	return flow.WorkflowRun{ID: uuid.NewString(), Status: "STARTING"}, s.err
}

type referenceHTTPAuth struct{ err error }

func (a referenceHTTPAuth) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "446a761e-2f8d-42ac-83cc-c2f1ee9ef0da", TokenVersion: 1}, a.err
}

func TestReferenceExecutionStartHTTPRejectsUnsafeInputs(t *testing.T) {
	project, execution := uuid.NewString(), uuid.NewString()
	body := `{"execution_hash":"` + strings.Repeat("a", 64) + `","idempotency_key":"full-reference"}`
	for _, test := range []struct {
		name, body, suffix string
		authErr, err       error
		status, calls      int
	}{
		{name: "start", body: body, status: 202, calls: 1},
		{name: "unknown", body: strings.TrimSuffix(body, "}") + `,"should_dispatch":true}`, status: 422},
		{name: "duplicate", body: strings.TrimSuffix(body, "}") + `,"idempotency_key":"another"}`, status: 422},
		{name: "query", body: body, suffix: "?retry=true", status: 422},
		{name: "auth", body: body, authErr: errors.New("private auth"), status: 401},
		{name: "private_error", body: body, err: errors.New("private credential"), status: 500, calls: 1},
		{name: "forbidden", body: body, err: &genapp.Error{Code: "forbidden", Message: "Forbidden", Status: 403}, status: 403, calls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			starter := &referenceHTTPStarter{err: test.err}
			mux := http.NewServeMux()
			adapter.NewReferenceExecutionStartHandler(starter, referenceHTTPAuth{test.authErr}).Register(mux)
			request := httptest.NewRequest(http.MethodPost, "/api/projects/"+project+"/reference-executions/"+execution+"/workflow-runs"+test.suffix, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.status || starter.calls != test.calls || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, starter.calls, response.Body)
			}
			if starter.calls > 0 && (starter.command.ProjectID != project || starter.command.ExecutionRef.ID != execution || starter.command.ExecutionRef.Revision != 1) {
				t.Fatal("path identity changed")
			}
		})
	}
}
