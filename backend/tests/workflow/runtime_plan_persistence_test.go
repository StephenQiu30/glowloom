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
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
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
		"script", "evidence", "story", "story-review", "bible-review",
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
	blockedBible := plan.Nodes[1]
	if _, blockedErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: blockedBible.NodeRunID, NodeID: blockedBible.NodeID,
		Executor: blockedBible.Executor, Attempt: 1,
	}); blockedErr == nil {
		t.Fatal("downstream node executed before its upstream output existed")
	}
	var blockedProjection model.NodeRunProjection
	if err = database.First(&blockedProjection, "id = ?", blockedBible.NodeRunID).Error; err != nil {
		t.Fatalf("load blocked downstream node: %v", err)
	}
	if blockedProjection.Status != "QUEUED" || blockedProjection.InputHash != nil || executor.CallCount() != 0 {
		t.Fatalf("blocked downstream projection = %#v executor calls=%d", blockedProjection, executor.CallCount())
	}
	node := plan.Nodes[0]
	command := workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: node.NodeRunID, NodeID: node.NodeID,
		Executor: node.Executor, Attempt: 1,
	}
	if _, err = runtimeService.ExecuteNode(ctx, command); err == nil {
		t.Fatal("real node projection reported the first executor failure as success")
	}
	var retryProjection model.NodeRunProjection
	if err = database.First(&retryProjection, "id = ?", node.NodeRunID).Error; err != nil {
		t.Fatalf("load retrying node input projection: %v", err)
	}
	if retryProjection.Status != "RETRYING" || retryProjection.InputHash == nil || len(retryProjection.Input) == 0 {
		t.Fatalf("retrying node lost its frozen input: %#v", retryProjection)
	}
	result, err := runtimeService.ExecuteNode(ctx, command)
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("retry persisted node execution: result=%#v err=%v", result, err)
	}
	executorCommands := executor.Commands()
	if len(executorCommands) != 2 || executorCommands[1].WorkspaceID != started.run.WorkspaceID ||
		executorCommands[1].ProjectID != started.run.ProjectID || executorCommands[1].InitiatorUserID != actor.UserID ||
		executorCommands[1].InitiatorTokenVersion != actor.TokenVersion {
		t.Fatalf("persisted workflow actor was not propagated to node executor: %#v", executorCommands)
	}
	replayedResult, replayErr := runtimeService.ExecuteNode(ctx, command)
	if replayErr != nil || executor.CallCount() != 2 || replayedResult.OutputHash != result.OutputHash {
		t.Fatalf("replay persisted node execution: result=%#v calls=%d err=%v", replayedResult, executor.CallCount(), replayErr)
	}
	var projection model.NodeRunProjection
	if err = database.First(&projection, "id = ?", node.NodeRunID).Error; err != nil {
		t.Fatalf("load persisted node projection: %v", err)
	}
	if projection.Status != "SUCCEEDED" || projection.Attempt != 2 || projection.Executor != node.Executor ||
		projection.RiskLevel != node.RiskLevel || projection.ActiveClaimToken != nil || projection.OutputHash == nil ||
		*projection.OutputHash != result.OutputHash || projection.InputHash == nil ||
		retryProjection.InputHash == nil || *projection.InputHash != *retryProjection.InputHash {
		t.Fatalf("persisted node projection = %#v", projection)
	}
	persistedOutput, _, persistedOutputHash, outputErr := workflow.ParseNodeOutput(json.RawMessage(projection.Output))
	if outputErr != nil || persistedOutputHash != result.OutputHash || persistedOutput.Bindings[0] != result.Output.Bindings[0] {
		t.Fatalf("persisted node output = %#v hash=%s err=%v", persistedOutput, persistedOutputHash, outputErr)
	}
	persistedInput, _, persistedInputHash, inputErr := workflow.ParseNodeInput(json.RawMessage(projection.Input))
	if inputErr != nil || persistedInputHash != *projection.InputHash || len(persistedInput.Bindings) != 0 ||
		len(persistedInput.FrozenInputs) != 1 || persistedInput.FrozenInputs[0].Hash != fixture.normalizedHash {
		t.Fatalf("persisted root node input = %#v hash=%s err=%v", persistedInput, persistedInputHash, inputErr)
	}

	evidence := plan.Nodes[1]
	evidenceResult, evidenceErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: evidence.NodeRunID, NodeID: evidence.NodeID,
		Executor: evidence.Executor, Attempt: 1,
	})
	if evidenceErr != nil || evidenceResult.Status != "SUCCEEDED" || executor.CallCount() != 3 {
		t.Fatalf("execute source evidence node: result=%#v calls=%d err=%v", evidenceResult, executor.CallCount(), evidenceErr)
	}
	var evidenceProjection model.NodeRunProjection
	if err = database.First(&evidenceProjection, "id = ?", evidence.NodeRunID).Error; err != nil {
		t.Fatalf("load source evidence projection: %v", err)
	}
	evidenceInput, _, evidenceInputHash, evidenceInputErr := workflow.ParseNodeInput(json.RawMessage(evidenceProjection.Input))
	if evidenceInputErr != nil || evidenceProjection.InputHash == nil || evidenceInputHash != *evidenceProjection.InputHash ||
		len(evidenceInput.Bindings) != 1 || evidenceInput.Bindings[0].Port != "script" ||
		evidenceInput.Bindings[0].SourceNodeID != "script" || evidenceInput.Bindings[0].ContentHash != result.Output.Bindings[0].ContentHash {
		t.Fatalf("source evidence input = %#v hash=%s err=%v", evidenceInput, evidenceInputHash, evidenceInputErr)
	}
	if evidenceProjection.CacheKey == nil || len(*evidenceProjection.CacheKey) != 64 {
		t.Fatalf("cacheable source evidence projection lost its cache key: %#v", evidenceProjection)
	}
	var initialCacheCount int64
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("workspace_id = ? AND cache_key = ?", started.run.WorkspaceID, *evidenceProjection.CacheKey).
		Count(&initialCacheCount).Error; err != nil || initialCacheCount != 1 {
		t.Fatalf("committed runtime node cache count = %d err=%v", initialCacheCount, err)
	}

	secondRun, secondStartErr := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "runtime-plan-cache-reuse",
	})
	secondRequest := <-starter.requests
	if secondStartErr != nil || secondRun.Status != "RUNNING" {
		t.Fatalf("start cache reuse workflow: run=%#v err=%v", secondRun, secondStartErr)
	}
	secondPlan, secondPlanErr := runtimeService.LoadExecutionPlan(ctx, secondRequest)
	if secondPlanErr != nil || len(secondPlan.Nodes) != len(plan.Nodes) {
		t.Fatalf("load cache reuse plan: plan=%#v err=%v", secondPlan, secondPlanErr)
	}
	secondScript := secondPlan.Nodes[0]
	secondScriptResult, secondScriptErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: secondRequest.WorkflowRunID, NodeRunID: secondScript.NodeRunID, NodeID: secondScript.NodeID,
		Executor: secondScript.Executor, Attempt: 1,
	})
	if secondScriptErr != nil || secondScriptResult.Status != "SUCCEEDED" || executor.CallCount() != 4 {
		t.Fatalf("execute cache reuse upstream: result=%#v calls=%d err=%v", secondScriptResult, executor.CallCount(), secondScriptErr)
	}
	secondEvidence := secondPlan.Nodes[1]
	secondEvidenceResult, secondEvidenceErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: secondRequest.WorkflowRunID, NodeRunID: secondEvidence.NodeRunID, NodeID: secondEvidence.NodeID,
		Executor: secondEvidence.Executor, Attempt: 1,
	})
	if secondEvidenceErr != nil || secondEvidenceResult.Status != "CACHED" ||
		secondEvidenceResult.OutputHash != evidenceResult.OutputHash || executor.CallCount() != 4 {
		t.Fatalf("reuse persisted runtime cache: result=%#v calls=%d err=%v", secondEvidenceResult, executor.CallCount(), secondEvidenceErr)
	}
	var secondEvidenceProjection model.NodeRunProjection
	if err = database.First(&secondEvidenceProjection, "id = ?", secondEvidence.NodeRunID).Error; err != nil {
		t.Fatalf("load cached source evidence projection: %v", err)
	}
	var reusedCacheCount int64
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("workspace_id = ? AND cache_key = ?", started.run.WorkspaceID, *evidenceProjection.CacheKey).
		Count(&reusedCacheCount).Error; err != nil {
		t.Fatalf("count reused runtime node cache: %v", err)
	}
	if secondEvidenceProjection.Status != "CACHED" || secondEvidenceProjection.ActiveClaimToken != nil ||
		secondEvidenceProjection.OutputHash == nil || *secondEvidenceProjection.OutputHash != evidenceResult.OutputHash ||
		secondEvidenceProjection.CacheKey == nil || *secondEvidenceProjection.CacheKey != *evidenceProjection.CacheKey || reusedCacheCount != 1 {
		t.Fatalf("cached source evidence projection = %#v cache count=%d", secondEvidenceProjection, reusedCacheCount)
	}
	story := plan.Nodes[2]
	storyResult, storyErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: story.NodeRunID, NodeID: story.NodeID,
		Executor: story.Executor, Attempt: 1,
	})
	if storyErr != nil || storyResult.Status != "SUCCEEDED" || executor.CallCount() != 5 {
		t.Fatalf("execute story analysis node: result=%#v calls=%d err=%v", storyResult, executor.CallCount(), storyErr)
	}
	storyReview := plan.Nodes[3]
	storyReviewResult, storyReviewErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: storyReview.NodeRunID, NodeID: storyReview.NodeID,
		Executor: storyReview.Executor, Attempt: 1,
	})
	if storyReviewErr != nil || storyReviewResult.Status != "SUCCEEDED" || executor.CallCount() != 6 {
		t.Fatalf("execute story review node: result=%#v calls=%d err=%v", storyReviewResult, executor.CallCount(), storyReviewErr)
	}
	gate := plan.Nodes[4]
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
	var candidateIDs []string
	if err = json.Unmarshal(humanTask.CandidateIDs, &candidateIDs); err != nil {
		t.Fatalf("decode human gate candidates: %v", err)
	}
	waitingInput, _, waitingInputHash, waitingInputErr := workflow.ParseNodeInput(json.RawMessage(waitingNode.Input))
	if humanTaskCount != 1 || waitingRun.Status != "WAITING_HUMAN" || waitingNode.Status != "WAITING_HUMAN" ||
		humanTask.SubjectType != "story_reconciliation_candidate" ||
		humanTask.SubjectID.String() != storyReviewResult.Output.Bindings[0].ReferenceID ||
		humanTask.SubjectRevision != 1 || humanTask.SubjectRevision == waitingNode.Revision ||
		humanTask.SubjectHash != storyReviewResult.Output.Bindings[0].ContentHash ||
		len(candidateIDs) != 1 || candidateIDs[0] != storyReviewResult.Output.Bindings[0].ReferenceID ||
		waitingInputErr != nil || waitingNode.InputHash == nil || waitingInputHash != *waitingNode.InputHash ||
		len(waitingInput.Bindings) != 1 || waitingInput.Bindings[0].ReferenceID != candidateIDs[0] {
		t.Fatalf("human gate projection = task %#v run %#v node %#v", humanTask, waitingRun, waitingNode)
	}
	completion := workflow.CompleteRunCommand{WorkflowRunID: request.WorkflowRunID}
	if err = runtimeService.CompleteRun(ctx, completion); err == nil {
		t.Fatal("run completed while its human gate was still waiting")
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
