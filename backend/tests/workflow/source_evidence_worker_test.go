package workflow_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
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
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test endpoints to run the Source Evidence Workflow journey")
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
	now := time.Now().UTC().Add(-time.Minute)
	if _, err = authoringgorm.New(database).EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	text := "第1集\n林一😀进入大厅。\n\n场景二\n钟声响起。\n\n第二集\n林一回头，乙回答。\n\n第12集\n终章。"
	digest := sha256.Sum256([]byte(text))
	fixture.normalizedHash = hex.EncodeToString(digest[:])
	if err = database.Model(&model.DocumentRevision{}).Where("id = ?", fixture.scriptRevisionID).Updates(map[string]any{
		"raw_text": text, "raw_hash": fixture.normalizedHash,
		"normalized_text": text, "normalized_hash": fixture.normalizedHash,
		"codepoint_count": len([]rune(text)),
	}).Error; err != nil {
		t.Fatalf("update two-Episode source fixture: %v", err)
	}

	authoringService := authoringapp.NewService(authoringgorm.New(database), authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	actor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, actor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED",
		Graph: authoring.Graph{
			Nodes: []authoring.Node{
				{ID: "script", DefinitionKey: "input.script_revision", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"` + fixture.scriptRevisionID.String() + `"}`)},
				{ID: "evidence", DefinitionKey: "agent.source_evidence", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "story", DefinitionKey: "agent.story_analysis", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "story-review", DefinitionKey: "agent.story_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"max_repair_rounds":2}`)},
				{ID: "bible-review", DefinitionKey: "human.production_bible_review", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{"expected_bible_version":1}`)},
				{ID: "bible-materialization", DefinitionKey: "production.bible_materialization", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "episode-segmentation", DefinitionKey: "agent.episode_segmentation", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "episode-review", DefinitionKey: "human.episode_plan_review", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{
				{ID: "script-to-evidence", FromNodeID: "script", FromPort: "script", ToNodeID: "evidence", ToPort: "script"},
				{ID: "evidence-to-story", FromNodeID: "evidence", FromPort: "evidence", ToNodeID: "story", ToPort: "evidence"},
				{ID: "story-to-review", FromNodeID: "story", FromPort: "candidate", ToNodeID: "story-review", ToPort: "candidate"},
				{ID: "review-to-bible", FromNodeID: "story-review", FromPort: "candidate", ToNodeID: "bible-review", ToPort: "candidate"},
				{ID: "bible-to-materialization", FromNodeID: "bible-review", FromPort: "bible", ToNodeID: "bible-materialization", ToPort: "bible"},
				{ID: "evidence-to-segmentation", FromNodeID: "evidence", FromPort: "evidence", ToNodeID: "episode-segmentation", ToPort: "evidence"},
				{ID: "materialization-to-segmentation", FromNodeID: "bible-materialization", FromPort: "materialization", ToNodeID: "episode-segmentation", ToPort: "materialization"},
				{ID: "segmentation-to-episode-review", FromNodeID: "episode-segmentation", FromPort: "candidate", ToNodeID: "episode-review", ToPort: "candidate"},
			},
		},
		Layout: json.RawMessage(`{"guided":{"step":3}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "source-evidence-authoring-create",
	})
	if err != nil {
		t.Fatalf("create Source Evidence authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "source-evidence-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish Source Evidence authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default",
		TaskQueue: "lanverse-source-evidence-worker-test-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	bibleStore := biblegorm.New(database)
	evidenceService := bibleapp.NewSourceEvidenceService(bibleStore, bibleapp.SourceEvidenceConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		MaxShardCodePoints: 24, OverlapCodePoints: 3,
	})
	storyAnalysisService := bibleapp.NewStoryAnalysisService(bibleStore, bibleapp.StoryAnalysisConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, FanIn: 2,
	})
	episodeSegmentationService := bibleapp.NewEpisodeSegmentationService(bibleStore, bibleapp.EpisodeSegmentationConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	storyReviewService := bibleapp.NewStoryReviewService(
		bibleStore,
		bibleapp.NewStoryCandidateRepairService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}),
		bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, ClaimLease: time.Minute,
	})
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	planningService := planningapp.NewService(planninggorm.New(database), planningapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		evidenceService,
		storyAnalysisService,
		storyReviewService,
		bibleService,
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningService,
		storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		reviewService,
		nil, nil, episodeSegmentationService,
	)
	if err != nil {
		t.Fatalf("compose Source Evidence Workflow Runtime: %v", err)
	}
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		t.Fatalf("compose Source Evidence Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Source Evidence Temporal Worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	agentContext, stopAgent := context.WithCancel(ctx)
	lateRelease := make(chan struct{})
	deadlineRecoveryRelease := make(chan struct{})
	var releaseOnce sync.Once
	var deadlineReleaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(lateRelease) })
		deadlineReleaseOnce.Do(func() { close(deadlineRecoveryRelease) })
	})
	agent := &recoveringSourceEvidenceAgent{
		failed: map[string]bool{}, lateRelease: lateRelease,
		deadlineRecoveryRelease: deadlineRecoveryRelease,
	}
	agentLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentWorker := bibleapp.NewSourceEvidenceWorker(
		bibleStore, evidenceService, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	storyWorker := bibleapp.NewStoryAnalysisWorker(
		bibleStore, storyAnalysisService, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	storyReviewWorker := bibleapp.NewStoryReviewWorker(
		bibleStore, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	episodeSegmentationWorker := bibleapp.NewEpisodeSegmentationWorker(
		bibleStore, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	bibleWorker := bibleapp.NewWorker(
		bibleStore, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	go agentWorker.Run(agentContext)
	go agentWorker.Run(agentContext)
	go storyWorker.Run(agentContext)
	go storyWorker.Run(agentContext)
	go storyReviewWorker.Run(agentContext)
	go storyReviewWorker.Run(agentContext)
	go episodeSegmentationWorker.Run(agentContext)
	go episodeSegmentationWorker.Run(agentContext)
	go bibleWorker.Run(agentContext)
	t.Cleanup(stopAgent)

	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "source-evidence-start",
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start Source Evidence Workflow: run=%#v err=%v", run, err)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			var count int64
			if queryErr := database.Model(&model.ShardManifest{}).
				Where("workflow_run_id = ?", run.ID).Count(&count).Error; queryErr == nil && count >= 2 {
				releaseOnce.Do(func() { close(lateRelease) })
				return
			}
			select {
			case <-agentContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	recoveryDeadline := time.Now().Add(20 * time.Second)
	var deadlineFailure model.AgentInvocation
	for {
		var failedInvocations []model.AgentInvocation
		lookup := database.Where(
			"workflow_run_id = ? AND request_type IN ? AND status = ?",
			run.ID, []string{"story_analysis_shard", "story_reconcile_shard"}, "failed",
		).Order("updated_at DESC").Find(&failedInvocations)
		if lookup.Error != nil {
			t.Fatalf("load deadline-failed Story invocation: %v", lookup.Error)
		}
		for index := range failedInvocations {
			if strings.Contains(string(failedInvocations[index].Error), "execution_deadline_exceeded") {
				deadlineFailure = failedInvocations[index]
				break
			}
		}
		if deadlineFailure.ID != uuid.Nil {
			break
		}
		var observedRun model.WorkflowRun
		if err = database.First(&observedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load Workflow while waiting for deadline: %v", err)
		}
		if observedRun.Status == "FAILED" || observedRun.Status == "CANCELLED" || time.Now().After(recoveryDeadline) {
			t.Fatalf(
				"Story deadline did not remain recoverable: run_status=%s failed_invocations=%d",
				observedRun.Status, len(failedInvocations),
			)
		}
		time.Sleep(20 * time.Millisecond)
	}
	deadlineIdentity := struct {
		ID, StageInstanceKey, InputHash, ManifestHash string
		ClaimVersion, Attempts                        int
	}{
		ID: deadlineFailure.ID.String(), StageInstanceKey: deadlineFailure.StageInstanceKey,
		InputHash: deadlineFailure.InputHash, ManifestHash: deadlineFailure.ShardManifestHash,
		ClaimVersion: deadlineFailure.ClaimVersion, Attempts: deadlineFailure.Attempts,
	}
	var successfulSibling model.AgentInvocation
	if err = database.Where(
		"node_run_id = ? AND id <> ? AND status = ?", deadlineFailure.NodeRunID, deadlineFailure.ID, "succeeded",
	).Order("updated_at").First(&successfulSibling).Error; err != nil {
		t.Fatalf("deadline failure removed or preempted successful sibling: %v", err)
	}
	var siblingRevision model.StageCandidateRevision
	if err = database.First(&siblingRevision, "source_invocation_id = ?", successfulSibling.ID).Error; err != nil {
		t.Fatalf("load successful sibling Candidate Revision: %v", err)
	}
	recovered, err := storyAnalysisService.Recover(ctx, bibleapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, bibleapp.StoryAnalysisRecoveryCommand{
		WorkflowRunID: run.ID, NodeRunID: deadlineFailure.NodeRunID.String(),
		IdempotencyKey: "recover-story-deadline",
	})
	if err != nil || recovered.InvocationID != deadlineIdentity.ID || recovered.Status != "queued" || recovered.ReceiptID == "" {
		t.Fatalf("recover Story deadline: result=%#v err=%v", recovered, err)
	}
	replayedRecovery, err := storyAnalysisService.Recover(ctx, bibleapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, bibleapp.StoryAnalysisRecoveryCommand{
		WorkflowRunID: run.ID, NodeRunID: deadlineFailure.NodeRunID.String(),
		IdempotencyKey: "recover-story-deadline",
	})
	if err != nil || replayedRecovery.ReceiptID != recovered.ReceiptID || replayedRecovery.InvocationID != recovered.InvocationID {
		t.Fatalf("replay Story deadline recovery: result=%#v err=%v", replayedRecovery, err)
	}
	for {
		if err = database.First(&deadlineFailure, "id = ?", deadlineIdentity.ID).Error; err != nil {
			t.Fatalf("reload recovered Story invocation: %v", err)
		}
		if deadlineFailure.Status == "running" && deadlineFailure.ClaimVersion == deadlineIdentity.ClaimVersion+1 {
			break
		}
		if time.Now().After(recoveryDeadline) {
			t.Fatalf("recovered Story invocation was not reclaimed: %#v", deadlineFailure)
		}
		time.Sleep(10 * time.Millisecond)
	}
	lateApplied, err := bibleStore.FailStoryAnalysisInvocation(
		ctx, deadlineIdentity.ID, deadlineIdentity.ClaimVersion, "failed", "late_worker",
		"旧 Worker 的迟到结果", false, time.Now().UTC(),
	)
	if err != nil || lateApplied {
		t.Fatalf("pre-recovery claim crossed the recovery fence: applied=%v err=%v", lateApplied, err)
	}
	deadlineReleaseOnce.Do(func() { close(deadlineRecoveryRelease) })

	deadline := time.Now().Add(50 * time.Second)
	var persistedRun model.WorkflowRun
	var bibleGateNode model.NodeRunProjection
	var bibleTask model.HumanTask
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load Source Evidence Workflow Run: %v", err)
		}
		if persistedRun.Status == "WAITING_HUMAN" {
			if queryErr := database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "bible-review").First(&bibleGateNode).Error; queryErr == nil &&
				bibleGateNode.Status == "WAITING_HUMAN" {
				if queryErr = database.Where("node_run_id = ?", bibleGateNode.ID).First(&bibleTask).Error; queryErr == nil {
					break
				}
			}
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("Source Evidence Workflow did not reach the Production Bible Human Gate: %#v", persistedRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var storyReviewNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "story-review").First(&storyReviewNode).Error; err != nil {
		t.Fatalf("load reviewed Story Candidate NodeRun: %v", err)
	}
	reviewedCandidateOutput, _, _, err := workflow.ParseNodeOutput(json.RawMessage(storyReviewNode.Output))
	if err != nil || len(reviewedCandidateOutput.Bindings) != 1 {
		t.Fatalf("parse reviewed Story Candidate output: output=%#v err=%v", reviewedCandidateOutput, err)
	}
	reviewedCandidate := reviewedCandidateOutput.Bindings[0]
	var taskCandidates []string
	if err = json.Unmarshal(bibleTask.CandidateIDs, &taskCandidates); err != nil || len(taskCandidates) != 1 ||
		bibleTask.SubjectType != "story_reconciliation_candidate" || bibleTask.SubjectID.String() != reviewedCandidate.ReferenceID ||
		bibleTask.SubjectHash != reviewedCandidate.ContentHash || taskCandidates[0] != reviewedCandidate.ReferenceID {
		t.Fatalf("Production Bible HumanTask did not freeze the reviewed Candidate: task=%#v candidates=%v err=%v", bibleTask, taskCandidates, err)
	}
	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claimed, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: bibleTask.ID.String(), ExpectedRevision: bibleTask.Revision, IdempotencyKey: "story-bible-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Production Bible HumanTask: %v", err)
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: claimed.Task.ID, ClaimToken: claimed.ClaimToken, ExpectedTaskRevision: claimed.Task.Revision,
		ExpectedSubjectRevision: claimed.Task.SubjectRevision, ExpectedSubjectHash: claimed.Task.SubjectHash,
		Decision: "approved", IdempotencyKey: "story-bible-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Production Bible HumanTask: %v", err)
	}
	unknownSignaler := &unknownOnceWorkflowSignaler{delegate: temporalRuntime}
	signalService := workflowapp.NewSignalService(workflowStore, unknownSignaler, workflowapp.SignalConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Owner: workflowproduction.New(bibleService, planningService, storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		})),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: run.ID, NodeRunID: bibleGateNode.ID.String(),
		HumanTaskID: decision.Task.ID, ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: decision.Decision.Decision,
		IdempotencyKey: "story-bible-review-signal",
	}
	unknownIntent, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, signalCommand)
	if err != nil || unknownIntent.Status != "unknown" || unknownIntent.AttemptNo != 1 {
		t.Fatalf("persist unknown Production Bible Signal: intent=%#v err=%v", unknownIntent, err)
	}
	var version model.ProductionBibleVersion
	if err = database.Where("review_decision_id = ?", decision.Decision.ID).First(&version).Error; err != nil {
		t.Fatalf("load confirmed Production Bible Version after unknown Signal: %v", err)
	}
	var confirmReceipt model.CommandReceipt
	if err = database.Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "production_bible.confirm", version.ID,
	).First(&confirmReceipt).Error; err != nil {
		t.Fatalf("load Production Bible confirmation Receipt: %v", err)
	}
	var confirmedSnapshot bibledomain.StoryReconciliationCandidate
	if err = json.Unmarshal(version.Snapshot, &confirmedSnapshot); err != nil {
		t.Fatalf("decode confirmed Production Bible Version: %v", err)
	}
	expectedAssetCount, expectedStateCount, generatedBeforeReceipt := 0, 0, 0
	for _, entity := range confirmedSnapshot.CanonicalEntities {
		if entity.Kind != "character" && entity.Kind != "location" && entity.Kind != "prop" {
			continue
		}
		expectedAssetCount++
		stateCount, hasBase := len(entity.States), false
		for _, state := range entity.States {
			hasBase = hasBase || state.StateKey == "base"
		}
		if !hasBase {
			stateCount++
		}
		expectedStateCount += stateCount
		generatedBeforeReceipt += 3 + stateCount
	}
	if expectedAssetCount == 0 || expectedStateCount < expectedAssetCount {
		t.Fatalf("confirmed Production Bible has no materializable identities: %#v", confirmedSnapshot)
	}
	generatedIDs := 0
	generatedFactIDs := make([]string, 0, generatedBeforeReceipt)
	failingMaterializer := bibleapp.NewService(bibleStore, bibleapp.Config{
		Now: func() time.Time { return time.Now().UTC() },
		NewID: func() string {
			generatedIDs++
			if generatedIDs == generatedBeforeReceipt+1 {
				return confirmReceipt.ID.String()
			}
			id := uuid.NewString()
			generatedFactIDs = append(generatedFactIDs, id)
			return id
		},
	})
	_, err = failingMaterializer.MaterializeConfirmedBible(ctx, bibleapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, bibleapp.MaterializeCommand{
		BibleVersionID: version.ID.String(), ExpectedVersion: version.Version,
		ExpectedContentHash: version.ContentHash, IdempotencyKey: "story-bible-materialization-rollback",
	})
	var materializationConflict *bibleapp.Error
	if !errors.As(err, &materializationConflict) || materializationConflict.Code != "resource_conflict" {
		t.Fatalf("forced materialization Receipt collision error=%#v", err)
	}
	var rolledBackAssetCount, rolledBackStateCount, rolledBackSpecificationCount int64
	var rolledBackBindingCount, rolledBackBindingStateCount, rolledBackReceiptCount int64
	if err = database.Model(&model.Asset{}).Where("project_id = ?", fixture.projectID).
		Count(&rolledBackAssetCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AssetState{}).Where("project_id = ?", fixture.projectID).
		Count(&rolledBackStateCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ProductionBibleSpecificationVersion{}).Where("project_id = ?", fixture.projectID).
		Count(&rolledBackSpecificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ProductionBinding{}).Where("bible_version_id = ?", version.ID).
		Count(&rolledBackBindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ProductionBindingState{}).Where("production_binding_id IN ?", generatedFactIDs).
		Count(&rolledBackBindingStateCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND idempotency_key = ?",
		fixture.workspaceID, "production_bible.materialize_confirmed", "story-bible-materialization-rollback",
	).Count(&rolledBackReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBackAssetCount != 0 || rolledBackStateCount != 0 || rolledBackSpecificationCount != 0 ||
		rolledBackBindingCount != 0 || rolledBackBindingStateCount != 0 || rolledBackReceiptCount != 0 {
		t.Fatalf(
			"failed Production Bible materialization leaked facts: assets=%d states=%d specifications=%d bindings=%d binding_states=%d receipts=%d",
			rolledBackAssetCount, rolledBackStateCount, rolledBackSpecificationCount,
			rolledBackBindingCount, rolledBackBindingStateCount, rolledBackReceiptCount,
		)
	}
	var versionCount, receiptCount, legacyBibleCount, artifactCount, episodeCount, shotCount, storyGraphCount int64
	for target, destination := range map[any]*int64{
		&model.ProductionBibleVersion{}: &versionCount,
		&model.ProductionBible{}:        &legacyBibleCount,
		&model.Artifact{}:               &artifactCount,
		&model.Episode{}:                &episodeCount,
		&model.StoryboardShot{}:         &shotCount,
		&model.StoryGraphVersion{}:      &storyGraphCount,
	} {
		if err = database.Model(target).Where("workspace_id = ?", fixture.workspaceID).Count(destination).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?", fixture.workspaceID, "production_bible.confirm", version.ID,
	).Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if versionCount != 1 || receiptCount != 1 || legacyBibleCount != 0 || artifactCount != 0 || episodeCount != 0 || shotCount != 0 || storyGraphCount != 0 {
		t.Fatalf("Production Bible confirm boundary counts: version=%d receipt=%d legacy=%d artifact=%d episode=%d shot=%d storygraph=%d",
			versionCount, receiptCount, legacyBibleCount, artifactCount, episodeCount, shotCount, storyGraphCount)
	}
	completedIntent, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, signalCommand)
	if err != nil || completedIntent.ID != unknownIntent.ID || completedIntent.Status != "completed" || completedIntent.AttemptNo != 2 {
		t.Fatalf("recover unknown Production Bible Signal: intent=%#v err=%v", completedIntent, err)
	}
	replayedIntent, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, signalCommand)
	if err != nil || replayedIntent.ID != completedIntent.ID || replayedIntent.Status != "completed" {
		t.Fatalf("replay completed Production Bible Signal: intent=%#v err=%v", replayedIntent, err)
	}
	var episodeGateNode model.NodeRunProjection
	var episodeTask model.HumanTask
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("reload Source Evidence Workflow Run: %v", err)
		}
		if persistedRun.Status == "WAITING_HUMAN" {
			if queryErr := database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "episode-review").First(&episodeGateNode).Error; queryErr == nil &&
				episodeGateNode.Status == "WAITING_HUMAN" {
				if queryErr = database.Where("node_run_id = ?", episodeGateNode.ID).First(&episodeTask).Error; queryErr == nil {
					break
				}
			}
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("Source Evidence Workflow did not reach the Episode Plan Human Gate: %#v", persistedRun)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "bible-review").First(&bibleGateNode).Error; err != nil {
		t.Fatal(err)
	}
	bibleOutput, _, bibleOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(bibleGateNode.Output))
	if err != nil || bibleGateNode.Status != "SUCCEEDED" || bibleGateNode.OutputHash == nil ||
		*bibleGateNode.OutputHash != bibleOutputHash || len(bibleOutput.Bindings) != 1 ||
		bibleOutput.Bindings[0].ValueType != "production_bible_version" ||
		bibleOutput.Bindings[0].ReferenceID != version.ID.String() ||
		bibleOutput.Bindings[0].ReferenceVersion != "1" || bibleOutput.Bindings[0].ContentHash != version.ContentHash {
		t.Fatalf("Production Bible Gate output=%#v node=%#v err=%v", bibleOutput, bibleGateNode, err)
	}
	var materializationNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "bible-materialization").
		First(&materializationNode).Error; err != nil {
		t.Fatalf("load Production Bible materialization NodeRun: %v", err)
	}
	materializationOutput, _, materializationOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(materializationNode.Output))
	if err != nil || materializationNode.Status != "SUCCEEDED" || materializationNode.OutputHash == nil ||
		*materializationNode.OutputHash != materializationOutputHash || len(materializationOutput.Bindings) != 1 ||
		materializationOutput.Bindings[0].Port != "materialization" ||
		materializationOutput.Bindings[0].ValueType != "production_bible_materialization" ||
		materializationOutput.Bindings[0].ReferenceID != version.ID.String() ||
		materializationOutput.Bindings[0].ReferenceVersion != "1" {
		t.Fatalf("Production Bible materialization output=%#v node=%#v err=%v", materializationOutput, materializationNode, err)
	}
	var segmentationNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "episode-segmentation").
		First(&segmentationNode).Error; err != nil {
		t.Fatalf("load Episode segmentation NodeRun: %v", err)
	}
	segmentationOutput, _, segmentationOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(segmentationNode.Output))
	if err != nil || segmentationNode.Status != "SUCCEEDED" || segmentationNode.OutputHash == nil ||
		*segmentationNode.OutputHash != segmentationOutputHash || len(segmentationOutput.Bindings) != 1 ||
		segmentationOutput.Bindings[0].Port != "candidate" ||
		segmentationOutput.Bindings[0].ValueType != "episode_segmentation_candidate" {
		t.Fatalf("Episode segmentation output=%#v node=%#v err=%v", segmentationOutput, segmentationNode, err)
	}
	var segmentationInvocation model.AgentInvocation
	if err = database.Where("node_run_id = ? AND request_type = ?", segmentationNode.ID, "episode_segmentation").
		First(&segmentationInvocation).Error; err != nil {
		t.Fatalf("load Episode segmentation invocation: %v", err)
	}
	var segmentationRevisionCount, segmentationHeadCount, postSegmentationEpisodeCount int64
	if err = database.Model(&model.StageCandidateRevision{}).
		Where("source_invocation_id = ?", segmentationInvocation.ID).Count(&segmentationRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StageCandidateHead{}).
		Where("stage_instance_key = ?", segmentationInvocation.StageInstanceKey).Count(&segmentationHeadCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).
		Count(&postSegmentationEpisodeCount).Error; err != nil {
		t.Fatal(err)
	}
	var segmentationCandidate bibledomain.EpisodeSegmentationCandidate
	agent.mutex.Lock()
	segmentInvocationID := agent.segmentInvocationID
	agent.mutex.Unlock()
	if err = json.Unmarshal(segmentationInvocation.Candidate, &segmentationCandidate); err != nil ||
		segmentationInvocation.Status != "succeeded" || segmentationInvocation.Attempts < 2 ||
		segmentationInvocation.ID.String() != segmentInvocationID ||
		len(segmentationCandidate.Boundaries) != 3 || segmentationCandidate.Boundaries[0].AbsoluteStart != 0 ||
		segmentationCandidate.Boundaries[len(segmentationCandidate.Boundaries)-1].AbsoluteEnd != len([]rune(text)) ||
		segmentationRevisionCount != 1 || segmentationHeadCount != 1 || postSegmentationEpisodeCount != 0 {
		t.Fatalf(
			"Episode segmentation facts: invocation=%#v candidate=%#v revisions=%d heads=%d episodes=%d err=%v",
			segmentationInvocation, segmentationCandidate, segmentationRevisionCount, segmentationHeadCount,
			postSegmentationEpisodeCount, err,
		)
	}
	var episodeTaskCandidateIDs []string
	if err = json.Unmarshal(episodeTask.CandidateIDs, &episodeTaskCandidateIDs); err != nil || len(episodeTaskCandidateIDs) != 1 ||
		episodeTask.SubjectType != "episode_plan_candidate" || episodeTask.SubjectID.String() != segmentationOutput.Bindings[0].ReferenceID ||
		episodeTask.SubjectRevision != 1 || episodeTask.SubjectHash != segmentationOutput.Bindings[0].ContentHash ||
		episodeTaskCandidateIDs[0] != segmentationOutput.Bindings[0].ReferenceID {
		t.Fatalf("Episode Plan HumanTask did not freeze the segmentation Candidate: task=%#v candidates=%v err=%v", episodeTask, episodeTaskCandidateIDs, err)
	}

	duplicateVersionID := uuid.NewString()
	rollbackIDCall := 0
	rollbackPlanningService := planningapp.NewService(planninggorm.New(database), planningapp.Config{
		Now: func() time.Time { return time.Now().UTC() },
		NewID: func() string {
			rollbackIDCall++
			switch rollbackIDCall {
			case 3, 5:
				return duplicateVersionID
			default:
				return uuid.NewString()
			}
		},
	})
	_, err = rollbackPlanningService.ApplyEpisodePlan(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID:       segmentationOutput.Bindings[0].ReferenceID,
		CandidateRevisionHash:     segmentationOutput.Bindings[0].ContentHash,
		ExpectedCandidateRevision: 1, ReviewDecisionID: uuid.NewString(),
		IdempotencyKey: "episode-plan-forced-batch-rollback",
	})
	if err == nil {
		t.Fatal("forced second ScriptVersion collision did not fail the Episode set transaction")
	}
	var rolledBackEpisodeCount, rolledBackVersionCount, rolledBackEventCount, rolledBackEpisodeReceiptCount int64
	if err = database.Model(&model.Episode{}).Where("project_id = ?", fixture.projectID).Count(&rolledBackEpisodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EpisodeScriptVersion{}).Where("project_id = ?", fixture.projectID).Count(&rolledBackVersionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.OutboxEvent{}).Where("project_id = ? AND event_type = ?", fixture.projectID, "ScriptVersionPublished").Count(&rolledBackEventCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND idempotency_key = ?",
		fixture.workspaceID, "episode_plan.apply", "episode-plan-forced-batch-rollback",
	).Count(&rolledBackEpisodeReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBackEpisodeCount != 0 || rolledBackVersionCount != 0 || rolledBackEventCount != 0 || rolledBackEpisodeReceiptCount != 0 {
		t.Fatalf("failed Episode set transaction leaked facts: episodes=%d versions=%d events=%d receipts=%d",
			rolledBackEpisodeCount, rolledBackVersionCount, rolledBackEventCount, rolledBackEpisodeReceiptCount)
	}

	episodeClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: episodeTask.ID.String(), ExpectedRevision: episodeTask.Revision,
		IdempotencyKey: "episode-plan-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Episode Plan HumanTask: %v", err)
	}
	episodeDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: episodeClaim.Task.ID, ClaimToken: episodeClaim.ClaimToken,
		ExpectedTaskRevision:    episodeClaim.Task.Revision,
		ExpectedSubjectRevision: episodeClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     episodeClaim.Task.SubjectHash,
		Decision:                "approved", IdempotencyKey: "episode-plan-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Episode Plan HumanTask: %v", err)
	}
	episodeSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: run.ID, NodeRunID: episodeGateNode.ID.String(),
		HumanTaskID: episodeDecision.Task.ID, ReviewDecisionID: episodeDecision.Decision.ID,
		SubjectRevision: episodeDecision.Decision.SubjectRevision, Decision: episodeDecision.Decision.Decision,
		IdempotencyKey: "episode-plan-review-signal",
	}
	episodeSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, episodeSignalCommand)
	if err != nil || episodeSignal.Status != "completed" {
		t.Fatalf("resume Episode Plan Human Gate: intent=%#v err=%v", episodeSignal, err)
	}
	replayedEpisodeSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, episodeSignalCommand)
	if err != nil || replayedEpisodeSignal.ID != episodeSignal.ID || replayedEpisodeSignal.Status != "completed" {
		t.Fatalf("replay Episode Plan Human Gate signal: intent=%#v err=%v", replayedEpisodeSignal, err)
	}
	completionDeadline := time.Now().Add(20 * time.Second)
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("reload completed Episode Plan Workflow Run: %v", err)
		}
		if persistedRun.Status == "SUCCEEDED" {
			break
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(completionDeadline) {
			t.Fatalf("Source Evidence Workflow did not complete after Episode Plan approval: %#v", persistedRun)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = database.First(&episodeGateNode, "id = ?", episodeGateNode.ID).Error; err != nil {
		t.Fatal(err)
	}
	episodeOutput, _, episodeOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(episodeGateNode.Output))
	if err != nil || episodeGateNode.Status != "SUCCEEDED" || episodeGateNode.OutputHash == nil ||
		*episodeGateNode.OutputHash != episodeOutputHash || len(episodeOutput.Bindings) != 1 ||
		episodeOutput.Bindings[0].Port != "episodes" || episodeOutput.Bindings[0].ValueType != "episode_set" ||
		episodeOutput.Bindings[0].ReferenceVersion != "1" || len(episodeOutput.Bindings[0].ContentHash) != 64 {
		t.Fatalf("Episode Plan Gate output=%#v node=%#v err=%v", episodeOutput, episodeGateNode, err)
	}
	var episodeSetReceipt model.CommandReceipt
	if err = database.Where("id = ? AND workspace_id = ? AND operation = ?", episodeOutput.Bindings[0].ReferenceID, fixture.workspaceID, "episode_plan.apply").
		First(&episodeSetReceipt).Error; err != nil {
		t.Fatalf("load Episode set owner Receipt: %v", err)
	}
	var publishedEpisodes []model.Episode
	if err = database.Where("project_id = ?", fixture.projectID).Order("position").Find(&publishedEpisodes).Error; err != nil {
		t.Fatal(err)
	}
	var publishedVersions []model.EpisodeScriptVersion
	if err = database.Where("project_id = ?", fixture.projectID).Order("source_start").Find(&publishedVersions).Error; err != nil {
		t.Fatal(err)
	}
	var publishedEventCount int64
	if err = database.Model(&model.OutboxEvent{}).Where(
		"project_id = ? AND event_type = ? AND source_receipt_id = ?",
		fixture.projectID, "ScriptVersionPublished", episodeSetReceipt.ID,
	).Count(&publishedEventCount).Error; err != nil {
		t.Fatal(err)
	}
	joined, previousEnd := "", 0
	for index := range publishedEpisodes {
		episode, scriptVersion := publishedEpisodes[index], publishedVersions[index]
		if episode.Position != index+1 || episode.CurrentScriptVersionID == nil || *episode.CurrentScriptVersionID != scriptVersion.ID ||
			scriptVersion.EpisodeID != episode.ID || scriptVersion.Status != "published" || scriptVersion.VersionNo != 1 ||
			scriptVersion.SourceStart != previousEnd || scriptVersion.ContentHash != bibledomain.SourceTextHash(scriptVersion.Content) {
			t.Fatalf("published Episode %d is not bound to its exact ScriptVersion: episode=%#v version=%#v", index, episode, scriptVersion)
		}
		joined += scriptVersion.Content
		previousEnd = scriptVersion.SourceEnd
	}
	if len(publishedEpisodes) != len(segmentationCandidate.Boundaries) || len(publishedVersions) != len(segmentationCandidate.Boundaries) ||
		publishedEventCount != int64(len(segmentationCandidate.Boundaries)) || joined != text || previousEnd != len([]rune(text)) {
		t.Fatalf("published Episode set is incomplete: episodes=%d versions=%d events=%d source_end=%d joined=%q",
			len(publishedEpisodes), len(publishedVersions), publishedEventCount, previousEnd, joined)
	}

	replayCommand := planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID:       segmentationOutput.Bindings[0].ReferenceID,
		CandidateRevisionHash:     segmentationOutput.Bindings[0].ContentHash,
		ExpectedCandidateRevision: 1, ReviewDecisionID: episodeDecision.Decision.ID,
		IdempotencyKey: "workflow-review:" + episodeDecision.Decision.ID,
	}
	replayResults := make(chan planningapp.ApplyEpisodePlanResult, 2)
	replayErrors := make(chan error, 2)
	for range 2 {
		go func() {
			result, applyErr := planningService.ApplyEpisodePlan(ctx, planningapp.Actor{
				UserID: fixture.userID.String(), TokenVersion: 1,
			}, replayCommand)
			replayResults <- result
			replayErrors <- applyErr
		}()
	}
	for range 2 {
		result, applyErr := <-replayResults, <-replayErrors
		if applyErr != nil || result.Receipt.ID != episodeSetReceipt.ID.String() ||
			result.Set.ID != episodeOutput.Bindings[0].ReferenceID || result.Set.ContentHash != episodeOutput.Bindings[0].ContentHash {
			t.Fatalf("concurrent Episode set replay drifted: result=%#v err=%v", result, applyErr)
		}
	}
	_, err = planningService.ApplyEpisodePlan(ctx, planningapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID:       segmentationOutput.Bindings[0].ReferenceID,
		CandidateRevisionHash:     segmentationOutput.Bindings[0].ContentHash,
		ExpectedCandidateRevision: 1, ReviewDecisionID: uuid.NewString(),
		IdempotencyKey: "episode-plan-boundary-conflict",
	})
	var boundaryConflict *planningapp.Error
	if !errors.As(err, &boundaryConflict) || boundaryConflict.Code != "resource_conflict" {
		t.Fatalf("existing Episode boundaries did not reject a second owner application: %#v", err)
	}
	var conflictReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND idempotency_key = ?",
		fixture.workspaceID, "episode_plan.apply", "episode-plan-boundary-conflict",
	).Count(&conflictReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if conflictReceiptCount != 0 {
		t.Fatalf("boundary conflict created an Episode Plan Receipt: %d", conflictReceiptCount)
	}
	var assets []model.Asset
	if err = database.Where("project_id = ?", fixture.projectID).Order("identity_key").Find(&assets).Error; err != nil {
		t.Fatal(err)
	}
	assetIDs := make([]uuid.UUID, 0, len(assets))
	for _, asset := range assets {
		assetIDs = append(assetIDs, asset.ID)
	}
	var states []model.AssetState
	if err = database.Where("asset_id IN ?", assetIDs).Order("asset_id, state_key, revision").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	var specifications []model.ProductionBibleSpecificationVersion
	if err = database.Where("source_bible_version_id = ?", version.ID).Order("asset_id, version").Find(&specifications).Error; err != nil {
		t.Fatal(err)
	}
	var bindings []model.ProductionBinding
	if err = database.Where("bible_version_id = ?", version.ID).Order("entity_key").Find(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	bindingIDs := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		bindingIDs = append(bindingIDs, binding.ID)
	}
	var bindingStateCount, materializationReceiptCount int64
	if err = database.Model(&model.ProductionBindingState{}).Where("production_binding_id IN ?", bindingIDs).
		Count(&bindingStateCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "production_bible.materialize_confirmed", version.ID,
	).Count(&materializationReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if len(assets) != expectedAssetCount || len(states) != expectedStateCount ||
		len(specifications) != expectedAssetCount || len(bindings) != expectedAssetCount ||
		bindingStateCount != int64(expectedStateCount) || materializationReceiptCount != 1 ||
		materializationOutput.Bindings[0].ContentHash == version.ContentHash {
		t.Fatalf(
			"Production Bible materialization facts: assets=%d states=%d specifications=%d bindings=%d binding_states=%d receipts=%d output=%#v",
			len(assets), len(states), len(specifications), len(bindings), bindingStateCount,
			materializationReceiptCount, materializationOutput,
		)
	}
	var bindingState model.ProductionBindingState
	if err = database.Where("production_binding_id IN ?", bindingIDs).Order("production_binding_id, position").
		First(&bindingState).Error; err != nil {
		t.Fatalf("load Production Binding State: %v", err)
	}
	immutableWrites := []struct {
		name string
		err  error
		want error
	}{
		{"Asset update", database.Model(&assets[0]).Update("identity_key", "character:drift").Error, model.ErrImmutableAsset},
		{"AssetState delete", database.Delete(&states[0]).Error, model.ErrImmutableAssetState},
		{"SpecificationVersion update", database.Model(&specifications[0]).Update("version", 2).Error, model.ErrImmutableBibleSpecificationVersion},
		{"ProductionBinding delete", database.Delete(&bindings[0]).Error, model.ErrImmutableProductionBinding},
		{"ProductionBindingState delete", database.Delete(&bindingState).Error, model.ErrImmutableProductionBindingState},
	}
	for _, write := range immutableWrites {
		if !errors.Is(write.err, write.want) {
			t.Fatalf("%s error=%v", write.name, write.err)
		}
	}
	var confirmedCandidate, parentCandidate model.StageCandidateRevision
	if err = database.First(&confirmedCandidate, "id = ?", version.CandidateRevisionID).Error; err != nil {
		t.Fatalf("load confirmed Production Bible Candidate: %v", err)
	}
	if confirmedCandidate.ParentCandidateRevisionID == nil {
		t.Fatal("confirmed Production Bible Candidate has no repair parent for Head drift verification")
	}
	if err = database.First(&parentCandidate, "id = ?", *confirmedCandidate.ParentCandidateRevisionID).Error; err != nil {
		t.Fatalf("load Production Bible Candidate parent: %v", err)
	}
	moveCandidateHead := func(target model.StageCandidateRevision) {
		t.Helper()
		updated := database.Model(&model.StageCandidateHead{}).Where(
			"workspace_id = ? AND stage_instance_key = ?", fixture.workspaceID, confirmedCandidate.StageInstanceKey,
		).Updates(map[string]any{
			"current_revision_id": target.ID, "current_candidate_revision_hash": target.CandidateRevisionHash,
			"revision": target.RevisionNo, "updated_at": time.Now().UTC(),
		})
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("move Production Bible Candidate Head to revision %d: rows=%d err=%v", target.RevisionNo, updated.RowsAffected, updated.Error)
		}
	}
	openCandidateTask := func(nodeRunID string) reviewapp.OpenCommand {
		t.Helper()
		return reviewapp.OpenCommand{
			WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
			WorkflowRunID: run.ID, NodeRunID: nodeRunID,
			SubjectType: "story_reconciliation_candidate", SubjectID: confirmedCandidate.ID.String(),
			SubjectRevision: int(confirmedCandidate.RevisionNo), SubjectHash: confirmedCandidate.CandidateRevisionHash,
			CandidateIDs: []string{confirmedCandidate.ID.String()}, RubricVersion: "production-bible-v1",
			AllowedDecisions: []string{"approved", "rejected", "changes_requested"},
		}
	}

	staleTask, err := reviewService.Open(ctx, openCandidateTask(uuid.NewString()))
	if err != nil {
		t.Fatalf("open pre-decision stale Production Bible task: %v", err)
	}
	staleClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: staleTask.ID, ExpectedRevision: staleTask.Revision, IdempotencyKey: "story-bible-stale-claim",
	})
	if err != nil {
		t.Fatalf("claim pre-decision stale Production Bible task: %v", err)
	}
	moveCandidateHead(parentCandidate)
	_, err = reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: staleClaim.Task.ID, ClaimToken: staleClaim.ClaimToken,
		ExpectedTaskRevision: staleClaim.Task.Revision, ExpectedSubjectRevision: staleClaim.Task.SubjectRevision,
		ExpectedSubjectHash: staleClaim.Task.SubjectHash, Decision: "approved",
		IdempotencyKey: "story-bible-stale-decision",
	})
	var reviewConflict *reviewapp.Error
	if !errors.As(err, &reviewConflict) || reviewConflict.Code != "resource_conflict" {
		t.Fatalf("pre-decision Candidate Head drift error=%#v", err)
	}
	var persistedStaleTask model.HumanTask
	if err = database.First(&persistedStaleTask, "id = ?", staleTask.ID).Error; err != nil ||
		persistedStaleTask.Status != "STALE" || persistedStaleTask.ClaimedBy != nil ||
		persistedStaleTask.ClaimToken != nil || persistedStaleTask.ClaimExpiresAt != nil {
		t.Fatalf("pre-decision Candidate Head drift did not stale task: task=%#v err=%v", persistedStaleTask, err)
	}
	var staleDecisionCount, staleDecisionReceiptCount int64
	if err = database.Model(&model.ReviewDecision{}).Where("human_task_id = ?", staleTask.ID).Count(&staleDecisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND idempotency_key = ?",
		fixture.workspaceID, "review.human_task.decide", "story-bible-stale-decision",
	).Count(&staleDecisionReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if staleDecisionCount != 0 || staleDecisionReceiptCount != 0 {
		t.Fatalf("stale Production Bible decision leaked facts: decisions=%d receipts=%d", staleDecisionCount, staleDecisionReceiptCount)
	}

	moveCandidateHead(confirmedCandidate)
	ownerConflictTask, err := reviewService.Open(ctx, openCandidateTask(uuid.NewString()))
	if err != nil {
		t.Fatalf("open post-decision conflict Production Bible task: %v", err)
	}
	ownerConflictClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: ownerConflictTask.ID, ExpectedRevision: ownerConflictTask.Revision,
		IdempotencyKey: "story-bible-owner-conflict-claim",
	})
	if err != nil {
		t.Fatalf("claim post-decision conflict Production Bible task: %v", err)
	}
	ownerConflictDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: ownerConflictClaim.Task.ID, ClaimToken: ownerConflictClaim.ClaimToken,
		ExpectedTaskRevision:    ownerConflictClaim.Task.Revision,
		ExpectedSubjectRevision: ownerConflictClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     ownerConflictClaim.Task.SubjectHash, Decision: "approved",
		IdempotencyKey: "story-bible-owner-conflict-decision",
	})
	if err != nil {
		t.Fatalf("approve post-decision conflict Production Bible task: %v", err)
	}
	moveCandidateHead(parentCandidate)
	_, err = bibleService.Confirm(ctx, bibleapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, bibleapp.ConfirmCommand{
		CandidateRevisionID: confirmedCandidate.ID.String(), CandidateRevisionHash: confirmedCandidate.CandidateRevisionHash,
		ExpectedCandidateRevision: confirmedCandidate.RevisionNo,
		DocumentRevisionID:        fixture.scriptRevisionID.String(), DocumentRevisionHash: fixture.normalizedHash,
		ExpectedVersion: 2, ReviewDecisionID: ownerConflictDecision.Decision.ID,
		IdempotencyKey: "story-bible-owner-head-conflict",
	})
	var bibleConflict *bibleapp.Error
	if !errors.As(err, &bibleConflict) || bibleConflict.Code != "resource_conflict" {
		t.Fatalf("post-decision Candidate Head drift error=%#v", err)
	}
	var ownerConflictVersionCount, ownerConflictReceiptCount int64
	if err = database.Model(&model.ProductionBibleVersion{}).Where("workspace_id = ?", fixture.workspaceID).
		Count(&ownerConflictVersionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND idempotency_key = ?",
		fixture.workspaceID, "production_bible.confirm", "story-bible-owner-head-conflict",
	).Count(&ownerConflictReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerConflictVersionCount != 1 || ownerConflictReceiptCount != 0 {
		t.Fatalf("post-decision Candidate Head drift leaked facts: versions=%d receipts=%d", ownerConflictVersionCount, ownerConflictReceiptCount)
	}
	moveCandidateHead(confirmedCandidate)
	if err = database.Model(&version).Update("content_hash", strings.Repeat("f", 64)).Error; !errors.Is(err, model.ErrImmutableProductionBibleVersion) {
		t.Fatalf("Production Bible Version update error=%v", err)
	}
	if err = database.Delete(&version).Error; !errors.Is(err, model.ErrImmutableProductionBibleVersion) {
		t.Fatalf("Production Bible Version delete error=%v", err)
	}
	stopAgent()

	var definition model.WorkflowDefinitionVersion
	if err = database.First(&definition, "id = ?", persistedRun.WorkflowDefinitionVersionID).Error; err != nil {
		t.Fatalf("load Source Evidence WorkflowDefinitionVersion: %v", err)
	}
	var evidenceNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "evidence").First(&evidenceNode).Error; err != nil {
		t.Fatalf("load Source Evidence NodeRun: %v", err)
	}
	if evidenceNode.Status != "SUCCEEDED" || evidenceNode.OutputHash == nil {
		t.Fatalf("Source Evidence NodeRun did not persist output: %#v", evidenceNode)
	}
	var manifest model.ShardManifest
	if err = database.Where("workflow_run_id = ? AND node_run_id = ?", run.ID, evidenceNode.ID).
		Order("version DESC").First(&manifest).Error; err != nil {
		t.Fatalf("load Source Evidence ShardManifest: %v", err)
	}
	if manifest.Version != 2 || manifest.ParentManifestHash == nil ||
		manifest.Stage != bibledomain.SourceEvidenceStage || manifest.RootInputHash != fixture.normalizedHash ||
		manifest.CreatedAt.Before(definition.CreatedAt) || manifest.CreatedAt.Before(evidenceNode.CreatedAt) {
		t.Fatalf("Source Evidence was not definition-first: definition=%#v node=%#v manifest=%#v", definition, evidenceNode, manifest)
	}
	var manifestCount int64
	if err = database.Model(&model.ShardManifest{}).
		Where("workflow_run_id = ? AND node_run_id = ?", run.ID, evidenceNode.ID).
		Count(&manifestCount).Error; err != nil || manifestCount != 2 {
		t.Fatalf("Source Evidence reshard manifest count=%d err=%v", manifestCount, err)
	}
	var shards []bibledomain.SourceEvidenceShard
	if err = json.Unmarshal(manifest.Shards, &shards); err != nil {
		t.Fatal(err)
	}
	manifestDomain := bibledomain.SourceEvidenceManifest{
		ManifestID: manifest.ID.String(), Version: manifest.Version,
		ParentManifestHash: manifest.ParentManifestHash, WorkspaceID: manifest.WorkspaceID.String(),
		WorkflowRunID: manifest.WorkflowRunID.String(), NodeRunID: manifest.NodeRunID.String(),
		Stage: manifest.Stage, RootInputHash: manifest.RootInputHash, Shards: shards,
		CoverageHash: manifest.CoverageHash, ManifestHash: manifest.ManifestHash,
	}
	if err = bibledomain.ValidateSourceEvidenceManifest(manifestDomain, text); err != nil {
		t.Fatalf("persisted Source Evidence manifest is invalid: %v", err)
	}
	if err = database.Model(&manifest).Update("coverage_hash", strings.Repeat("f", 64)).Error; !errors.Is(err, model.ErrImmutableShardManifest) {
		t.Fatalf("Source Evidence manifest update error=%v", err)
	}
	if err = database.Delete(&manifest).Error; !errors.Is(err, model.ErrImmutableShardManifest) {
		t.Fatalf("Source Evidence manifest delete error=%v", err)
	}
	activeShardCount := 0
	for _, shard := range shards {
		if shard.Status == "active" {
			activeShardCount++
		}
	}

	var invocations []model.AgentInvocation
	if err = database.Where("node_run_id = ? AND shard_manifest_id = ? AND shard_manifest_version = ?", evidenceNode.ID, manifest.ID, manifest.Version).
		Order("shard_key").Find(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(invocations) != activeShardCount || len(invocations) < 2 {
		t.Fatalf("Source Evidence shard invocation count=%d active_shards=%d", len(invocations), activeShardCount)
	}
	for _, invocation := range invocations {
		if invocation.Status != "succeeded" || invocation.Attempts < 2 || invocation.WorkflowRunID == nil ||
			invocation.NodeRunID == nil || *invocation.NodeRunID != evidenceNode.ID ||
			invocation.ShardManifestID == nil || *invocation.ShardManifestID != manifest.ID ||
			invocation.ShardManifestVersion == nil || *invocation.ShardManifestVersion != manifest.Version {
			t.Fatalf("Source Evidence invocation did not recover the same identity: %#v", invocation)
		}
	}
	var invocationRevisionCount, aggregateRevisionCount int64
	if err = database.Model(&model.StageCandidateRevision{}).
		Where("workspace_id = ? AND origin_kind = ?", fixture.workspaceID, "invocation").
		Count(&invocationRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StageCandidateRevision{}).
		Where("workspace_id = ? AND origin_kind = ?", fixture.workspaceID, "aggregate").
		Count(&aggregateRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	var oldInvocation model.AgentInvocation
	if err = database.Where("node_run_id = ? AND shard_manifest_version = ? AND status = ?", evidenceNode.ID, 1, "succeeded").
		First(&oldInvocation).Error; err != nil {
		t.Fatalf("late old-manifest result was not retained for audit: %v", err)
	}
	var oldRevision model.StageCandidateRevision
	if err = database.First(&oldRevision, "source_invocation_id = ?", oldInvocation.ID).Error; err != nil {
		t.Fatalf("late old-manifest Candidate Revision was not retained: %v", err)
	}
	var aggregateRevision model.StageCandidateRevision
	if err = database.First(&aggregateRevision, "workspace_id = ? AND origin_kind = ?", fixture.workspaceID, "aggregate").Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(aggregateRevision.AggregateOrigin), oldRevision.ID.String()) {
		t.Fatalf("late old-manifest Candidate entered current aggregate: %s", aggregateRevision.AggregateOrigin)
	}
	var succeededInvocationCount int64
	if err = database.Model(&model.AgentInvocation{}).
		Where("workspace_id = ? AND status = ?", fixture.workspaceID, "succeeded").
		Count(&succeededInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if invocationRevisionCount != succeededInvocationCount || aggregateRevisionCount != 1 {
		t.Fatalf("Source Evidence revisions: invocation=%d aggregate=%d", invocationRevisionCount, aggregateRevisionCount)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(evidenceNode.Output))
	if err != nil || outputHash != *evidenceNode.OutputHash || len(output.Bindings) != 1 ||
		output.Bindings[0].Port != "evidence" || output.Bindings[0].ValueType != "source_evidence_candidate" {
		t.Fatalf("Source Evidence Node output=%#v hash=%s err=%v", output, outputHash, err)
	}

	var storyNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "story").First(&storyNode).Error; err != nil {
		t.Fatalf("load Story analysis NodeRun: %v", err)
	}
	if storyNode.Status != "SUCCEEDED" || storyNode.OutputHash == nil || storyNode.CreatedAt.Before(definition.CreatedAt) {
		t.Fatalf("Story analysis NodeRun did not complete definition-first: %#v", storyNode)
	}
	var recoveredInvocation model.AgentInvocation
	if err = database.First(&recoveredInvocation, "id = ?", deadlineIdentity.ID).Error; err != nil {
		t.Fatalf("load completed recovered Story invocation: %v", err)
	}
	if recoveredInvocation.Status != "succeeded" || recoveredInvocation.StageInstanceKey != deadlineIdentity.StageInstanceKey ||
		recoveredInvocation.InputHash != deadlineIdentity.InputHash || recoveredInvocation.ShardManifestHash != deadlineIdentity.ManifestHash ||
		recoveredInvocation.ClaimVersion != deadlineIdentity.ClaimVersion+1 || recoveredInvocation.Attempts != deadlineIdentity.Attempts+1 {
		t.Fatalf("Story recovery changed identity or did not complete on the next fenced claim: %#v", recoveredInvocation)
	}
	var unchangedSibling model.AgentInvocation
	if err = database.First(&unchangedSibling, "id = ?", successfulSibling.ID).Error; err != nil {
		t.Fatalf("reload successful sibling after recovery: %v", err)
	}
	var unchangedSiblingRevision model.StageCandidateRevision
	if err = database.First(&unchangedSiblingRevision, "id = ?", siblingRevision.ID).Error; err != nil {
		t.Fatalf("reload successful sibling Candidate Revision after recovery: %v", err)
	}
	if unchangedSibling.Status != "succeeded" || unchangedSibling.ResultHash == nil ||
		unchangedSiblingRevision.CandidateRevisionHash != siblingRevision.CandidateRevisionHash ||
		unchangedSiblingRevision.SourceInvocationID == nil || *unchangedSiblingRevision.SourceInvocationID != successfulSibling.ID {
		t.Fatalf("Story recovery changed a successful sibling fact: invocation=%#v revision=%#v", unchangedSibling, unchangedSiblingRevision)
	}
	var storyManifests []model.ShardManifest
	if err = database.Where("node_run_id = ?", storyNode.ID).Order("stage").Order("version").Find(&storyManifests).Error; err != nil || len(storyManifests) != 5 {
		t.Fatalf("Story analysis manifest pair=%d err=%v", len(storyManifests), err)
	}
	var analyzeManifest bibledomain.StoryAnalysisManifest
	var reconcileManifest bibledomain.StoryReconcileManifest
	for _, record := range storyManifests {
		switch record.Stage {
		case bibledomain.AnalyzeStoryStage:
			if record.Version < analyzeManifest.Version {
				continue
			}
			var values []bibledomain.StoryAnalysisShard
			if err = json.Unmarshal(record.Shards, &values); err != nil {
				t.Fatal(err)
			}
			analyzeManifest = bibledomain.StoryAnalysisManifest{
				ManifestID: record.ID.String(), Version: record.Version, ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
				WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(), Stage: record.Stage,
				RootInputHash: record.RootInputHash, Shards: values, CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
			}
		case bibledomain.ReconcileStoryStage:
			if record.Version < reconcileManifest.Version {
				continue
			}
			var values []bibledomain.StoryReconcileShard
			if err = json.Unmarshal(record.Shards, &values); err != nil {
				t.Fatal(err)
			}
			referenced := map[string]bool{}
			for _, value := range values {
				if value.Status != "active" {
					continue
				}
				for _, child := range value.Children {
					if child.Stage == bibledomain.ReconcileStoryStage {
						referenced[child.ShardKey] = true
					}
				}
			}
			rootKey := ""
			for _, value := range values {
				if value.Status == "active" && !referenced[value.Key] {
					rootKey = value.Key
				}
			}
			reconcileManifest = bibledomain.StoryReconcileManifest{
				ManifestID: record.ID.String(), Version: record.Version, ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
				WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(), Stage: record.Stage,
				RootInputHash: record.RootInputHash, FanIn: 2, RootShardKey: rootKey,
				Shards: values, CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
			}
		}
	}
	if analyzeManifest.Version != 2 || analyzeManifest.ParentManifestHash == nil ||
		reconcileManifest.Version != 3 || reconcileManifest.ParentManifestHash == nil {
		t.Fatalf("Story analysis budget failures did not publish versioned manifests: analyze=%#v reconcile=%#v", analyzeManifest, reconcileManifest)
	}
	if err = bibledomain.ValidateStoryAnalysisManifests(analyzeManifest, reconcileManifest); err != nil {
		t.Fatalf("persisted Story analysis map/reduce topology is invalid: %v", err)
	}
	var storyInvocations []model.AgentInvocation
	if err = database.Where("node_run_id = ?", storyNode.ID).Order("stage").Order("shard_key").Find(&storyInvocations).Error; err != nil {
		t.Fatal(err)
	}
	budgetFailures := 0
	for _, invocation := range storyInvocations {
		request, decodeErr := agentgorm.StageInvocation(invocation)
		if decodeErr != nil || invocation.Status == "succeeded" && invocation.Attempts < 1 {
			t.Fatalf("Story analysis invocation is not replayable: %#v err=%v", invocation, decodeErr)
		}
		if invocation.Status == "failed" {
			switch {
			case strings.Contains(string(invocation.Error), "execution_budget_exceeded"):
				budgetFailures++
			case !strings.Contains(string(invocation.Error), "manifest_superseded"):
				t.Fatalf("Story analysis failed unexpectedly: %#v", invocation)
			}
		} else if invocation.Status != "succeeded" {
			t.Fatalf("Story analysis left an invocation unresolved: %#v", invocation)
		}
		if invocation.Status == "succeeded" && invocation.Stage == bibledomain.ReconcileStoryStage &&
			(len(request.Payload.UpstreamCandidates) < 1 || len(request.Payload.UpstreamCandidates) > 2) {
			t.Fatalf("Story reconcile invocation exceeded fan-in: %#v", request.Payload.UpstreamCandidates)
		}
	}
	if budgetFailures != 2 {
		t.Fatalf("Story analysis budget failure count=%d want=2", budgetFailures)
	}
	storyOutput, _, storyOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(storyNode.Output))
	if err != nil || storyOutputHash != *storyNode.OutputHash || len(storyOutput.Bindings) != 1 ||
		storyOutput.Bindings[0].Port != "candidate" || storyOutput.Bindings[0].ValueType != "story_reconciliation_candidate" {
		t.Fatalf("Story analysis Node output=%#v hash=%s err=%v", storyOutput, storyOutputHash, err)
	}
	var reviewNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "story-review").First(&reviewNode).Error; err != nil {
		t.Fatalf("load Story review NodeRun: %v", err)
	}
	if reviewNode.Status != "SUCCEEDED" || reviewNode.OutputHash == nil {
		t.Fatalf("Story review NodeRun did not complete: %#v", reviewNode)
	}
	reviewOutput, _, reviewOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(reviewNode.Output))
	if err != nil || reviewOutputHash != *reviewNode.OutputHash || len(reviewOutput.Bindings) != 1 ||
		reviewOutput.Bindings[0].ReferenceVersion != "2" ||
		reviewOutput.Bindings[0].ReferenceID == storyOutput.Bindings[0].ReferenceID ||
		reviewOutput.Bindings[0].ContentHash == storyOutput.Bindings[0].ContentHash {
		t.Fatalf("Story review did not publish the repaired Candidate Revision: output=%#v err=%v", reviewOutput, err)
	}
	var reviewManifests []model.ShardManifest
	if err = database.Where("node_run_id = ? AND stage = ?", reviewNode.ID, bibledomain.ReviewStoryGraphStage).
		Order("version").Find(&reviewManifests).Error; err != nil || len(reviewManifests) != 2 ||
		reviewManifests[1].ParentManifestHash == nil ||
		*reviewManifests[1].ParentManifestHash != reviewManifests[0].ManifestHash {
		t.Fatalf("Story review did not persist a two-version bounded lineage: manifests=%#v err=%v", reviewManifests, err)
	}
	var reviewInvocationCount, repairInvocationCount int64
	if err = database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND stage = ? AND status = ?", reviewNode.ID, bibledomain.ReviewStoryGraphStage, "succeeded").
		Count(&reviewInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND stage = ? AND status = ?", reviewNode.ID, "repair_candidate", "succeeded").
		Count(&repairInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if reviewInvocationCount != 2 || repairInvocationCount != 1 {
		t.Fatalf("bounded Story review invocation counts: review=%d repair=%d", reviewInvocationCount, repairInvocationCount)
	}
	beforeReplay := len(storyInvocations)
	replayed, err := storyAnalysisService.Ensure(ctx, bibleapp.StoryAnalysisCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		WorkflowRunID: run.ID, NodeRunID: storyNode.ID.String(),
		EvidenceCandidateRevisionID:   output.Bindings[0].ReferenceID,
		EvidenceCandidateRevisionHash: output.Bindings[0].ContentHash,
	})
	if !errors.Is(err, bibleapp.ErrStoryAnalysisUpstreamStale) || replayed.Status != "" {
		t.Fatalf("Story analysis replay ignored the repaired Candidate Head: state=%#v err=%v", replayed, err)
	}
	var afterReplay int64
	if err = database.Model(&model.AgentInvocation{}).Where("node_run_id = ?", storyNode.ID).Count(&afterReplay).Error; err != nil || afterReplay != int64(beforeReplay) {
		t.Fatalf("Story analysis replay created invocations: before=%d after=%d err=%v", beforeReplay, afterReplay, err)
	}
	var analyzeInvocation model.AgentInvocation
	if err = database.Where("node_run_id = ? AND stage = ? AND shard_manifest_version = ? AND status = ?",
		storyNode.ID, bibledomain.AnalyzeStoryStage, analyzeManifest.Version, "succeeded").
		Order("shard_key").First(&analyzeInvocation).Error; err != nil {
		t.Fatal(err)
	}
	analyzeRequest, err := agentgorm.StageInvocation(analyzeInvocation)
	if err != nil || len(analyzeRequest.Payload.UpstreamCandidates) != 1 {
		t.Fatalf("load Story analysis upstream: request=%#v err=%v", analyzeRequest, err)
	}
	upstreamID := uuid.MustParse(analyzeRequest.Payload.UpstreamCandidates[0].CandidateRevisionID)
	var upstreamRevision model.StageCandidateRevision
	if err = database.First(&upstreamRevision, "id = ?", upstreamID).Error; err != nil {
		t.Fatal(err)
	}
	repairedHash := bibledomain.SourceTextHash(upstreamRevision.ID.String() + ":fixture-repair")
	parentHash := upstreamRevision.CandidateRevisionHash
	repairOrigin := append(
		upstreamRevision.Candidate[:0:0],
		[]byte(`{"repair_invocation_id":"fixture","repair_result_hash":"fixture"}`)...,
	)
	repaired := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: upstreamRevision.WorkspaceID,
		StageInstanceKey: upstreamRevision.StageInstanceKey, RevisionNo: upstreamRevision.RevisionNo + 1,
		ParentCandidateRevisionID: &upstreamRevision.ID, ParentCandidateRevisionHash: &parentHash,
		OriginKind: "repair", RepairOrigin: repairOrigin,
		Candidate: upstreamRevision.Candidate, CandidateContentHash: upstreamRevision.CandidateContentHash,
		CandidateRevisionHash: repairedHash, CreatedAt: time.Now().UTC(),
	}
	if err = database.Omit("Workspace", "ParentCandidateRevision", "SourceInvocation").Create(&repaired).Error; err != nil {
		t.Fatalf("create fixture repaired Evidence revision: %v", err)
	}
	if err = database.Model(&model.StageCandidateHead{}).Where("stage_instance_key = ?", upstreamRevision.StageInstanceKey).
		Updates(map[string]any{
			"current_revision_id": repaired.ID, "current_candidate_revision_hash": repairedHash,
			"revision": repaired.RevisionNo, "updated_at": time.Now().UTC(),
		}).Error; err != nil {
		t.Fatalf("switch fixture Evidence Head: %v", err)
	}
	claimVersion := analyzeInvocation.ClaimVersion + 1
	leaseExpiresAt := time.Now().UTC().Add(time.Minute)
	if err = database.Model(&analyzeInvocation).Updates(map[string]any{
		"status": "running", "claim_version": claimVersion, "lease_expires_at": leaseExpiresAt,
		"attempts": analyzeInvocation.Attempts + 1, "started_at": time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("prepare fixture stale invocation claim: %v", err)
	}
	if err = bibleStore.ValidateStoryAnalysisInvocation(ctx, analyzeInvocation.ID.String(), claimVersion, time.Now().UTC()); !errors.Is(err, bibleapp.ErrStoryAnalysisUpstreamStale) {
		t.Fatalf("Story analysis did not reject a stale upstream Head: %v", err)
	}
}

