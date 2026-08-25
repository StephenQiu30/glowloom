package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestRuntimePlanWaitsForCommittedStartAndRestoresCompiledOrder(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL runtime plan journey")
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
	authoringStore := authoringgorm.New(database)
	now := time.Date(2026, time.August, 25, 22, 0, 0, 0, time.UTC)
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
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "runtime-plan-authoring-create",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "runtime-plan-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	starter := newBlockingWorkflowStarter()
	startService := workflowapp.NewStartService(compiler, workflowStore, starter, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	startResult := make(chan struct {
		run workflow.WorkflowRun
		err error
	}, 1)
	go func() {
		run, startErr := startService.Start(ctx, actor, workflowapp.StartCommand{
			AuthoringRevisionID: revision.ID, IdempotencyKey: "runtime-plan-start",
		})
		startResult <- struct {
			run workflow.WorkflowRun
			err error
		}{run: run, err: startErr}
	}()
	request := <-starter.requests

	runtimeService := workflowapp.NewRuntimeService(workflowStore)
	if _, err = runtimeService.LoadExecutionPlan(ctx, request); err == nil {
		t.Fatal("runtime plan became executable before Start Intent and Run projection committed")
	}
	close(starter.release)
	started := <-startResult
	if started.err != nil || started.run.Status != "RUNNING" {
		t.Fatalf("finish workflow start: run=%#v err=%v", started.run, started.err)
	}

	plan, err := runtimeService.LoadExecutionPlan(ctx, request)
	if err != nil {
		t.Fatalf("load committed runtime plan: %v", err)
	}
	wantOrder := []string{
		"script", "bible", "bible-review", "episodes", "structure", "structure-review", "storyboard", "storyboard-review", "export",
	}
	actualOrder := make([]string, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		actualOrder = append(actualOrder, node.NodeID)
		if node.NodeRunID == "" || node.Executor == "" || node.RiskLevel == "" {
			t.Fatalf("incomplete runtime node: %#v", node)
		}
	}
	if !slices.Equal(actualOrder, wantOrder) {
		t.Fatalf("runtime node order = %v, want %v", actualOrder, wantOrder)
	}
	if plan.WorkflowRunID != request.WorkflowRunID || plan.DefinitionVersionID != request.DefinitionVersionID ||
		plan.RunInputSnapshotID != request.RunInputSnapshotID || plan.DefinitionContentHash != request.DefinitionContentHash ||
		plan.InputSnapshotHash != request.InputSnapshotHash {
		t.Fatalf("runtime plan lost frozen start identity: %#v", plan)
	}

	executor := &scriptedNodeExecutor{failures: 1}
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 5 * time.Minute,
	})
	runtimeService = workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: executor, HumanTasks: workflowreview.New(reviewService),
	})
	node := plan.Nodes[1]
	command := workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: node.NodeRunID, NodeID: node.NodeID,
		Executor: node.Executor, Attempt: 1,
	}
	if _, err = runtimeService.ExecuteNode(ctx, command); err == nil {
		t.Fatal("real node projection reported the first executor failure as success")
	}
	result, err := runtimeService.ExecuteNode(ctx, command)
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("retry persisted node execution: result=%#v err=%v", result, err)
	}
	if _, err = runtimeService.ExecuteNode(ctx, command); err != nil || executor.CallCount() != 2 {
		t.Fatalf("replay persisted node execution: calls=%d err=%v", executor.CallCount(), err)
	}
	var projection model.NodeRunProjection
	if err = database.First(&projection, "id = ?", node.NodeRunID).Error; err != nil {
		t.Fatalf("load persisted node projection: %v", err)
	}
	if projection.Status != "SUCCEEDED" || projection.Attempt != 2 || projection.Executor != node.Executor ||
		projection.RiskLevel != node.RiskLevel || projection.ActiveClaimToken != nil {
		t.Fatalf("persisted node projection = %#v", projection)
	}
	gate := plan.Nodes[2]
	gateCommand := workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: gate.NodeRunID, NodeID: gate.NodeID,
		Executor: gate.Executor, Attempt: 1,
	}
	if err = runtimeService.OpenHumanGate(ctx, gateCommand); err != nil {
		t.Fatalf("open persisted human gate: %v", err)
	}
	var humanTask model.HumanTask
	if err = database.Where("node_run_id = ?", gate.NodeRunID).First(&humanTask).Error; err != nil {
		t.Fatalf("load workflow human task: %v", err)
	}
	gateRevision := humanTask.SubjectRevision
	if err = runtimeService.OpenHumanGate(ctx, gateCommand); err != nil {
		t.Fatalf("replay persisted human gate: %v", err)
	}
	var humanTaskCount int64
	if err = database.Model(&model.HumanTask{}).Where("node_run_id = ?", gate.NodeRunID).Count(&humanTaskCount).Error; err != nil {
		t.Fatalf("count workflow human tasks: %v", err)
	}
	var waitingRun model.WorkflowRun
	var waitingNode model.NodeRunProjection
	if err = database.First(&waitingRun, "id = ?", request.WorkflowRunID).Error; err != nil {
		t.Fatalf("load waiting workflow run: %v", err)
	}
	if err = database.First(&waitingNode, "id = ?", gate.NodeRunID).Error; err != nil {
		t.Fatalf("load waiting node run: %v", err)
	}
	if humanTaskCount != 1 || waitingRun.Status != "WAITING_HUMAN" || waitingNode.Status != "WAITING_HUMAN" ||
		humanTask.SubjectID.String() != gate.NodeRunID || gateRevision != waitingNode.Revision {
		t.Fatalf("human gate projection = task %#v run %#v node %#v", humanTask, waitingRun, waitingNode)
	}
	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claimed, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: humanTask.ID.String(), ExpectedRevision: humanTask.Revision, IdempotencyKey: "workflow-gate-claim",
	})
	if err != nil {
		t.Fatalf("claim workflow human task: %v", err)
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: humanTask.ID.String(), ClaimToken: claimed.ClaimToken, ExpectedTaskRevision: claimed.Task.Revision,
		ExpectedSubjectRevision: claimed.Task.SubjectRevision, Decision: "approved", IdempotencyKey: "workflow-gate-decision",
	})
	if err != nil {
		t.Fatalf("decide workflow human task: %v", err)
	}
	signaler := &scriptedSignaler{outcomes: []workflow.SignalObservation{
		{Outcome: workflow.SignalOutcomeUnknown},
		{Outcome: workflow.SignalOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
	}}
	signalService := workflowapp.NewSignalService(workflowStore, signaler, workflowapp.SignalConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: waitingRun.WorkspaceID.String(), WorkflowRunID: waitingRun.ID.String(), NodeRunID: waitingNode.ID.String(),
		HumanTaskID: decision.Task.ID, ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: decision.Decision.Decision,
		IdempotencyKey: "workflow-gate-signal",
	}
	unknownSignal, err := signalService.SignalHumanGate(ctx, actor, signalCommand)
	if err != nil || unknownSignal.Status != "unknown" {
		t.Fatalf("persist unknown human gate signal: intent=%#v err=%v", unknownSignal, err)
	}
	completedSignal, err := signalService.SignalHumanGate(ctx, actor, signalCommand)
	if err != nil || completedSignal.Status != "completed" || completedSignal.ID != unknownSignal.ID {
		t.Fatalf("reconcile persisted human gate signal: intent=%#v err=%v", completedSignal, err)
	}
	var applyCount, signalIntentCount, signalReceiptCount int64
	if err = database.Model(&model.WorkflowHumanGateApplyReceipt{}).Count(&applyCount).Error; err != nil {
		t.Fatalf("count human gate apply receipts: %v", err)
	}
	if err = database.Model(&model.WorkflowSignalIntent{}).Count(&signalIntentCount).Error; err != nil {
		t.Fatalf("count signal intents: %v", err)
	}
	if err = database.Model(&model.WorkflowSignalReceipt{}).Count(&signalReceiptCount).Error; err != nil {
		t.Fatalf("count signal receipts: %v", err)
	}
	if applyCount != 1 || signalIntentCount != 1 || signalReceiptCount != 2 {
		t.Fatalf("signal fact counts = apply %d intents %d receipts %d", applyCount, signalIntentCount, signalReceiptCount)
	}
	completion := workflow.CompleteRunCommand{WorkflowRunID: request.WorkflowRunID}
	if err = runtimeService.CompleteRun(ctx, completion); err == nil {
		t.Fatal("run completed while queued nodes still existed")
	}
	if err = database.Model(&model.NodeRunProjection{}).Where("workflow_run_id = ?", request.WorkflowRunID).
		Updates(map[string]any{"status": "SUCCEEDED", "active_claim_token": nil}).Error; err != nil {
		t.Fatalf("prepare completed node projections: %v", err)
	}
	if err = database.Model(&model.WorkflowRun{}).Where("id = ?", request.WorkflowRunID).Update("status", "RUNNING").Error; err != nil {
		t.Fatalf("prepare completable workflow run: %v", err)
	}
	if err = runtimeService.CompleteRun(ctx, completion); err != nil {
		t.Fatalf("complete workflow run: %v", err)
	}
	if err = runtimeService.CompleteRun(ctx, completion); err != nil {
		t.Fatalf("replay workflow completion: %v", err)
	}
	var completedRun model.WorkflowRun
	if err = database.First(&completedRun, "id = ?", request.WorkflowRunID).Error; err != nil {
		t.Fatalf("load completed workflow run: %v", err)
	}
	if completedRun.Status != "SUCCEEDED" || completedRun.ProgressStage != "completed" {
		t.Fatalf("completed workflow run = %#v", completedRun)
	}
}

type blockingWorkflowStarter struct {
	requests chan workflow.StartRequest
	release  chan struct{}
}

func newBlockingWorkflowStarter() *blockingWorkflowStarter {
	return &blockingWorkflowStarter{requests: make(chan workflow.StartRequest, 1), release: make(chan struct{})}
}

func (starter *blockingWorkflowStarter) Start(_ context.Context, request workflow.StartRequest) (workflow.StartObservation, error) {
	starter.requests <- request
	<-starter.release
	return workflow.StartObservation{Outcome: workflow.StartOutcomeStarted, ObservedInputHash: request.InputHash}, nil
}
