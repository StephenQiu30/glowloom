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

func TestProductionWorkflowWorkerConfirmsEpisodeStructuresThroughBatchGate(t *testing.T) {
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
				{ID: "structure", DefinitionKey: "production.episode_structure", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "structure-review", DefinitionKey: "human.episode_structure_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{
				{ID: "script-bible", FromNodeID: "script", FromPort: "script", ToNodeID: "bible", ToPort: "script"},
				{ID: "bible-review", FromNodeID: "bible", FromPort: "candidate", ToNodeID: "bible-review", ToPort: "candidate"},
				{ID: "script-episodes", FromNodeID: "script", FromPort: "script", ToNodeID: "episodes", ToPort: "script"},
				{ID: "review-episodes", FromNodeID: "bible-review", FromPort: "bible", ToNodeID: "episodes", ToPort: "bible"},
				{ID: "episodes-review", FromNodeID: "episodes", FromPort: "candidate", ToNodeID: "episodes-review", ToPort: "candidate"},
				{ID: "episodes-structure", FromNodeID: "episodes-review", FromPort: "episodes", ToNodeID: "structure", ToPort: "episodes"},
				{ID: "structure-review", FromNodeID: "structure", FromPort: "candidate", ToNodeID: "structure-review", ToPort: "candidate"},
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
		persistedPlanSignal.Status != "completed" || confirmReceiptCount != 1 || episodeCount != 1 || commitCount != 1 {
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
	claimedStructure, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: structureTask.ID.String(), ExpectedRevision: structureTask.Revision,
		IdempotencyKey: "episode-structure-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Episode Structure review: %v", err)
	}
	structureDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: structureTask.ID.String(), ClaimToken: claimedStructure.ClaimToken,
		ExpectedTaskRevision: claimedStructure.Task.Revision, ExpectedSubjectRevision: claimedStructure.Task.SubjectRevision,
		Decision: "approved", IdempotencyKey: "episode-structure-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Episode Structure review: %v", err)
	}
	preconfirmedStructures, err := planningService.ConfirmPublishedStructureBatch(ctx, planningActor, planningapp.ConfirmStructureBatchCommand{
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
			UserID: fixture.userID.String(), TokenVersion: 1,
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
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, structureSignalCommand)
	if err != nil || replayedStructureSignal.ID != structureSignal.ID || replayedStructureSignal.Status != "completed" {
		t.Fatalf("replay Episode Structure gate signal: intent=%#v err=%v", replayedStructureSignal, err)
	}

	completionDeadline := time.Now().Add(10 * time.Second)
	var completedRun model.WorkflowRun
	for {
		if err = database.First(&completedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load completed Episode Structure Workflow: %v", err)
		}
		if completedRun.Status == "SUCCEEDED" {
			break
		}
		if completedRun.Status == "FAILED" || time.Now().After(completionDeadline) {
			t.Fatalf("Episode Structure Workflow did not complete after review: %#v", completedRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
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
		if confirmed.Status != "confirmed" || confirmed.ConfirmedBy == nil || *confirmed.ConfirmedBy != fixture.userID.String() {
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
