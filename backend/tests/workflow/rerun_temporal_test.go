package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
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
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowRerunExecutesOnlyDirtyClosureOnRealTemporalAndReplays(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_TEMPORAL_ADDRESS to run the real rerun journey")
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
	catalog := rerunTemporalCatalog(t)
	now := time.Date(2026, time.August, 26, 11, 0, 0, 0, time.UTC)
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist rerun catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: rerunTemporalGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "temporal-rerun-authoring-create",
	})
	if err != nil {
		t.Fatalf("create rerun authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "temporal-rerun-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish rerun authoring revision: %v", err)
	}
	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	fakeStarter := &scriptedWorkflowStarter{outcomes: []string{"started"}}
	sourceStartService := workflowapp.NewStartService(compiler, workflowStore, fakeStarter, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	sourceRun, err := sourceStartService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "temporal-rerun-source",
	})
	if err != nil {
		t.Fatalf("start source run: %v", err)
	}
	sourceRuntime := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: &scriptedNodeExecutor{},
	})
	sourcePlan, err := sourceRuntime.LoadExecutionPlan(ctx, workflowStarterRequests(fakeStarter)[0])
	if err != nil {
		t.Fatalf("load source plan: %v", err)
	}
	if _, err = sourceRuntime.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: sourceRun.ID, NodeRunID: sourcePlan.Nodes[0].NodeRunID, NodeID: "source",
		Executor: sourcePlan.Nodes[0].Executor, Attempt: 1,
	}); err != nil {
		t.Fatalf("complete source upstream node: %v", err)
	}
	failedNode := sourcePlan.Nodes[1]
	if err = sourceRuntime.FailRun(ctx, workflow.FailRunCommand{
		WorkflowRunID: sourceRun.ID, NodeRunID: failedNode.NodeRunID,
		NodeID: failedNode.NodeID, FailureCode: "node_activity_failed",
	}); err != nil {
		t.Fatalf("persist source failure: %v", err)
	}
	var sourceRecord model.WorkflowRun
	if err = database.First(&sourceRecord, "id = ?", sourceRun.ID).Error; err != nil {
		t.Fatalf("load source run projection: %v", err)
	}
	if sourceRecord.Status != "FAILED" || sourceRecord.ProgressStage != "node:transform:failed" ||
		sourceRecord.NextAction == nil || *sourceRecord.NextAction != "rerun_failed_node" ||
		!strings.Contains(string(sourceRecord.Error), "node_activity_failed") {
		t.Fatalf("source failure projection = %#v", sourceRecord)
	}

	taskQueue := "lanverse-rerun-" + uuid.NewString()
	temporalRuntime, err := temporaladapter.New(temporaladapter.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	rerunExecutor := &scriptedNodeExecutor{}
	rerunRuntime := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: rerunExecutor,
	})
	runtimeWorker, err := temporalRuntime.NewWorker(rerunRuntime)
	if err != nil {
		t.Fatalf("register rerun worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start rerun worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	rerunStartService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	rerun, err := rerunStartService.Rerun(ctx, actor, workflowapp.RerunCommand{
		SourceWorkflowRunID: sourceRun.ID, RootNodeID: "transform", IdempotencyKey: "temporal-rerun-transform",
	})
	if err != nil {
		t.Fatalf("start real Temporal rerun: %v", err)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddress, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal history client: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	var result temporaladapter.RunResult
	if err = temporalClient.GetWorkflow(ctx, rerun.TemporalWorkflowID, "").Get(ctx, &result); err != nil {
		t.Fatalf("wait for rerun completion: %v", err)
	}
	if result.WorkflowRunID != rerun.ID || result.Status != "SUCCEEDED" {
		t.Fatalf("rerun result = %#v", result)
	}
	commands := rerunExecutor.Commands()
	executed := make([]string, 0, len(commands))
	for _, command := range commands {
		executed = append(executed, command.NodeID)
	}
	if !slices.Equal(executed, []string{"transform", "export"}) {
		t.Fatalf("Temporal rerun executed nodes %v, want [transform export]", executed)
	}
	var persistedRun model.WorkflowRun
	if err = database.First(&persistedRun, "id = ?", rerun.ID).Error; err != nil {
		t.Fatalf("load completed rerun: %v", err)
	}
	if persistedRun.Status != "SUCCEEDED" || persistedRun.SourceWorkflowRunID == nil ||
		*persistedRun.SourceWorkflowRunID != sourceRecord.ID || persistedRun.RerunRootNodeID == nil ||
		*persistedRun.RerunRootNodeID != "transform" {
		t.Fatalf("completed rerun projection = %#v", persistedRun)
	}
	var rerunNodes []model.NodeRunProjection
	if err = database.Where("workflow_run_id = ?", rerun.ID).Order("node_id ASC").Find(&rerunNodes).Error; err != nil {
		t.Fatalf("load rerun nodes: %v", err)
	}
	if len(rerunNodes) != 3 || rerunNodes[0].NodeID != "export" || rerunNodes[0].Status != "SUCCEEDED" ||
		rerunNodes[1].NodeID != "source" || rerunNodes[1].Status != "SKIPPED" || rerunNodes[1].ReusedFromNodeRunID == nil ||
		rerunNodes[2].NodeID != "transform" || rerunNodes[2].Status != "SUCCEEDED" {
		t.Fatalf("rerun node projections = %#v", rerunNodes)
	}

	iterator := temporalClient.GetWorkflowHistory(
		ctx, rerun.TemporalWorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	for iterator.HasNext() {
		event, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("read rerun history: %v", nextErr)
		}
		history.Events = append(history.Events, event)
	}
	if len(history.Events) == 0 {
		t.Fatal("Temporal rerun returned an empty history")
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay rerun history: %v", err)
	}
}

