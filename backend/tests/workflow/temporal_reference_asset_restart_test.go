package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	referenceAssetWorkerHelperFlag      = "LANVERSE_REFERENCE_ASSET_TEMPORAL_WORKER_HELPER"
	referenceAssetWorkerHelperAddress   = "LANVERSE_REFERENCE_ASSET_TEMPORAL_WORKER_ADDRESS"
	referenceAssetWorkerHelperTaskQueue = "LANVERSE_REFERENCE_ASSET_TEMPORAL_WORKER_TASK_QUEUE"
	referenceAssetWorkerHelperMode      = "LANVERSE_REFERENCE_ASSET_TEMPORAL_WORKER_MODE"

	referenceAssetWorkerModeRetrying       = "retrying"
	referenceAssetWorkerModeNeedsAttention = "needs_attention"
)

func TestTemporalReferenceAssetPollingRecoversAcrossWorkerProcesses(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_TEMPORAL_ADDRESS to run the real reference asset recovery journey")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 30, 15, 0, 0, 0, time.UTC)
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatalf("build system catalog: %v", err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph:  referenceAssetRecoveryGraph(t, uuid.NewString(), uuid.NewString()),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "reference-asset-temporal-recovery-create",
	})
	if err != nil {
		t.Fatalf("create reference asset authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "reference-asset-temporal-recovery-publish",
	})
	if err != nil {
		t.Fatalf("publish reference asset authoring revision: %v", err)
	}

	taskQueue := "lanverse-reference-asset-recovery-" + uuid.NewString()
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
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "reference-asset-temporal-recovery-start",
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start reference asset Workflow: run=%#v err=%v", run, err)
	}

	var node model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "reference-assets").First(&node).Error; err != nil {
		t.Fatalf("load reference asset node projection: %v", err)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddress, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal history client: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	firstWorker, firstOutput := startReferenceAssetTemporalWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, referenceAssetWorkerModeRetrying,
	)
	firstPollActivityID := "execute-node:" + node.ID.String()
	waitForCompletedActivity(t, ctx, temporalClient, run.TemporalWorkflowID, firstPollActivityID)
	waitForReferenceAssetPollTimer(t, ctx, temporalClient, run.TemporalWorkflowID, firstPollActivityID)
	if err = database.First(&node, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("reload retrying reference asset node: %v", err)
	}
	var retryingRun model.WorkflowRun
	if err = database.First(&retryingRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload retrying Workflow run: %v", err)
	}
	if node.Status != "RETRYING" || node.Attempt != 1 || node.ActiveClaimToken != nil ||
		retryingRun.Status != "RETRYING" {
		t.Fatalf("first Worker did not persist one retrying poll: run=%#v node=%#v", retryingRun, node)
	}
	stopWorkflowWorkerProcess(t, firstWorker, firstOutput)

	secondWorker, secondOutput := startReferenceAssetTemporalWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, referenceAssetWorkerModeNeedsAttention,
	)
	var result temporaladapter.RunResult
	if err = temporalClient.GetWorkflow(ctx, run.TemporalWorkflowID, "").Get(ctx, &result); err != nil {
		t.Fatalf("wait for recovered reference asset Workflow: %v", err)
	}
	if result.WorkflowRunID != run.ID || result.Status != workflowdomain.NodeActivityNeedsAttention {
		t.Fatalf("recovered reference asset Workflow result = %#v", result)
	}
	stopWorkflowWorkerProcess(t, secondWorker, secondOutput)

	var finalRun model.WorkflowRun
	if err = database.First(&finalRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load recovered Workflow projection: %v", err)
	}
	if err = database.First(&node, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load recovered reference asset node projection: %v", err)
	}
	var failure map[string]string
	if err = json.Unmarshal(finalRun.Error, &failure); err != nil {
		t.Fatalf("decode recovered Workflow attention error: %v", err)
	}
	if finalRun.Status != "NEEDS_ATTENTION" || finalRun.NextAction == nil ||
		*finalRun.NextAction != workflowdomain.ManualProviderReconciliationNextAction ||
		failure["code"] != workflowdomain.ProviderOutcomeUnknownErrorCode ||
		node.Status != "FAILED" || node.Attempt != 2 || node.ActiveClaimToken != nil || node.OutputHash != nil {
		t.Fatalf("recovered attention projections drifted: run=%#v node=%#v failure=%#v", finalRun, node, failure)
	}

	history, humanGateSignals, activityStarts, activityCompletions := loadRecoveredWorkflowHistory(
		t, ctx, temporalClient, run.TemporalWorkflowID,
	)
	if humanGateSignals != 0 {
		t.Fatalf("reference asset recovery received %d Human Gate signals, want 0", humanGateSignals)
	}
	wantActivities := []string{
		"load-execution-plan",
		firstPollActivityID,
		firstPollActivityID + ":poll:1",
	}
	if len(activityStarts) != len(wantActivities) || len(activityCompletions) != len(wantActivities) {
		t.Fatalf("reference asset activity sets: starts=%#v completions=%#v", activityStarts, activityCompletions)
	}
	for _, activityID := range wantActivities {
		if activityStarts[activityID] != 1 || activityCompletions[activityID] != 1 {
			t.Fatalf(
				"reference asset activity %q counts: starts=%d completions=%d, want 1/1",
				activityID, activityStarts[activityID], activityCompletions[activityID],
			)
		}
	}
	if activityStarts["complete-run"] != 0 || activityCompletions["complete-run"] != 0 ||
		activityStarts["fail-run:"+node.ID.String()] != 0 || activityCompletions["fail-run:"+node.ID.String()] != 0 {
		t.Fatalf("NEEDS_ATTENTION called a terminal projector: starts=%#v completions=%#v", activityStarts, activityCompletions)
	}
	timerStarts, timerFires := 0, 0
	for _, event := range history.Events {
		if event.GetTimerStartedEventAttributes() != nil {
			timerStarts++
		}
		if event.GetTimerFiredEventAttributes() != nil {
			timerFires++
		}
	}
	if timerStarts != 1 || timerFires != 1 {
		t.Fatalf("reference asset poll timers: started=%d fired=%d, want 1/1", timerStarts, timerFires)
	}

	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay recovered reference asset Workflow history: %v", err)
	}
}

