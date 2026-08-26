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
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorkflowWorkerCreatesStoryboardDraftSetForEveryConfirmedEpisode(t *testing.T) {
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
		Updates(explicitTwoEpisodeRevisionValues(t)).Error; err != nil {
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
				{ID: "episodes", DefinitionKey: "production.episode_plan", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{"episode_count":2}`)},
				{ID: "episodes-review", DefinitionKey: "human.episode_plan_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "structure", DefinitionKey: "production.episode_structure", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "structure-review", DefinitionKey: "human.episode_structure_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "storyboard", DefinitionKey: "agent.storyboard_draft", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "storyboard-review", DefinitionKey: "human.storyboard_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "export", DefinitionKey: "production.storyboard_export", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{
				{ID: "script-bible", FromNodeID: "script", FromPort: "script", ToNodeID: "bible", ToPort: "script"},
				{ID: "bible-review", FromNodeID: "bible", FromPort: "candidate", ToNodeID: "bible-review", ToPort: "candidate"},
				{ID: "script-episodes", FromNodeID: "script", FromPort: "script", ToNodeID: "episodes", ToPort: "script"},
				{ID: "review-episodes", FromNodeID: "bible-review", FromPort: "bible", ToNodeID: "episodes", ToPort: "bible"},
				{ID: "episodes-review", FromNodeID: "episodes", FromPort: "candidate", ToNodeID: "episodes-review", ToPort: "candidate"},
				{ID: "episodes-structure", FromNodeID: "episodes-review", FromPort: "episodes", ToNodeID: "structure", ToPort: "episodes"},
				{ID: "structure-review", FromNodeID: "structure", FromPort: "candidate", ToNodeID: "structure-review", ToPort: "candidate"},
				{ID: "review-storyboard", FromNodeID: "structure-review", FromPort: "structures", ToNodeID: "storyboard", ToPort: "structures"},
				{ID: "storyboard-review", FromNodeID: "storyboard", FromPort: "candidate", ToNodeID: "storyboard-review", ToPort: "candidate"},
				{ID: "storyboard-export", FromNodeID: "storyboard-review", FromPort: "storyboards", ToNodeID: "export", ToPort: "storyboards"},
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
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
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
	storyboardStore := storyboardgorm.New(database)
	storyboardService := storyboardapp.NewService(storyboardStore, storyboardapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, ClaimLease: time.Minute,
	})
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}),
		bibleService, projectService, planningService, storyboardService, reviewService,
		nil, nil,
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
	go storyboardapp.NewWorker(
		storyboardStore, successfulStoryboardAgent{}, func() time.Time { return time.Now().UTC() },
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
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, Owner: workflowproduction.New(bibleService, planningService, storyboardService),
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
		binding.ContentHash != plan.InputHash || plan.Status != "review_ready" || plan.RequestedEpisodeCount == nil || *plan.RequestedEpisodeCount != 2 ||
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
	preconfirmedPlan, err := planningService.ConfirmPlan(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.ConfirmPlanCommand{
		PlanID: plan.ID.String(), ExpectedRevision: 1, IdempotencyKey: "workflow-review:" + planDecision.Decision.ID,
	})
	if err != nil || preconfirmedPlan.View.Plan.Status != "confirmed" || preconfirmedPlan.View.Plan.Revision != 2 || preconfirmedPlan.Receipt.ID == "" {
		t.Fatalf("precommit Episode Plan owner confirmation: result=%#v err=%v", preconfirmedPlan, err)
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

	structureRun, structureGateNode, structureTask := waitGate("structure-review", 15*time.Second)
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
	if len(proposals) != 2 || !proposals[0].IsLocked || !proposals[1].IsLocked {
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
	if preconfirmedPlan.Receipt.ID != confirmReceipt.ID.String() {
		t.Fatalf("Episode Plan signal did not replay precommitted owner receipt: precommitted=%#v receipt=%#v", preconfirmedPlan.Receipt, confirmReceipt)
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
	if plan.Status != "materialized" || plan.Revision != 3 || plan.ConfirmedBy == nil || *plan.ConfirmedBy != fixture.userID ||
		planGateNode.Status != "SUCCEEDED" || confirmedBinding.Port != "episodes" || confirmedBinding.ValueType != "episode_plan" ||
		confirmedBinding.ReferenceID != plan.ID.String() || confirmedBinding.ReferenceVersion != "2" || confirmedBinding.ContentHash != plan.InputHash ||
		applyReceipt.OwnerReceiptID == nil || *applyReceipt.OwnerReceiptID != confirmReceipt.ID || applyReceipt.OwnerOperation == nil || *applyReceipt.OwnerOperation != "episode_plan.confirm" ||
		persistedPlanSignal.Status != "completed" || confirmReceiptCount != 1 || episodeCount != 2 || commitCount != 1 {
		t.Fatalf("confirmed Episode Plan boundary: plan=%#v node=%#v binding=%#v apply=%#v signal=%#v effects=%d/%d", plan, planGateNode, confirmedBinding, applyReceipt, persistedPlanSignal, episodeCount, commitCount)
	}

	var structureNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "structure").First(&structureNode).Error; err != nil {
		t.Fatalf("load Episode Structure node: %v", err)
	}
	structureOutput, _, structureOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(structureNode.Output))
	if err != nil || len(structureOutput.Bindings) != 1 || structureNode.OutputHash == nil || *structureNode.OutputHash != structureOutputHash {
		t.Fatalf("parse Episode Structure candidate output: output=%#v node=%#v err=%v", structureOutput, structureNode, err)
	}
	structureBinding := structureOutput.Bindings[0]
	var structureCandidates []string
	if err = json.Unmarshal(structureTask.CandidateIDs, &structureCandidates); err != nil {
		t.Fatalf("decode Episode Structure review candidates: %v", err)
	}
	var commit model.ImportCommit
	if err = database.First(&commit, "id = ?", structureBinding.ReferenceID).Error; err != nil {
		t.Fatalf("load published ImportCommit: %v", err)
	}
	var episode model.Episode
	if err = database.Where("project_id = ?", fixture.projectID).First(&episode).Error; err != nil {
		t.Fatalf("load materialized Episode: %v", err)
	}
	var scriptVersion model.EpisodeScriptVersion
	if err = database.Where("episode_id = ?", episode.ID).First(&scriptVersion).Error; err != nil {
		t.Fatalf("load published Episode Script Version: %v", err)
	}
	var structure model.EpisodeStructure
	if err = database.Where("episode_id = ?", episode.ID).First(&structure).Error; err != nil {
		t.Fatalf("load Episode Structure candidate: %v", err)
	}
	batch, err := planningService.GetPublishedStructureBatch(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, plan.ID.String())
	if err != nil {
		t.Fatalf("load published Episode Structure batch: %v", err)
	}
	var materializeReceiptCount, publishReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_plan.materialize", commit.ID).Count(&materializeReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_plan.publish", commit.ID).Count(&publishReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if structureRun.Status != "WAITING_HUMAN" || structureNode.Status != "SUCCEEDED" || structureGateNode.Status != "WAITING_HUMAN" ||
		structureBinding.Port != "candidate" || structureBinding.ValueType != "episode_structure_candidate" ||
		structureBinding.ReferenceID != commit.ID.String() || structureBinding.ReferenceVersion != "2" || structureBinding.ContentHash != batch.ContentHash ||
		len(structureCandidates) != 1 || structureCandidates[0] != commit.ID.String() || commit.Status != "published" || commit.Revision != 2 ||
		episode.Status != "active" || episode.Revision != 2 || episode.CurrentScriptVersionID == nil || *episode.CurrentScriptVersionID != scriptVersion.ID ||
		scriptVersion.Status != "published" || structure.Status != "needs_review" || structure.Revision != 1 || len(structure.ResultHash) != 64 ||
		materializeReceiptCount != 1 || publishReceiptCount != 1 {
		t.Fatalf("published Episode Structure boundary: run=%#v node=%#v gate=%#v binding=%#v candidates=%v commit=%#v episode=%#v version=%#v structure=%#v receipts=%d/%d", structureRun, structureNode, structureGateNode, structureBinding, structureCandidates, commit, episode, scriptVersion, structure, materializeReceiptCount, publishReceiptCount)
	}

	planningActor := planningapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	if _, err = planningService.ConfirmPublishedStructureBatch(ctx, planningActor, planningapp.ConfirmStructureBatchCommand{
		CommitID: commit.ID.String(), ExpectedRevision: 2, ExpectedContentHash: batch.ContentHash,
		IdempotencyKey: "episode-structure-premature-confirm",
	}); err == nil {
		t.Fatal("Episode Structure batch confirmation succeeded before required tasks were accepted")
	}
	for _, candidate := range batch.Structures {
		current := candidate
		for _, scene := range candidate.Scenes {
			for _, task := range scene.Tasks {
				if !task.Required {
					continue
				}
				current, err = planningService.AcceptTask(ctx, planningActor, planningapp.AcceptTaskCommand{
					StructureCommand: planningapp.StructureCommand{
						StructureID: current.ID, ExpectedRevision: current.Revision,
						IdempotencyKey: "episode-structure-accept:" + task.ID,
					},
					TaskID: task.ID,
				})
				if err != nil {
					t.Fatalf("accept required Episode Structure task %s: %v", task.ID, err)
				}
			}
		}
	}
	structureReviewerID := uuid.New()
	if err = database.Create(&model.UserAccount{
		ID: structureReviewerID, EmailNormalized: structureReviewerID.String() + "@example.test", PasswordHash: "test-only",
		TokenVersion: 1, DisplayName: "Structure Reviewer", Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create Structure reviewer: %v", err)
	}
	if err = database.Create(&model.Membership{
		ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: structureReviewerID,
		Role: "editor", Status: "active", JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create Structure reviewer membership: %v", err)
	}
	structureReviewActor := reviewapp.Actor{UserID: structureReviewerID.String(), TokenVersion: 1}
	claimedStructure, err := reviewService.Claim(ctx, structureReviewActor, reviewapp.ClaimCommand{
		TaskID: structureTask.ID.String(), ExpectedRevision: structureTask.Revision,
		IdempotencyKey: "episode-structure-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Episode Structure review: %v", err)
	}
	structureDecision, err := reviewService.Decide(ctx, structureReviewActor, reviewapp.DecideCommand{
		TaskID: structureTask.ID.String(), ClaimToken: claimedStructure.ClaimToken,
		ExpectedTaskRevision: claimedStructure.Task.Revision, ExpectedSubjectRevision: claimedStructure.Task.SubjectRevision,
		Decision: "approved", IdempotencyKey: "episode-structure-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Episode Structure review: %v", err)
	}
	preconfirmedStructures, err := planningService.ConfirmPublishedStructureBatch(ctx, planningapp.Actor{
		UserID: structureReviewerID.String(), TokenVersion: 1,
	}, planningapp.ConfirmStructureBatchCommand{
		CommitID: commit.ID.String(), ExpectedRevision: 2, ExpectedContentHash: batch.ContentHash,
		IdempotencyKey: "workflow-review:" + structureDecision.Decision.ID,
	})
	if err != nil || preconfirmedStructures.Receipt.ID == "" || len(preconfirmedStructures.Batch.Structures) != len(batch.Structures) {
		t.Fatalf("precommit Episode Structure batch confirmation: result=%#v err=%v", preconfirmedStructures, err)
	}
	structureSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: structureRun.WorkspaceID.String(), WorkflowRunID: run.ID, NodeRunID: structureGateNode.ID.String(),
		HumanTaskID: structureDecision.Task.ID, ReviewDecisionID: structureDecision.Decision.ID,
		SubjectRevision: structureDecision.Decision.SubjectRevision, Decision: structureDecision.Decision.Decision,
		IdempotencyKey: "episode-structure-review-signal",
	}
	var structureSignal workflow.SignalIntent
	signalDeadline = time.Now().Add(5 * time.Second)
	for {
		structureSignal, err = signalService.SignalHumanGate(ctx, workflowapp.Actor{
			UserID: structureReviewerID.String(), TokenVersion: 1,
		}, structureSignalCommand)
		if err == nil && structureSignal.Status == "completed" {
			break
		}
		if err != nil || time.Now().After(signalDeadline) {
			t.Fatalf("signal approved Episode Structure gate: intent=%#v err=%v", structureSignal, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	replayedStructureSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: structureReviewerID.String(), TokenVersion: 1,
	}, structureSignalCommand)
	if err != nil || replayedStructureSignal.ID != structureSignal.ID || replayedStructureSignal.Status != "completed" {
		t.Fatalf("replay Episode Structure gate signal: intent=%#v err=%v", replayedStructureSignal, err)
	}

	storyboardRun, storyboardGateNode, storyboardTask := waitGate("storyboard-review", 20*time.Second)
	completedRun := storyboardRun
	if err = database.First(&structureGateNode, "id = ?", structureGateNode.ID).Error; err != nil {
		t.Fatalf("reload applied Episode Structure gate: %v", err)
	}
	formalStructures, _, formalStructuresHash, err := workflow.ParseNodeOutput(json.RawMessage(structureGateNode.Output))
	if err != nil || len(formalStructures.Bindings) != 1 || structureGateNode.OutputHash == nil || *structureGateNode.OutputHash != formalStructuresHash {
		t.Fatalf("parse confirmed Episode Structure gate output: output=%#v node=%#v err=%v", formalStructures, structureGateNode, err)
	}
	formalBinding := formalStructures.Bindings[0]
	confirmedBatch, err := planningService.GetPublishedStructureBatch(ctx, planningActor, plan.ID.String())
	if err != nil {
		t.Fatalf("load confirmed Episode Structure batch: %v", err)
	}
	var batchReceipt model.CommandReceipt
	if err = database.Where("operation = ? AND resource_id = ?", "episode_structure.confirm_batch", commit.ID).First(&batchReceipt).Error; err != nil {
		t.Fatalf("load Episode Structure batch receipt: %v", err)
	}
	var batchReceiptCount, individualConfirmReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "episode_structure.confirm_batch", commit.ID).Count(&batchReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ?", structureRun.WorkspaceID, "episode_structure.confirm",
	).Count(&individualConfirmReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	var structureApply model.WorkflowHumanGateApplyReceipt
	if err = database.Where("node_run_id = ?", structureGateNode.ID).First(&structureApply).Error; err != nil {
		t.Fatalf("load Episode Structure gate apply receipt: %v", err)
	}
	for _, confirmed := range confirmedBatch.Structures {
		if confirmed.Status != "confirmed" || confirmed.ConfirmedBy == nil || *confirmed.ConfirmedBy != structureReviewerID.String() {
			t.Fatalf("Episode Structure was not confirmed by batch owner: %#v", confirmed)
		}
		for _, scene := range confirmed.Scenes {
			for _, task := range scene.Tasks {
				if task.Required && task.Status != "accepted" {
					t.Fatalf("confirmed Episode Structure retained an unaccepted required task: %#v", task)
				}
			}
		}
	}
	if structureGateNode.Status != "SUCCEEDED" || formalBinding.Port != "structures" || formalBinding.ValueType != "episode_structures" ||
		formalBinding.ReferenceID != commit.ID.String() || formalBinding.ReferenceVersion != "2" || formalBinding.ContentHash != batch.ContentHash ||
		confirmedBatch.ContentHash != batch.ContentHash || preconfirmedStructures.Receipt.ID != batchReceipt.ID.String() ||
		structureApply.OwnerReceiptID == nil || *structureApply.OwnerReceiptID != batchReceipt.ID ||
		structureApply.OwnerOperation == nil || *structureApply.OwnerOperation != "episode_structure.confirm_batch" ||
		batchReceiptCount != 1 || individualConfirmReceiptCount != 0 {
		t.Fatalf("confirmed Episode Structure batch boundary: run=%#v gate=%#v binding=%#v batch=%#v preconfirmed=%#v receipt=%#v apply=%#v counts=%d/%d", completedRun, structureGateNode, formalBinding, confirmedBatch, preconfirmedStructures, batchReceipt, structureApply, batchReceiptCount, individualConfirmReceiptCount)
	}

	var storyboardNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "storyboard").First(&storyboardNode).Error; err != nil {
		t.Fatalf("load Storyboard Draft Set node: %v", err)
	}
	storyboardOutput, _, storyboardOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(storyboardNode.Output))
	if err != nil || len(storyboardOutput.Bindings) != 1 || storyboardNode.OutputHash == nil || *storyboardNode.OutputHash != storyboardOutputHash {
		t.Fatalf("parse Storyboard Draft Set output: output=%#v node=%#v err=%v", storyboardOutput, storyboardNode, err)
	}
	storyboardBinding := storyboardOutput.Bindings[0]
	var set model.StoryboardDraftSet
	if err = database.First(&set, "id = ?", storyboardBinding.ReferenceID).Error; err != nil {
		t.Fatalf("load Storyboard Draft Set: %v", err)
	}
	var setBatches []struct {
		BatchID           string  `json:"batch_id"`
		EpisodeID         string  `json:"episode_id"`
		StructureID       string  `json:"structure_id"`
		ScriptVersionID   string  `json:"script_version_id"`
		InputHash         string  `json:"input_hash"`
		BaselineOrderHash string  `json:"baseline_order_hash"`
		ResultHash        *string `json:"result_hash"`
	}
	if err = json.Unmarshal(set.Batches, &setBatches); err != nil {
		t.Fatalf("decode Storyboard Draft Set batches: %v", err)
	}
	setBatchIDs := make([]string, len(setBatches))
	for index, reference := range setBatches {
		setBatchIDs[index] = reference.BatchID
	}
	var storyboardCandidates []string
	if err = json.Unmarshal(storyboardTask.CandidateIDs, &storyboardCandidates); err != nil {
		t.Fatalf("decode Storyboard review candidates: %v", err)
	}
	var setCount, draftBatchCount, invocationCount, createSetReceiptCount int64
	if err = database.Model(&model.StoryboardDraftSet{}).Where("structure_commit_id = ?", commit.ID).Count(&setCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StoryboardDraftBatch{}).Where("project_id = ?", fixture.projectID).Count(&draftBatchCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AgentInvocation{}).Where(
		"kind = ? AND status = ? AND request_id IN ?", "storyboard_draft", "succeeded", setBatchIDs,
	).Count(&invocationCount).Error; err != nil {
		t.Fatal(err)
	}
	var storyboardInvocations []model.AgentInvocation
	if err = database.Where("request_id IN ?", setBatchIDs).Find(&storyboardInvocations).Error; err != nil {
		t.Fatal(err)
	}
	for _, invocation := range storyboardInvocations {
		var executionPolicy contract.ExecutionPolicy
		if err = json.Unmarshal(invocation.ExecutionPolicy, &executionPolicy); err != nil || executionPolicy.ValidateFor("storyboard_draft") != nil || executionPolicy.MaxModelCalls != 1 {
			t.Fatalf("Storyboard execution policy = %#v err=%v", executionPolicy, err)
		}
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "storyboard.create_set", set.ID).Count(&createSetReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	for _, reference := range setBatches {
		var draft model.StoryboardDraftBatch
		if err = database.First(&draft, "id = ?", reference.BatchID).Error; err != nil {
			t.Fatalf("load Storyboard Draft Batch %s: %v", reference.BatchID, err)
		}
		if draft.EpisodeID.String() != reference.EpisodeID || draft.StructureID.String() != reference.StructureID ||
			draft.ScriptVersionID.String() != reference.ScriptVersionID || draft.InputHash != reference.InputHash ||
			draft.ResultHash == nil || reference.ResultHash == nil || *draft.ResultHash != *reference.ResultHash || draft.Status != "needs_review" {
			t.Fatalf("Storyboard Draft Set batch drifted: set=%#v batch=%#v", reference, draft)
		}
	}
	if storyboardRun.Status != "WAITING_HUMAN" || storyboardNode.Status != "SUCCEEDED" || storyboardGateNode.Status != "WAITING_HUMAN" ||
		storyboardBinding.Port != "candidate" || storyboardBinding.ValueType != "storyboard_candidate" ||
		storyboardBinding.ReferenceID != set.ID.String() || storyboardBinding.ReferenceVersion != "2" ||
		set.ResultHash == nil || storyboardBinding.ContentHash != *set.ResultHash || set.Status != "needs_review" || set.Revision != 2 ||
		set.StructureCommitID != commit.ID || set.StructureRevision != 2 || set.StructureContentHash != batch.ContentHash ||
		len(setBatches) != 2 || len(storyboardCandidates) != 1 || storyboardCandidates[0] != set.ID.String() ||
		setCount != 1 || draftBatchCount != 2 || invocationCount != 2 || createSetReceiptCount != 1 {
		t.Fatalf("Storyboard Draft Set boundary: run=%#v node=%#v gate=%#v binding=%#v set=%#v batches=%#v candidates=%v counts=%d/%d/%d/%d", storyboardRun, storyboardNode, storyboardGateNode, storyboardBinding, set, setBatches, storyboardCandidates, setCount, draftBatchCount, invocationCount, createSetReceiptCount)
	}

	storyboardActor := storyboardapp.Actor{UserID: structureReviewerID.String(), TokenVersion: 1}
	if _, err = storyboardService.ApplySet(ctx, storyboardActor, storyboardapp.ApplySetCommand{
		SetID: set.ID.String(), ExpectedRevision: 2, ExpectedCandidateHash: *set.ResultHash,
		IdempotencyKey: "storyboard-set-premature-apply",
	}); err == nil {
		t.Fatal("Storyboard Draft Set applied before every Batch was approved")
	}
	for _, reference := range setBatches {
		draft, loadErr := storyboardService.GetBatch(ctx, storyboardActor, reference.BatchID)
		if loadErr != nil {
			t.Fatalf("load Storyboard Batch for review %s: %v", reference.BatchID, loadErr)
		}
		for _, shot := range draft.Candidate.Shots {
			draft, err = storyboardService.Decide(ctx, storyboardActor, storyboardapp.DecisionCommand{
				BatchID: draft.ID, ProposalKey: shot.ProposalKey, Action: "accepted", ExpectedRevision: draft.Revision,
				IdempotencyKey: "workflow-storyboard-accept:" + draft.ID + ":" + shot.ProposalKey,
			})
			if err != nil {
				t.Fatalf("accept Storyboard candidate %s/%s: %v", draft.ID, shot.ProposalKey, err)
			}
		}
		draft, err = storyboardService.Approve(ctx, storyboardActor, storyboardapp.RevisionCommand{
			BatchID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "workflow-storyboard-approve:" + draft.ID,
		})
		if err != nil || draft.Status != "approved" {
			t.Fatalf("approve Storyboard Batch %s: batch=%#v err=%v", draft.ID, draft, err)
		}
		preflight, preflightErr := storyboardService.PreflightApply(ctx, storyboardActor, draft.ID, draft.Revision)
		if preflightErr != nil || preflight.Created != len(draft.Candidate.Shots) {
			t.Fatalf("preflight Storyboard Batch %s: preflight=%#v err=%v", draft.ID, preflight, preflightErr)
		}
	}
	staleReference := setBatches[len(setBatches)-1]
	staleShotID := uuid.New()
	if err = database.Create(&model.StoryboardShot{
		ID: staleShotID, WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID,
		EpisodeID: uuid.MustParse(staleReference.EpisodeID), BatchID: uuid.MustParse(staleReference.BatchID),
		ProposalKey: "concurrent-shot", Position: 99, Title: "并发正式镜头",
		NarrativeUnitIDs: []byte(`[]`), Spec: []byte(`{"duration_ms":1000}`),
		ContentHash: staleReference.InputHash, Status: "active", Revision: 1,
		CreatedBy: structureReviewerID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed concurrent formal Shot: %v", err)
	}
	if _, err = storyboardService.ApplySet(ctx, storyboardActor, storyboardapp.ApplySetCommand{
		SetID: set.ID.String(), ExpectedRevision: 2, ExpectedCandidateHash: *set.ResultHash,
		IdempotencyKey: "storyboard-set-stale-baseline",
	}); err == nil {
		t.Fatal("Storyboard Draft Set accepted a changed formal Shot baseline")
	}
	var rolledBackFirstBatch model.StoryboardDraftBatch
	var rolledBackSet model.StoryboardDraftSet
	var rolledBackFirstEpisodeShots int64
	if err = database.First(&rolledBackFirstBatch, "id = ?", setBatches[0].BatchID).Error; err != nil {
		t.Fatalf("load first Storyboard Batch after rejected Set apply: %v", err)
	}
	if err = database.First(&rolledBackSet, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("load Storyboard Draft Set after rejected apply: %v", err)
	}
	if err = database.Model(&model.StoryboardShot{}).
		Where("batch_id = ? AND status = ?", setBatches[0].BatchID, "active").
		Count(&rolledBackFirstEpisodeShots).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBackFirstBatch.Status != "approved" || rolledBackSet.Status != "needs_review" ||
		rolledBackSet.Revision != 2 || rolledBackFirstEpisodeShots != 0 {
		t.Fatalf("Storyboard Draft Set did not roll back atomically: batch=%#v set=%#v first_episode_shots=%d", rolledBackFirstBatch, rolledBackSet, rolledBackFirstEpisodeShots)
	}
	if err = database.Delete(&model.StoryboardShot{}, "id = ?", staleShotID).Error; err != nil {
		t.Fatalf("remove concurrent formal Shot fixture: %v", err)
	}

	claimedStoryboard, err := reviewService.Claim(ctx, structureReviewActor, reviewapp.ClaimCommand{
		TaskID: storyboardTask.ID.String(), ExpectedRevision: storyboardTask.Revision,
		IdempotencyKey: "storyboard-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Storyboard review: %v", err)
	}
	storyboardDecision, err := reviewService.Decide(ctx, structureReviewActor, reviewapp.DecideCommand{
		TaskID: storyboardTask.ID.String(), ClaimToken: claimedStoryboard.ClaimToken,
		ExpectedTaskRevision: claimedStoryboard.Task.Revision, ExpectedSubjectRevision: claimedStoryboard.Task.SubjectRevision,
		Decision: "approved", IdempotencyKey: "storyboard-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Storyboard review: %v", err)
	}
	preappliedSet, err := storyboardService.ApplySet(ctx, storyboardActor, storyboardapp.ApplySetCommand{
		SetID: set.ID.String(), ExpectedRevision: 2, ExpectedCandidateHash: *set.ResultHash,
		IdempotencyKey: "workflow-review:" + storyboardDecision.Decision.ID,
	})
	if err != nil || preappliedSet.Set.Status != "applied" || preappliedSet.Receipt.ID == "" {
		t.Fatalf("precommit Storyboard Set apply: result=%#v err=%v", preappliedSet, err)
	}
	var exportNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "export").First(&exportNode).Error; err != nil {
		t.Fatalf("load pending Storyboard Export node: %v", err)
	}
	exportActor := storyboardapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	var driftedExportShot model.StoryboardShot
	if err = database.Where("episode_id = ? AND status = ?", setBatches[len(setBatches)-1].EpisodeID, "active").
		First(&driftedExportShot).Error; err != nil {
		t.Fatalf("load formal Shot for export drift: %v", err)
	}
	originalExportShotHash := driftedExportShot.ContentHash
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", driftedExportShot.ID).
		Update("content_hash", strings.Repeat("9", 64)).Error; err != nil {
		t.Fatalf("drift formal Shot before export: %v", err)
	}
	if _, err = storyboardService.CreateExportSet(ctx, exportActor, storyboardapp.CreateExportSetCommand{
		SetID: preappliedSet.Set.ID, ExpectedRevision: preappliedSet.Set.Revision,
		ExpectedResultHash: *preappliedSet.Set.ResultHash, IdempotencyKey: "workflow-export-drift",
	}); err == nil {
		t.Fatal("Storyboard Export Set accepted formal Shots that drifted from the applied Set")
	}
	var rejectedExportSetCount, rejectedEpisodeExportCount, rejectedExportReceiptCount int64
	if err = database.Model(&model.StoryboardExportSet{}).Where("draft_set_id = ?", preappliedSet.Set.ID).
		Count(&rejectedExportSetCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StoryboardExport{}).Where("project_id = ? AND export_set_id IS NOT NULL", fixture.projectID).
		Count(&rejectedEpisodeExportCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND idempotency_key = ?", "storyboard.create_export_set", "workflow-export-drift").
		Count(&rejectedExportReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if rejectedExportSetCount != 0 || rejectedEpisodeExportCount != 0 || rejectedExportReceiptCount != 0 {
		t.Fatalf("rejected Storyboard Export left partial facts: sets=%d exports=%d receipts=%d", rejectedExportSetCount, rejectedEpisodeExportCount, rejectedExportReceiptCount)
	}
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", driftedExportShot.ID).
		Update("content_hash", originalExportShotHash).Error; err != nil {
		t.Fatalf("restore formal Shot after export drift: %v", err)
	}
	precreatedExportSet, err := storyboardService.CreateExportSet(ctx, exportActor, storyboardapp.CreateExportSetCommand{
		SetID: preappliedSet.Set.ID, ExpectedRevision: preappliedSet.Set.Revision,
		ExpectedResultHash: *preappliedSet.Set.ResultHash,
		IdempotencyKey:     "workflow-node:" + exportNode.ID.String() + ":attempt:1",
	})
	if err != nil || precreatedExportSet.Status != "succeeded" || len(precreatedExportSet.Exports) != 2 {
		t.Fatalf("precommit Storyboard Export Set: set=%#v err=%v", precreatedExportSet, err)
	}
	storyboardSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: storyboardRun.WorkspaceID.String(), WorkflowRunID: run.ID, NodeRunID: storyboardGateNode.ID.String(),
		HumanTaskID: storyboardDecision.Task.ID, ReviewDecisionID: storyboardDecision.Decision.ID,
		SubjectRevision: storyboardDecision.Decision.SubjectRevision, Decision: storyboardDecision.Decision.Decision,
		IdempotencyKey: "storyboard-review-signal",
	}
	var storyboardSignal workflow.SignalIntent
	signalDeadline = time.Now().Add(5 * time.Second)
	for {
		storyboardSignal, err = signalService.SignalHumanGate(ctx, workflowapp.Actor{
			UserID: structureReviewerID.String(), TokenVersion: 1,
		}, storyboardSignalCommand)
		if err == nil && storyboardSignal.Status == "completed" {
			break
		}
		if err != nil || time.Now().After(signalDeadline) {
			t.Fatalf("signal approved Storyboard gate: intent=%#v err=%v", storyboardSignal, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	replayedStoryboardSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: structureReviewerID.String(), TokenVersion: 1,
	}, storyboardSignalCommand)
	if err != nil || replayedStoryboardSignal.ID != storyboardSignal.ID || replayedStoryboardSignal.Status != "completed" {
		t.Fatalf("replay Storyboard gate signal: intent=%#v err=%v", replayedStoryboardSignal, err)
	}

	completionDeadline := time.Now().Add(10 * time.Second)
	var completedWorkflow model.WorkflowRun
	for {
		if err = database.First(&completedWorkflow, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load completed Storyboard Workflow: %v", err)
		}
		if completedWorkflow.Status == "SUCCEEDED" {
			break
		}
		if completedWorkflow.Status == "FAILED" || time.Now().After(completionDeadline) {
			t.Fatalf("Storyboard Workflow did not complete after review: %#v", completedWorkflow)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err = database.First(&storyboardGateNode, "id = ?", storyboardGateNode.ID).Error; err != nil {
		t.Fatalf("reload applied Storyboard gate: %v", err)
	}
	formalStoryboards, _, formalStoryboardsHash, err := workflow.ParseNodeOutput(json.RawMessage(storyboardGateNode.Output))
	if err != nil || len(formalStoryboards.Bindings) != 1 || storyboardGateNode.OutputHash == nil || *storyboardGateNode.OutputHash != formalStoryboardsHash {
		t.Fatalf("parse formal Storyboards output: output=%#v node=%#v err=%v", formalStoryboards, storyboardGateNode, err)
	}
	formalStoryboardBinding := formalStoryboards.Bindings[0]
	if err = database.First(&set, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload applied Storyboard Draft Set: %v", err)
	}
	var setReceipt model.CommandReceipt
	if err = database.Where("operation = ? AND resource_id = ?", "storyboard.apply_set", set.ID).First(&setReceipt).Error; err != nil {
		t.Fatalf("load Storyboard Set owner receipt: %v", err)
	}
	var setApplyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.Where("node_run_id = ?", storyboardGateNode.ID).First(&setApplyReceipt).Error; err != nil {
		t.Fatalf("load Storyboard gate apply receipt: %v", err)
	}
	var appliedBatchCount, activeShotCount, setReceiptCount int64
	if err = database.Model(&model.StoryboardDraftBatch{}).Where("id IN ? AND status = ?", setBatchIDs, "applied").Count(&appliedBatchCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StoryboardShot{}).Where("batch_id IN ? AND status = ?", setBatchIDs, "active").Count(&activeShotCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("operation = ? AND resource_id = ?", "storyboard.apply_set", set.ID).Count(&setReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if completedWorkflow.Status != "SUCCEEDED" || storyboardGateNode.Status != "SUCCEEDED" ||
		formalStoryboardBinding.Port != "storyboards" || formalStoryboardBinding.ValueType != "storyboards" ||
		formalStoryboardBinding.ReferenceID != set.ID.String() || formalStoryboardBinding.ReferenceVersion != "3" ||
		set.Status != "applied" || set.Revision != 3 || set.ResultHash == nil || formalStoryboardBinding.ContentHash != *set.ResultHash ||
		formalStoryboardBinding.ContentHash == storyboardBinding.ContentHash || preappliedSet.Receipt.ID != setReceipt.ID.String() ||
		setApplyReceipt.OwnerReceiptID == nil || *setApplyReceipt.OwnerReceiptID != setReceipt.ID ||
		setApplyReceipt.OwnerOperation == nil || *setApplyReceipt.OwnerOperation != "storyboard.apply_set" ||
		appliedBatchCount != 2 || activeShotCount != 2 || setReceiptCount != 1 {
		t.Fatalf("formal Storyboard Set boundary: run=%#v gate=%#v binding=%#v set=%#v preapplied=%#v receipt=%#v apply=%#v counts=%d/%d/%d", completedWorkflow, storyboardGateNode, formalStoryboardBinding, set, preappliedSet, setReceipt, setApplyReceipt, appliedBatchCount, activeShotCount, setReceiptCount)
	}
	if err = database.First(&exportNode, "id = ?", exportNode.ID).Error; err != nil {
		t.Fatalf("reload completed Storyboard Export node: %v", err)
	}
	exportOutput, _, exportOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(exportNode.Output))
	if err != nil || exportNode.Status != "SUCCEEDED" || len(exportOutput.Bindings) != 1 ||
		exportNode.OutputHash == nil || *exportNode.OutputHash != exportOutputHash {
		t.Fatalf("parse Storyboard Export output: output=%#v node=%#v err=%v", exportOutput, exportNode, err)
	}
	exportBinding := exportOutput.Bindings[0]
	var exportSet model.StoryboardExportSet
	if err = database.First(&exportSet, "id = ?", exportBinding.ReferenceID).Error; err != nil {
		t.Fatalf("load Storyboard Export Set: %v", err)
	}
	loadedExportSet, err := storyboardService.GetExportSet(ctx, exportActor, exportBinding.ReferenceID)
	if err != nil || loadedExportSet.ID != precreatedExportSet.ID || loadedExportSet.ContentHash != precreatedExportSet.ContentHash {
		t.Fatalf("load Storyboard Export Set through Owner: set=%#v err=%v", loadedExportSet, err)
	}
	var exportReferences []struct {
		ExportID    string `json:"export_id"`
		EpisodeID   string `json:"episode_id"`
		OrderHash   string `json:"order_hash"`
		ContentHash string `json:"content_hash"`
	}
	if err = json.Unmarshal(exportSet.Exports, &exportReferences); err != nil {
		t.Fatalf("decode Storyboard Export Set references: %v", err)
	}
	exportIDs := make([]uuid.UUID, len(exportReferences))
	for index, reference := range exportReferences {
		exportIDs[index] = uuid.MustParse(reference.ExportID)
		if reference.EpisodeID != setBatches[index].EpisodeID || len(reference.OrderHash) != 64 || len(reference.ContentHash) != 64 {
			t.Fatalf("Storyboard Export reference %d = %#v", index, reference)
		}
		episodeExport, loadErr := storyboardService.GetExport(ctx, exportActor, reference.ExportID)
		if loadErr != nil || episodeExport.ExportSetID == nil || *episodeExport.ExportSetID != exportSet.ID.String() ||
			episodeExport.EpisodeID != reference.EpisodeID || episodeExport.ContentHash != reference.ContentHash ||
			episodeExport.Status != "succeeded" || len(episodeExport.Files) != 4 || len(episodeExport.Package) == 0 {
			t.Fatalf("load per-Episode Storyboard Export through Owner: export=%#v err=%v", episodeExport, loadErr)
		}
	}
	var episodeExports []model.StoryboardExport
	if err = database.Where("id IN ?", exportIDs).Find(&episodeExports).Error; err != nil {
		t.Fatalf("load per-Episode Storyboard Exports: %v", err)
	}
	for _, episodeExport := range episodeExports {
		if episodeExport.ExportSetID == nil || *episodeExport.ExportSetID != exportSet.ID ||
			episodeExport.Status != "succeeded" || len(episodeExport.Package) == 0 ||
			len(episodeExport.ContentHash) != 64 || len(episodeExport.InputHash) != 64 {
			t.Fatalf("invalid per-Episode Storyboard Export: %#v", episodeExport)
		}
	}
	var exportSetReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND resource_id = ?", "storyboard.create_export_set", exportSet.ID).
		Count(&exportSetReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if exportBinding.Port != "export" || exportBinding.ValueType != "storyboard_export" ||
		exportBinding.ReferenceID != precreatedExportSet.ID || exportBinding.ReferenceVersion != "1" ||
		exportBinding.ContentHash != precreatedExportSet.ContentHash || exportSet.DraftSetID != set.ID ||
		exportSet.DraftSetRevision != set.Revision || exportSet.Status != "succeeded" ||
		exportSet.ContentHash != exportBinding.ContentHash || len(exportReferences) != 2 ||
		len(episodeExports) != 2 || exportSetReceiptCount != 1 {
		t.Fatalf("Storyboard Export Set boundary: binding=%#v set=%#v precreated=%#v references=%#v exports=%#v receipt_count=%d", exportBinding, exportSet, precreatedExportSet, exportReferences, episodeExports, exportSetReceiptCount)
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

func explicitTwoEpisodeRevisionValues(t *testing.T) map[string]any {
	t.Helper()
	text := ""
	blocks := make([]planningdomain.Block, 0, 6)
	appendBlock := func(kind, value string) {
		if text != "" {
			text += "\n"
		}
		start := utf8.RuneCountInString(text)
		text += value
		blocks = append(blocks, planningdomain.Block{
			ID: uuid.NewString(), Position: len(blocks) + 1, Kind: kind,
			SourceStart: start, SourceEnd: utf8.RuneCountInString(text),
		})
	}
	appendBlock("episode_marker", "第一集")
	appendBlock("title", "《雨巷》")
	appendBlock("scene_heading", "内景·雨巷·夜\n小兰：走吧")
	appendBlock("episode_marker", "第二集")
	appendBlock("title", "《车站》")
	appendBlock("scene_heading", "外景·车站·晨\n阿明：到了")
	encoded, err := json.Marshal(blocks)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"raw_text": text, "normalized_text": text, "codepoint_count": utf8.RuneCountInString(text), "blocks": string(encoded),
	}
}

type successfulStoryboardAgent struct{}

func (successfulStoryboardAgent) Invoke(_ context.Context, invocation contract.Invocation) (contract.Result, error) {
	var payload struct {
		Units []struct {
			ID string `json:"unit_version_id"`
		} `json:"units"`
	}
	if err := json.Unmarshal(invocation.Payload, &payload); err != nil {
		return contract.Result{}, err
	}
	unitIDs := make([]string, len(payload.Units))
	for index, unit := range payload.Units {
		unitIDs[index] = unit.ID
	}
	candidate, err := json.Marshal(map[string]any{"shots": []map[string]any{{
		"proposal_key": "shot-1", "position": 1, "title": "全场关键分镜",
		"narrative_unit_version_ids": unitIDs, "spec": map[string]any{"duration_ms": 1000},
		"asset_references": []any{}, "risk_codes": []any{},
	}}})
	if err != nil {
		return contract.Result{}, err
	}
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

var _ storyboardapp.AgentClient = successfulStoryboardAgent{}
