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

func TestProductionWorkflowWorkerExecutesAuthorizedScriptWorkflow(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the production Workflow Worker journey")
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
	now := time.Date(2026, time.August, 26, 6, 0, 0, 0, time.UTC)
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
		Graph: authoring.Graph{Nodes: []authoring.Node{{
			ID: "script", DefinitionKey: "input.script_revision", DefinitionVersion: "1.0.0",
			Config: json.RawMessage(`{"document_revision_id":"` + fixture.scriptRevisionID.String() + `"}`),
		}}},
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "production-worker-authoring-create",
	})
	if err != nil {
		t.Fatalf("create single-script authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "production-worker-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish single-script authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: "lanverse-production-worker-test-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	scriptService := scriptapp.NewService(
		scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	bibleService := bibleapp.NewService(biblegorm.New(database), bibleapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore, scriptService, bibleService,
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		reviewService,
	)
	if err != nil {
		t.Fatalf("compose production Workflow Runtime: %v", err)
	}
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		t.Fatalf("compose production Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start production Temporal Worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "production-worker-start",
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start single-script Temporal Workflow: run=%#v err=%v", run, err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var persistedRun model.WorkflowRun
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load production Workflow Run: %v", err)
		}
		if persistedRun.Status == "SUCCEEDED" {
			break
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("production Workflow did not complete: %#v", persistedRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var node model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "script").First(&node).Error; err != nil {
		t.Fatalf("load production script Node Run: %v", err)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(node.Output))
	if err != nil || node.Status != "SUCCEEDED" || node.OutputHash == nil || *node.OutputHash != outputHash ||
		len(output.Bindings) != 1 || output.Bindings[0].ReferenceID != fixture.scriptRevisionID.String() ||
		output.Bindings[0].ContentHash != fixture.normalizedHash {
		t.Fatalf("production script Node Run output=%#v projection=%#v err=%v", output, node, err)
	}
}
