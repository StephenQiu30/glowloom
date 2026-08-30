package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
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
	storyboardgeneration "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/generation"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestFormalShotWorkflowSelectsAndBindsImageOnRealPostgresAndTemporal(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test variables to run the formal Shot Workflow journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open formal Shot Workflow database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize formal Shot Workflow GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	countRecords := func(value any, query string, arguments ...any) (int64, error) {
		var count int64
		err := database.Model(value).Where(query, arguments...).Count(&count).Error
		return count, err
	}
	fixture := seedCompilerProject(t, create, now)
	shotID := seedFormalStoryboardShot(t, create, fixture, now)
	set, bundles := seedShotWorkflowCandidateSet(t, create, fixture, now)
	catalog, err := authoring.SystemShotCatalog()
	if err != nil {
		t.Fatalf("build formal Shot Workflow catalog: %v", err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist formal Shot Workflow catalog: %v", err)
	}
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph: authoring.Graph{
			Nodes: []authoring.Node{
				{ID: "shot", DefinitionKey: "input.production_shot", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{"shot_id":"` + shotID.String() + `"}`)},
				{ID: "candidates", DefinitionKey: "input.generation_candidate_set", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"provider_job_id":"` + set.ID + `"}`)},
				{ID: "image-review", DefinitionKey: "human.generation_image_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "bind-image", DefinitionKey: "production.shot_image_binding", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{
				{ID: "shot-candidates", FromNodeID: "shot", FromPort: "shot", ToNodeID: "candidates", ToPort: "shot"},
				{ID: "candidate-review", FromNodeID: "candidates", FromPort: "candidates", ToNodeID: "image-review", ToPort: "candidates"},
				{ID: "shot-binding", FromNodeID: "shot", FromPort: "shot", ToNodeID: "bind-image", ToPort: "shot"},
				{ID: "target-binding", FromNodeID: "shot", FromPort: "binding_target", ToNodeID: "bind-image", ToPort: "binding_target"},
				{ID: "review-binding", FromNodeID: "image-review", FromPort: "selection", ToNodeID: "bind-image", ToPort: "selection"},
			},
		},
		Layout: json.RawMessage(`{"guided":{"step":"shot-image"}}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "formal-shot-authoring-create",
	})
	if err != nil {
		t.Fatalf("create formal Shot Workflow authoring: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "formal-shot-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish formal Shot Workflow authoring: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: time.Minute,
	})
	selectionService := generationapp.NewSelectionService(
		generationgorm.New(database), generationCandidateReadiness{bundles: bundles},
		generationreview.NewDecisionReader(reviewService),
		generationapp.SelectionConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	imageBindings := storyboardapp.NewShotImageBindingService(
		storyboardgorm.New(database), storyboardgeneration.NewSelectedImageSource(selectionService),
		storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	setSource := &generationCandidateSetSource{set: set}
	bibleStore := biblegorm.New(database)
	evidenceService := bibleapp.NewSourceEvidenceService(bibleStore, bibleapp.SourceEvidenceConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	storyReviewService := bibleapp.NewStoryReviewService(
		bibleStore,
		bibleapp.NewStoryCandidateRepairService(bibleStore, bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		evidenceService,
		bibleapp.NewStoryAnalysisService(bibleStore, bibleapp.StoryAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString, FanIn: 2,
		}),
		storyReviewService,
		bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		planningapp.NewEpisodePlanningService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		reviewService, imageBindings, setSource, nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("compose formal Shot Workflow runtime: %v", err)
	}
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: "lanverse-shot-workflow-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect formal Shot Workflow Temporal: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		t.Fatalf("register formal Shot Workflow worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start formal Shot Workflow worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	workflowActor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	started, err := startService.Start(ctx, workflowActor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "formal-shot-start",
	})
	if err != nil || started.Status != "RUNNING" {
		t.Fatalf("start formal Shot Workflow: run=%#v err=%v", started, err)
	}
	var definition model.WorkflowDefinitionVersion
	if err = database.First(&definition, "id = ?", started.DefinitionVersionID).Error; err != nil ||
		definition.WorkflowType != workflowtemporal.ShotProductionWorkflowName {
		t.Fatalf("formal Shot Workflow definition type drifted: definition=%#v err=%v", definition, err)
	}

	task := waitForShotWorkflowTask(t, ctx, func(value *model.HumanTask) error {
		return database.WithContext(ctx).Where("workflow_run_id = ?", started.ID).First(value).Error
	})
	var beforeReview []model.NodeRunProjection
	if err = database.Where("workflow_run_id = ?", started.ID).Find(&beforeReview).Error; err != nil {
		t.Fatalf("load formal Shot Workflow pre-review projections: %v", err)
	}
	preReviewStatus := make(map[string]string, len(beforeReview))
	for _, node := range beforeReview {
		preReviewStatus[node.NodeID] = node.Status
	}
	if preReviewStatus["shot"] != "SUCCEEDED" || preReviewStatus["candidates"] != "SUCCEEDED" ||
		preReviewStatus["image-review"] != "WAITING_HUMAN" || preReviewStatus["bind-image"] != "QUEUED" {
		t.Fatalf("formal Shot Workflow did not stop at ordered Human Gate: %v", preReviewStatus)
	}
	var frozenCandidates []string
	if err = json.Unmarshal(task.CandidateIDs, &frozenCandidates); err != nil || len(frozenCandidates) != 2 ||
		!slices.Equal(frozenCandidates, []string{set.Candidates[0].ID, set.Candidates[1].ID}) {
		t.Fatalf("formal Shot HumanTask candidates drifted: %v err=%v", frozenCandidates, err)
	}
	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: task.ID.String(), ExpectedRevision: task.Revision, IdempotencyKey: "formal-shot-review-claim",
	})
	if err != nil {
		t.Fatalf("claim formal Shot image review: %v", err)
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: task.ID.String(), ClaimToken: claim.ClaimToken, ExpectedTaskRevision: claim.Task.Revision,
		ExpectedSubjectRevision: claim.Task.SubjectRevision, ExpectedSubjectHash: claim.Task.SubjectHash, Decision: "selected",
		SelectedCandidateID: set.Candidates[1].ID, IdempotencyKey: "formal-shot-review-select",
	})
	if err != nil {
		t.Fatalf("select formal Shot image candidate: %v", err)
	}
	signals := workflowapp.NewSignalService(workflowStore, temporalRuntime, workflowapp.SignalConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Owner: workflowgeneration.NewHumanGateApplier(selectionService),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: task.NodeRunID.String(),
		HumanTaskID: task.ID.String(), ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: "selected", IdempotencyKey: "formal-shot-signal",
	}
	signal, err := signals.SignalHumanGate(ctx, workflowActor, signalCommand)
	if err != nil || signal.Status != "completed" {
		t.Fatalf("signal formal Shot image selection: signal=%#v err=%v", signal, err)
	}
	replayedSignal, err := signals.SignalHumanGate(ctx, workflowActor, signalCommand)
	if err != nil || replayedSignal.ID != signal.ID || replayedSignal.Status != "completed" {
		t.Fatalf("replay formal Shot image selection signal: signal=%#v err=%v", replayedSignal, err)
	}
	completed := waitForShotWorkflowStatus(t, ctx, func(value *model.WorkflowRun) error {
		return database.WithContext(ctx).First(value, "id = ?", started.ID).Error
	}, "SUCCEEDED")
	if completed.ProgressStage != "completed" {
		t.Fatalf("formal Shot Workflow progress stage drifted: %#v", completed)
	}

	var binding model.StoryboardShotImageBindingVersion
	if err = database.Where("shot_id = ?", shotID).First(&binding).Error; err != nil ||
		binding.BindingRevision != 1 || binding.CandidateID.String() != set.Candidates[1].ID {
		t.Fatalf("formal Shot image binding drifted: binding=%#v err=%v", binding, err)
	}
	current, err := imageBindings.RequireCurrentShotImage(ctx, storyboardapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, shotID.String())
	if err != nil || current.ID != binding.ID.String() || current.Revision != 1 {
		t.Fatalf("read formal Shot current image binding: current=%#v err=%v", current, err)
	}
	assertFormalShotWorkflowFacts(t, func(value *[]model.NodeRunProjection) error {
		return database.Where("workflow_run_id = ?", started.ID).Find(value).Error
	}, countRecords, fixture, started.ID, shotID, binding.ID)
	replayFormalShotWorkflowHistory(t, ctx, temporalAddress, started.TemporalWorkflowID)

	rerunCommand := workflowapp.RerunCommand{
		SourceWorkflowRunID: started.ID, RootNodeID: "shot", IdempotencyKey: "formal-shot-rerun",
	}
	rerun, err := startService.Rerun(ctx, workflowActor, rerunCommand)
	if err != nil || rerun.Status != "RUNNING" || rerun.SourceWorkflowRunID == nil ||
		*rerun.SourceWorkflowRunID != started.ID || rerun.RerunRootNodeID == nil || *rerun.RerunRootNodeID != "shot" ||
		rerun.DefinitionVersionID != started.DefinitionVersionID || rerun.RunInputSnapshotID != started.RunInputSnapshotID {
		t.Fatalf("start formal single Shot rerun: run=%#v err=%v", rerun, err)
	}
	replayedRerun, err := startService.Rerun(ctx, workflowActor, rerunCommand)
	if err != nil || replayedRerun.ID != rerun.ID || replayedRerun.TemporalWorkflowID != rerun.TemporalWorkflowID {
		t.Fatalf("replay formal single Shot rerun command: run=%#v err=%v", replayedRerun, err)
	}
	rerunTask := waitForShotWorkflowTask(t, ctx, func(value *model.HumanTask) error {
		return database.WithContext(ctx).Where("workflow_run_id = ?", rerun.ID).First(value).Error
	})
	rerunClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: rerunTask.ID.String(), ExpectedRevision: rerunTask.Revision, IdempotencyKey: "formal-shot-rerun-review-claim",
	})
	if err != nil {
		t.Fatalf("claim formal single Shot rerun review: %v", err)
	}
	rerunDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: rerunTask.ID.String(), ClaimToken: rerunClaim.ClaimToken, ExpectedTaskRevision: rerunClaim.Task.Revision,
		ExpectedSubjectRevision: rerunClaim.Task.SubjectRevision, ExpectedSubjectHash: rerunClaim.Task.SubjectHash, Decision: "selected",
		SelectedCandidateID: set.Candidates[0].ID, IdempotencyKey: "formal-shot-rerun-review-select",
	})
	if err != nil {
		t.Fatalf("select replacement image for formal single Shot rerun: %v", err)
	}
	rerunSignal, err := signals.SignalHumanGate(ctx, workflowActor, workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: rerun.ID, NodeRunID: rerunTask.NodeRunID.String(),
		HumanTaskID: rerunTask.ID.String(), ReviewDecisionID: rerunDecision.Decision.ID,
		SubjectRevision: rerunDecision.Decision.SubjectRevision, Decision: "selected",
		IdempotencyKey: "formal-shot-rerun-signal",
	})
	if err != nil || rerunSignal.Status != "completed" {
		t.Fatalf("signal formal single Shot rerun selection: signal=%#v err=%v", rerunSignal, err)
	}
	waitForShotWorkflowStatus(t, ctx, func(value *model.WorkflowRun) error {
		return database.WithContext(ctx).First(value, "id = ?", rerun.ID).Error
	}, "SUCCEEDED")
	var replacementBinding model.StoryboardShotImageBindingVersion
	if err = database.Where("shot_id = ?", shotID).Order("binding_revision DESC").First(&replacementBinding).Error; err != nil ||
		replacementBinding.BindingRevision != 2 || replacementBinding.CandidateID.String() != set.Candidates[0].ID {
		t.Fatalf("formal single Shot rerun replacement binding drifted: binding=%#v err=%v", replacementBinding, err)
	}
	current, err = imageBindings.RequireCurrentShotImage(ctx, storyboardapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, shotID.String())
	if err != nil || current.ID != replacementBinding.ID.String() || current.Revision != 2 {
		t.Fatalf("read formal single Shot rerun current binding: current=%#v err=%v", current, err)
	}
	assertFormalShotRerunFacts(
		t,
		func(value *model.WorkflowRun) error { return database.First(value, "id = ?", started.ID).Error },
		func(value *[]model.NodeRunProjection) error {
			return database.Where("workflow_run_id = ?", rerun.ID).Find(value).Error
		},
		countRecords,
		started,
		rerun,
		replacementBinding.ID,
	)
	replayFormalShotWorkflowHistory(t, ctx, temporalAddress, rerun.TemporalWorkflowID)

	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive formal Shot before failure journey: %v", err)
	}
	failed, err := startService.Start(ctx, workflowActor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "formal-shot-archived-start",
	})
	if err != nil || failed.Status != "RUNNING" {
		t.Fatalf("start archived Shot failure journey: run=%#v err=%v", failed, err)
	}
	waitForShotWorkflowStatus(t, ctx, func(value *model.WorkflowRun) error {
		return database.WithContext(ctx).First(value, "id = ?", failed.ID).Error
	}, "FAILED")
	var failedTaskCount, bindingCount int64
	if err = database.Model(&model.HumanTask{}).Where("workflow_run_id = ?", failed.ID).Count(&failedTaskCount).Error; err != nil {
		t.Fatalf("count archived Shot HumanTasks: %v", err)
	}
	if err = database.Model(&model.StoryboardShotImageBindingVersion{}).Where("shot_id = ?", shotID).Count(&bindingCount).Error; err != nil {
		t.Fatalf("count formal Shot image bindings after failure: %v", err)
	}
	if failedTaskCount != 0 || bindingCount != 2 {
		t.Fatalf("archived Shot crossed source boundary: tasks=%d bindings=%d", failedTaskCount, bindingCount)
	}
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).Update("status", "active").Error; err != nil {
		t.Fatalf("restore formal Shot status: %v", err)
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", fixture.userID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke formal Shot actor token: %v", err)
	}
	if _, err = imageBindings.RequireActiveShot(ctx, storyboardapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, shotID.String()); err == nil {
		t.Fatal("revoked actor reloaded formal active Shot")
	}
}

func assertFormalShotRerunFacts(
	t *testing.T,
	loadSource func(*model.WorkflowRun) error,
	loadNodes func(*[]model.NodeRunProjection) error,
	countRecords func(any, string, ...any) (int64, error),
	source, rerun workflow.WorkflowRun,
	bindingID uuid.UUID,
) {
	t.Helper()
	var sourceRecord model.WorkflowRun
	if err := loadSource(&sourceRecord); err != nil || sourceRecord.Status != "SUCCEEDED" ||
		sourceRecord.SourceWorkflowRunID != nil || sourceRecord.RerunRootNodeID != nil {
		t.Fatalf("formal single Shot rerun changed source run: run=%#v err=%v", sourceRecord, err)
	}
	var nodes []model.NodeRunProjection
	if err := loadNodes(&nodes); err != nil {
		t.Fatalf("load formal single Shot rerun projections: %v", err)
	}
	statuses := make(map[string]string, len(nodes))
	var shotNode, bindingNode model.NodeRunProjection
	for _, node := range nodes {
		statuses[node.NodeID] = node.Status
		if node.ReusedFromNodeRunID != nil {
			t.Fatalf("single Shot root rerun unexpectedly reused node %s from %s", node.NodeID, node.ReusedFromNodeRunID)
		}
		switch node.NodeID {
		case "shot":
			shotNode = node
		case "bind-image":
			bindingNode = node
		}
	}
	if len(nodes) != 4 || statuses["shot"] != "SUCCEEDED" || statuses["candidates"] != "SUCCEEDED" ||
		statuses["image-review"] != "SUCCEEDED" || statuses["bind-image"] != "SUCCEEDED" {
		t.Fatalf("formal single Shot rerun projections drifted: %v", statuses)
	}
	shotOutput, _, shotOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(shotNode.Output))
	if err != nil || shotNode.OutputHash == nil || *shotNode.OutputHash != shotOutputHash {
		t.Fatalf("parse formal single Shot rerun source output: output=%#v node=%#v err=%v", shotOutput, shotNode, err)
	}
	shotBindings := make(map[string]workflow.NodeOutputBinding, len(shotOutput.Bindings))
	for _, binding := range shotOutput.Bindings {
		shotBindings[binding.Port] = binding
	}
	if len(shotBindings) != 2 || shotBindings["binding_target"].ReferenceVersion != "2" ||
		shotBindings["binding_target"].ReferenceID != shotBindings["shot"].ReferenceID {
		t.Fatalf("formal single Shot rerun did not freeze replacement target: %#v", shotBindings)
	}
	bindingOutput, _, bindingOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(bindingNode.Output))
	if err != nil || bindingNode.OutputHash == nil || *bindingNode.OutputHash != bindingOutputHash ||
		len(bindingOutput.Bindings) != 1 || bindingOutput.Bindings[0].ReferenceID != bindingID.String() ||
		bindingOutput.Bindings[0].ReferenceVersion != "2" {
		t.Fatalf("formal single Shot rerun binding output drifted: output=%#v node=%#v err=%v", bindingOutput, bindingNode, err)
	}
	checks := []struct {
		name  string
		value any
	}{
		{"HumanTask", &model.HumanTask{}},
		{"CandidateSelection", &model.GenerationCandidateSelection{}},
		{"SignalIntent", &model.WorkflowSignalIntent{}},
		{"SignalReceipt", &model.WorkflowSignalReceipt{}},
	}
	for _, check := range checks {
		count, countErr := countRecords(check.value, "workflow_run_id = ?", rerun.ID)
		if countErr != nil || count != 1 {
			t.Fatalf("formal single Shot rerun %s facts=%d err=%v", check.name, count, countErr)
		}
	}
}

func seedShotWorkflowCandidateSet(
	t *testing.T,
	create func(any) error,
	fixture compilerProjectFixture,
	now time.Time,
) (generationdomain.CandidateSet, map[string]generationdomain.CandidateWithReport) {
	t.Helper()
	providerJobID := uuid.New()
	candidateIDs := []uuid.UUID{uuid.New(), uuid.New()}
	slices.SortFunc(candidateIDs, func(left, right uuid.UUID) int {
		return strings.Compare(left.String(), right.String())
	})
	references := make([]generationdomain.CandidateReference, len(candidateIDs))
	bundles := make(map[string]generationdomain.CandidateWithReport, len(candidateIDs))
	for index, candidateID := range candidateIDs {
		artifactID, reportID := uuid.New(), uuid.New()
		providerCallID, providerReceiptID := uuid.New(), uuid.New()
		artifactHash := strings.Repeat(string(rune('a'+index)), 64)
		reportHash := strings.Repeat(string(rune('c'+index)), 64)
		outputKey := "shot-image-" + string(rune('1'+index))
		width, height := 1280+index, 720+index
		if err := create(&model.Artifact{
			ID: artifactID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			SourceType: "generation_provider_receipt", SourceID: providerReceiptID, OutputKey: outputKey,
			MediaType: "image/png", SHA256: artifactHash, SizeBytes: 1024, Status: "READY",
			Width: &width, Height: &height, Revision: 2, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed formal Shot Workflow Artifact: %v", err)
		}
		if err := create(&model.GenerationCandidate{
			ID: candidateID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			ProviderJobID: providerJobID, ProviderCallID: providerCallID, ProviderReceiptID: providerReceiptID,
			OutputKey: outputKey, ArtifactID: artifactID,
			ArtifactRevision: 2, ArtifactSHA256: artifactHash, MediaType: "image/png",
			Width: width, Height: height, Status: "QC_PASSED", Revision: 1,
			CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed formal Shot Workflow Candidate: %v", err)
		}
		references[index] = generationdomain.CandidateReference{
			ID: candidateID.String(), Revision: 1, ArtifactID: artifactID.String(), ArtifactRevision: 2,
			ArtifactSHA256: artifactHash, QCReportID: reportID.String(), QCReportHash: reportHash,
		}
		bundles[candidateID.String()] = generationdomain.CandidateWithReport{
			Candidate: generationdomain.Candidate{
				ID: candidateID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
				ProviderJobID: providerJobID.String(), ProviderCallID: providerCallID.String(), ProviderReceiptID: providerReceiptID.String(),
				OutputKey: outputKey, ArtifactID: artifactID.String(),
				ArtifactRevision: 2, ArtifactSHA256: artifactHash, MediaType: "image/png",
				Width: width, Height: height, Status: generationdomain.CandidateQCPassed, Revision: 1,
				CreatedBy: fixture.userID.String(), CreatedAt: now, UpdatedAt: now,
			},
			Report: generationdomain.QCReport{
				ID: reportID.String(), WorkspaceID: fixture.workspaceID.String(), CandidateID: candidateID.String(),
				Status: generationdomain.QCPassed, ReportHash: reportHash, CreatedAt: now,
			},
		}
	}
	contentHash, err := platformcommand.InputHash(struct {
		Candidates []generationdomain.CandidateReference `json:"candidates"`
	}{Candidates: references})
	if err != nil {
		t.Fatalf("hash formal Shot Workflow CandidateSet: %v", err)
	}
	return generationdomain.CandidateSet{
		ID: providerJobID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		ProviderReceiptSetHash: strings.Repeat("c", 64), Candidates: references, ContentHash: contentHash, Revision: 1,
	}, bundles
}

func waitForShotWorkflowTask(
	t *testing.T,
	ctx context.Context,
	load func(*model.HumanTask) error,
) model.HumanTask {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var task model.HumanTask
		err := load(&task)
		if err == nil {
			return task
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for formal Shot Workflow HumanTask: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for formal Shot Workflow HumanTask: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func waitForShotWorkflowStatus(
	t *testing.T,
	ctx context.Context,
	load func(*model.WorkflowRun) error,
	status string,
) model.WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var run model.WorkflowRun
		err := load(&run)
		if err == nil && run.Status == status {
			return run
		}
		if err == nil && (run.Status == "FAILED" || run.Status == "CANCELLED") && run.Status != status {
			t.Fatalf("formal Shot Workflow reached %s instead of %s: %#v", run.Status, status, run)
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for formal Shot Workflow status %s: run=%#v err=%v", status, run, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for formal Shot Workflow status %s: %v", status, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func assertFormalShotWorkflowFacts(
	t *testing.T,
	loadNodes func(*[]model.NodeRunProjection) error,
	countRecords func(any, string, ...any) (int64, error),
	fixture compilerProjectFixture,
	runID string,
	shotID, bindingID uuid.UUID,
) {
	t.Helper()
	var nodes []model.NodeRunProjection
	if err := loadNodes(&nodes); err != nil {
		t.Fatalf("load formal Shot Workflow terminal nodes: %v", err)
	}
	statuses := make(map[string]string, len(nodes))
	var bindingNode model.NodeRunProjection
	for _, node := range nodes {
		statuses[node.NodeID] = node.Status
		if node.NodeID == "bind-image" {
			bindingNode = node
		}
	}
	if len(nodes) != 4 || statuses["shot"] != "SUCCEEDED" || statuses["candidates"] != "SUCCEEDED" ||
		statuses["image-review"] != "SUCCEEDED" || statuses["bind-image"] != "SUCCEEDED" {
		t.Fatalf("formal Shot Workflow terminal projections drifted: %v", statuses)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(bindingNode.Output))
	if err != nil || bindingNode.OutputHash == nil || *bindingNode.OutputHash != outputHash ||
		len(output.Bindings) != 1 || output.Bindings[0].ReferenceID != bindingID.String() ||
		output.Bindings[0].ValueType != "production_shot_image_binding" {
		t.Fatalf("formal Shot binding Node output drifted: output=%#v node=%#v err=%v", output, bindingNode, err)
	}
	counts := []struct {
		name  string
		value any
		query string
		args  []any
	}{
		{"CandidateSelection", &model.GenerationCandidateSelection{}, "workflow_run_id = ?", []any{runID}},
		{"HumanTask", &model.HumanTask{}, "workflow_run_id = ?", []any{runID}},
		{"HumanGateApplyReceipt", &model.WorkflowHumanGateApplyReceipt{}, "workflow_run_id = ?", []any{runID}},
		{"SignalIntent", &model.WorkflowSignalIntent{}, "workflow_run_id = ?", []any{runID}},
		{"SignalReceipt", &model.WorkflowSignalReceipt{}, "workflow_run_id = ?", []any{runID}},
		{"ShotImageBinding", &model.StoryboardShotImageBindingVersion{}, "shot_id = ? AND id = ?", []any{shotID, bindingID}},
		{"SelectionOwnerReceipt", &model.CommandReceipt{}, "workspace_id = ? AND operation = ?", []any{fixture.workspaceID, "generation.candidate.select"}},
		{"BindingOwnerReceipt", &model.CommandReceipt{}, "workspace_id = ? AND operation = ?", []any{fixture.workspaceID, "storyboard.shot.bind_selected_image"}},
	}
	for _, check := range counts {
		count, countErr := countRecords(check.value, check.query, check.args...)
		if countErr != nil {
			t.Fatalf("count formal Shot Workflow %s facts: %v", check.name, countErr)
		}
		if count != 1 {
			t.Fatalf("formal Shot Workflow %s facts=%d, want 1", check.name, count)
		}
	}
}

func replayFormalShotWorkflowHistory(t *testing.T, ctx context.Context, temporalAddress, workflowID string) {
	t.Helper()
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddress, Namespace: "default"})
	if err != nil {
		t.Fatalf("dial formal Shot Workflow Temporal history: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	iterator := temporalClient.GetWorkflowHistory(ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	history := &historypb.History{}
	workflowType := ""
	for iterator.HasNext() {
		event, historyErr := iterator.Next()
		if historyErr != nil {
			t.Fatalf("read formal Shot Workflow Temporal history: %v", historyErr)
		}
		history.Events = append(history.Events, event)
		if started := event.GetWorkflowExecutionStartedEventAttributes(); started != nil && started.WorkflowType != nil {
			workflowType = started.WorkflowType.Name
		}
	}
	if len(history.Events) == 0 || workflowType != workflowtemporal.ShotProductionWorkflowName {
		t.Fatalf("formal Shot Workflow Temporal history type=%q events=%d", workflowType, len(history.Events))
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		workflowtemporal.ShotProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: workflowtemporal.ShotProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay formal Shot Workflow Temporal history: %v", err)
	}
}
