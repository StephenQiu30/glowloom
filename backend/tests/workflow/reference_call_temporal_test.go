package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
)

type referenceCallTemporalActivities struct {
	*workflowapp.RuntimeService
	lost          atomic.Bool
	activityCalls atomic.Int64
	dropResponse  bool
}

type referenceCallNodeReplyLoss struct {
	workflowapp.NodeExecutor
	lost atomic.Bool
}

func (e *referenceCallNodeReplyLoss) Execute(ctx context.Context, command flow.NodeExecutorCommand) (flow.NodeExecutorResult, error) {
	result, err := e.NodeExecutor.Execute(ctx, command)
	if err == nil && result.Status == "SUCCEEDED" && e.lost.CompareAndSwap(false, true) {
		return flow.NodeExecutorResult{}, errors.New("injected reply loss after business receipt commit")
	}
	return result, err
}

func (a *referenceCallTemporalActivities) ExecuteNode(ctx context.Context, command flow.NodeActivityCommand) (flow.NodeActivityResult, error) {
	a.activityCalls.Add(1)
	result, err := a.RuntimeService.ExecuteNode(ctx, command)
	if err == nil && result.Status == "SUCCEEDED" && a.dropResponse && a.lost.CompareAndSwap(false, true) {
		// Simulate a lost Activity reply AFTER both the business receipt and node
		// projection commit. The next Temporal attempt must replay persisted output.
		return flow.NodeActivityResult{}, errors.New("injected lost committed activity reply")
	}
	return result, err
}

