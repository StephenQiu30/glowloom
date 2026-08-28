package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionScriptNodeExecutorReadsAuthorizedImmutableRevision(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Production script node executor journey")
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
	now := time.Date(2026, time.August, 26, 5, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	var revision model.DocumentRevision
	if err = database.First(&revision, "id = ?", fixture.scriptRevisionID).Error; err != nil {
		t.Fatalf("load script revision fixture: %v", err)
	}

	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{"document_revision_id":"` + fixture.scriptRevisionID.String() + `"}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build script node input: %v", err)
	}
	executor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(
			scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
		nil,
		nil,
		nil,
		bibleapp.NewService(
			biblegorm.New(database), bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	command := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "script",
			Executor: "workflow.input.script_revision", Attempt: 1,
		},
		WorkspaceID: revision.WorkspaceID.String(), ProjectID: fixture.projectID.String(),
		InitiatorUserID: fixture.userID.String(), InitiatorTokenVersion: 1,
		IdempotencyKey: "workflow-script-node:" + uuid.NewString(),
		Input:          input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "script", ValueType: "script_revision", Required: true}},
	}
	result, err := executor.Execute(ctx, command)
	if err != nil || result.Status != "SUCCEEDED" || len(result.Output.Bindings) != 1 {
		t.Fatalf("execute production script node: result=%#v err=%v", result, err)
	}
	binding := result.Output.Bindings[0]
	if binding.Port != "script" || binding.ValueType != "script_revision" ||
		binding.ReferenceID != fixture.scriptRevisionID.String() || binding.ReferenceVersion != "1" ||
		binding.ContentHash != fixture.normalizedHash {
		t.Fatalf("production script output = %#v", binding)
	}

	drifted := command
	drifted.Input.FrozenInputs[0].Version = "2"
	_, _, drifted.InputHash, err = workflow.BuildNodeInput(drifted.Input)
	if err != nil {
		t.Fatalf("build drifted script input: %v", err)
	}
	if _, err = executor.Execute(ctx, drifted); err == nil {
		t.Fatal("production script executor accepted a frozen revision version that drifted from PostgreSQL")
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", fixture.userID).
		Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke initiating token version: %v", err)
	}
	if _, err = executor.Execute(ctx, command); err == nil {
		t.Fatal("production script executor continued after the initiating token version was revoked")
	}
}

