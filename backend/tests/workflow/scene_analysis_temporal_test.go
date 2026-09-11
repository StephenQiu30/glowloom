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
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	presetgorm "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/gormdb"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	worldgorm "github.com/StephenQiu30/lanverse/backend/internal/production/world/adapter/gormdb"
	worldapp "github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestSceneAnalysisGatesAndBoundedRepairsResumeRealTemporalWorkflow(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the Scene Analysis gates journey")
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
	presetStore := presetgorm.NewProjectSelectionStore(database)
	_, visualSelection, _ := freezeVisualFoundationPreset(
		t, ctx, presetStore, fixture, now, "structure-identity-temporal-visual-preset:"+fixture.projectID.String(),
	)

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
	deterministicRuntime := &deterministicSceneAnalysisRuntime{now: now, reviewIssue: true}
	sceneService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), deterministicRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 8),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	bibleStore := biblegorm.New(database)
	projectService := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	structureIdentityQuery := bibleapp.NewStructureIdentityQuery(bibleStore, projectService)
	productionWorldService, err := workflowapp.NewProductionWorldAssemblyService(
		workflowStore,
		workflowapp.ProductionWorldAssemblyConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	if err != nil {
		t.Fatal(err)
	}
	productionGraphService := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{
		Now: func() time.Time { return now.Add(2 * time.Minute) }, NewID: uuid.NewString,
	})
	visualSourceService, err := worldapp.NewVisualFoundationSourceService(
		worldgorm.NewVisualFoundationSourceRepository(database),
	)
	if err != nil {
		t.Fatal(err)
	}
	visualStore, err := agentgorm.NewVisualFoundationStore(
		database,
		workflowgorm.ValidateCurrentVisualFoundationInput,
	)
	if err != nil {
		t.Fatal(err)
	}
	visualRuntime := &deterministicVisualFoundationRuntime{now: now}
	visualService, err := agentapp.NewVisualFoundationExecutionService(
		visualStore,
		visualRuntime,
		dispatchSigner,
		agentapp.VisualFoundationExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 8),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeExecutor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(scriptStore, nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil, nil, nil, nil, nil, nil, nil, productionGraphService, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{
			Sources: sourceService, Candidates: sceneService, StructureIdentities: structureIdentityQuery,
			ProductionWorld: productionWorldService,
			VisualFoundation: &workflowproduction.VisualFoundationDependencies{
				Selections: presetStore, FindRelease: presetcatalog.FindCuratedRelease,
				Worlds:  storygraphapp.NewQueryService(storygraphgorm.New(database)),
				Sources: visualSourceService, Candidates: visualService,
			},
		},
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
		query := database.Where(
			"workflow_run_id = ? AND subject_type = ?", started.ID, "structure_identity_gate_input",
		)
		var taskCount int64
		if countErr := query.Model(&model.HumanTask{}).Count(&taskCount).Error; countErr != nil {
			return false, countErr
		}
		if taskCount == 0 {
			return false, nil
		}
		if taskCount != 1 {
			return false, fmt.Errorf("workflow has %d Structure Identity tasks", taskCount)
		}
		loadErr := query.First(&task).Error
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
	reviewDetail, err := reviewService.GetTask(ctx, reviewActor, task.ID.String())
	reviewGate, _, decodeReviewErr := workflow.DecodeStructureIdentityGateInput(reviewDetail.Subject)
	if err != nil || decodeReviewErr != nil || reviewGate.InputHash != gateInput.InputHash {
		t.Fatalf("read frozen Structure Identity review subject: err=%v decode=%v", err, decodeReviewErr)
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
	userNote := "只修复冻结证据指出的首段解释，不扩大范围。"
	validChange := &reviewdomain.ChangeRequest{
		IssueRefs:    append([]string(nil), invalidChange.IssueRefs...),
		EvidenceRefs: append([]reviewdomain.ChangeEvidenceRef(nil), invalidChange.EvidenceRefs...),
		ChangeSpec: reviewdomain.ChangeSpec{
			Operation: change.Operation, TargetKeys: append([]string(nil), change.TargetKeys...),
			AffectedScopeKeys: append([]string(nil), change.AffectedScopeKeys...),
		},
		ReasonCode: "source_interpretation_incorrect", UserNote: &userNote,
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: task.ID.String(), ClaimToken: claim.ClaimToken, Decision: "changes_requested",
		ExpectedTaskRevision: claim.Task.Revision, ExpectedSubjectRevision: claim.Task.SubjectRevision,
		ExpectedSubjectHash: claim.Task.SubjectHash, ChangeRequest: validChange,
		IdempotencyKey: "structure-identity-temporal-decision:" + task.ID.String(),
	})
	if err != nil {
		t.Fatalf("request bounded Structure Identity repair: %v", err)
	}

	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	productionWorldConfirmation := worldapp.NewConfirmationService(
		worldgorm.NewStore(database), func() time.Time { return now }, uuid.NewString,
	)
	signalService := workflowapp.NewSignalService(workflowStore, temporalRuntime, workflowapp.SignalConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Owner: workflowproduction.New(nil, bibleService, projectService, nil, nil, nil, productionWorldConfirmation),
	})
	workflowActor := workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}
	coordinator := workflowapp.NewHumanGateCoordinator(
		workflowreview.NewDecisionReader(reviewService), signalService, workflowStore, startService,
	)
	coordination, err := coordinator.ResumeHumanGate(ctx, workflowActor, decision.Decision.ID)
	if err != nil || coordination.WorkflowResumeStatus != "completed" {
		t.Fatalf("resume bounded Structure Identity decision: coordination=%#v err=%v", coordination, err)
	}

	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var run model.WorkflowRun
		if loadErr := database.First(&run, "id = ?", started.ID).Error; loadErr != nil {
			return false, loadErr
		}
		return run.Status == "NEEDS_ATTENTION" && run.ProgressStage == "human_gate:changes_requested", nil
	})
	coordination, err = coordinator.ResumeHumanGate(ctx, workflowActor, decision.Decision.ID)
	if err != nil || coordination.RepairWorkflowRunID == "" {
		t.Fatalf("start bounded Structure Identity repair: coordination=%#v err=%v", coordination, err)
	}
	var repairRun model.WorkflowRun
	var repairTask model.HumanTask
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		if loadErr := database.First(&repairRun, "id = ?", coordination.RepairWorkflowRunID).Error; loadErr != nil {
			return false, loadErr
		}
		if repairRun.Status != "WAITING_HUMAN" {
			return false, nil
		}
		query := database.Where(
			"workflow_run_id = ? AND subject_type = ?", repairRun.ID, "structure_identity_gate_input",
		)
		var taskCount int64
		if countErr := query.Model(&model.HumanTask{}).Count(&taskCount).Error; countErr != nil {
			return false, countErr
		}
		if taskCount == 0 {
			return false, nil
		}
		if taskCount != 1 {
			return false, fmt.Errorf("repair workflow has %d Structure Identity tasks", taskCount)
		}
		loadErr := query.First(&repairTask).Error
		return loadErr == nil && repairTask.Status == "OPEN", loadErr
	})
	if repairRun.SourceWorkflowRunID == nil || repairRun.SourceWorkflowRunID.String() != started.ID ||
		repairRun.RerunRootNodeID == nil || *repairRun.RerunRootNodeID != "spans" ||
		repairRun.RepairDecisionID == nil || repairRun.RepairDecisionID.String() != decision.Decision.ID ||
		repairRun.RepairDecisionHash == nil || *repairRun.RepairDecisionHash != decision.Decision.DecisionPayloadHash {
		t.Fatalf("bounded repair run identity drifted: %#v", repairRun)
	}
	if repairTask.SubjectID == task.SubjectID || repairTask.SubjectHash == task.SubjectHash ||
		deterministicRuntime.repair == nil || deterministicRuntime.repairStage != "propose_script_spans" ||
		deterministicRuntime.repair.ReviewDecisionID != decision.Decision.ID ||
		deterministicRuntime.repair.DecisionPayloadHash != decision.Decision.DecisionPayloadHash ||
		deterministicRuntime.repairHasNote {
		t.Fatalf("bounded repair did not produce a new frozen review subject: task=%#v repair=%#v runtime=%#v",
			task, repairTask, deterministicRuntime)
	}
	var persistedGateInput model.WorkflowHumanGateInput
	if err = database.First(&persistedGateInput, "id = ?", task.SubjectID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedGateInput.InputHash != gateInput.InputHash || string(persistedGateInput.Input) != string(gateInput.Input) {
		t.Fatal("original Structure Identity Gate input changed during repair")
	}
	var applyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.First(&applyReceipt, "review_decision_id = ?", decision.Decision.ID).Error; err != nil {
		t.Fatal(err)
	}
	var persistedIntent model.WorkflowSignalIntent
	if err = database.First(&persistedIntent, "review_decision_id = ?", decision.Decision.ID).Error; err != nil {
		t.Fatal(err)
	}
	var repairRunCount, applyCount, signalIntentCount, signalReceiptCount int64
	if err = database.Model(&model.WorkflowRun{}).Where(
		"repair_decision_id = ?", decision.Decision.ID,
	).Count(&repairRunCount).Error; err != nil {
		t.Fatal(err)
	}
	for value, count := range map[any]*int64{
		&model.WorkflowHumanGateApplyReceipt{}: &applyCount,
		&model.WorkflowSignalIntent{}:          &signalIntentCount,
		&model.WorkflowSignalReceipt{}:         &signalReceiptCount,
	} {
		if err = database.Model(value).Where("workspace_id = ?", fixture.workspaceID).Count(count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if applyReceipt.DecisionPayloadHash != decision.Decision.DecisionPayloadHash ||
		persistedIntent.DecisionPayloadHash != decision.Decision.DecisionPayloadHash || applyCount != 1 ||
		signalIntentCount != 1 || signalReceiptCount != 1 || repairRunCount != 1 {
		t.Fatalf("Structure Identity Temporal repair facts: repair=%d apply=%d intent=%d receipt=%d",
			repairRunCount, applyCount, signalIntentCount, signalReceiptCount)
	}
	repairClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: repairTask.ID.String(), ExpectedRevision: repairTask.Revision,
		IdempotencyKey: "structure-identity-temporal-repair-claim:" + repairTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("claim repaired Structure Identity review: %v", err)
	}
	repairApproval, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: repairTask.ID.String(), ClaimToken: repairClaim.ClaimToken, Decision: "approved",
		ExpectedTaskRevision: repairClaim.Task.Revision, ExpectedSubjectRevision: repairClaim.Task.SubjectRevision,
		ExpectedSubjectHash: repairClaim.Task.SubjectHash,
		IdempotencyKey:      "structure-identity-temporal-repair-approval:" + repairTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("approve repaired Structure Identity review: %v", err)
	}
	approvalCoordination, err := coordinator.ResumeHumanGate(ctx, workflowActor, repairApproval.Decision.ID)
	if err != nil || approvalCoordination.WorkflowResumeStatus != "completed" {
		t.Fatalf("resume approved Structure Identity repair: coordination=%#v err=%v", approvalCoordination, err)
	}
	var productionWorldTask model.HumanTask
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var current model.WorkflowRun
		if loadErr := database.First(&current, "id = ?", repairRun.ID).Error; loadErr != nil {
			return false, loadErr
		}
		if current.Status != "WAITING_HUMAN" {
			return false, nil
		}
		query := database.Where(
			"workflow_run_id = ? AND subject_type = ?", repairRun.ID, "production_world_gate_input",
		)
		var taskCount int64
		if countErr := query.Model(&model.HumanTask{}).Count(&taskCount).Error; countErr != nil {
			return false, countErr
		}
		if taskCount == 0 {
			return false, nil
		}
		if taskCount != 1 {
			return false, fmt.Errorf("workflow has %d Production World tasks", taskCount)
		}
		loadErr := query.First(&productionWorldTask).Error
		return loadErr == nil && productionWorldTask.Status == "OPEN", loadErr
	})
	assertNoPostProductionWorldFacts(t, func(record any) (int64, error) {
		var count int64
		err := database.Model(record).Where("project_id = ?", fixture.projectID).Count(&count).Error
		return count, err
	})
	productionWorldClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: productionWorldTask.ID.String(), ExpectedRevision: productionWorldTask.Revision,
		IdempotencyKey: "production-world-temporal-claim:" + productionWorldTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("claim Production World review: %v", err)
	}
	var productionWorldGateInput model.WorkflowHumanGateInput
	if err = database.First(&productionWorldGateInput, "id = ?", productionWorldTask.SubjectID).Error; err != nil {
		t.Fatal(err)
	}
	productionWorldGate, _, gateErr := workflow.DecodeProductionWorldGateInput(
		json.RawMessage(productionWorldGateInput.Input),
	)
	if gateErr != nil || productionWorldGate.InputHash != productionWorldTask.SubjectHash ||
		productionWorldGateInput.InputHash != productionWorldTask.SubjectHash {
		t.Fatalf("read frozen Production World Gate input: decode=%v", gateErr)
	}
	productionWorldTaskDetail, err := reviewService.GetTask(ctx, reviewActor, productionWorldTask.ID.String())
	if err != nil {
		t.Fatalf("read Production World review detail: %v", err)
	}
	productionWorldDetail, _, detailErr := workflow.DecodeProductionWorldReviewDetail(
		productionWorldTaskDetail.Subject,
	)
	if detailErr != nil || len(productionWorldDetail.Views.Interactions) != 1 {
		t.Fatalf("decode Production World review detail: detail=%#v err=%v", productionWorldDetail, detailErr)
	}
	interaction := productionWorldDetail.Views.Interactions[0]
	productionWorldSelection := workflow.ProductionWorldRepairSelection{
		Operation: workflow.ProductionWorldRepairReviseInteraction, TargetKeys: []string{interaction.InteractionKey},
	}
	productionWorldClosure, err := workflow.NewProductionWorldRepairClosure(
		productionWorldDetail.Views,
		productionWorldSelection,
	)
	if err != nil {
		t.Fatalf("derive Production World repair closure: %v", err)
	}
	productionWorldNote := "只修正冻结证据覆盖的交互几何，不修改其他制作世界事实。"
	typedProductionWorldChange := workflow.ProductionWorldChangeRequest{
		IssueRefs: []string{},
		EvidenceRefs: []workflow.HumanGateEvidenceRef{{
			SourceVersionID: productionWorldGate.Subject.SourceVersion.VersionID,
			SourceStart:     interaction.Evidence.SourceStart,
			SourceEnd:       interaction.Evidence.SourceEnd,
			TextHash:        interaction.Evidence.TextHash,
		}},
		ChangeSpec: workflow.ProductionWorldRepairChange{
			Operation:         productionWorldSelection.Operation,
			TargetKeys:        productionWorldSelection.TargetKeys,
			AffectedScopeKeys: productionWorldClosure.AllKeys(),
		},
		ReasonCode: "interaction_incorrect", UserNote: &productionWorldNote,
	}
	if err = workflow.ValidateProductionWorldChangeRequest(
		productionWorldGate,
		productionWorldDetail,
		typedProductionWorldChange,
	); err != nil {
		t.Fatalf(
			"validate bounded Production World repair request: targets=%#v closure=%#v err=%v",
			productionWorldDetail.RepairTargets,
			productionWorldClosure,
			err,
		)
	}
	productionWorldDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: productionWorldTask.ID.String(), ClaimToken: productionWorldClaim.ClaimToken,
		Decision: "changes_requested", ExpectedTaskRevision: productionWorldClaim.Task.Revision,
		ExpectedSubjectRevision: productionWorldClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     productionWorldClaim.Task.SubjectHash,
		ChangeRequest: &reviewdomain.ChangeRequest{
			IssueRefs: typedProductionWorldChange.IssueRefs,
			EvidenceRefs: []reviewdomain.ChangeEvidenceRef{{
				SourceVersionID: typedProductionWorldChange.EvidenceRefs[0].SourceVersionID,
				SourceStart:     typedProductionWorldChange.EvidenceRefs[0].SourceStart,
				SourceEnd:       typedProductionWorldChange.EvidenceRefs[0].SourceEnd,
				TextHash:        typedProductionWorldChange.EvidenceRefs[0].TextHash,
			}},
			ChangeSpec: reviewdomain.ChangeSpec{
				Operation:         typedProductionWorldChange.ChangeSpec.Operation,
				TargetKeys:        typedProductionWorldChange.ChangeSpec.TargetKeys,
				AffectedScopeKeys: typedProductionWorldChange.ChangeSpec.AffectedScopeKeys,
			},
			ReasonCode: typedProductionWorldChange.ReasonCode, UserNote: typedProductionWorldChange.UserNote,
		},
		IdempotencyKey: "production-world-temporal-change:" + productionWorldTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("request bounded Production World repair: %v", err)
	}
	productionWorldCoordination, err := coordinator.ResumeHumanGate(
		ctx, workflowActor, productionWorldDecision.Decision.ID,
	)
	if err != nil || productionWorldCoordination.WorkflowResumeStatus != "completed" {
		t.Fatalf("resume bounded Production World decision: coordination=%#v err=%v", productionWorldCoordination, err)
	}
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var current model.WorkflowRun
		if loadErr := database.First(&current, "id = ?", repairRun.ID).Error; loadErr != nil {
			return false, loadErr
		}
		return current.Status == "NEEDS_ATTENTION" && current.ProgressStage == "human_gate:changes_requested", nil
	})
	productionWorldCoordination, err = coordinator.ResumeHumanGate(
		ctx, workflowActor, productionWorldDecision.Decision.ID,
	)
	if err != nil || productionWorldCoordination.RepairWorkflowRunID == "" {
		t.Fatalf("start bounded Production World repair: coordination=%#v err=%v", productionWorldCoordination, err)
	}
	var productionWorldRepairRun model.WorkflowRun
	var repairedProductionWorldTask model.HumanTask
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		if loadErr := database.First(
			&productionWorldRepairRun,
			"id = ?",
			productionWorldCoordination.RepairWorkflowRunID,
		).Error; loadErr != nil {
			return false, loadErr
		}
		if productionWorldRepairRun.Status != "WAITING_HUMAN" {
			return false, nil
		}
		query := database.Where(
			"workflow_run_id = ? AND subject_type = ?",
			productionWorldRepairRun.ID,
			"production_world_gate_input",
		)
		var taskCount int64
		if countErr := query.Model(&model.HumanTask{}).Count(&taskCount).Error; countErr != nil {
			return false, countErr
		}
		if taskCount == 0 {
			return false, nil
		}
		if taskCount != 1 {
			return false, fmt.Errorf("Production World repair workflow has %d review tasks", taskCount)
		}
		loadErr := query.First(&repairedProductionWorldTask).Error
		return loadErr == nil && repairedProductionWorldTask.Status == "OPEN", loadErr
	})
	if productionWorldRepairRun.SourceWorkflowRunID == nil ||
		*productionWorldRepairRun.SourceWorkflowRunID != repairRun.ID ||
		productionWorldRepairRun.RerunRootNodeID == nil ||
		*productionWorldRepairRun.RerunRootNodeID != "interaction-continuity" ||
		productionWorldRepairRun.RepairDecisionID == nil ||
		productionWorldRepairRun.RepairDecisionID.String() != productionWorldDecision.Decision.ID ||
		productionWorldRepairRun.RepairDecisionHash == nil ||
		*productionWorldRepairRun.RepairDecisionHash != productionWorldDecision.Decision.DecisionPayloadHash ||
		deterministicRuntime.productionRepair == nil ||
		deterministicRuntime.productionRepairStage != "reconcile_interaction_continuity" ||
		deterministicRuntime.productionRepairNote {
		t.Fatalf(
			"bounded Production World repair identity drifted: run=%#v runtime=%#v",
			productionWorldRepairRun,
			deterministicRuntime,
		)
	}
	productionWorldRepairClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: repairedProductionWorldTask.ID.String(), ExpectedRevision: repairedProductionWorldTask.Revision,
		IdempotencyKey: "production-world-temporal-repair-claim:" + repairedProductionWorldTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("claim repaired Production World review: %v", err)
	}
	productionWorldApproval, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: repairedProductionWorldTask.ID.String(), ClaimToken: productionWorldRepairClaim.ClaimToken,
		Decision: "approved", ExpectedTaskRevision: productionWorldRepairClaim.Task.Revision,
		ExpectedSubjectRevision: productionWorldRepairClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     productionWorldRepairClaim.Task.SubjectHash,
		IdempotencyKey:          "production-world-temporal-repair-approval:" + repairedProductionWorldTask.ID.String(),
	})
	if err != nil {
		t.Fatalf("approve repaired Production World review: %v", err)
	}
	productionWorldApprovalCoordination, err := coordinator.ResumeHumanGate(
		ctx, workflowActor, productionWorldApproval.Decision.ID,
	)
	if err != nil || productionWorldApprovalCoordination.WorkflowResumeStatus != "completed" {
		t.Fatalf(
			"resume approved Production World repair: coordination=%#v err=%v",
			productionWorldApprovalCoordination,
			err,
		)
	}
	waitForStructureIdentityTemporalFact(t, ctx, func() (bool, error) {
		var current model.WorkflowRun
		if loadErr := database.First(&current, "id = ?", productionWorldRepairRun.ID).Error; loadErr != nil {
			return false, loadErr
		}
		return current.Status == "SUCCEEDED", nil
	})
	var productionWorldApply model.WorkflowHumanGateApplyReceipt
	if err = database.First(
		&productionWorldApply, "review_decision_id = ?", productionWorldApproval.Decision.ID,
	).Error; err != nil {
		t.Fatal(err)
	}
	var productionWorldCommandReceipt model.CommandReceipt
	if productionWorldApply.OwnerReceiptID == nil || productionWorldApply.OwnerOperation == nil ||
		*productionWorldApply.OwnerOperation != worlddomain.ConfirmProductionWorldOperation {
		t.Fatalf("Production World Temporal Apply Receipt = %#v", productionWorldApply)
	}
	if err = database.First(&productionWorldCommandReceipt, "id = ?", *productionWorldApply.OwnerReceiptID).Error; err != nil {
		t.Fatal(err)
	}
	if productionWorldCommandReceipt.Operation != worlddomain.ConfirmProductionWorldOperation ||
		productionWorldCommandReceipt.CreatedBy != fixture.userID {
		t.Fatalf("Production World Temporal CommandReceipt = %#v", productionWorldCommandReceipt)
	}
	var productionGraphNode model.NodeRunProjection
	if err = database.Where(
		"workflow_run_id = ? AND node_id = ?", productionWorldRepairRun.ID, "production-storygraph",
	).First(&productionGraphNode).Error; err != nil {
		t.Fatalf("query Temporal Production StoryGraph node: %v", err)
	}
	if productionGraphNode.Status != "SUCCEEDED" || productionGraphNode.OutputHash == nil {
		t.Fatalf("Temporal Production StoryGraph node = %#v", productionGraphNode)
	}
	productionGraphOutput, _, _, graphOutputErr := workflow.ParseNodeOutput(json.RawMessage(productionGraphNode.Output))
	if graphOutputErr != nil || len(productionGraphOutput.Bindings) != 1 ||
		productionGraphOutput.Bindings[0].ValueType != "storygraph_version" {
		t.Fatalf("Temporal Production StoryGraph output=%#v err=%v", productionGraphOutput, graphOutputErr)
	}
	var productionGraphVersion model.StoryGraphVersion
	if err = database.First(&productionGraphVersion, "id = ?", productionGraphOutput.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatalf("query Temporal Production StoryGraph version: %v", err)
	}
	if productionGraphVersion.SchemaVersion != storygraphdomain.ProductionSchemaID ||
		productionGraphVersion.ContentHash != productionGraphOutput.Bindings[0].ContentHash ||
		len(productionGraphVersion.CompilationInput) == 0 {
		t.Fatalf("Temporal Production StoryGraph version = %#v", productionGraphVersion)
	}
	var presetNode model.NodeRunProjection
	if err = database.Where(
		"workflow_run_id = ? AND node_id = ?", productionWorldRepairRun.ID, "preset-selection",
	).First(&presetNode).Error; err != nil {
		t.Fatalf("query Temporal Project Preset selection node: %v", err)
	}
	presetOutput, _, _, presetOutputErr := workflow.ParseNodeOutput(json.RawMessage(presetNode.Output))
	if presetNode.Status != "SUCCEEDED" || presetNode.OutputHash == nil || presetOutputErr != nil ||
		len(presetOutput.Bindings) != 1 || presetOutput.Bindings[0].ReferenceID != visualSelection.ID ||
		presetOutput.Bindings[0].ContentHash != visualSelection.ContentHash {
		t.Fatalf("Temporal Project Preset selection output=%#v node=%#v err=%v", presetOutput, presetNode, presetOutputErr)
	}
	var visualNode model.NodeRunProjection
	if err = database.Where(
		"workflow_run_id = ? AND node_id = ?", productionWorldRepairRun.ID, "visual-foundation",
	).First(&visualNode).Error; err != nil {
		t.Fatalf("query Temporal Visual Foundation node: %v", err)
	}
	visualOutput, _, _, visualOutputErr := workflow.ParseNodeOutput(json.RawMessage(visualNode.Output))
	if visualNode.Status != "SUCCEEDED" || visualNode.OutputHash == nil || visualOutputErr != nil ||
		len(visualOutput.Bindings) != 1 || visualOutput.Bindings[0].ValueType != "visual_foundation_candidate" {
		t.Fatalf("Temporal Visual Foundation output=%#v node=%#v err=%v", visualOutput, visualNode, visualOutputErr)
	}
	var visualCandidate model.SceneAnalysisCandidateRevision
	if err = database.First(&visualCandidate, "id = ?", visualOutput.Bindings[0].ReferenceID).Error; err != nil ||
		visualCandidate.CandidateType != "visual_foundation_candidate" || visualRuntime.calls == 0 {
		t.Fatalf("Temporal Visual Foundation Candidate=%#v calls=%d err=%v", visualCandidate, visualRuntime.calls, err)
	}
	var productionEntityInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where(
		"workflow_run_id = ? AND stage_key = ?", repairRun.ID, "derive_production_entities",
	).First(&productionEntityInvocation).Error; err != nil {
		t.Fatalf("query Temporal Production Entity invocation: %v", err)
	}
	var productionEntityCandidate model.SceneAnalysisCandidateRevision
	if err = database.First(
		&productionEntityCandidate, "source_invocation_id = ?", productionEntityInvocation.ID,
	).Error; err != nil {
		t.Fatalf("query Temporal Production Entity Candidate: %v", err)
	}
	var productionEntityPayload contract.SceneAnalysisPayload
	var productionEntityInput contract.ProductionEntityDerivationInput
	if err = json.Unmarshal(productionEntityInvocation.Payload, &productionEntityPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(productionEntityPayload.StageInput, &productionEntityInput); err != nil ||
		productionEntityCandidate.CandidateType != "production_entity_fragment_candidate" ||
		contract.ValidateProductionEntityFragmentCandidate(json.RawMessage(productionEntityCandidate.Candidate), productionEntityInput) != nil {
		t.Fatalf("validate Temporal Production Entity Candidate: input=%#v candidate=%#v err=%v",
			productionEntityInput, productionEntityCandidate, err)
	}
	var productionEntityFacts contract.ProductionEntityFragmentCandidate
	if err = json.Unmarshal(productionEntityCandidate.Candidate, &productionEntityFacts); err != nil {
		t.Fatal(err)
	}
	for _, entity := range productionEntityFacts.Entities {
		if _, parseErr := uuid.Parse(entity.IdentityKey); parseErr != nil {
			t.Fatalf("Production Entity used a label instead of the formal identity key %q", entity.IdentityKey)
		}
		var asset model.Asset
		if err = database.Where(
			"project_id = ? AND identity_key = ?", fixture.projectID, entity.IdentityKey,
		).First(&asset).Error; err != nil || asset.Revision != 1 || asset.ID.String() == entity.IdentityKey {
			t.Fatalf("formal Production Asset identity drifted: entity=%#v asset=%#v err=%v", entity, asset, err)
		}
		for _, state := range entity.States {
			var persistedState model.AssetState
			if err = database.Where(
				"asset_id = ? AND state_key = ?", asset.ID, state.StateKey,
			).Order("revision DESC").First(&persistedState).Error; err != nil ||
				persistedState.Revision < 1 || persistedState.ID == asset.ID {
				t.Fatalf("formal Production Asset state drifted: state=%#v persisted=%#v err=%v", state, persistedState, err)
			}
		}
	}
	var sceneBindingInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where(
		"workflow_run_id = ? AND stage_key = ?", repairRun.ID, "bind_scene_occurrences",
	).First(&sceneBindingInvocation).Error; err != nil {
		t.Fatalf("query Temporal Scene binding invocation: %v", err)
	}
	var sceneBindingCandidate model.SceneAnalysisCandidateRevision
	if err = database.First(
		&sceneBindingCandidate, "source_invocation_id = ?", sceneBindingInvocation.ID,
	).Error; err != nil {
		t.Fatalf("query Temporal Scene binding Candidate: %v", err)
	}
	var sceneBindingPayload contract.SceneAnalysisPayload
	var sceneBindingInput contract.SceneOccurrenceBindingInput
	if err = json.Unmarshal(sceneBindingInvocation.Payload, &sceneBindingPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(sceneBindingPayload.StageInput, &sceneBindingInput); err != nil ||
		sceneBindingCandidate.CandidateType != "scene_binding_fragment_candidate" ||
		sceneBindingInput.ProductionEntityCandidateRevisionID != productionEntityCandidate.ID.String() ||
		contract.ValidateSceneBindingFragmentCandidate(
			json.RawMessage(sceneBindingCandidate.Candidate), sceneBindingInput,
		) != nil {
		t.Fatalf("validate Temporal Scene binding Candidate: input=%#v candidate=%#v err=%v",
			sceneBindingInput, sceneBindingCandidate, err)
	}
	var continuityInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where(
		"workflow_run_id = ? AND stage_key = ?", repairRun.ID, "reconcile_interaction_continuity",
	).First(&continuityInvocation).Error; err != nil {
		t.Fatalf("query Temporal Interaction/Continuity invocation: %v", err)
	}
	var continuityCandidate model.SceneAnalysisCandidateRevision
	if err = database.First(
		&continuityCandidate, "source_invocation_id = ?", continuityInvocation.ID,
	).Error; err != nil {
		t.Fatalf("query Temporal Interaction/Continuity Candidate: %v", err)
	}
	var continuityPayload contract.SceneAnalysisPayload
	var continuityInput contract.InteractionContinuityInput
	if err = json.Unmarshal(continuityInvocation.Payload, &continuityPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(continuityPayload.StageInput, &continuityInput); err != nil ||
		continuityCandidate.CandidateType != "continuity_fragment_candidate" ||
		continuityInput.SceneBindingCandidateRevisionID != sceneBindingCandidate.ID.String() ||
		contract.ValidateInteractionContinuityCandidate(
			json.RawMessage(continuityCandidate.Candidate), continuityInput,
		) != nil {
		t.Fatalf("validate Temporal Interaction/Continuity Candidate: input=%#v candidate=%#v err=%v",
			continuityInput, continuityCandidate, err)
	}
	var repairedContinuityInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where(
		"workflow_run_id = ? AND stage_key = ?",
		productionWorldRepairRun.ID,
		"reconcile_interaction_continuity",
	).First(&repairedContinuityInvocation).Error; err != nil {
		t.Fatalf("query repaired Interaction/Continuity invocation: %v", err)
	}
	var repairedContinuityCandidate model.SceneAnalysisCandidateRevision
	if err = database.First(
		&repairedContinuityCandidate,
		"source_invocation_id = ?",
		repairedContinuityInvocation.ID,
	).Error; err != nil {
		t.Fatalf("query repaired Interaction/Continuity Candidate: %v", err)
	}
	var repairedContinuityPayload contract.SceneAnalysisPayload
	var repairedContinuityInput contract.InteractionContinuityInput
	if err = json.Unmarshal(repairedContinuityInvocation.Payload, &repairedContinuityPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(repairedContinuityPayload.StageInput, &repairedContinuityInput); err != nil ||
		repairedContinuityPayload.ProductionRepair == nil ||
		repairedContinuityPayload.ProductionRepair.ReviewDecisionID != productionWorldDecision.Decision.ID ||
		repairedContinuityPayload.ProductionRepair.DecisionPayloadHash != productionWorldDecision.Decision.DecisionPayloadHash ||
		repairedContinuityPayload.ProductionRepair.BaseCandidate.Identity.CandidateRevisionID != continuityCandidate.ID.String() ||
		repairedContinuityInput.SceneBindingCandidateRevisionID != sceneBindingCandidate.ID.String() ||
		repairedContinuityCandidate.CandidateContentHash == continuityCandidate.CandidateContentHash ||
		jsonContainsKey(json.RawMessage(repairedContinuityInvocation.Payload), "user_note") ||
		contract.ValidateInteractionContinuityCandidate(
			json.RawMessage(repairedContinuityCandidate.Candidate),
			repairedContinuityInput,
		) != nil ||
		contract.ValidateProductionWorldRepairCandidate(
			*repairedContinuityPayload.ProductionRepair,
			"reconcile_interaction_continuity",
			json.RawMessage(repairedContinuityCandidate.Candidate),
		) != nil {
		t.Fatalf(
			"validate bounded repaired Interaction/Continuity Candidate: payload=%#v input=%#v candidate=%#v err=%v",
			repairedContinuityPayload,
			repairedContinuityInput,
			repairedContinuityCandidate,
			err,
		)
	}
	var reusedProductionStageInvocationCount int64
	if err = database.Model(&model.SceneAnalysisInvocationRecord{}).Where(
		"workflow_run_id = ? AND stage_key IN ?",
		productionWorldRepairRun.ID,
		[]string{"derive_production_entities", "bind_scene_occurrences"},
	).Count(&reusedProductionStageInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	var reusedProductionStageCount int64
	if err = database.Model(&model.NodeRunProjection{}).Where(
		"workflow_run_id = ? AND node_id IN ? AND status = ? AND reused_from_node_run_id IS NOT NULL",
		productionWorldRepairRun.ID,
		[]string{"production-entities", "scene-bindings"},
		"SKIPPED",
	).Count(&reusedProductionStageCount).Error; err != nil {
		t.Fatal(err)
	}
	if reusedProductionStageInvocationCount != 0 || reusedProductionStageCount != 2 ||
		repairedProductionWorldTask.SubjectID == productionWorldTask.SubjectID ||
		repairedProductionWorldTask.SubjectHash == productionWorldTask.SubjectHash {
		t.Fatalf(
			"Production World repair did not reuse upstream and freeze a new Gate: invocations=%d reused=%d original=%#v repaired=%#v",
			reusedProductionStageInvocationCount,
			reusedProductionStageCount,
			productionWorldTask,
			repairedProductionWorldTask,
		)
	}
}

func assertNoPostProductionWorldFacts(t *testing.T, countProjectFacts func(any) (int64, error)) {
	t.Helper()
	for _, fact := range []struct {
		name  string
		model any
	}{
		{name: "reference target", model: &model.GenerationTarget{}},
		{name: "generation intent", model: &model.GenerationIntent{}},
		{name: "generated media candidate", model: &model.GenerationCandidate{}},
		{name: "generated media selection", model: &model.GenerationCandidateSelection{}},
		{name: "media artifact", model: &model.Artifact{}},
		{name: "StoryGraph version", model: &model.StoryGraphVersion{}},
		{name: "StoryGraph head", model: &model.StoryGraphHead{}},
		{name: "Storyboard draft set", model: &model.StoryboardDraftSet{}},
		{name: "Storyboard batch", model: &model.StoryboardDraftBatch{}},
		{name: "Storyboard shot", model: &model.StoryboardShot{}},
	} {
		count, err := countProjectFacts(fact.model)
		if err != nil {
			t.Fatalf("count pre-Gate 2 %s facts: %v", fact.name, err)
		}
		if count != 0 {
			t.Fatalf("pre-Gate 2 %s fact count = %d, want 0", fact.name, count)
		}
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
