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

func TestWorkflowRerunDerivesDirtyClosureAndReusesCanonicalUpstreamProjection(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow rerun journey")
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
	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
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
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "rerun-authoring-create",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "rerun-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	starter := &scriptedWorkflowStarter{outcomes: []string{"started", "started"}}
	startService := workflowapp.NewStartService(compiler, workflowStore, starter, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	sourceRun, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "rerun-source-start",
	})
	if err != nil || sourceRun.Status != "RUNNING" {
		t.Fatalf("start source workflow: run=%#v err=%v", sourceRun, err)
	}
	sourceRequest := workflowStarterRequests(starter)[0]
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: &scriptedNodeExecutor{},
	})
	sourcePlan, err := runtimeService.LoadExecutionPlan(ctx, sourceRequest)
	if err != nil {
		t.Fatalf("load source execution plan: %v", err)
	}
	scriptNode := sourcePlan.Nodes[0]
	if _, err = runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: sourceRun.ID, NodeRunID: scriptNode.NodeRunID, NodeID: scriptNode.NodeID,
		Executor: scriptNode.Executor, Attempt: 1,
	}); err != nil {
		t.Fatalf("complete reusable source node: %v", err)
	}
	var sourceScript model.NodeRunProjection
	if err = database.First(&sourceScript, "id = ?", scriptNode.NodeRunID).Error; err != nil {
		t.Fatalf("load source script projection: %v", err)
	}
	failedNode := sourcePlan.Nodes[1]
	failureCommand := workflow.FailRunCommand{
		WorkflowRunID: sourceRun.ID, NodeRunID: failedNode.NodeRunID,
		NodeID: failedNode.NodeID, FailureCode: "node_activity_failed",
	}
	if err = runtimeService.FailRun(ctx, failureCommand); err != nil {
		t.Fatalf("project source failure: %v", err)
	}
	if err = runtimeService.FailRun(ctx, failureCommand); err != nil {
		t.Fatalf("replay source failure projection: %v", err)
	}
	var sourceRecord model.WorkflowRun
	if err = database.First(&sourceRecord, "id = ?", sourceRun.ID).Error; err != nil {
		t.Fatalf("load source run: %v", err)
	}
	if sourceRecord.Status != "FAILED" || sourceRecord.ProgressStage != "node:bible:failed" ||
		sourceRecord.NextAction == nil || *sourceRecord.NextAction != "rerun_failed_node" ||
		!strings.Contains(string(sourceRecord.Error), "node_activity_failed") {
		t.Fatalf("source failure projection = %#v", sourceRecord)
	}

	rerun, err := startService.Rerun(ctx, actor, workflowapp.RerunCommand{
		SourceWorkflowRunID: sourceRun.ID, RootNodeID: "bible", IdempotencyKey: "rerun-bible",
	})
	if err != nil || rerun.Status != "RUNNING" || rerun.SourceWorkflowRunID == nil || *rerun.SourceWorkflowRunID != sourceRun.ID ||
		rerun.RerunRootNodeID == nil || *rerun.RerunRootNodeID != "bible" || rerun.ID == sourceRun.ID {
		t.Fatalf("derive workflow rerun: run=%#v err=%v", rerun, err)
	}
	rerunRequest := workflowStarterRequests(starter)[1]
	if rerunRequest.SourceWorkflowRunID != sourceRun.ID || rerunRequest.RerunRootNodeID != "bible" {
		t.Fatalf("rerun Temporal input lost source scope: %#v", rerunRequest)
	}
	rerunPlan, err := runtimeService.LoadExecutionPlan(ctx, rerunRequest)
	if err != nil {
		t.Fatalf("load rerun execution plan: %v", err)
	}
	wantDirtyOrder := []string{
		"bible", "bible-review", "episodes", "episodes-review", "structure", "structure-review",
		"storyboard", "storyboard-review", "export",
	}
	actualDirtyOrder := make([]string, 0, len(rerunPlan.Nodes))
	for _, node := range rerunPlan.Nodes {
		actualDirtyOrder = append(actualDirtyOrder, node.NodeID)
	}
	if !slices.Equal(actualDirtyOrder, wantDirtyOrder) {
		t.Fatalf("rerun dirty order = %v, want %v", actualDirtyOrder, wantDirtyOrder)
	}
	var reusedScript model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", rerun.ID, "script").First(&reusedScript).Error; err != nil {
		t.Fatalf("load reused script projection: %v", err)
	}
	if reusedScript.Status != "SKIPPED" || reusedScript.ReusedFromNodeRunID == nil ||
		*reusedScript.ReusedFromNodeRunID != sourceScript.ID || reusedScript.InputHash == nil || reusedScript.OutputHash == nil ||
		*reusedScript.InputHash != *sourceScript.InputHash || *reusedScript.OutputHash != *sourceScript.OutputHash ||
		reusedScript.Attempt != 0 || reusedScript.CacheKey != nil {
		t.Fatalf("reused script projection = %#v", reusedScript)
	}
	if err = database.Model(&model.NodeRunProjection{}).Where("id = ?", reusedScript.ID).
		Update("reused_from_node_run_id", nil).Error; err != nil {
		t.Fatalf("tamper reused source marker: %v", err)
	}
	if _, err = runtimeService.LoadExecutionPlan(ctx, rerunRequest); err == nil {
		t.Fatal("runtime accepted a reused projection without its source NodeRun")
	}
	if err = database.Model(&model.NodeRunProjection{}).Where("id = ?", reusedScript.ID).
		Update("reused_from_node_run_id", sourceScript.ID).Error; err != nil {
		t.Fatalf("restore reused source marker: %v", err)
	}
	var cacheCount int64
	if err = database.Model(&model.NodeCacheEntry{}).Where("source_workflow_run_id = ?", rerun.ID).Count(&cacheCount).Error; err != nil {
		t.Fatalf("count rerun node cache entries: %v", err)
	}
	if cacheCount != 0 {
		t.Fatalf("reused upstream wrote %d node cache entries", cacheCount)
	}

	executor := &scriptedNodeExecutor{}
	runtimeService = workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString, Executor: executor,
	})
	firstDirty := rerunPlan.Nodes[0]
	if _, err = runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: rerun.ID, NodeRunID: firstDirty.NodeRunID, NodeID: firstDirty.NodeID,
		Executor: firstDirty.Executor, Attempt: 1,
	}); err != nil {
		t.Fatalf("execute rerun root from reused upstream: %v", err)
	}
	commands := executor.Commands()
	if len(commands) != 1 || len(commands[0].Input.Bindings) != 1 ||
		commands[0].Input.Bindings[0].SourceNodeID != "script" ||
		commands[0].Input.Bindings[0].ContentHash != sourceScriptOutputHash(t, sourceScript) {
		t.Fatalf("rerun root input did not use source script output: %#v", commands)
	}

	replayed, err := startService.Rerun(ctx, actor, workflowapp.RerunCommand{
		SourceWorkflowRunID: sourceRun.ID, RootNodeID: "bible", IdempotencyKey: "rerun-bible",
	})
	if err != nil || replayed.ID != rerun.ID || starter.CallCount() != 2 {
		t.Fatalf("rerun replay created another Temporal start: run=%#v calls=%d err=%v", replayed, starter.CallCount(), err)
	}
	if _, err = startService.Rerun(ctx, actor, workflowapp.RerunCommand{
		SourceWorkflowRunID: sourceRun.ID, RootNodeID: "script", IdempotencyKey: "rerun-bible",
	}); err == nil || starter.CallCount() != 2 {
		t.Fatalf("rerun idempotency key accepted a different root: calls=%d err=%v", starter.CallCount(), err)
	}
	if _, err = startService.Rerun(ctx, actor, workflowapp.RerunCommand{
		SourceWorkflowRunID: sourceRun.ID, RootNodeID: "storyboard", IdempotencyKey: "rerun-invalid-upstream",
	}); err == nil {
		t.Fatal("rerun accepted a root whose required source upstream was incomplete")
	}
	var runCount int64
	if err = database.Model(&model.WorkflowRun{}).Where("authoring_revision_id = ?", revision.ID).Count(&runCount).Error; err != nil {
		t.Fatalf("count source and rerun: %v", err)
	}
	if runCount != 2 {
		t.Fatalf("workflow run count = %d, want source plus one rerun", runCount)
	}
	var unchangedSource model.WorkflowRun
	if err = database.First(&unchangedSource, "id = ?", sourceRun.ID).Error; err != nil {
		t.Fatalf("reload source run: %v", err)
	}
	if unchangedSource.Status != "FAILED" || unchangedSource.SourceWorkflowRunID != nil || unchangedSource.RerunRootNodeID != nil {
		t.Fatalf("source run was mutated by rerun: %#v", unchangedSource)
	}
}

func workflowStarterRequests(starter *scriptedWorkflowStarter) []workflow.StartRequest {
	starter.mu.Lock()
	defer starter.mu.Unlock()
	return append([]workflow.StartRequest(nil), starter.requests...)
}

func sourceScriptOutputHash(t *testing.T, projection model.NodeRunProjection) string {
	t.Helper()
	output, _, _, err := workflow.ParseNodeOutput(json.RawMessage(projection.Output))
	if err != nil || len(output.Bindings) != 1 {
		t.Fatalf("parse source script output: output=%#v err=%v", output, err)
	}
	return output.Bindings[0].ContentHash
}