func TestProductionBibleNodeExecutorDurablyWaitsForOneAuthorizedCandidate(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Production Bible node executor journey")
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
	now := time.Date(2026, time.August, 26, 7, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	var revision model.DocumentRevision
	if err = database.First(&revision, "id = ?", fixture.scriptRevisionID).Error; err != nil {
		t.Fatalf("load Production Bible script revision: %v", err)
	}
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	executor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(
			scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
		nil,
		nil,
		nil,
		bibleService,
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "script", ValueType: "script_revision", SourceKind: workflow.NodeInputSourceNodeOutput,
			SourceNodeID: "script", SourcePort: "script", ReferenceID: fixture.scriptRevisionID.String(),
			ReferenceVersion: "1", ContentHash: fixture.normalizedHash,
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build Production Bible node input: %v", err)
	}
	command := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "bible",
			Executor: "activity.production_bible", Attempt: 1,
		},
		WorkspaceID: revision.WorkspaceID.String(), ProjectID: fixture.projectID.String(),
		InitiatorUserID: fixture.userID.String(), InitiatorTokenVersion: 1,
		IdempotencyKey: "workflow-bible-node:" + uuid.NewString(),
		Input:          input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{
			Key: "candidate", ValueType: "production_bible_candidate", Required: true,
		}},
	}

	pending, err := executor.Execute(ctx, command)
	if err != nil || pending.Status != "RETRYING" || pending.Output.SchemaVersion != "" {
		t.Fatalf("queue Production Bible candidate: result=%#v err=%v", pending, err)
	}
	workerContext, stopWorker := context.WithCancel(ctx)
	worker := bibleapp.NewWorker(
		bibleStore, successfulProductionBibleAgent{}, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	go worker.Run(workerContext)
	t.Cleanup(stopWorker)

	deadline := time.Now().Add(3 * time.Second)
	var result workflow.NodeExecutorResult
	for {
		result, err = executor.Execute(ctx, command)
		if err == nil && result.Status == "SUCCEEDED" {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("await Production Bible candidate: result=%#v err=%v", result, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopWorker()
	if len(result.Output.Bindings) != 1 {
		t.Fatalf("Production Bible output = %#v", result.Output)
	}
	binding := result.Output.Bindings[0]
	if binding.Port != "candidate" || binding.ValueType != "production_bible_candidate" ||
		binding.ReferenceVersion == "" || len(binding.ContentHash) != 64 {
		t.Fatalf("Production Bible candidate binding = %#v", binding)
	}
	var bibleCount, invocationCount, receiptCount int64
	if err = database.Model(&model.ProductionBible{}).Where("id = ?", binding.ReferenceID).Count(&bibleCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AgentInvocation{}).Where("request_id = ?", binding.ReferenceID).Count(&invocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND idempotency_key = ?", "production_bible.create", command.IdempotencyKey).
		Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if bibleCount != 1 || invocationCount != 1 || receiptCount != 1 {
		t.Fatalf("durable Production Bible facts: bible=%d invocation=%d receipt=%d", bibleCount, invocationCount, receiptCount)
	}
}

func TestProductionEpisodePlanCandidateAndStructurePublishRecovery(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Episode Plan node executor journey")
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
	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	if err = database.Model(&model.DocumentRevision{}).Where("id = ?", fixture.scriptRevisionID).
		Updates(explicitEpisodeRevisionValues(t)).Error; err != nil {
		t.Fatalf("prepare explicit episode marker fixture: %v", err)
	}
	bibleID, taskID := uuid.New(), uuid.New()
	resultHash := strings.Repeat("3", 64)
	confirmedAt := now
	confirmedBy := fixture.userID
	if err = database.Table(model.WorkflowTask{}.TableName()).Create(map[string]any{
		"id": taskID, "workspace_id": fixture.workspaceID, "task_type": "production_bible",
		"request_type": "production_bible", "request_id": bibleID, "scope": `{}`, "status": "succeeded",
		"progress_stage": "completed", "cancel_status": "none", "revision": 1, "created_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatalf("seed confirmed Bible task: %v", err)
	}
	workspaceID := fixture.workspaceID
	if err = database.Table(model.ProductionBible{}.TableName()).Create(map[string]any{
		"id": bibleID, "workspace_id": workspaceID, "project_id": fixture.projectID,
		"document_revision_id": fixture.scriptRevisionID, "task_id": taskID, "status": "confirmed",
		"input_hash": fixture.normalizedHash, "result_hash": resultHash, "engine_version": "test-v1",
		"model_name": "deterministic", "prompt_version": "test-v1", "schema_version": "production-bible-v1",
		"harness_version": "test-v1", "checkpoint_revision": 0,
		"candidate": `{"entities":[],"world_entries":[],"review_issues":[]}`, "review_decisions": `{}`, "error": `{}`,
		"revision": 2, "confirmed_at": confirmedAt, "confirmed_by": confirmedBy, "created_by": fixture.userID,
		"created_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatalf("seed confirmed Production Bible: %v", err)
	}

	planningService := planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	executor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil,
		nil,
		nil,
		bibleapp.NewService(biblegorm.New(database), bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningService,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{"episode_count":1}`),
		Bindings: []workflow.NodeInputBinding{
			{Port: "script", ValueType: "script_revision", SourceKind: workflow.NodeInputSourceNodeOutput, SourceNodeID: "script", SourcePort: "script", ReferenceID: fixture.scriptRevisionID.String(), ReferenceVersion: "1", ContentHash: fixture.normalizedHash},
			{Port: "bible", ValueType: "production_bible", SourceKind: workflow.NodeInputSourceNodeOutput, SourceNodeID: "bible-review", SourcePort: "bible", ReferenceID: bibleID.String(), ReferenceVersion: "2", ContentHash: resultHash},
		},
		FrozenInputs: []authoring.FrozenReference{{Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash}},
	})
	if err != nil {
		t.Fatalf("build Episode Plan node input: %v", err)
	}
	command := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "episodes", Executor: "activity.episode_plan", Attempt: 1},
		WorkspaceID:         workspaceID.String(), ProjectID: fixture.projectID.String(), InitiatorUserID: fixture.userID.String(), InitiatorTokenVersion: 1,
		IdempotencyKey: "workflow-episode-plan:" + uuid.NewString(), Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "episode_plan_candidate", Required: true}},
	}
	first, err := executor.Execute(ctx, command)
	if err != nil || first.Status != "SUCCEEDED" || len(first.Output.Bindings) != 1 {
		t.Fatalf("execute Episode Plan candidate node: result=%#v err=%v", first, err)
	}
	second, err := executor.Execute(ctx, command)
	if err != nil || second.Status != "SUCCEEDED" || second.Output.Bindings[0] != first.Output.Bindings[0] {
		t.Fatalf("replay Episode Plan candidate node: first=%#v second=%#v err=%v", first, second, err)
	}
	binding := first.Output.Bindings[0]
	if binding.Port != "candidate" || binding.ValueType != "episode_plan_candidate" || binding.ReferenceVersion != "1" || len(binding.ContentHash) != 64 {
		t.Fatalf("Episode Plan candidate binding = %#v", binding)
	}
	var plan model.EpisodePlan
	if err = database.First(&plan, "id = ?", binding.ReferenceID).Error; err != nil {
		t.Fatalf("load Episode Plan candidate: %v", err)
	}
	var planCount, receiptCount, episodeCount, commitCount int64
	if err = database.Model(&model.EpisodePlan{}).Where("project_id = ?", fixture.projectID).Count(&planCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND idempotency_key = ?", "episode_plan.create", command.IdempotencyKey).Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).Count(&episodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ImportCommit{}).Where("project_id = ?", fixture.projectID).Count(&commitCount).Error; err != nil {
		t.Fatal(err)
	}
	if plan.Status != "review_ready" || plan.TargetDurationMS != 90_000 || plan.RequestedEpisodeCount == nil || *plan.RequestedEpisodeCount != 1 ||
		binding.ContentHash != plan.InputHash || planCount != 1 || receiptCount != 1 || episodeCount != 0 || commitCount != 0 {
		t.Fatalf("Episode Plan candidate facts: plan=%#v plans=%d receipts=%d episodes=%d commits=%d", plan, planCount, receiptCount, episodeCount, commitCount)
	}
	confirmed, err := planningService.ConfirmPlan(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.ConfirmPlanCommand{
		PlanID: plan.ID.String(), ExpectedRevision: 1, IdempotencyKey: "structure-recovery-confirm",
	})
	if err != nil {
		t.Fatalf("confirm recovery Episode Plan: %v", err)
	}
	materialized, err := planningService.Materialize(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.MaterializeCommand{
		PlanID: plan.ID.String(), Mode: "append_new", ExpectedPlanRevision: 2,
		ExpectedProjectRevision: confirmed.View.Impact.ProjectRevision,
		ExpectedActiveOrderHash: confirmed.View.Impact.ActiveOrderHash,
		IdempotencyKey:          "structure-recovery-precommitted-materialization",
	})
	if err != nil || materialized.Status != "materialized" || materialized.Revision != 1 {
		t.Fatalf("precommit recovery materialization: commit=%#v err=%v", materialized, err)
	}
	structureInput, _, structureInputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "episodes", ValueType: "episode_plan", SourceKind: workflow.NodeInputSourceNodeOutput,
			SourceNodeID: "episodes-review", SourcePort: "episodes", ReferenceID: plan.ID.String(),
			ReferenceVersion: "2", ContentHash: plan.InputHash,
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build Episode Structure recovery input: %v", err)
	}
	structureCommand := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "structure",
			Executor: "activity.episode_structure", Attempt: 1,
		},
		WorkspaceID: workspaceID.String(), ProjectID: fixture.projectID.String(),
		InitiatorUserID: fixture.userID.String(), InitiatorTokenVersion: 1,
		IdempotencyKey: "workflow-episode-structure:" + uuid.NewString(), Input: structureInput, InputHash: structureInputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "episode_structure_candidate", Required: true}},
	}
	structureResult, err := executor.Execute(ctx, structureCommand)
	if err != nil || structureResult.Status != "SUCCEEDED" || len(structureResult.Output.Bindings) != 1 {
		t.Fatalf("resume Episode Structure after materialization: result=%#v err=%v", structureResult, err)
	}
	replayedStructure, err := executor.Execute(ctx, structureCommand)
	if err != nil || replayedStructure.Status != "SUCCEEDED" || replayedStructure.Output.Bindings[0] != structureResult.Output.Bindings[0] {
		t.Fatalf("replay published Episode Structure: first=%#v replay=%#v err=%v", structureResult, replayedStructure, err)
	}
	var materializeReceiptCount, publishReceiptCount, structureCount int64
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_plan.materialize", materialized.ID).Count(&materializeReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_plan.publish", materialized.ID).Count(&publishReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).Count(&episodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EpisodeStructure{}).Where("project_id = ?", fixture.projectID).Count(&structureCount).Error; err != nil {
		t.Fatal(err)
	}
	if structureResult.Output.Bindings[0].ReferenceID != materialized.ID || materializeReceiptCount != 1 || publishReceiptCount != 1 || episodeCount != 1 || structureCount != 1 {
		t.Fatalf("Episode Structure recovery facts: binding=%#v receipts=%d/%d episodes=%d structures=%d", structureResult.Output.Bindings[0], materializeReceiptCount, publishReceiptCount, episodeCount, structureCount)
	}
	if err = database.Model(&model.ProductionBible{}).Where("id = ?", bibleID).Update("status", "needs_review").Error; err != nil {
		t.Fatal(err)
	}
	command.IdempotencyKey = "workflow-episode-plan:" + uuid.NewString()
	if _, err = executor.Execute(ctx, command); err == nil {
		t.Fatal("Episode Plan executor accepted a Production Bible that is no longer confirmed")
	}
	if err = database.Model(&model.EpisodePlan{}).Where("project_id = ?", fixture.projectID).Count(&planCount).Error; err != nil || planCount != 1 {
		t.Fatalf("invalid Bible created another Episode Plan: count=%d err=%v", planCount, err)
	}
}

func explicitEpisodeRevisionValues(t *testing.T) map[string]any {
	t.Helper()
	text := "第一集\n《雨巷》\n内景·雨巷·夜\n小兰：走吧"
	markerEnd := utf8.RuneCountInString("第一集")
	titleStart := markerEnd + 1
	titleEnd := titleStart + utf8.RuneCountInString("《雨巷》")
	sceneStart := titleEnd + 1
	blocks, err := json.Marshal([]planningdomain.Block{
		{ID: uuid.NewString(), Position: 1, Kind: "episode_marker", SourceStart: 0, SourceEnd: markerEnd},
		{ID: uuid.NewString(), Position: 2, Kind: "title", SourceStart: titleStart, SourceEnd: titleEnd},
		{ID: uuid.NewString(), Position: 3, Kind: "scene_heading", SourceStart: sceneStart, SourceEnd: utf8.RuneCountInString(text)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"raw_text": text, "normalized_text": text, "codepoint_count": utf8.RuneCountInString(text), "blocks": string(blocks),
	}
}

type successfulProductionBibleAgent struct{}

func (successfulProductionBibleAgent) Invoke(_ context.Context, invocation contract.StageInvocation, _ int, _ int64) (contract.StageResult, error) {
	candidate := json.RawMessage(`{"entities":[],"world_entries":[],"claims":[],"arcs":[],"review_issues":[]}`)
	resultHash, err := contract.CanonicalHash(candidate)
	if err != nil {
		return contract.StageResult{}, err
	}
	return contract.StageResult{
		InvocationID: invocation.InvocationID, Kind: invocation.Kind, WireSchemaVersion: invocation.WireSchemaVersion,
		Stage: invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey, Status: "succeeded",
		CandidateType: "story_analysis_candidate", Candidate: candidate, InputHash: invocation.InputHash,
		ResultHash: &resultHash, Issues: []contract.StageIssue{},
		Executor: contract.Executor{Name: "workflow-test", Version: "1", Model: "deterministic"},
	}, nil
}

var _ bibleapp.AgentClient = successfulProductionBibleAgent{}
