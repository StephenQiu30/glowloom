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
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
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
		bibleapp.NewService(
			biblegorm.New(database), bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
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
		bibleService,
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

type successfulProductionBibleAgent struct{}

func (successfulProductionBibleAgent) Invoke(_ context.Context, invocation contract.Invocation) (contract.Result, error) {
	candidate := json.RawMessage(`{"entities":[],"world_entries":[],"review_issues":[]}`)
	resultHash, err := contract.CanonicalHash(candidate)
	if err != nil {
		return contract.Result{}, err
	}
	return contract.Result{
		InvocationID: invocation.InvocationID, Kind: invocation.Kind, InputHash: invocation.InputHash,
		Status: "succeeded", SchemaVersion: contract.SchemaVersion, Candidate: candidate, ResultHash: &resultHash,
		Executor: contract.Executor{Name: "workflow-test", Version: "1", Model: "deterministic"},
	}, nil
}

var _ bibleapp.AgentClient = successfulProductionBibleAgent{}