func executeReferenceCallThroughTemporal(t *testing.T, parent context.Context, database *generationtestgorm.Database, address string, actor genapp.Actor, command genapp.ClaimReferenceCallCommand, execution *genapp.ReferenceCallExecutionService, recovery *genapp.ReferenceCallDispatchService, expectAttention bool, beforeStart func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	var document model.ScriptDocument
	if err := database.Where("project_id = ?", command.ProjectID).First(&document).Error; err != nil {
		t.Fatal(err)
	}
	var revision model.DocumentRevision
	if err := database.Where("document_id = ?", document.ID).Order("version_no").First(&revision).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, time.Now().UTC(), uuid.NewString); err != nil {
		t.Fatal(err)
	}
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{Now: time.Now, NewID: uuid.NewString})
	config, _ := json.Marshal(map[string]any{"execution_ref": command.ExecutionRef, "call_key": command.CallKey})
	key := "reference-call-" + command.CallKey
	draft, err := authoringService.Create(ctx, authoringapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, authoringapp.CreateCommand{ProjectID: command.ProjectID, AuthoringMode: "GUIDED", Graph: authoring.Graph{Nodes: []authoring.Node{{ID: "reference-image", DefinitionKey: "generation.reference_image_call", DefinitionVersion: "1.0.0", Config: config}}}, Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{Kind: "script_revision", ID: revision.ID.String(), Version: strconv.Itoa(revision.VersionNo), Hash: revision.NormalizedHash}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	published, err := authoringService.Publish(ctx, authoringapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, authoringapp.PublishCommand{DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: key + "-publish"})
	if err != nil {
		t.Fatal(err)
	}
	callExecutor, err := workflowgeneration.NewReferenceCallNodeExecutor(execution, recovery)
	if err != nil {
		t.Fatal(err)
	}
	var nodeExecutor workflowapp.NodeExecutor = callExecutor
	replyLoss := &referenceCallNodeReplyLoss{NodeExecutor: callExecutor}
	if !expectAttention {
		nodeExecutor = replyLoss
	}
	router, err := workflowexecution.NewNodeExecutor(workflowproduction.NewNodeExecutor(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), workflowgeneration.NewNodeExecutor(nil, nil, nil, nil, nil, nil), nodeExecutor)
	if err != nil {
		t.Fatal(err)
	}
	workflowStore := workflowgorm.New(database)
	activities := &referenceCallTemporalActivities{RuntimeService: workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{Now: time.Now, NewID: uuid.NewString, Executor: router}), dropResponse: !expectAttention}
	runtime, err := temporaladapter.New(temporaladapter.Config{Address: address, Namespace: "default", TaskQueue: "lanverse-reference-call-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtimeWorker, err := runtime.NewWorker(activities)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatal(err)
	}
	defer runtimeWorker.Stop()
	compiler := workflowapp.NewService(workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: time.Now, NewID: uuid.NewString})
	starter := workflowapp.NewStartService(compiler, workflowStore, runtime, workflowapp.StartConfig{Now: time.Now, NewID: uuid.NewString})
	if beforeStart != nil {
		beforeStart()
	}
	run, err := starter.Start(ctx, workflowapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, workflowapp.StartCommand{AuthoringRevisionID: published.ID, IdempotencyKey: key + "-start"})
	if err != nil {
		t.Fatal(err)
	}
	historyClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	defer historyClient.Close()
	finished := false
	defer func() {
		if !finished {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			_ = historyClient.CancelWorkflow(cleanupCtx, run.TemporalWorkflowID, "")
		}
	}()
	var result temporaladapter.RunResult
	if err = historyClient.GetWorkflow(ctx, run.TemporalWorkflowID, "").Get(ctx, &result); err != nil {
		t.Fatal(err)
	}
	finished = true
	wantStatus := "SUCCEEDED"
	if expectAttention {
		wantStatus = flow.NodeActivityNeedsAttention
	}
	if result.Status != wantStatus || result.WorkflowRunID != run.ID {
		t.Fatalf("Temporal result: %+v", result)
	}
	var storedRun model.WorkflowRun
	var node model.NodeRunProjection
	if err = database.First(&storedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "reference-image").First(&node).Error; err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != wantStatus || node.CacheKey != nil {
		t.Fatalf("wrong persisted run/cache: %s %+v", storedRun.Status, node.CacheKey)
	}
	if expectAttention {
		if node.Status != "FAILED" || node.OutputHash != nil || storedRun.NextAction == nil || *storedRun.NextAction != flow.ManualProviderReconciliationNextAction {
			t.Fatal("unknown call was published as success")
		}
	} else {
		if activities.activityCalls.Load() != 3 || !activities.lost.Load() || !replyLoss.lost.Load() || node.Status != "SUCCEEDED" || node.Attempt != 2 {
			t.Fatalf("committed replay calls=%d node attempts=%d", activities.activityCalls.Load(), node.Attempt)
		}
		state, err := execution.Execute(ctx, actor, command)
		if err != nil || state.Receipt == nil {
			t.Fatalf("read receipt: %v", err)
		}
		output, _, _, err := flow.ParseNodeOutput(json.RawMessage(node.Output))
		if err != nil || len(output.Bindings) != 1 || output.Bindings[0].ReferenceID != state.Receipt.SubmissionToken || output.Bindings[0].ContentHash != state.Receipt.ContentHash {
			t.Fatal("node output differs from committed receipt")
		}
	}
	history, _, _, _ := loadRecoveredWorkflowHistory(t, ctx, historyClient, run.TemporalWorkflowID)
	timers := 0
	for _, event := range history.Events {
		var payloadSets []*commonpb.Payloads
		if attributes := event.GetWorkflowExecutionStartedEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Input)
		}
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Input)
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Result)
		}
		for _, set := range payloadSets {
			for _, payload := range set.GetPayloads() {
				for _, forbidden := range []string{"sk-reference-execution-fixture", "staging/reference/", "api_key", "ciphertext", "b64_json"} {
					if strings.Contains(string(payload.Data), forbidden) {
						t.Fatalf("Temporal history leaked %s", forbidden)
					}
				}
			}
		}
		if event.GetTimerStartedEventAttributes() != nil {
			timers++
		}
	}
	if expectAttention && timers == 0 {
		t.Fatal("expiry recovery did not use a durable Temporal timer")
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(temporaladapter.EpisodeProductionWorkflow, temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName})
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("Reference call Replay: %v", err)
	}
}

var _ temporaladapter.RuntimeActivities = (*referenceCallTemporalActivities)(nil)
