package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowhttp "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/httpapi"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowHTTPStartsQueriesAndRerunsOnRealPostgresAndTemporal(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_TEMPORAL_ADDRESS to run the real HTTP journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 26, 17, 0, 0, 0, time.UTC)
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, rerunTemporalCatalog(t), now, uuid.NewString); err != nil {
		t.Fatalf("persist HTTP journey catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	draft, err := authoringService.Create(ctx, authoringapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: rerunTemporalGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: "lanverse.rerun-test", CatalogVersion: "1.0.0", IdempotencyKey: "http-authoring-create",
	})
	if err != nil {
		t.Fatalf("create HTTP journey draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "http-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish HTTP journey revision: %v", err)
	}

	taskQueue := "lanverse-http-" + uuid.NewString()
	temporalRuntime, err := temporaladapter.New(temporaladapter.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	if err = temporalRuntime.Ping(ctx); err != nil {
		t.Fatalf("check Temporal runtime health: %v", err)
	}
	workflowStore := workflowgorm.New(database)
	executor := &scriptedNodeExecutor{}
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: executor,
	})
	runtimeWorker, err := temporalRuntime.NewWorker(runtimeService)
	if err != nil {
		t.Fatalf("register HTTP journey worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start HTTP journey worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(
		compiler, workflowStore, temporalRuntime,
		workflowapp.StartConfig{Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		}, NewID: uuid.NewString},
	)
	queryService := workflowapp.NewQueryService(workflowStore)
	controlService := workflowapp.NewControlService(
		workflowStore, temporalRuntime,
		workflowapp.ControlConfig{Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		}, NewID: uuid.NewString},
	)
	authNow := time.Date(2026, time.August, 26, 17, 0, 0, 0, time.UTC)
	issuer := authentication.NewIssuer(
		"workflow-http-secret", "lanverse-api", "lanverse-web", time.Hour,
		func() time.Time { return authNow }, uuid.NewString,
	)
	token, err := issuer.Issue(fixture.userID.String(), 1)
	if err != nil {
		t.Fatalf("issue HTTP journey token: %v", err)
	}
	verifier := authentication.NewVerifier(
		"workflow-http-secret", "lanverse-api", "lanverse-web", func() time.Time { return authNow },
	)
	mux := http.NewServeMux()
	workflowhttp.New(startService, queryService, controlService, verifier).Register(mux)

	startResponse := workflowRequest(t, mux, token, http.MethodPost, "/api/v1/workflow-runs", map[string]any{
		"authoring_revision_id": revision.ID, "idempotency_key": "http-start",
	})
	if startResponse.Code != http.StatusAccepted {
		t.Fatalf("start response=%d %s", startResponse.Code, startResponse.Body.String())
	}
	started := decodeWorkflowResponse(t, startResponse)
	if started.Data.Run.ID == "" || started.Data.Run.Status != "RUNNING" || len(started.Data.Nodes) != 3 {
		t.Fatalf("start projection = %#v", started.Data)
	}
	waitForTemporalWorkflow(t, ctx, temporalAddress, started.Data.Run.TemporalWorkflowID)

	queryResponse := workflowRequest(
		t, mux, token, http.MethodGet, "/api/v1/workflow-runs/"+started.Data.Run.ID, nil,
	)
	queried := decodeWorkflowResponse(t, queryResponse)
	if queryResponse.Code != http.StatusOK || queried.Data.Run.Status != "SUCCEEDED" ||
		!equalNodeStatuses(queried.Data.Nodes, []string{"source:SUCCEEDED", "transform:SUCCEEDED", "export:SUCCEEDED"}) {
		t.Fatalf("completed query response=%d projection=%#v", queryResponse.Code, queried.Data)
	}

	rerunResponse := workflowRequest(
		t, mux, token, http.MethodPost, "/api/v1/workflow-runs/"+started.Data.Run.ID+"/reruns", map[string]any{
			"root_node_id": "transform", "idempotency_key": "http-rerun",
		},
	)
	if rerunResponse.Code != http.StatusAccepted {
		t.Fatalf("rerun response=%d %s", rerunResponse.Code, rerunResponse.Body.String())
	}
	rerun := decodeWorkflowResponse(t, rerunResponse)
	if rerun.Data.Run.SourceWorkflowRunID == nil || *rerun.Data.Run.SourceWorkflowRunID != started.Data.Run.ID ||
		rerun.Data.Run.RerunRootNodeID == nil || *rerun.Data.Run.RerunRootNodeID != "transform" {
		t.Fatalf("rerun identity = %#v", rerun.Data.Run)
	}
	waitForTemporalWorkflow(t, ctx, temporalAddress, rerun.Data.Run.TemporalWorkflowID)

	rerunQuery := workflowRequest(t, mux, token, http.MethodGet, "/api/v1/workflow-runs/"+rerun.Data.Run.ID, nil)
	completedRerun := decodeWorkflowResponse(t, rerunQuery)
	if rerunQuery.Code != http.StatusOK || completedRerun.Data.Run.Status != "SUCCEEDED" ||
		!equalNodeStatuses(completedRerun.Data.Nodes, []string{"source:SKIPPED", "transform:SUCCEEDED", "export:SUCCEEDED"}) {
		t.Fatalf("completed rerun response=%d projection=%#v", rerunQuery.Code, completedRerun.Data)
	}
	commands := executor.Commands()
	executed := make([]string, len(commands))
	for index, command := range commands {
		executed[index] = command.NodeID
	}
	if !slices.Equal(executed, []string{"source", "transform", "export", "transform", "export"}) {
		t.Fatalf("HTTP journey executed nodes %v", executed)
	}

	executor.status = "RETRYING"
	controlStartResponse := workflowRequest(t, mux, token, http.MethodPost, "/api/v1/workflow-runs", map[string]any{
		"authoring_revision_id": revision.ID, "idempotency_key": "http-control-start",
	})
	if controlStartResponse.Code != http.StatusAccepted {
		t.Fatalf("control start response=%d %s", controlStartResponse.Code, controlStartResponse.Body.String())
	}
	controlRun := decodeWorkflowResponse(t, controlStartResponse)
	controlRun = waitForWorkflowHTTPStatus(t, mux, token, controlRun.Data.Run.ID, "RETRYING")

	pauseResponse := workflowRequest(
		t, mux, token, http.MethodPost, "/api/v1/workflow-runs/"+controlRun.Data.Run.ID+"/controls", map[string]any{
			"action": "pause", "expected_revision": controlRun.Data.Run.Revision, "idempotency_key": "http-pause",
		},
	)
	paused := decodeWorkflowResponse(t, pauseResponse)
	if pauseResponse.Code != http.StatusAccepted || paused.Data.Run.Status != "PAUSED" ||
		!strings.Contains(pauseResponse.Body.String(), `"action":"pause"`) {
		t.Fatalf("pause response=%d %s", pauseResponse.Code, pauseResponse.Body.String())
	}

	resumeResponse := workflowRequest(
		t, mux, token, http.MethodPost, "/api/v1/workflow-runs/"+controlRun.Data.Run.ID+"/controls", map[string]any{
			"action": "resume", "expected_revision": paused.Data.Run.Revision, "idempotency_key": "http-resume",
		},
	)
	resumed := decodeWorkflowResponse(t, resumeResponse)
	if resumeResponse.Code != http.StatusAccepted || resumed.Data.Run.Status != "RETRYING" ||
		!strings.Contains(resumeResponse.Body.String(), `"action":"resume"`) {
		t.Fatalf("resume response=%d %s", resumeResponse.Code, resumeResponse.Body.String())
	}

	cancelBody := map[string]any{
		"action": "cancel", "expected_revision": resumed.Data.Run.Revision, "idempotency_key": "http-cancel",
	}
	cancelResponse := workflowRequest(
		t, mux, token, http.MethodPost, "/api/v1/workflow-runs/"+controlRun.Data.Run.ID+"/controls", cancelBody,
	)
	if cancelResponse.Code != http.StatusAccepted || !strings.Contains(cancelResponse.Body.String(), `"action":"cancel"`) {
		t.Fatalf("cancel request response=%d %s", cancelResponse.Code, cancelResponse.Body.String())
	}
	waitForTemporalCancellation(t, ctx, temporalAddress, controlRun.Data.Run.TemporalWorkflowID)
	reconciledCancel := workflowRequest(
		t, mux, token, http.MethodPost, "/api/v1/workflow-runs/"+controlRun.Data.Run.ID+"/controls", cancelBody,
	)
	cancelled := decodeWorkflowResponse(t, reconciledCancel)
	if reconciledCancel.Code != http.StatusAccepted || cancelled.Data.Run.Status != "CANCELLED" {
		t.Fatalf("cancel reconciliation response=%d %s", reconciledCancel.Code, reconciledCancel.Body.String())
	}
}