func rerunTemporalCatalog(t *testing.T) authoring.Catalog {
	t.Helper()
	port := func(key string) authoring.PortDefinition {
		return authoring.PortDefinition{Key: key, ValueType: "fact", Required: true}
	}
	definition := func(key, executor string, inputs, outputs []authoring.PortDefinition) authoring.NodeDefinition {
		return authoring.NodeDefinition{
			Key: key, Version: "1.0.0", Name: key, Category: "production", Executor: executor,
			InputPorts: inputs, OutputPorts: outputs,
			ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			CachePolicy:  "never", RiskLevel: "low", Executable: true,
		}
	}
	catalog, err := authoring.NewCatalog("lanverse.rerun-test", "1.0.0", []authoring.NodeDefinition{
		definition("test.source", "activity.test.source", nil, []authoring.PortDefinition{port("fact")}),
		definition("test.transform", "activity.test.transform", []authoring.PortDefinition{port("fact")}, []authoring.PortDefinition{port("fact")}),
		definition("test.export", "activity.test.export", []authoring.PortDefinition{port("fact")}, []authoring.PortDefinition{port("result")}),
	})
	if err != nil {
		t.Fatalf("build rerun catalog: %v", err)
	}
	return catalog
}

func rerunTemporalGraph() authoring.Graph {
	node := func(id, key string) authoring.Node {
		return authoring.Node{ID: id, DefinitionKey: key, DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)}
	}
	edge := func(id, from, fromPort, to, toPort string) authoring.Edge {
		return authoring.Edge{ID: id, FromNodeID: from, FromPort: fromPort, ToNodeID: to, ToPort: toPort}
	}
	return authoring.Graph{
		Nodes: []authoring.Node{
			node("source", "test.source"), node("transform", "test.transform"), node("export", "test.export"),
		},
		Edges: []authoring.Edge{
			edge("source-transform", "source", "fact", "transform", "fact"),
			edge("transform-export", "transform", "fact", "export", "fact"),
		},
	}
}
