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
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestWorkflowQueryAuthorizesCurrentMembershipAndReturnsNodeProjection(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow query journey")
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
	now := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	owner := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringapp.Actor(owner), authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "workflow-query-authoring-create",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringapp.Actor(owner), authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "workflow-query-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}
	store := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), store, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	starter := &scriptedWorkflowStarter{outcomes: []string{"started"}}
	startService := workflowapp.NewStartService(compiler, store, starter, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, owner, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-query-start",
	})
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	viewerID := uuid.New()
	if err = database.Create(&model.UserAccount{
		ID: viewerID, EmailNormalized: viewerID.String() + "@example.test", PasswordHash: "test-only",
		TokenVersion: 1, DisplayName: "Viewer", Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err = database.Create(&model.Membership{
		ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: viewerID, Role: "viewer", Status: "active",
		JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	outsiderID := uuid.New()
	if err = database.Create(&model.UserAccount{
		ID: outsiderID, EmailNormalized: outsiderID.String() + "@example.test", PasswordHash: "test-only",
		TokenVersion: 1, DisplayName: "Outsider", Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}

	queries := workflowapp.NewQueryService(store)
	view, err := queries.GetRun(ctx, owner, run.ID)
	if err != nil {
		t.Fatalf("query workflow as owner: %v", err)
	}
	actualOrder := make([]string, len(view.Nodes))
	for index, node := range view.Nodes {
		actualOrder[index] = node.NodeID
	}
	wantOrder := []string{
		"script", "evidence", "story", "story-review", "bible-review",
	}
	if view.Run.ID != run.ID || !slices.Equal(actualOrder, wantOrder) {
		t.Fatalf("workflow query projection: run=%#v nodes=%v", view.Run, actualOrder)
	}
	if _, err = queries.GetRun(ctx, workflowapp.Actor{UserID: viewerID.String(), TokenVersion: 1}, run.ID); err != nil {
		t.Fatalf("query workflow as viewer: %v", err)
	}
	if _, err = startService.Start(ctx, workflowapp.Actor{UserID: viewerID.String(), TokenVersion: 1}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-query-viewer-start",
	}); err == nil {
		t.Fatal("viewer was authorized to start a workflow")
	}
	controller := &scriptedController{outcomes: []workflow.ControlObservation{{
		Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request",
	}}}
	controls := workflowapp.NewControlService(store, controller, workflowapp.ControlConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	if _, err = controls.Pause(ctx, workflowapp.Actor{UserID: viewerID.String(), TokenVersion: 1}, workflowapp.PauseCommand{
		WorkflowRunID: run.ID, ExpectedRevision: run.Revision, IdempotencyKey: "workflow-query-viewer-pause",
	}); err == nil || len(controller.Requests()) != 0 {
		t.Fatalf("viewer workflow control err=%v requests=%#v", err, controller.Requests())
	}
	if _, err = queries.GetRun(ctx, workflowapp.Actor{UserID: outsiderID.String(), TokenVersion: 1}, run.ID); err == nil {
		t.Fatal("cross-workspace workflow query was authorized")
	}
	if _, err = queries.GetRun(ctx, workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 2}, run.ID); err == nil {
		t.Fatal("revoked workflow query token was authorized")
	}
	var unchanged model.WorkflowRun
	if err = database.First(&unchanged, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload workflow run: %v", err)
	}
	if unchanged.Revision != run.Revision || unchanged.Status != run.Status {
		t.Fatalf("workflow query wrote run projection: before=%#v after=%#v", run, unchanged)
	}
}