type unknownOnceWorkflowSignaler struct {
	mutex    sync.Mutex
	delegate workflowapp.WorkflowSignaler
	attempts int
}

func (signaler *unknownOnceWorkflowSignaler) Signal(
	ctx context.Context,
	request workflow.SignalRequest,
) (workflow.SignalObservation, error) {
	signaler.mutex.Lock()
	signaler.attempts++
	attempt := signaler.attempts
	signaler.mutex.Unlock()
	if attempt == 1 {
		return workflow.SignalObservation{Outcome: workflow.SignalOutcomeUnknown}, nil
	}
	return signaler.delegate.Signal(ctx, request)
}

type recoveringSourceEvidenceAgent struct {
	mutex                   sync.Mutex
	failed                  map[string]bool
	budget                  bool
	storyMapBudget          bool
	storyReduceBudget       bool
	storyDeadline           bool
	storyDeadlineID         string
	segmentUnknown          bool
	segmentInvocationID     string
	originalCalls           int
	lateRelease             <-chan struct{}
	deadlineRecoveryRelease <-chan struct{}
}

func (agent *recoveringSourceEvidenceAgent) Invoke(
	_ context.Context,
	invocation agentcontract.StageInvocation,
	_ int,
	_ int64,
) (agentcontract.StageResult, error) {
	if invocation.Payload.Stage == bibledomain.EpisodeSegmentationStage {
		agent.mutex.Lock()
		if !agent.segmentUnknown {
			agent.segmentUnknown = true
			agent.segmentInvocationID = invocation.InvocationID
			agent.mutex.Unlock()
			return agentcontract.StageResult{}, errors.New("injected Episode segmentation transport outcome unknown")
		}
		if agent.segmentInvocationID != invocation.InvocationID {
			agent.mutex.Unlock()
			return agentcontract.StageResult{}, errors.New("Episode segmentation retry changed invocation identity")
		}
		agent.mutex.Unlock()
		return episodeSegmentationFixtureResult(invocation)
	}
	if invocation.Payload.Stage == bibledomain.ReviewStoryGraphStage || invocation.Payload.Stage == "repair_candidate" {
		return storyReviewFixtureResult(invocation)
	}
	if invocation.Payload.Stage == bibledomain.AnalyzeStoryStage || invocation.Payload.Stage == bibledomain.ReconcileStoryStage {
		agent.mutex.Lock()
		itemCount, countErr := storyFixtureInputItemCount(invocation)
		budget := false
		deadline := false
		waitForRecovery := false
		if countErr == nil && itemCount >= 2 {
			if invocation.Payload.Stage == bibledomain.AnalyzeStoryStage && !agent.storyMapBudget {
				agent.storyMapBudget, budget = true, true
			}
			if invocation.Payload.Stage == bibledomain.ReconcileStoryStage && !agent.storyReduceBudget {
				agent.storyReduceBudget, budget = true, true
			}
		}
		if countErr == nil && !budget && agent.storyMapBudget &&
			invocation.Payload.Stage == bibledomain.AnalyzeStoryStage &&
			invocation.Payload.ShardManifestRef.Version >= 2 {
			if !agent.storyDeadline {
				agent.storyDeadline, deadline = true, true
				agent.storyDeadlineID = invocation.InvocationID
			} else if agent.storyDeadlineID == invocation.InvocationID {
				waitForRecovery = true
			}
		}
		agent.mutex.Unlock()
		if countErr != nil {
			return agentcontract.StageResult{}, countErr
		}
		if budget {
			return storyFixtureBudgetExceeded(invocation), nil
		}
		if deadline {
			return storyFixtureDeadlineExceeded(invocation), nil
		}
		if waitForRecovery {
			<-agent.deadlineRecoveryRelease
		}
		return storyAnalysisFixtureResult(invocation)
	}
	agent.mutex.Lock()
	var release <-chan struct{}
	if invocation.Payload.ShardManifestRef.Version == 1 {
		agent.originalCalls++
		if agent.originalCalls == 1 {
			release = agent.lateRelease
		} else if !agent.budget {
			agent.budget = true
			agent.mutex.Unlock()
			return agentcontract.StageResult{
				InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
				WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
				Stage:             bibledomain.SourceEvidenceStage, ShardKey: invocation.Payload.ShardKey,
				Status: "failed", CandidateType: "source_evidence_candidate",
				Candidate: json.RawMessage("null"), InputHash: invocation.InputHash,
				Issues: []agentcontract.StageIssue{},
				Executor: agentcontract.Executor{
					Name: "test-agent", Version: "source-evidence-v1", Model: "deterministic-fixture",
				},
				Error: &agentcontract.ResultError{
					Code: "execution_budget_exceeded", Summary: "fixture requires a smaller immutable shard", Retryable: false,
				},
			}, nil
		}
	}
	if release == nil && !agent.failed[invocation.InvocationID] {
		agent.failed[invocation.InvocationID] = true
		agent.mutex.Unlock()
		return agentcontract.StageResult{}, errors.New("injected transport outcome unknown")
	}
	agent.mutex.Unlock()
	if release != nil {
		<-release
	}
	var input struct {
		LogicalStart   int    `json:"logical_start"`
		LogicalEnd     int    `json:"logical_end"`
		ContextStart   int    `json:"context_start"`
		NormalizedText string `json:"normalized_text"`
	}
	if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
		return agentcontract.StageResult{}, err
	}
	contextRunes := []rune(input.NormalizedText)
	localStart := input.LogicalStart - input.ContextStart
	localEnd := localStart + input.LogicalEnd - input.LogicalStart
	anchor := string(contextRunes[localStart:localEnd])
	candidate, err := json.Marshal(bibledomain.SourceEvidenceCandidate{
		Observations: []bibledomain.SourceObservation{{
			ObservationKey: "observation:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-"),
			Kind:           "event", ProposedKey: "event:" + invocation.Payload.ShardKey,
			Label: "分片原文", Facts: []string{"只记录冻结分片"}, Ambiguities: []string{},
			Evidence: []bibledomain.Evidence{{
				SourceStart: localStart, SourceEnd: localEnd,
				TextHash: bibledomain.SourceTextHash(anchor), ExactAnchor: anchor,
			}},
		}, {
			ObservationKey: "observation-detail:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-"),
			Kind:           "event", ProposedKey: "detail:" + invocation.Payload.ShardKey,
			Label: "分片细节", Facts: []string{"保留第二个候选条目"}, Ambiguities: []string{},
			Evidence: []bibledomain.Evidence{{
				SourceStart: localStart, SourceEnd: localEnd,
				TextHash: bibledomain.SourceTextHash(anchor), ExactAnchor: anchor,
			}},
		}},
		ReviewIssues: []bibledomain.SourceEvidenceIssue{},
	})
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	resultHash, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             bibledomain.SourceEvidenceStage, ShardKey: invocation.Payload.ShardKey,
		Status: "succeeded", CandidateType: "source_evidence_candidate",
		Candidate: candidate, InputHash: invocation.InputHash, ResultHash: &resultHash,
		Issues:   []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "source-evidence-v1", Model: "deterministic-fixture"},
	}, nil
}

func episodeSegmentationFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var input agentcontract.EpisodeSegmentationStageInput
	if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
		return agentcontract.StageResult{}, err
	}
	if len(input.MarkerHints) == 0 {
		return agentcontract.StageResult{}, errors.New("fixture Episode segmentation received no explicit marker")
	}
	slices.SortFunc(input.MarkerHints, func(left, right agentcontract.EpisodeSegmentationMarkerHint) int {
		return left.Evidence.SourceStart - right.Evidence.SourceStart
	})
	boundaries := make([]bibledomain.EpisodeBoundary, 0, len(input.MarkerHints))
	for index, marker := range input.MarkerHints {
		end := input.SourceCodePoints
		if index+1 < len(input.MarkerHints) {
			end = input.MarkerHints[index+1].Evidence.SourceStart
		}
		boundaries = append(boundaries, bibledomain.EpisodeBoundary{
			BoundaryKey: "episode:" + strconv.Itoa(index+1), EpisodeOrder: index + 1,
			Title: marker.Label, AbsoluteStart: marker.Evidence.SourceStart, AbsoluteEnd: end,
			Evidence: []bibledomain.Evidence{{
				SourceStart: marker.Evidence.SourceStart, SourceEnd: marker.Evidence.SourceEnd,
				TextHash: marker.Evidence.TextHash, ExactAnchor: marker.Evidence.ExactAnchor,
				EpisodeNumber: marker.Evidence.EpisodeNumber,
			}},
		})
	}
	candidate, err := json.Marshal(bibledomain.EpisodeSegmentationCandidate{
		Boundaries: boundaries, ReviewIssues: []bibledomain.ReviewIssue{},
	})
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	resultHash, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "succeeded", CandidateType: "episode_segmentation_candidate",
		Candidate: candidate, InputHash: invocation.InputHash, ResultHash: &resultHash,
		Issues:   []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "episode-segmentation-v1", Model: "deterministic-fixture"},
	}, nil
}

func storyReviewFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var candidate json.RawMessage
	switch invocation.Payload.Stage {
	case bibledomain.ReviewStoryGraphStage:
		var input agentcontract.StoryGraphReviewStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return agentcontract.StageResult{}, err
		}
		var target bibledomain.StoryReconciliationCandidate
		if err := json.Unmarshal(input.TargetCandidate, &target); err != nil || len(target.CanonicalEntities) == 0 {
			return agentcontract.StageResult{}, errors.New("fixture Story review received no canonical entity")
		}
		issues := []agentcontract.StoryGraphReviewIssue{}
		entity := target.CanonicalEntities[0]
		if entity.CanonicalName != "已修复角色" {
			if len(entity.Evidence) == 0 {
				return agentcontract.StageResult{}, errors.New("fixture Story review received no entity Evidence")
			}
			evidence := entity.Evidence[0]
			subject := entity.EntityKey
			issues = append(issues, agentcontract.StoryGraphReviewIssue{
				IssueKey: "review:canonical-name", Code: "canonical_name_ambiguous", Severity: "blocking",
				Scope: "entity", SubjectKey: &subject, Summary: "角色规范名需要统一",
				Evidence: []agentcontract.StoryGraphEvidence{{
					SourceStart: evidence.SourceStart, SourceEnd: evidence.SourceEnd,
					TextHash: evidence.TextHash, ExactAnchor: evidence.ExactAnchor,
				}},
			})
		}
		encoded, err := json.Marshal(agentcontract.StoryGraphReviewCandidate{
			ReviewedStage:               input.ReviewedStage,
			TargetCandidateRevisionID:   input.TargetCandidateRevisionID,
			TargetCandidateRevisionHash: input.TargetCandidateRevisionHash,
			ReviewIssues:                issues,
		})
		if err != nil {
			return agentcontract.StageResult{}, err
		}
		candidate = encoded
	case "repair_candidate":
		var input agentcontract.StoryGraphRepairStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil || len(input.AllowedTargets) != 1 {
			return agentcontract.StageResult{}, errors.New("fixture Story repair received an invalid boundary")
		}
		replacement := "已修复角色"
		encoded, err := json.Marshal(agentcontract.CandidateRepairPatch{
			TargetCandidateRevisionID:   input.TargetCandidateRevisionID,
			TargetCandidateRevisionHash: input.TargetCandidateRevisionHash,
			Operations: []agentcontract.StoryGraphRepairOperation{{
				TargetCandidateKey: input.AllowedTargets[0].CandidateKey,
				BaseFragmentHash:   input.AllowedTargets[0].BaseFragmentHash,
				FieldName:          "canonical_name",
				Replacement:        agentcontract.StoryGraphRepairReplacement{Text: &replacement},
			}},
			ReviewIssues: []agentcontract.StoryGraphReviewIssue{},
		})
		if err != nil {
			return agentcontract.StageResult{}, err
		}
		candidate = encoded
	default:
		return agentcontract.StageResult{}, errors.New("fixture Story review stage is unsupported")
	}
	resultHash, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	candidateType, ok := agentcontract.CandidateTypeForStage(invocation.Payload.Stage)
	if !ok {
		return agentcontract.StageResult{}, errors.New("fixture Story review stage has no candidate type")
	}
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "succeeded", CandidateType: candidateType, Candidate: candidate,
		InputHash: invocation.InputHash, ResultHash: &resultHash, Issues: []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "story-review-v1", Model: "deterministic-fixture"},
	}, nil
}