type workflowHTTPResponse struct {
	Data struct {
		Run struct {
			ID                  string  `json:"id"`
			Status              string  `json:"status"`
			Revision            int     `json:"revision"`
			TemporalWorkflowID  string  `json:"temporal_workflow_id"`
			SourceWorkflowRunID *string `json:"source_workflow_run_id"`
			RerunRootNodeID     *string `json:"rerun_root_node_id"`
		}
		Nodes []struct {
			NodeID string `json:"node_id"`
			Status string `json:"status"`
		}
	}
}

func workflowRequest(
	t *testing.T,
	handler http.Handler,
	token, method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode %s %s request: %v", method, path, err)
		}
		input = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, input)
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeWorkflowResponse(t *testing.T, response *httptest.ResponseRecorder) workflowHTTPResponse {
	t.Helper()
	var decoded workflowHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode workflow response %d %s: %v", response.Code, response.Body.String(), err)
	}
	return decoded
}

func waitForTemporalWorkflow(t *testing.T, ctx context.Context, address, workflowID string) {
	t.Helper()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal history client: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	var result temporaladapter.RunResult
	if err = temporalClient.GetWorkflow(ctx, workflowID, "").Get(ctx, &result); err != nil {
		t.Fatalf("wait for Temporal workflow %s: %v", workflowID, err)
	}
	if result.Status != "SUCCEEDED" {
		t.Fatalf("Temporal workflow %s result = %#v", workflowID, result)
	}
}

