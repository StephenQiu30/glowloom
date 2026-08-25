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
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorkflowWorkerConfirmsEpisodePlanThroughIndependentHumanGate(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the Episode Plan Workflow journey")
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
	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	if _, err = authoringgorm.New(database).EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	if err = database.Model(&model.DocumentRevision{}).Where("id = ?", fixture.scriptRevisionID).
		Updates(explicitEpisodeRevisionValues(t)).Error; err != nil {
		t.Fatalf("prepare explicit episode marker fixture: %v", err)
	}
	authoringService := authoringapp.NewService(authoringgorm.New(database), authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph: authoring.Graph{
			Nodes: []authoring.Node{
				{ID: "script", DefinitionKey: "input.script_revision", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"` + fixture.scriptRevisionID.String() + `"}`)},
				{ID: "bible", DefinitionKey: "agent.production_bible", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "bible-review", DefinitionKey: "human.production_bible_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "episodes", DefinitionKey: "production.episode_plan", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{"episode_count":1}`)},
				{ID: "episodes-review", DefinitionKey: "human.episode_plan_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{
				{ID: "script-bible", FromNodeID: "script", FromPort: "script", ToNodeID: "bible", ToPort: "script"},
				{ID: "bible-review", FromNodeID: "bible", FromPort: "candidate", ToNodeID: "bible-review", ToPort: "candidate"},
				{ID: "script-episodes", FromNodeID: "script", FromPort: "script", ToNodeID: "episodes", ToPort: "script"},
				{ID: "review-episodes", FromNodeID: "bible-review", FromPort: "bible", ToNodeID: "episodes", ToPort: "bible"},
				{ID: "episodes-review", FromNodeID: "episodes", FromPort: "candidate", ToNodeID: "episodes-review", ToPort: "candidate"},
			},
		},
		Layout: json.RawMessage(`{"guided":{"step":4}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "episode-plan-worker-authoring-create",
	})
	if err != nil {
		t.Fatalf("create Episode Plan authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "episode-plan-worker-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish Episode Plan authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: "lanverse-episode-plan-worker-test-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	projectService := projectapp.NewService(projectgorm.New(database), func() time.Time { return time.Now().UTC() }, uuid.NewString)
	planningService := planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, ClaimLease: time.Minute,
	})
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}),
		bibleService, projectService, planningService, reviewService,
	)
	if err != nil {
		t.Fatalf("compose Episode Plan Workflow Runtime: %v", err)
	}
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		t.Fatalf("compose Episode Plan Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Episode Plan Temporal Worker: %v", err)
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
	run, err := startService.Start(ctx, workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "episode-plan-worker-start",
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start Episode Plan Workflow: run=%#v err=%v", run, err)
	}

	waitGate := func(nodeID string, timeout time.Duration) (model.WorkflowRun, model.NodeRunProjection, model.HumanTask) {
		return waitForWorkflowHumanGate(t, nodeID, timeout, func() (model.WorkflowRun, model.NodeRunProjection, model.HumanTask, bool, error) {
			var persistedRun model.WorkflowRun
			var node model.NodeRunProjection
			var task model.HumanTask
			if loadErr := database.First(&persistedRun, "id = ?", run.ID).Error; loadErr != nil {
				return persistedRun, node, task, false, loadErr
			}
			if loadErr := database.Where("workflow_run_id = ? AND node_id = ?", run.ID, nodeID).First(&node).Error; loadErr != nil {
				return persistedRun, node, task, false, nil
			}
			if persistedRun.Status != "WAITING_HUMAN" || node.Status != "WAITING_HUMAN" {
				return persistedRun, node, task, false, nil
			}
			if loadErr := database.Where("node_run_id = ?", node.ID).First(&task).Error; loadErr != nil {
				return persistedRun, node, task, false, nil
			}
			return persistedRun, node, task, true, nil
		})
	}
	bibleRun, bibleNode, bibleTask := waitGate("bible-review", 20*time.Second)
	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claimed, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: bibleTask.ID.String(), ExpectedRevision: bibleTask.Revision, IdempotencyKey: "episode-plan-bible-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Production Bible review: %v", err)
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: bibleTask.ID.String(), ClaimToken: claimed.ClaimToken, ExpectedTaskRevision: claimed.Task.Revision,
		ExpectedSubjectRevision: claimed.Task.SubjectRevision, Decision: "approved", IdempotencyKey: "episode-plan-bible-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Production Bible review: %v", err)
	}
	signalService := workflowapp.NewSignalService(workflowStore, temporalRuntime, workflowapp.SignalConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, Owner: workflowproduction.New(bibleService, planningService),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: bibleRun.WorkspaceID.String(), WorkflowRunID: run.ID, NodeRunID: bibleNode.ID.String(),
		HumanTaskID: decision.Task.ID, ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: decision.Decision.Decision,
		IdempotencyKey: "episode-plan-bible-review-signal",
	}
	signalDeadline := time.Now().Add(5 * time.Second)
	for {
		intent, signalErr := signalService.SignalHumanGate(ctx, workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, signalCommand)
		if signalErr == nil && intent.Status == "completed" {
			break
		}
		if signalErr != nil || time.Now().After(signalDeadline) {
			t.Fatalf("signal approved Production Bible gate: intent=%#v err=%v", intent, signalErr)
		}
		time.Sleep(25 * time.Millisecond)
	}

	_, planGateNode, planTask := waitGate("episodes-review", 15*time.Second)
	var planNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "episodes").First(&planNode).Error; err != nil {
		t.Fatalf("load Episode Plan node: %v", err)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(planNode.Output))
	if err != nil || planNode.Status != "SUCCEEDED" || planNode.OutputHash == nil || *planNode.OutputHash != outputHash || len(output.Bindings) != 1 {
		t.Fatalf("Episode Plan workflow output=%#v node=%#v err=%v", output, planNode, err)
	}
	binding := output.Bindings[0]
	var candidateIDs []string
	if err = json.Unmarshal(planTask.CandidateIDs, &candidateIDs); err != nil {
		t.Fatalf("decode Episode Plan review candidates: %v", err)
	}
	var plan model.EpisodePlan
	if err = database.First(&plan, "id = ?", binding.ReferenceID).Error; err != nil {
		t.Fatalf("load Workflow Episode Plan: %v", err)
	}
	var planCount, receiptCount, episodeCount, commitCount int64
	if err = database.Model(&model.EpisodePlan{}).Where("project_id = ?", fixture.projectID).Count(&planCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_plan.create", plan.ID).Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).Count(&episodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ImportCommit{}).Where("project_id = ?", fixture.projectID).Count(&commitCount).Error; err != nil {
		t.Fatal(err)
	}
	if binding.Port != "candidate" || binding.ValueType != "episode_plan_candidate" || binding.ReferenceVersion != "1" ||
		binding.ContentHash != plan.InputHash || plan.Status != "review_ready" || plan.RequestedEpisodeCount == nil || *plan.RequestedEpisodeCount != 1 ||
		planGateNode.Status != "WAITING_HUMAN" || len(candidateIDs) != 1 || candidateIDs[0] != plan.ID.String() ||
		planCount != 1 || receiptCount != 1 || episodeCount != 0 || commitCount != 0 {
		t.Fatalf("Episode Plan review boundary: binding=%#v plan=%#v node=%#v candidates=%v counts=%d/%d/%d/%d", binding, plan, planGateNode, candidateIDs, planCount, receiptCount, episodeCount, commitCount)
	}

	claimedPlan, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: planTask.ID.String(), ExpectedRevision: planTask.Revision, IdempotencyKey: "episode-plan-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Episode Plan review: %v", err)
	}
	planDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: planTask.ID.String(), ClaimToken: claimedPlan.ClaimToken, ExpectedTaskRevision: claimedPlan.Task.Revision,
		ExpectedSubjectRevision: claimedPlan.Task.SubjectRevision, Decision: "approved", IdempotencyKey: "episode-plan-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Episode Plan review: %v", err)
	}
	planSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: bibleRun.WorkspaceID.String(), WorkflowRunID: run.ID, NodeRunID: planGateNode.ID.String(),
		HumanTaskID: planDecision.Task.ID, ReviewDecisionID: planDecision.Decision.ID,
		SubjectRevision: planDecision.Decision.SubjectRevision, Decision: planDecision.Decision.Decision,
		IdempotencyKey: "episode-plan-review-signal",
	}
	var planSignal workflow.SignalIntent
	signalDeadline = time.Now().Add(5 * time.Second)
	for {
		planSignal, err = signalService.SignalHumanGate(ctx, workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, planSignalCommand)
		if err == nil && planSignal.Status == "completed" {
			break
		}
		if err != nil || time.Now().After(signalDeadline) {
			t.Fatalf("signal approved Episode Plan gate: intent=%#v err=%v", planSignal, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	replayedPlanSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, planSignalCommand)
	if err != nil || replayedPlanSignal.ID != planSignal.ID || replayedPlanSignal.Status != "completed" {
		t.Fatalf("replay Episode Plan gate signal: intent=%#v err=%v", replayedPlanSignal, err)
	}

	completionDeadline := time.Now().Add(10 * time.Second)
	var completedRun model.WorkflowRun
	for {
		if err = database.First(&completedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load completed Episode Plan Workflow: %v", err)
		}
		if completedRun.Status == "SUCCEEDED" {
			break
		}
		if completedRun.Status == "FAILED" || time.Now().After(completionDeadline) {
			t.Fatalf("Episode Plan Workflow did not complete after review: %#v", completedRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err = database.First(&planGateNode, "id = ?", planGateNode.ID).Error; err != nil {
		t.Fatalf("reload applied Episode Plan gate: %v", err)
	}
	if err = database.First(&plan, "id = ?", plan.ID).Error; err != nil {
		t.Fatalf("reload confirmed Episode Plan: %v", err)
	}
	var proposals []planningdomain.Proposal
	if err = json.Unmarshal(plan.Proposals, &proposals); err != nil {
		t.Fatalf("decode confirmed Episode Plan proposals: %v", err)
	}
	if len(proposals) != 1 || !proposals[0].IsLocked {
		t.Fatalf("confirmed Episode Plan proposals are not locked: %#v", proposals)
	}
	gateOutput, _, gateOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(planGateNode.Output))
	if err != nil || len(gateOutput.Bindings) != 1 || planGateNode.OutputHash == nil || *planGateNode.OutputHash != gateOutputHash {
		t.Fatalf("parse confirmed Episode Plan gate output: output=%#v node=%#v err=%v", gateOutput, planGateNode, err)
	}
	confirmedBinding := gateOutput.Bindings[0]
	var confirmReceipt model.CommandReceipt
	if err = database.Where("operation = ? AND resource_id = ?", "episode_plan.confirm", plan.ID).First(&confirmReceipt).Error; err != nil {
		t.Fatalf("load Episode Plan owner receipt: %v", err)
	}
	replayedConfirmation, err := planningService.ConfirmPlan(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.ConfirmPlanCommand{
		PlanID: plan.ID.String(), ExpectedRevision: 1, IdempotencyKey: "workflow-review:" + planDecision.Decision.ID,
	})
	if err != nil || replayedConfirmation.View.Plan.Revision != 2 || replayedConfirmation.Receipt.ID != confirmReceipt.ID.String() {
		t.Fatalf("replay Episode Plan owner confirmation: result=%#v receipt=%#v err=%v", replayedConfirmation, confirmReceipt, err)
	}
	var confirmReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND resource_id = ?", "episode_plan.confirm", plan.ID).Count(&confirmReceiptCount).Error; err != nil {
		t.Fatalf("count Episode Plan owner receipts: %v", err)
	}
	var applyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.Where("node_run_id = ?", planGateNode.ID).First(&applyReceipt).Error; err != nil {
		t.Fatalf("load Episode Plan gate apply receipt: %v", err)
	}
	var persistedPlanSignal model.WorkflowSignalIntent
	if err = database.First(&persistedPlanSignal, "id = ?", planSignal.ID).Error; err != nil {
		t.Fatalf("load Episode Plan signal intent: %v", err)
	}
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).Count(&episodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ImportCommit{}).Where("project_id = ?", fixture.projectID).Count(&commitCount).Error; err != nil {
		t.Fatal(err)
	}
	if plan.Status != "confirmed" || plan.Revision != 2 || plan.ConfirmedBy == nil || *plan.ConfirmedBy != fixture.userID ||
		planGateNode.Status != "SUCCEEDED" || confirmedBinding.Port != "episodes" || confirmedBinding.ValueType != "episode_plan" ||
		confirmedBinding.ReferenceID != plan.ID.String() || confirmedBinding.ReferenceVersion != "2" || confirmedBinding.ContentHash != plan.InputHash ||
		applyReceipt.OwnerReceiptID == nil || *applyReceipt.OwnerReceiptID != confirmReceipt.ID || applyReceipt.OwnerOperation == nil || *applyReceipt.OwnerOperation != "episode_plan.confirm" ||
		persistedPlanSignal.Status != "completed" || confirmReceiptCount != 1 || episodeCount != 0 || commitCount != 0 {
		t.Fatalf("confirmed Episode Plan boundary: plan=%#v node=%#v binding=%#v apply=%#v signal=%#v effects=%d/%d", plan, planGateNode, confirmedBinding, applyReceipt, persistedPlanSignal, episodeCount, commitCount)
	}
}

func waitForWorkflowHumanGate(
	t *testing.T,
	nodeID string,
	timeout time.Duration,
	load func() (model.WorkflowRun, model.NodeRunProjection, model.HumanTask, bool, error),
) (model.WorkflowRun, model.NodeRunProjection, model.HumanTask) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		run, node, task, found, err := load()
		if err != nil {
			t.Fatalf("load Workflow Run while waiting for %s: %v", nodeID, err)
		}
		if found {
			return run, node, task
		}
		if run.Status == "FAILED" || run.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("Workflow did not reach Human Gate %s: run=%#v node=%#v", nodeID, run, node)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
