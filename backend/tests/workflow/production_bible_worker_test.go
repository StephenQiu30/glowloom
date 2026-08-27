package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorkflowWorkerDurablyCompletesBibleCandidate(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the Production Bible Workflow journey")
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
	now := time.Date(2026, time.August, 26, 7, 30, 0, 0, time.UTC)
	if _, err = authoringgorm.New(database).EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringgorm.New(database), authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	actor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, actor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph: authoring.Graph{
			Nodes: []authoring.Node{
				{ID: "script", DefinitionKey: "input.script_revision", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"` + fixture.scriptRevisionID.String() + `"}`)},
				{ID: "bible", DefinitionKey: "agent.production_bible", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{{ID: "script-to-bible", FromNodeID: "script", FromPort: "script", ToNodeID: "bible", ToPort: "script"}},
		},
		Layout: json.RawMessage(`{"guided":{"step":2}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "production-bible-worker-authoring-create",
	})
	if err != nil {
		t.Fatalf("create Script to Bible authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "production-bible-worker-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish Script to Bible authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: "lanverse-production-bible-worker-test-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	evidenceService := bibleapp.NewSourceEvidenceService(bibleStore, bibleapp.SourceEvidenceConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	storyReviewService := bibleapp.NewStoryReviewService(
		bibleStore,
		bibleapp.NewStoryCandidateRepairService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}),
		bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(
			scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
		evidenceService,
		bibleapp.NewStoryAnalysisService(bibleStore, bibleapp.StoryAnalysisConfig{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, FanIn: 2,
		}),
		storyReviewService,
		bibleService,
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		}),
		nil, nil,
	)
	if err != nil {
		t.Fatalf("compose Production Bible Workflow Runtime: %v", err)
	}
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		t.Fatalf("compose Production Bible Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Production Bible Temporal Worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	agentContext, stopAgent := context.WithCancel(ctx)
	go bibleapp.NewWorker(
		bibleStore, successfulProductionBibleAgent{}, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Run(agentContext)
	t.Cleanup(stopAgent)

	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "production-bible-worker-start",
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start Production Bible Workflow: run=%#v err=%v", run, err)
	}

	deadline := time.Now().Add(20 * time.Second)
	var persistedRun model.WorkflowRun
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load Production Bible Workflow Run: %v", err)
		}
		if persistedRun.Status == "SUCCEEDED" {
			break
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("Production Bible Workflow did not complete: %#v", persistedRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopAgent()
	var nodes []model.NodeRunProjection
	if err = database.Where("workflow_run_id = ?", run.ID).Order("node_id").Find(&nodes).Error; err != nil {
		t.Fatalf("load Production Bible Node Runs: %v", err)
	}
	if len(nodes) != 2 || nodes[0].NodeID != "bible" || nodes[0].Status != "SUCCEEDED" || nodes[0].Attempt < 2 ||
		nodes[0].OutputHash == nil || nodes[1].NodeID != "script" || nodes[1].Status != "SUCCEEDED" {
		t.Fatalf("Production Bible Node Runs = %#v", nodes)
	}
	bibleOutput, _, bibleOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(nodes[0].Output))
	if err != nil || bibleOutputHash != *nodes[0].OutputHash || len(bibleOutput.Bindings) != 1 {
		t.Fatalf("Production Bible node output=%#v hash=%s err=%v", bibleOutput, bibleOutputHash, err)
	}
	binding := bibleOutput.Bindings[0]
	if binding.Port != "candidate" || binding.ValueType != "production_bible_candidate" {
		t.Fatalf("Production Bible candidate output = %#v", binding)
	}
	var bible model.ProductionBible
	if err = database.First(&bible, "id = ?", binding.ReferenceID).Error; err != nil {
		t.Fatalf("load Production Bible candidate: %v", err)
	}
	var invocationCount, receiptCount, candidateRevisionCount, candidateHeadCount int64
	if err = database.Model(&model.AgentInvocation{}).Where("request_id = ?", bible.ID).Count(&invocationCount).Error; err != nil {
		t.Fatal(err)
	}
	var invocation model.AgentInvocation
	if err = database.First(&invocation, "request_id = ?", bible.ID).Error; err != nil {
		t.Fatal(err)
	}
	var executionPolicy contract.StageExecutionPolicy
	if err = json.Unmarshal(invocation.ExecutionPolicy, &executionPolicy); err != nil || executionPolicy.Validate() != nil || executionPolicy.MaxModelCalls != 2 || invocation.Kind != "storygraph_stage" || invocation.Stage != "analyze_story" {
		t.Fatalf("Production Bible execution policy = %#v err=%v", executionPolicy, err)
	}
	var executor contract.Executor
	if err = json.Unmarshal(invocation.Executor, &executor); err != nil || executor.Name == "" || executor.Version == "" || executor.Model == "" {
		t.Fatalf("Production Bible executor identity = %#v err=%v", executor, err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND resource_id = ?", "production_bible.create", bible.ID).
		Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StageCandidateRevision{}).Where("source_invocation_id = ?", invocation.ID).Count(&candidateRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StageCandidateHead{}).Where("stage_instance_key = ?", invocation.StageInstanceKey).Count(&candidateHeadCount).Error; err != nil {
		t.Fatal(err)
	}
	if bible.Status != "needs_review" || bible.ResultHash == nil || *bible.ResultHash != binding.ContentHash ||
		invocationCount != 1 || receiptCount != 1 || candidateRevisionCount != 1 || candidateHeadCount != 1 {
		t.Fatalf("Production Bible facts: bible=%#v invocations=%d receipts=%d revisions=%d heads=%d", bible, invocationCount, receiptCount, candidateRevisionCount, candidateHeadCount)
	}
}