func TestReferenceAssetTemporalWorkerProcessHelper(t *testing.T) {
	if os.Getenv(referenceAssetWorkerHelperFlag) != "1" {
		t.Skip("subprocess helper")
	}
	database, err := platformdatabase.Open(context.Background(), os.Getenv("LANVERSE_TEST_DATABASE_URL"), io.Discard)
	if err != nil {
		t.Fatalf("open helper database: %v", err)
	}
	defer func() { _ = platformdatabase.Close(database) }()
	temporalRuntime, err := temporaladapter.New(temporaladapter.Config{
		Address: os.Getenv(referenceAssetWorkerHelperAddress), Namespace: "default",
		TaskQueue: os.Getenv(referenceAssetWorkerHelperTaskQueue),
	})
	if err != nil {
		t.Fatalf("connect helper Temporal runtime: %v", err)
	}
	defer temporalRuntime.Close()
	runtimeService := workflowapp.NewRuntimeService(workflowgorm.New(database), workflowapp.RuntimeConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Executor: &referenceAssetPollingExecutor{mode: os.Getenv(referenceAssetWorkerHelperMode)},
	})
	runtimeWorker, err := temporalRuntime.NewWorker(runtimeService)
	if err != nil {
		t.Fatalf("create helper Temporal worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start helper Temporal worker: %v", err)
	}
	defer runtimeWorker.Stop()
	select {}
}

type referenceAssetPollingExecutor struct {
	mode string
}

func (executor *referenceAssetPollingExecutor) Execute(
	_ context.Context,
	command workflowdomain.NodeExecutorCommand,
) (workflowdomain.NodeExecutorResult, error) {
	if command.NodeID != "reference-assets" || command.Executor != "activity.reference_asset_generation" ||
		command.Attempt != 1 {
		return workflowdomain.NodeExecutorResult{}, fmt.Errorf("unexpected reference asset poll command: %#v", command.NodeActivityCommand)
	}
	switch executor.mode {
	case referenceAssetWorkerModeRetrying:
		return workflowdomain.NodeExecutorResult{Status: "RETRYING"}, nil
	case referenceAssetWorkerModeNeedsAttention:
		return workflowdomain.NodeExecutorResult{
			Status:     workflowdomain.NodeActivityNeedsAttention,
			ErrorCode:  workflowdomain.ProviderOutcomeUnknownErrorCode,
			NextAction: workflowdomain.ManualProviderReconciliationNextAction,
		}, nil
	default:
		return workflowdomain.NodeExecutorResult{}, fmt.Errorf("invalid reference asset poll mode %q", executor.mode)
	}
}

func referenceAssetRecoveryGraph(t *testing.T, assetID string, assetStateID string) authoring.Graph {
	t.Helper()
	config, err := json.Marshal(map[string]string{"asset_id": assetID, "asset_state_id": assetStateID})
	if err != nil {
		t.Fatalf("encode reference asset node config: %v", err)
	}
	return authoring.Graph{
		Nodes: []authoring.Node{{
			ID: "reference-assets", DefinitionKey: "generation.reference_asset", DefinitionVersion: "1.0.0",
			Config: config,
		}},
		Variables: map[string]json.RawMessage{
			"approved-intents": json.RawMessage(`{"status":"approved"}`),
		},
		Bindings: []authoring.Binding{{
			NodeID: "reference-assets", Port: "intents", Variable: "approved-intents",
			ValueType: "approved_storyboard_intents",
		}},
	}
}

func startReferenceAssetTemporalWorkerProcess(
	t *testing.T,
	databaseURL string,
	temporalAddress string,
	taskQueue string,
	mode string,
) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	command := exec.Command(os.Args[0], "-test.run=^TestReferenceAssetTemporalWorkerProcessHelper$", "-test.v")
	command.Env = append(os.Environ(),
		referenceAssetWorkerHelperFlag+"=1",
		"LANVERSE_TEST_DATABASE_URL="+databaseURL,
		referenceAssetWorkerHelperAddress+"="+temporalAddress,
		referenceAssetWorkerHelperTaskQueue+"="+taskQueue,
		referenceAssetWorkerHelperMode+"="+mode,
	)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatalf("start reference asset Temporal worker subprocess: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil && command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command, output
}

func waitForReferenceAssetPollTimer(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
	completedActivityID string,
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		iterator := temporalClient.GetWorkflowHistory(
			ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
		)
		scheduled := make(map[int64]string)
		completedEventID := int64(0)
		for iterator.HasNext() {
			event, err := iterator.Next()
			if err != nil {
				t.Fatalf("read Workflow history while waiting for reference asset timer: %v", err)
			}
			if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
				scheduled[event.GetEventId()] = attributes.GetActivityId()
			}
			if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil &&
				scheduled[attributes.GetScheduledEventId()] == completedActivityID {
				completedEventID = event.GetEventId()
			}
			if event.GetTimerStartedEventAttributes() != nil && completedEventID > 0 && event.GetEventId() > completedEventID {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("wait for reference asset Temporal poll timer: %v", ctx.Err())
		}
	}
}
