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
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
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
		"script", "bible", "bible-review", "episodes", "episodes-review", "structure", "structure-review", "storyboard", "storyboard-review", "export",
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

	bible := plan.Nodes[1]
	bibleResult, bibleErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: bible.NodeRunID, NodeID: bible.NodeID,
		Executor: bible.Executor, Attempt: 1,
	})
	if bibleErr != nil || bibleResult.Status != "SUCCEEDED" || executor.CallCount() != 3 {
		t.Fatalf("execute downstream bible node: result=%#v calls=%d err=%v", bibleResult, executor.CallCount(), bibleErr)
	}
	var bibleProjection model.NodeRunProjection
	if err = database.First(&bibleProjection, "id = ?", bible.NodeRunID).Error; err != nil {
		t.Fatalf("load bible node projection: %v", err)
	}
	bibleInput, _, bibleInputHash, bibleInputErr := workflow.ParseNodeInput(json.RawMessage(bibleProjection.Input))
	if bibleInputErr != nil || bibleProjection.InputHash == nil || bibleInputHash != *bibleProjection.InputHash ||
		len(bibleInput.Bindings) != 1 || bibleInput.Bindings[0].Port != "script" ||
		bibleInput.Bindings[0].SourceNodeID != "script" || bibleInput.Bindings[0].ContentHash != result.Output.Bindings[0].ContentHash {
		t.Fatalf("downstream bible input = %#v hash=%s err=%v", bibleInput, bibleInputHash, bibleInputErr)
	}
	if bibleProjection.CacheKey == nil || len(*bibleProjection.CacheKey) != 64 {
		t.Fatalf("cacheable bible projection lost its cache key: %#v", bibleProjection)
	}
	bibleReference := bibleResult.Output.Bindings[0]
	bibleID := uuid.MustParse(bibleReference.ReferenceID)
	workflowWorkspaceID := uuid.MustParse(started.run.WorkspaceID)
	bibleTaskID := uuid.New()
	resultHash := bibleReference.ContentHash
	if err = database.Create(&model.WorkflowTask{
		ID: bibleTaskID, WorkspaceID: workflowWorkspaceID, TaskType: "production_bible", RequestType: "production_bible",
		RequestID: bibleID, Scope: []byte(`{}`), Status: "succeeded", ProgressStage: "agent_result",
		CancelStatus: "none", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed production bible task for human gate: %v", err)
	}
	if err = database.Create(&model.ProductionBible{
		ID: bibleID, WorkspaceID: workflowWorkspaceID, ProjectID: fixture.projectID,
		DocumentRevisionID: fixture.scriptRevisionID, TaskID: bibleTaskID, Status: "needs_review",
		InputHash: fixture.normalizedHash, ResultHash: &resultHash, EngineVersion: "test-v1", ModelName: "deterministic",
		PromptVersion: "test-v1", SchemaVersion: "production-bible-schema-v1", HarnessVersion: "test-v1",
		CheckpointRevision: 1, Candidate: []byte(`{"entities":[],"world_entries":[],"review_issues":[]}`),
		ReviewDecisions: []byte(`{}`), Revision: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed reviewable production bible for human gate: %v", err)
	}
	var initialCacheCount int64
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("workspace_id = ? AND cache_key = ?", started.run.WorkspaceID, *bibleProjection.CacheKey).
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
	secondBible := secondPlan.Nodes[1]
	secondBibleResult, secondBibleErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: secondRequest.WorkflowRunID, NodeRunID: secondBible.NodeRunID, NodeID: secondBible.NodeID,
		Executor: secondBible.Executor, Attempt: 1,
	})
	if secondBibleErr != nil || secondBibleResult.Status != "CACHED" ||
		secondBibleResult.OutputHash != bibleResult.OutputHash || executor.CallCount() != 4 {
		t.Fatalf("reuse persisted runtime cache: result=%#v calls=%d err=%v", secondBibleResult, executor.CallCount(), secondBibleErr)
	}
	var secondBibleProjection model.NodeRunProjection
	if err = database.First(&secondBibleProjection, "id = ?", secondBible.NodeRunID).Error; err != nil {
		t.Fatalf("load cached bible projection: %v", err)
	}
	var reusedCacheCount int64
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("workspace_id = ? AND cache_key = ?", started.run.WorkspaceID, *bibleProjection.CacheKey).
		Count(&reusedCacheCount).Error; err != nil {
		t.Fatalf("count reused runtime node cache: %v", err)
	}
	if secondBibleProjection.Status != "CACHED" || secondBibleProjection.ActiveClaimToken != nil ||
		secondBibleProjection.OutputHash == nil || *secondBibleProjection.OutputHash != bibleResult.OutputHash ||
		secondBibleProjection.CacheKey == nil || *secondBibleProjection.CacheKey != *bibleProjection.CacheKey || reusedCacheCount != 1 {
		t.Fatalf("cached bible projection = %#v cache count=%d", secondBibleProjection, reusedCacheCount)
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
	var candidateIDs []string
	if err = json.Unmarshal(humanTask.CandidateIDs, &candidateIDs); err != nil {
		t.Fatalf("decode human gate candidates: %v", err)
	}
	waitingInput, _, waitingInputHash, waitingInputErr := workflow.ParseNodeInput(json.RawMessage(waitingNode.Input))
	if humanTaskCount != 1 || waitingRun.Status != "WAITING_HUMAN" || waitingNode.Status != "WAITING_HUMAN" ||
		humanTask.SubjectID.String() != gate.NodeRunID || gateRevision != waitingNode.Revision ||
		len(candidateIDs) != 1 || candidateIDs[0] != bibleResult.Output.Bindings[0].ReferenceID ||
		waitingInputErr != nil || waitingNode.InputHash == nil || waitingInputHash != *waitingNode.InputHash ||
		len(waitingInput.Bindings) != 1 || waitingInput.Bindings[0].ReferenceID != candidateIDs[0] {
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
		Owner: workflowproduction.New(
			bibleapp.NewService(biblegorm.New(database), bibleapp.Config{
				Now: func() time.Time { return now }, NewID: uuid.NewString,
			}),
			planningapp.NewService(planninggorm.New(database), planningapp.Config{
				Now: func() time.Time { return now }, NewID: uuid.NewString,
			}),
			nil,
		),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: waitingRun.WorkspaceID.String(), WorkflowRunID: waitingRun.ID.String(), NodeRunID: waitingNode.ID.String(),
		HumanTaskID: decision.Task.ID, ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: decision.Decision.Decision,
		IdempotencyKey: "workflow-gate-signal",
	}
	driftedSignal := signalCommand
	driftedSignal.Decision = "rejected"
	driftedSignal.IdempotencyKey = "workflow-gate-signal-drifted"
	if _, driftedErr := signalService.SignalHumanGate(ctx, actor, driftedSignal); driftedErr == nil {
		t.Fatal("human gate signal accepted a decision that drifted from ReviewDecision")
	}
	var driftedApplyCount, driftedIntentCount int64
	if err = database.Model(&model.WorkflowHumanGateApplyReceipt{}).Where("workflow_run_id = ?", waitingRun.ID).Count(&driftedApplyCount).Error; err != nil {
		t.Fatalf("count drifted gate apply receipts: %v", err)
	}
	if err = database.Model(&model.WorkflowSignalIntent{}).Where("workflow_run_id = ?", waitingRun.ID).Count(&driftedIntentCount).Error; err != nil {
		t.Fatalf("count drifted gate signal intents: %v", err)
	}
	if driftedApplyCount != 0 || driftedIntentCount != 0 || len(signaler.Requests()) != 0 {
		t.Fatalf("drifted gate signal left effects: apply=%d intents=%d requests=%d", driftedApplyCount, driftedIntentCount, len(signaler.Requests()))
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
	if err = database.Model(&model.WorkflowHumanGateApplyReceipt{}).Where("workflow_run_id = ?", waitingRun.ID).Count(&applyCount).Error; err != nil {
		t.Fatalf("count human gate apply receipts: %v", err)
	}
	if err = database.Model(&model.WorkflowSignalIntent{}).Where("workflow_run_id = ?", waitingRun.ID).Count(&signalIntentCount).Error; err != nil {
		t.Fatalf("count signal intents: %v", err)
	}
	if err = database.Model(&model.WorkflowSignalReceipt{}).Where("workflow_run_id = ?", waitingRun.ID).Count(&signalReceiptCount).Error; err != nil {
		t.Fatalf("count signal receipts: %v", err)
	}
	if applyCount != 1 || signalIntentCount != 1 || signalReceiptCount != 2 {
		t.Fatalf("signal fact counts = apply %d intents %d receipts %d", applyCount, signalIntentCount, signalReceiptCount)
	}
	var ownerReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ? AND resource_id = ?", waitingRun.WorkspaceID, "production_bible.confirm", bibleID).
		Count(&ownerReceiptCount).Error; err != nil || ownerReceiptCount != 1 {
		t.Fatalf("production owner receipt count = %d err=%v", ownerReceiptCount, err)
	}
	requests := signaler.Requests()
	if len(requests) != 2 || requests[0].OwnerReceiptID == "" || requests[0].OutputHash == "" ||
		requests[0].OutputHash != requests[1].OutputHash {
		t.Fatalf("human gate owner signal evidence = %#v", requests)
	}
	applyGate := workflow.ApplyHumanGateCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: gate.NodeRunID, NodeID: gate.NodeID,
		SignalIntentID: completedSignal.ID, Decision: "APPROVED", OwnerReceiptID: requests[0].OwnerReceiptID,
		Output: requests[0].Output, OutputHash: requests[0].OutputHash,
	}
	conflictingApply := applyGate
	conflictingApply.Decision = "REJECTED"
	if err = runtimeService.ApplyHumanGate(ctx, conflictingApply); err == nil {
		t.Fatal("human gate accepted a decision that drifted from the completed signal intent")
	}
	if err = runtimeService.ApplyHumanGate(ctx, applyGate); err != nil {
		t.Fatalf("apply approved human gate: %v", err)
	}
	if err = runtimeService.ApplyHumanGate(ctx, applyGate); err != nil {
		t.Fatalf("replay approved human gate: %v", err)
	}
	var appliedRun model.WorkflowRun
	var appliedNode model.NodeRunProjection
	if err = database.First(&appliedRun, "id = ?", request.WorkflowRunID).Error; err != nil {
		t.Fatalf("load applied workflow run: %v", err)
	}
	if err = database.First(&appliedNode, "id = ?", gate.NodeRunID).Error; err != nil {
		t.Fatalf("load applied human gate node: %v", err)
	}
	appliedOutput, _, appliedOutputHash, appliedOutputErr := workflow.ParseNodeOutput(json.RawMessage(appliedNode.Output))
	if appliedRun.Status != "RUNNING" || appliedRun.NextAction != nil || appliedNode.Status != "SUCCEEDED" ||
		appliedNode.Revision != waitingNode.Revision+1 || appliedNode.OutputHash == nil ||
		appliedOutputErr != nil || appliedOutputHash != *appliedNode.OutputHash ||
		len(appliedOutput.Bindings) != 1 || appliedOutput.Bindings[0].ReferenceID != bibleID.String() ||
		appliedOutput.Bindings[0].ReferenceVersion != "2" || appliedOutput.Bindings[0].ValueType != "production_bible" {
		t.Fatalf("applied human gate projection = run %#v node %#v", appliedRun, appliedNode)
	}
	episodes := plan.Nodes[3]
	episodesResult, episodesErr := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: episodes.NodeRunID, NodeID: episodes.NodeID,
		Executor: episodes.Executor, Attempt: 1,
	})
	if episodesErr != nil || episodesResult.Status != "SUCCEEDED" || executor.CallCount() != 5 {
		t.Fatalf("execute node downstream of human gate: result=%#v calls=%d err=%v", episodesResult, executor.CallCount(), episodesErr)
	}
	var episodesProjection model.NodeRunProjection
	if err = database.First(&episodesProjection, "id = ?", episodes.NodeRunID).Error; err != nil {
		t.Fatalf("load downstream episode projection: %v", err)
	}
	episodesInput, _, episodesInputHash, episodesInputErr := workflow.ParseNodeInput(json.RawMessage(episodesProjection.Input))
	var confirmedBibleInput *workflow.NodeInputBinding
	for index := range episodesInput.Bindings {
		if episodesInput.Bindings[index].Port == "bible" {
			confirmedBibleInput = &episodesInput.Bindings[index]
			break
		}
	}
	if episodesInputErr != nil || episodesProjection.InputHash == nil || episodesInputHash != *episodesProjection.InputHash ||
		confirmedBibleInput == nil || confirmedBibleInput.SourceNodeID != gate.NodeID ||
		confirmedBibleInput.ReferenceID != bibleID.String() || confirmedBibleInput.ReferenceVersion != "2" ||
		confirmedBibleInput.ValueType != "production_bible" || confirmedBibleInput.ContentHash != bibleReference.ContentHash {
		t.Fatalf("downstream episode input did not consume confirmed gate output: %#v err=%v", episodesInput, episodesInputErr)
	}
	completion := workflow.CompleteRunCommand{WorkflowRunID: request.WorkflowRunID}
	if err = runtimeService.CompleteRun(ctx, completion); err == nil {
		t.Fatal("run completed while queued nodes still existed")
	}
	if err = database.Model(&model.NodeRunProjection{}).Where("workflow_run_id = ?", request.WorkflowRunID).
		Updates(map[string]any{"status": "SUCCEEDED", "active_claim_token": nil}).Error; err != nil {
		t.Fatalf("prepare completed node projections: %v", err)
	}
	_, completedOutput, completedOutputHash, outputErr := workflow.BuildNodeOutput(successfulExecutorOutput())
	if outputErr != nil {
		t.Fatalf("build completed node output fixture: %v", outputErr)
	}
	_, completedInput, completedInputHash, completionInputErr := workflow.BuildNodeInput(successfulNodeInput())
	if completionInputErr != nil {
		t.Fatalf("build completed node input fixture: %v", completionInputErr)
	}
	if err = database.Model(&model.NodeRunProjection{}).
		Where("workflow_run_id = ? AND risk_level <> ? AND input_hash IS NULL", request.WorkflowRunID, "human_gate").
		Updates(model.NodeRunProjection{Input: []byte(completedInput), InputHash: &completedInputHash}).Error; err != nil {
		t.Fatalf("prepare completed node inputs: %v", err)
	}
	if err = database.Model(&model.NodeRunProjection{}).
		Where("workflow_run_id = ? AND risk_level <> ? AND output_hash IS NULL", request.WorkflowRunID, "human_gate").
		Updates(model.NodeRunProjection{Output: []byte(completedOutput), OutputHash: &completedOutputHash}).Error; err != nil {
		t.Fatalf("prepare completed node outputs: %v", err)
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
