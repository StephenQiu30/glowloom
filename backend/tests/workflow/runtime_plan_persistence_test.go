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
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
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
