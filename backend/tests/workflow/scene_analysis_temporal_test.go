package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestStructureIdentityGateResumesRealTemporalWorkflow(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the Structure Identity journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	now := time.Date(2026, time.September, 9, 8, 0, 0, 0, time.UTC)
	fixture := seedSceneAnalysisProject(t, func(value any) error { return database.Create(value).Error }, now)
	trackCompilerProjectFixture(compilerProjectFixture{
		userID: fixture.userID, workspaceID: fixture.workspaceID, projectID: fixture.projectID,
	})
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist System Catalog: %v", err)
	}

	scriptStore := scriptgorm.New(database)
	sourceService := scriptapp.NewSourceService(scriptStore, scriptapp.SourceConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	if _, err = sourceService.Accept(ctx, scriptapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, scriptapp.AcceptSourceCommand{
		ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(),
		ExpectedHeadRevision: 0, IdempotencyKey: "structure-identity-temporal-source:" + fixture.projectID.String(),
	}); err != nil {
		t.Fatalf("accept Script Source: %v", err)
	}

	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph: sceneAnalysisGraph(fixture.revisionID.String()), Layout: json.RawMessage(`{"guided":{"step":1}}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.revisionID.String(), Version: "1", Hash: fixture.sourceHash,
		}},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "structure-identity-temporal-authoring:" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("create Scene Analysis authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "structure-identity-temporal-publish:" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("publish Scene Analysis authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	dispatchSigner, err := agentgrant.NewSigner(
		"scene-analysis-temporal-test-secret-value",
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	sceneService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), &deterministicSceneAnalysisRuntime{now: now, reviewIssue: true}, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 8),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeExecutor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(scriptStore, nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{Sources: sourceService, Candidates: sceneService},
	)
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 15 * time.Minute,
	})
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString, Executor: nodeExecutor,
		HumanTasks: workflowreview.New(reviewService),
	})
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default",
		TaskQueue: "lanverse-structure-identity-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	runtimeWorker, err := temporalRuntime.NewWorker(runtimeService)
	if err != nil {
		t.Fatalf("compose Structure Identity Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Structure Identity Temporal Worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)

	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	started, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID,
		IdempotencyKey:      "structure-identity-temporal-run:" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("start Structure Identity workflow: %v", err)
	}

	var task model.HumanTask
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var run model.WorkflowRun
		if loadErr := database.First(&run, "id = ?", started.ID).Error; loadErr != nil {
			return false, loadErr
		}
		if run.Status != "WAITING_HUMAN" {
			return false, nil
		}
		loadErr := database.Where(
			"workflow_run_id = ? AND subject_type = ?", started.ID, "structure_identity_gate_input",
		).First(&task).Error
		return loadErr == nil && task.Status == "OPEN", loadErr
	})

	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: task.ID.String(), ExpectedRevision: task.Revision,
		IdempotencyKey: "structure-identity-temporal-claim:" + task.ID.String(),
	})
	if err != nil {
		t.Fatalf("claim Structure Identity review: %v", err)
	}
	var gateInput model.WorkflowHumanGateInput
	if err = database.First(&gateInput, "id = ?", task.SubjectID).Error; err != nil {
		t.Fatal(err)
	}
	gateContract, _, err := workflow.DecodeStructureIdentityGateInput(json.RawMessage(gateInput.Input))
	if err != nil || len(gateContract.RepairOptions) != 1 || len(gateContract.RepairOptions[0].AllowedChanges) != 1 {
		t.Fatalf("load Structure Identity repair option: gate=%#v err=%v", gateContract, err)
	}
	option := gateContract.RepairOptions[0]
	change := option.AllowedChanges[0]
	invalidChange := &reviewdomain.ChangeRequest{
		IssueRefs: []string{option.IssueKey},
		EvidenceRefs: []reviewdomain.ChangeEvidenceRef{{
			SourceVersionID: option.EvidenceRefs[0].SourceVersionID,
			SourceStart:     option.EvidenceRefs[0].SourceStart, SourceEnd: option.EvidenceRefs[0].SourceEnd,
			TextHash: option.EvidenceRefs[0].TextHash,
		}},
		ChangeSpec: reviewdomain.ChangeSpec{
			Operation: change.Operation, TargetKeys: append([]string(nil), change.TargetKeys...),
			AffectedScopeKeys: append(append([]string(nil), change.AffectedScopeKeys...),
				"scene:"+uuid.NewString()),
		},
		ReasonCode: "source_interpretation_incorrect",
	}
	if _, decisionErr := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: task.ID.String(), ClaimToken: claim.ClaimToken, Decision: "changes_requested",
		ExpectedTaskRevision: claim.Task.Revision, ExpectedSubjectRevision: claim.Task.SubjectRevision,
		ExpectedSubjectHash: claim.Task.SubjectHash, ChangeRequest: invalidChange,
		IdempotencyKey: "structure-identity-temporal-invalid-change:" + task.ID.String(),
	}); decisionErr == nil {
		t.Fatal("expanded Structure Identity repair scope was accepted")
	} else {
		var validationError *reviewapp.Error
		if !errors.As(decisionErr, &validationError) || validationError.Status != 422 {
			t.Fatalf("expanded Structure Identity repair scope error=%v", decisionErr)
		}
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: task.ID.String(), ClaimToken: claim.ClaimToken, Decision: "approved",
		ExpectedTaskRevision: claim.Task.Revision, ExpectedSubjectRevision: claim.Task.SubjectRevision,
		ExpectedSubjectHash: claim.Task.SubjectHash,
		IdempotencyKey:      "structure-identity-temporal-decision:" + task.ID.String(),
	})
	if err != nil {
		t.Fatalf("approve Structure Identity review: %v", err)
	}

	bibleService := bibleapp.NewService(biblegorm.New(database), bibleapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	projectService := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	signalService := workflowapp.NewSignalService(workflowStore, temporalRuntime, workflowapp.SignalConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Owner: workflowproduction.New(nil, bibleService, projectService, nil, nil, nil),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: task.NodeRunID.String(),
		HumanTaskID: task.ID.String(), ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: task.SubjectRevision, Decision: "approved",
		IdempotencyKey: "structure-identity-temporal-signal:" + decision.Decision.ID,
	}
	intent, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, signalCommand)
	if err != nil || intent.Status != "completed" {
		t.Fatalf("signal Structure Identity decision: intent=%#v err=%v", intent, err)
	}
	replayed, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, signalCommand)
	if err != nil || replayed.ID != intent.ID || replayed.Status != "completed" {
		t.Fatalf("replay Structure Identity decision: intent=%#v err=%v", replayed, err)
	}

	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var run model.WorkflowRun
		if loadErr := database.First(&run, "id = ?", started.ID).Error; loadErr != nil {
			return false, loadErr
		}
		return run.Status == "SUCCEEDED", nil
	})
	var gate model.NodeRunProjection
	if err = database.First(&gate, "id = ?", task.NodeRunID).Error; err != nil {
		t.Fatal(err)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(gate.Output))
	if err != nil || gate.Status != "SUCCEEDED" || gate.OutputHash == nil || *gate.OutputHash != outputHash ||
		len(output.Bindings) != 1 || output.Bindings[0].ValueType != "structure_identity_set_version" {
		t.Fatalf("Structure Identity Gate output=%#v projection=%#v err=%v", output, gate, err)
	}
	var version model.StructureIdentitySetVersion
	if err = database.First(&version, "id = ?", output.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatal(err)
	}
	var applyCount, signalIntentCount, signalReceiptCount int64
	for value, count := range map[any]*int64{
		&model.WorkflowHumanGateApplyReceipt{}: &applyCount,
		&model.WorkflowSignalIntent{}:          &signalIntentCount,
		&model.WorkflowSignalReceipt{}:         &signalReceiptCount,
	} {
		if err = database.Model(value).Where("workspace_id = ?", fixture.workspaceID).Count(count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if version.ReviewDecisionID.String() != decision.Decision.ID || applyCount != 1 ||
		signalIntentCount != 1 || signalReceiptCount != 1 {
		t.Fatalf("Structure Identity Temporal facts: version=%#v apply=%d intent=%d receipt=%d",
			version, applyCount, signalIntentCount, signalReceiptCount)
	}
}

func waitForStructureIdentityTemporalFact(
	t *testing.T,
	ctx context.Context,
	ready func() (bool, error),
) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, err := ready()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}
