package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
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

func TestWorkflowStartPersistsRunNodeProjectionAndReconcilesUnknownOutcome(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow start journey")
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
	now := time.Date(2026, time.August, 25, 20, 0, 0, 0, time.UTC)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now:   func() time.Time { return now },
		NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "start-authoring-create-1",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "start-authoring-publish-1",
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	starter := &scriptedWorkflowStarter{outcomes: []string{
		"started", "already_started", "already_started_mismatch", "unknown", "already_started",
	}}
	startService := workflowapp.NewStartService(compiler, workflowStore, starter, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}

	started, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-started-1",
	})
	if err != nil || started.Status != "RUNNING" || !strings.HasPrefix(started.TemporalWorkflowID, "lanverse:") {
		t.Fatalf("start workflow: run=%#v err=%v", started, err)
	}
	replayed, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-started-1",
	})
	if err != nil || replayed.ID != started.ID || starter.CallCount() != 1 {
		t.Fatalf("completed start replay invoked Temporal again: run=%#v calls=%d err=%v", replayed, starter.CallCount(), err)
	}

	alreadyStarted, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-already-1",
	})
	if err != nil || alreadyStarted.Status != "RUNNING" {
		t.Fatalf("reconcile matching AlreadyStarted: run=%#v err=%v", alreadyStarted, err)
	}
	conflicted, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-conflict-1",
	})
	if err != nil || conflicted.Status != "NEEDS_ATTENTION" || conflicted.ProgressStage != "start_conflict" {
		t.Fatalf("AlreadyStarted hash conflict was hidden: run=%#v err=%v", conflicted, err)
	}
	unknown, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-unknown-1",
	})
	if err != nil || unknown.Status != "NEEDS_ATTENTION" || unknown.ProgressStage != "start_unknown" {
		t.Fatalf("unknown start result was reported as success: run=%#v err=%v", unknown, err)
	}
	reconciled, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-start-unknown-1",
	})
	if err != nil || reconciled.ID != unknown.ID || reconciled.Status != "RUNNING" || starter.CallCount() != 5 {
		t.Fatalf("unknown start did not reconcile with stable identity: before=%#v after=%#v calls=%d err=%v", unknown, reconciled, starter.CallCount(), err)
	}
	if _, err = startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: draft.ID, IdempotencyKey: "workflow-start-draft-1",
	}); err == nil {
		t.Fatal("workflow start accepted a mutable AuthoringDraft identity")
	}

	var runRecords []model.WorkflowRun
	if err = database.Where("authoring_revision_id = ?", revision.ID).Find(&runRecords).Error; err != nil {
		t.Fatalf("load workflow runs: %v", err)
	}
	if len(runRecords) != 4 {
		t.Fatalf("workflow run count = %d, want 4", len(runRecords))
	}
	runIDs := make([]uuid.UUID, 0, len(runRecords))
	for _, run := range runRecords {
		if run.CreatedBy != fixture.userID || run.InitiatorTokenVersion != actor.TokenVersion {
			t.Fatalf("workflow run lost its initiating actor: %#v", run)
		}
		runIDs = append(runIDs, run.ID)
	}
	var nodeCount int64
	if err = database.Model(&model.NodeRunProjection{}).Where("workflow_run_id IN ?", runIDs).Count(&nodeCount).Error; err != nil {
		t.Fatalf("count node projections: %v", err)
	}
	wantNodeCount := int64(len(compilerJourneyGraph().Nodes) * len(runRecords))
	if nodeCount != wantNodeCount {
		t.Fatalf("node projection count = %d, want %d", nodeCount, wantNodeCount)
	}
	var intentRecords []model.WorkflowStartIntent
	if err = database.Where("workflow_run_id IN ?", runIDs).Find(&intentRecords).Error; err != nil {
		t.Fatalf("load start intents: %v", err)
	}
	if len(intentRecords) != 4 {
		t.Fatalf("start intent count = %d, want 4", len(intentRecords))
	}
	intentIDs := make([]uuid.UUID, 0, len(intentRecords))
	for _, intent := range intentRecords {
		intentIDs = append(intentIDs, intent.ID)
	}
	var receiptCount int64
	if err = database.Model(&model.WorkflowStartReceipt{}).Where("start_intent_id IN ?", intentIDs).Count(&receiptCount).Error; err != nil {
		t.Fatalf("count start receipts: %v", err)
	}
	if receiptCount != 5 {
		t.Fatalf("start receipt count = %d, want 5", receiptCount)
	}
}

type scriptedWorkflowStarter struct {
	mu       sync.Mutex
	outcomes []string
	requests []workflow.StartRequest
}

func (starter *scriptedWorkflowStarter) Start(_ context.Context, request workflow.StartRequest) (workflow.StartObservation, error) {
	starter.mu.Lock()
	defer starter.mu.Unlock()
	starter.requests = append(starter.requests, request)
	if len(starter.outcomes) == 0 {
		return workflow.StartObservation{Outcome: workflow.StartOutcomeUnknown}, nil
	}
	mode := starter.outcomes[0]
	starter.outcomes = starter.outcomes[1:]
	switch mode {
	case "started":
		return workflow.StartObservation{Outcome: workflow.StartOutcomeStarted, ObservedInputHash: request.InputHash}, nil
	case "already_started":
		return workflow.StartObservation{Outcome: workflow.StartOutcomeAlreadyStarted, ObservedInputHash: request.InputHash}, nil
	case "already_started_mismatch":
		return workflow.StartObservation{Outcome: workflow.StartOutcomeAlreadyStarted, ObservedInputHash: strings.Repeat("f", 64)}, nil
	default:
		return workflow.StartObservation{Outcome: workflow.StartOutcomeUnknown}, nil
	}
}

func (starter *scriptedWorkflowStarter) CallCount() int {
	starter.mu.Lock()
	defer starter.mu.Unlock()
	return len(starter.requests)
}
