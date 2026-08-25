package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestCancelControlPersistsFactsAndFencesLateNodeExecution(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow control journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 26, 5, 0, 0, 0, time.UTC)
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
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "control-authoring-create-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "control-authoring-publish-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(
		compiler, workflowStore, &scriptedWorkflowStarter{outcomes: []string{"started"}},
		workflowapp.StartConfig{Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		}, NewID: uuid.NewString},
	)
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	run, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "control-start-" + uuid.NewString(),
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start cancellable workflow: run=%#v err=%v", run, err)
	}
	var activeNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ?", run.ID).First(&activeNode).Error; err != nil {
		t.Fatalf("load node to execute during cancellation: %v", err)
	}
	blockingExecutor := &blockingControlNodeExecutor{
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		}, NewID: uuid.NewString, Executor: blockingExecutor,
	})
	executionResult := make(chan error, 1)
	go func() {
		_, executeErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
			WorkflowRunID: run.ID, NodeRunID: activeNode.ID.String(), NodeID: activeNode.NodeID,
			Executor: activeNode.Executor, Attempt: 1,
		})
		executionResult <- executeErr
	}()
	select {
	case <-blockingExecutor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("workflow node did not acquire a claim before cancellation")
	}
	var runningRun model.WorkflowRun
	if err = database.First(&runningRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload running workflow revision: %v", err)
	}
	controller := &scriptedController{outcomes: []workflow.ControlObservation{
		{Outcome: workflow.ControlOutcomeUnknown},
		{Outcome: workflow.ControlOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
	}}
	controlService := workflowapp.NewControlService(workflowStore, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	command := workflowapp.CancelCommand{
		WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: runningRun.Revision,
		IdempotencyKey: "control-cancel-" + uuid.NewString(),
	}
	unknown, err := controlService.Cancel(ctx, actor, command)
	if err != nil || unknown.Status != "unknown" {
		t.Fatalf("persist unknown cancellation: intent=%#v err=%v", unknown, err)
	}
	completed, err := controlService.Cancel(ctx, actor, command)
	if err != nil || completed.Status != "completed" || completed.ID != unknown.ID {
		t.Fatalf("reconcile persisted cancellation: intent=%#v err=%v", completed, err)
	}

	var persistedRun model.WorkflowRun
	if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load cancelled workflow run: %v", err)
	}
	var nonCancelledNodes int64
	if err = database.Model(&model.NodeRunProjection{}).
		Where("workflow_run_id = ? AND status <> ?", run.ID, "CANCELLED").Count(&nonCancelledNodes).Error; err != nil {
		t.Fatalf("count non-cancelled node projections: %v", err)
	}
	var intentCount, receiptCount int64
	if err = database.Model(&model.WorkflowControlIntent{}).Where("workflow_run_id = ?", run.ID).Count(&intentCount).Error; err != nil {
		t.Fatalf("count control intents: %v", err)
	}
	if err = database.Model(&model.WorkflowControlReceipt{}).Where("workflow_run_id = ?", run.ID).Count(&receiptCount).Error; err != nil {
		t.Fatalf("count control receipts: %v", err)
	}
	if persistedRun.Status != "CANCELLED" || persistedRun.ProgressStage != "cancelled" ||
		nonCancelledNodes != 0 || intentCount != 1 || receiptCount != 2 {
		t.Fatalf(
			"cancelled facts = run %#v non_cancelled_nodes %d intents %d receipts %d",
			persistedRun, nonCancelledNodes, intentCount, receiptCount,
		)
	}

	close(blockingExecutor.release)
	if err = <-executionResult; err == nil {
		t.Fatal("late success from the pre-cancel node claim overwrote cancellation")
	}
	executor := &scriptedNodeExecutor{}
	runtimeService = workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return now.Add(time.Second) }, NewID: uuid.NewString, Executor: executor,
	})
	if _, err = runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: run.ID, NodeRunID: activeNode.ID.String(), NodeID: activeNode.NodeID,
		Executor: activeNode.Executor, Attempt: 1,
	}); err == nil || executor.CallCount() != 0 {
		t.Fatalf("cancelled node accepted late execution: calls=%d err=%v", executor.CallCount(), err)
	}
}

type blockingControlNodeExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (executor *blockingControlNodeExecutor) Execute(
	context.Context,
	workflow.NodeExecutorCommand,
) (workflow.NodeActivityResult, error) {
	executor.started <- struct{}{}
	<-executor.release
	return workflow.NodeActivityResult{Status: "SUCCEEDED"}, nil
}
