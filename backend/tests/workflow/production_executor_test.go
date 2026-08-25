package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
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
	executor := workflowproduction.NewNodeExecutor(scriptapp.NewService(
		scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	))
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