func storyFixtureDeadlineExceeded(invocation agentcontract.StageInvocation) agentcontract.StageResult {
	candidateType, _ := agentcontract.CandidateTypeForStage(invocation.Payload.Stage)
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "failed", CandidateType: candidateType, Candidate: json.RawMessage("null"),
		InputHash: invocation.InputHash, Issues: []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "story-deadline-v1", Model: "deterministic-fixture"},
		Error: &agentcontract.ResultError{
			Code: "execution_deadline_exceeded", Summary: "fixture exhausted the frozen shard deadline", Retryable: false,
		},
	}
}

func storyFixtureBudgetExceeded(invocation agentcontract.StageInvocation) agentcontract.StageResult {
	candidateType, _ := agentcontract.CandidateTypeForStage(invocation.Payload.Stage)
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "failed", CandidateType: candidateType, Candidate: json.RawMessage("null"),
		InputHash: invocation.InputHash, Issues: []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "story-budget-v1", Model: "deterministic-fixture"},
		Error: &agentcontract.ResultError{
			Code: "execution_budget_exceeded", Summary: "fixture requires an exact candidate partition", Retryable: false,
		},
	}
}

func storyFixtureInputItemCount(invocation agentcontract.StageInvocation) (int, error) {
	switch invocation.Payload.Stage {
	case bibledomain.AnalyzeStoryStage:
		var input agentcontract.StoryAnalysisStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return 0, err
		}
		var candidate bibledomain.SourceEvidenceCandidate
		if err := json.Unmarshal(input.EvidenceCandidate, &candidate); err != nil {
			return 0, err
		}
		return bibledomain.SourceEvidenceCandidateItemCount(candidate), nil
	case bibledomain.ReconcileStoryStage:
		var input agentcontract.StoryReconciliationStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return 0, err
		}
		total := 0
		for _, child := range input.Candidates {
			switch input.CandidateType {
			case "story_analysis_candidate":
				var candidate bibledomain.StoryAnalysisCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return 0, err
				}
				total += bibledomain.StoryAnalysisCandidateItemCount(candidate)
			case "story_reconciliation_candidate":
				var candidate bibledomain.StoryReconciliationCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return 0, err
				}
				total += bibledomain.StoryReconciliationCandidateItemCount(candidate)
			}
		}
		return total, nil
	default:
		return 0, errors.New("fixture Story stage is unsupported")
	}
}

func storyAnalysisFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var candidate json.RawMessage
	switch invocation.Payload.Stage {
	case bibledomain.AnalyzeStoryStage:
		var input agentcontract.StoryAnalysisStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return agentcontract.StageResult{}, err
		}
		var evidence bibledomain.SourceEvidenceCandidate
		if err := json.Unmarshal(input.EvidenceCandidate, &evidence); err != nil {
			return agentcontract.StageResult{}, err
		}
		if len(evidence.Observations) == 0 || len(evidence.Observations[0].Evidence) == 0 {
			return agentcontract.StageResult{}, errors.New("fixture Story analysis received no Evidence")
		}
		value, err := json.Marshal(bibledomain.StoryAnalysisCandidate{
			Entities: []bibledomain.StoryEntityCandidate{{
				EntityKey: "character:" + invocation.Payload.ShardKey, Kind: "character",
				CanonicalName: "分片角色", NormalizedName: "分片角色", Aliases: []string{},
				StableSpec: storyFixtureAssetSpec(), EpisodeNumbers: []int{},
				Evidence: evidence.Observations[0].Evidence,
				States:   []bibledomain.StoryEntityStateCandidate{}, Ambiguities: []string{},
			}},
			WorldEntries: []bibledomain.StoryWorldEntryCandidate{},
			Claims:       []bibledomain.StoryClaimCandidate{}, Arcs: []bibledomain.StoryArcCandidate{},
			ReviewIssues: []bibledomain.ReviewIssue{},
		})
		if err != nil {
			return agentcontract.StageResult{}, err
		}
		candidate = value
	case bibledomain.ReconcileStoryStage:
		var input agentcontract.StoryReconciliationStageInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return agentcontract.StageResult{}, err
		}
		result := bibledomain.StoryReconciliationCandidate{
			CanonicalEntities:     []bibledomain.StoryEntityCandidate{},
			CanonicalWorldEntries: []bibledomain.StoryWorldEntryCandidate{},
			MergedClaims:          []bibledomain.StoryClaimCandidate{}, MergedArcs: []bibledomain.StoryArcCandidate{},
			Conflicts: []bibledomain.ReviewIssue{}, ReviewIssues: []bibledomain.ReviewIssue{},
		}
		for _, child := range input.Candidates {
			switch input.CandidateType {
			case "story_analysis_candidate":
				var value bibledomain.StoryAnalysisCandidate
				if err := json.Unmarshal(child.Candidate, &value); err != nil {
					return agentcontract.StageResult{}, err
				}
				result.CanonicalEntities = append(result.CanonicalEntities, value.Entities...)
				result.CanonicalWorldEntries = append(result.CanonicalWorldEntries, value.WorldEntries...)
				result.MergedClaims = append(result.MergedClaims, value.Claims...)
				result.MergedArcs = append(result.MergedArcs, value.Arcs...)
				result.ReviewIssues = append(result.ReviewIssues, value.ReviewIssues...)
			case "story_reconciliation_candidate":
				var value bibledomain.StoryReconciliationCandidate
				if err := json.Unmarshal(child.Candidate, &value); err != nil {
					return agentcontract.StageResult{}, err
				}
				result.CanonicalEntities = append(result.CanonicalEntities, value.CanonicalEntities...)
				result.CanonicalWorldEntries = append(result.CanonicalWorldEntries, value.CanonicalWorldEntries...)
				result.MergedClaims = append(result.MergedClaims, value.MergedClaims...)
				result.MergedArcs = append(result.MergedArcs, value.MergedArcs...)
				result.Conflicts = append(result.Conflicts, value.Conflicts...)
				result.ReviewIssues = append(result.ReviewIssues, value.ReviewIssues...)
			default:
				return agentcontract.StageResult{}, errors.New("fixture Story reconcile candidate type is invalid")
			}
		}
		value, err := json.Marshal(result)
		if err != nil {
			return agentcontract.StageResult{}, err
		}
		candidate = value
	default:
		return agentcontract.StageResult{}, errors.New("fixture Story stage is unsupported")
	}
	resultHash, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	candidateType, ok := agentcontract.CandidateTypeForStage(invocation.Payload.Stage)
	if !ok {
		return agentcontract.StageResult{}, errors.New("fixture Story stage has no candidate type")
	}
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "succeeded", CandidateType: candidateType, Candidate: candidate,
		InputHash: invocation.InputHash, ResultHash: &resultHash,
		Issues:   []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "story-analysis-v1", Model: "deterministic-fixture"},
	}, nil
}

func storyFixtureAssetSpec() bibledomain.AssetSpecCandidate {
	return bibledomain.AssetSpecCandidate{
		Temperament: []string{}, Goals: []string{}, Relationships: []string{},
		VisualElements: []string{}, NegativeConstraints: []string{},
		PerformanceTraits: []string{}, AllowedUsage: []string{},
	}
}