func waitForTemporalCancellation(t *testing.T, ctx context.Context, address, workflowID string) {
	t.Helper()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal cancellation client: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	if err = temporalClient.GetWorkflow(ctx, workflowID, "").Get(ctx, nil); !temporal.IsCanceledError(err) {
		t.Fatalf("Temporal workflow %s cancellation err=%v", workflowID, err)
	}
}

func waitForWorkflowHTTPStatus(
	t *testing.T,
	handler http.Handler,
	token, workflowRunID, status string,
) workflowHTTPResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response := workflowRequest(t, handler, token, http.MethodGet, "/api/v1/workflow-runs/"+workflowRunID, nil)
		view := decodeWorkflowResponse(t, response)
		if response.Code != http.StatusOK {
			t.Fatalf("poll workflow response=%d %s", response.Code, response.Body.String())
		}
		if view.Data.Run.Status == status {
			return view
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("workflow %s did not reach status %s", workflowRunID, status)
	return workflowHTTPResponse{}
}

func equalNodeStatuses(nodes []struct {
	NodeID string `json:"node_id"`
	Status string `json:"status"`
}, expected []string) bool {
	actual := make([]string, len(nodes))
	for index, node := range nodes {
		actual[index] = fmt.Sprintf("%s:%s", node.NodeID, node.Status)
	}
	return slices.Equal(actual, expected)
}
