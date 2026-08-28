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
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
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
	if err = database.Model(&model.Project{}).Where("id = ?", fixture.projectID).Updates(map[string]any{
		"visual_style": "cinematic noir", "aspect_ratio": "9:16",
	}).Error; err != nil {
		t.Fatalf("freeze Storyboard visual style: %v", err)
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
				{ID: "episode-analysis", DefinitionKey: "agent.episode_analysis", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "structure-review", DefinitionKey: "human.episode_structure_review", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{}`)},
				{ID: "storygraph", DefinitionKey: "production.storygraph_compile", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "storyboard", DefinitionKey: "agent.storyboard_draft", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{}`)},
				{ID: "intent-review", DefinitionKey: "human.storyboard_review", DefinitionVersion: "2.0.0", Config: json.RawMessage(`{}`)},
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
				{ID: "episode-review-to-analysis", FromNodeID: "episode-review", FromPort: "episodes", ToNodeID: "episode-analysis", ToPort: "episodes"},
				{ID: "materialization-to-analysis", FromNodeID: "bible-materialization", FromPort: "materialization", ToNodeID: "episode-analysis", ToPort: "materialization"},
				{ID: "analysis-to-structure-review", FromNodeID: "episode-analysis", FromPort: "candidate", ToNodeID: "structure-review", ToPort: "candidate"},
				{ID: "structure-to-storygraph", FromNodeID: "structure-review", FromPort: "structures", ToNodeID: "storygraph", ToPort: "structures"},
				{ID: "storygraph-to-storyboard", FromNodeID: "storygraph", FromPort: "storygraph", ToNodeID: "storyboard", ToPort: "storygraph"},
				{ID: "storyboard-to-intent-review", FromNodeID: "storyboard", FromPort: "candidate", ToNodeID: "intent-review", ToPort: "candidate"},
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
	planningStore := planninggorm.New(database)
	planningService := planningapp.NewService(planningStore, planningapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	episodePlanningService := planningapp.NewEpisodePlanningService(
		planningStore, planningapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	episodeAnalysisService := planningapp.NewEpisodeAnalysisService(
		planningStore, planningapp.EpisodeAnalysisConfig{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
			MaxShardCodePoints: 12, OverlapCodePoints: 2, AdjacentCodePoints: 4, FanIn: 2,
		},
	)
	storyGraphService := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	storyboardStore := storyboardgorm.New(database)
	storyboardService := storyboardapp.NewService(
		storyboardStore, storyboardapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		evidenceService,
		storyAnalysisService,
		storyReviewService,
		bibleService,
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningService, episodePlanningService, storyGraphService,
		storyboardService,
		reviewService,
		nil, nil, nil, nil, episodeSegmentationService, episodeAnalysisService,
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
	episodeAnalysisWorker := planningapp.NewEpisodeAnalysisWorker(
		planninggorm.New(database), agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	bibleWorker := bibleapp.NewWorker(
		bibleStore, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	storyboardWorker := storyboardapp.NewWorker(
		storyboardStore, agent, func() time.Time { return time.Now().UTC() },
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
	go episodeAnalysisWorker.Run(agentContext)
	go episodeAnalysisWorker.Run(agentContext)
	go bibleWorker.Run(agentContext)
	go storyboardWorker.Run(agentContext)
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
		Owner: workflowproduction.New(bibleService, planningService, episodePlanningService, storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{
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
	completionDeadline := time.Now().Add(30 * time.Second)
	var structureGateNode model.NodeRunProjection
	var structureTask model.HumanTask
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("reload Episode Planning Workflow Run: %v", err)
		}
		if persistedRun.Status == "WAITING_HUMAN" {
			if queryErr := database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "structure-review").First(&structureGateNode).Error; queryErr == nil &&
				structureGateNode.Status == "WAITING_HUMAN" {
				if queryErr = database.Where("node_run_id = ?", structureGateNode.ID).First(&structureTask).Error; queryErr == nil {
					break
				}
			}
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(completionDeadline) {
			t.Fatalf("Source Evidence Workflow did not reach the Episode Planning Human Gate: %#v", persistedRun)
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
	var episodeAnalysisNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "episode-analysis").
		First(&episodeAnalysisNode).Error; err != nil {
		t.Fatalf("load Episode analysis NodeRun: %v", err)
	}
	analysisOutput, _, analysisOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(episodeAnalysisNode.Output))
	if err != nil || episodeAnalysisNode.Status != "SUCCEEDED" || episodeAnalysisNode.OutputHash == nil ||
		*episodeAnalysisNode.OutputHash != analysisOutputHash || len(analysisOutput.Bindings) != 1 ||
		analysisOutput.Bindings[0].Port != "candidate" ||
		analysisOutput.Bindings[0].ValueType != "episode_planning_candidate_set" {
		t.Fatalf("Episode analysis output=%#v node=%#v err=%v", analysisOutput, episodeAnalysisNode, err)
	}
	var episodeAggregateRevision model.StageCandidateRevision
	if err = database.First(&episodeAggregateRevision, "id = ?", analysisOutput.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatalf("load Episode planning aggregate Candidate: %v", err)
	}
	var aggregate planningdomain.EpisodePlanningCandidateSet
	if err = json.Unmarshal(episodeAggregateRevision.Candidate, &aggregate); err != nil {
		t.Fatalf("decode Episode planning aggregate Candidate: %v", err)
	}
	var analysisInvocationCount, reconciliationInvocationCount, formalStructureCount int64
	if err = database.Model(&model.AgentInvocation{}).Where(
		"node_run_id = ? AND request_type = ? AND status = ?", episodeAnalysisNode.ID, "episode_analysis_shard", "succeeded",
	).Count(&analysisInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AgentInvocation{}).Where(
		"node_run_id = ? AND request_type = ? AND status = ?", episodeAnalysisNode.ID, "episode_reconcile_shard", "succeeded",
	).Count(&reconciliationInvocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EpisodeStructure{}).Where("project_id = ?", fixture.projectID).
		Count(&formalStructureCount).Error; err != nil {
		t.Fatal(err)
	}
	agent.mutex.Lock()
	recoveredEpisodeInvocationID := agent.episodeAnalysisID
	agent.mutex.Unlock()
	var recoveredEpisodeInvocation model.AgentInvocation
	if err = database.First(&recoveredEpisodeInvocation, "id = ?", recoveredEpisodeInvocationID).Error; err != nil {
		t.Fatalf("load recovered Episode analysis invocation: %v", err)
	}
	if aggregate.SchemaVersion != "episode-planning-candidate-set-v1" ||
		len(aggregate.Episodes) != len(publishedEpisodes) || analysisInvocationCount < int64(len(publishedEpisodes)) ||
		reconciliationInvocationCount < int64(len(publishedEpisodes)) || formalStructureCount != 0 ||
		episodeAggregateRevision.CandidateRevisionHash != analysisOutput.Bindings[0].ContentHash ||
		recoveredEpisodeInvocation.Attempts < 2 || recoveredEpisodeInvocation.Status != "succeeded" {
		t.Fatalf(
			"Episode analysis facts: aggregate=%#v maps=%d reduces=%d structures=%d recovered=%#v",
			aggregate, analysisInvocationCount, reconciliationInvocationCount, formalStructureCount, recoveredEpisodeInvocation,
		)
	}
	var structureTaskCandidateIDs []string
	if err = json.Unmarshal(structureTask.CandidateIDs, &structureTaskCandidateIDs); err != nil || len(structureTaskCandidateIDs) != 1 ||
		structureTask.SubjectType != "planning_candidate" || structureTask.SubjectID.String() != analysisOutput.Bindings[0].ReferenceID ||
		structureTask.SubjectRevision != 1 || structureTask.SubjectHash != analysisOutput.Bindings[0].ContentHash ||
		structureTaskCandidateIDs[0] != analysisOutput.Bindings[0].ReferenceID {
		t.Fatalf("Episode Planning HumanTask did not freeze the aggregate Candidate: task=%#v candidates=%v err=%v", structureTask, structureTaskCandidateIDs, err)
	}
	structureClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: structureTask.ID.String(), ExpectedRevision: structureTask.Revision,
		IdempotencyKey: "episode-planning-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Episode Planning HumanTask: %v", err)
	}
	structureDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: structureClaim.Task.ID, ClaimToken: structureClaim.ClaimToken,
		ExpectedTaskRevision:    structureClaim.Task.Revision,
		ExpectedSubjectRevision: structureClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     structureClaim.Task.SubjectHash,
		Decision:                "approved", IdempotencyKey: "episode-planning-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Episode Planning HumanTask: %v", err)
	}
	planningUnknownSignaler := &unknownOnceWorkflowSignaler{delegate: temporalRuntime}
	planningSignalService := workflowapp.NewSignalService(workflowStore, planningUnknownSignaler, workflowapp.SignalConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Owner: workflowproduction.New(bibleService, planningService, episodePlanningService, storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		})),
	})
	structureSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: run.ID, NodeRunID: structureGateNode.ID.String(),
		HumanTaskID: structureDecision.Task.ID, ReviewDecisionID: structureDecision.Decision.ID,
		SubjectRevision: structureDecision.Decision.SubjectRevision, Decision: structureDecision.Decision.Decision,
		IdempotencyKey: "episode-planning-review-signal",
	}
	unknownPlanningSignal, err := planningSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, structureSignalCommand)
	if err != nil || unknownPlanningSignal.Status != "unknown" || unknownPlanningSignal.AttemptNo != 1 {
		t.Fatalf("persist unknown Episode Planning Signal: intent=%#v err=%v", unknownPlanningSignal, err)
	}
	var planningReceipt model.CommandReceipt
	if err = database.Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "episode_planning.apply", episodeAggregateRevision.ID,
	).First(&planningReceipt).Error; err != nil {
		t.Fatalf("load Planning owner Receipt after unknown Signal: %v", err)
	}
	var planningSet planningapp.PlanningOwnerSetReference
	if err = json.Unmarshal(planningReceipt.Result, &planningSet); err != nil || planningSet.ID != planningReceipt.ID.String() ||
		planningSet.CandidateRevisionID != episodeAggregateRevision.ID.String() ||
		planningSet.CandidateRevisionHash != episodeAggregateRevision.CandidateRevisionHash ||
		planningSet.ReviewDecisionID != structureDecision.Decision.ID || len(planningSet.Structures) != len(aggregate.Episodes) {
		t.Fatalf("Planning owner Receipt result is incomplete: set=%#v err=%v", planningSet, err)
	}
	var appliedStructureCount, planningReceiptCount int64
	if err = database.Model(&model.EpisodeStructure{}).Where("project_id = ? AND status = ?", fixture.projectID, "confirmed").
		Count(&appliedStructureCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "episode_planning.apply", episodeAggregateRevision.ID,
	).Count(&planningReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if appliedStructureCount != int64(len(aggregate.Episodes)) || planningReceiptCount != 1 {
		t.Fatalf("Planning owner batch after unknown Signal: structures=%d receipts=%d", appliedStructureCount, planningReceiptCount)
	}
	completedPlanningSignal, err := planningSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, structureSignalCommand)
	if err != nil || completedPlanningSignal.ID != unknownPlanningSignal.ID ||
		completedPlanningSignal.Status != "completed" || completedPlanningSignal.AttemptNo != 2 {
		t.Fatalf("recover unknown Episode Planning Signal: intent=%#v err=%v", completedPlanningSignal, err)
	}
	replayedPlanningSignal, err := planningSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, structureSignalCommand)
	if err != nil || replayedPlanningSignal.ID != completedPlanningSignal.ID || replayedPlanningSignal.Status != "completed" {
		t.Fatalf("replay completed Episode Planning Signal: intent=%#v err=%v", replayedPlanningSignal, err)
	}
	completionDeadline = time.Now().Add(30 * time.Second)
	var storyboardGateNode model.NodeRunProjection
	var storyboardTask model.HumanTask
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("reload Storyboard Intent Workflow Run: %v", err)
		}
		if persistedRun.Status == "WAITING_HUMAN" {
			if queryErr := database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "intent-review").First(&storyboardGateNode).Error; queryErr == nil &&
				storyboardGateNode.Status == "WAITING_HUMAN" {
				if queryErr = database.Where("node_run_id = ?", storyboardGateNode.ID).First(&storyboardTask).Error; queryErr == nil {
					break
				}
			}
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(completionDeadline) {
			t.Fatalf("Source Evidence Workflow did not reach the Storyboard Intent Human Gate: %#v", persistedRun)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = database.First(&structureGateNode, "id = ?", structureGateNode.ID).Error; err != nil {
		t.Fatal(err)
	}
	structureOutput, _, structureOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(structureGateNode.Output))
	if err != nil || structureGateNode.Status != "SUCCEEDED" || structureGateNode.OutputHash == nil ||
		*structureGateNode.OutputHash != structureOutputHash || len(structureOutput.Bindings) != 1 ||
		structureOutput.Bindings[0].Port != "structures" || structureOutput.Bindings[0].ValueType != "planning_owner_set" ||
		structureOutput.Bindings[0].ReferenceID != planningReceipt.ID.String() ||
		structureOutput.Bindings[0].ReferenceVersion != "1" || structureOutput.Bindings[0].ContentHash != planningSet.ContentHash {
		t.Fatalf("Episode Planning Gate output=%#v node=%#v err=%v", structureOutput, structureGateNode, err)
	}
	for _, reference := range planningSet.Structures {
		var structure model.EpisodeStructure
		if err = database.First(&structure, "id = ?", reference.StructureID).Error; err != nil {
			t.Fatalf("load formal Planning Structure: %v", err)
		}
		var scenes []planningdomain.Scene
		if err = json.Unmarshal(structure.Scenes, &scenes); err != nil || len(scenes) == 0 || len(reference.Fragments) == 0 ||
			structure.Status != "confirmed" || structure.ResultHash != reference.ResultHash ||
			structure.ConfirmedBy == nil || *structure.ConfirmedBy != fixture.userID {
			t.Fatalf("formal Planning Structure is incomplete: record=%#v scenes=%#v reference=%#v err=%v", structure, scenes, reference, err)
		}
		for _, scene := range scenes {
			if scene.TemporaryKey == "" || len(scene.Evidence) == 0 || scene.Evidence[0].EpisodeNumber == nil {
				t.Fatalf("formal Scene lost temporary reverse trace or Evidence: %#v", scene)
			}
		}
	}
	var storyGraphNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "storygraph").First(&storyGraphNode).Error; err != nil {
		t.Fatalf("load StoryGraph compiler NodeRun: %v", err)
	}
	storyGraphOutput, _, storyGraphOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(storyGraphNode.Output))
	if err != nil || storyGraphNode.Status != "SUCCEEDED" || storyGraphNode.OutputHash == nil ||
		*storyGraphNode.OutputHash != storyGraphOutputHash || len(storyGraphOutput.Bindings) != 1 ||
		storyGraphOutput.Bindings[0].Port != "storygraph" || storyGraphOutput.Bindings[0].ValueType != "storygraph_version" ||
		storyGraphOutput.Bindings[0].ReferenceVersion != "1" || len(storyGraphOutput.Bindings[0].ContentHash) != 64 {
		t.Fatalf("StoryGraph compiler output=%#v node=%#v err=%v", storyGraphOutput, storyGraphNode, err)
	}
	var graphVersion model.StoryGraphVersion
	if err = database.First(&graphVersion, "id = ?", storyGraphOutput.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatalf("load published StoryGraph Version: %v", err)
	}
	var graphHead model.StoryGraphHead
	if err = database.First(&graphHead, "project_id = ?", fixture.projectID).Error; err != nil {
		t.Fatalf("load published StoryGraph Head: %v", err)
	}
	var graphNodes []storygraph.Node
	var graphEdges []storygraph.Edge
	if err = json.Unmarshal(graphVersion.Nodes, &graphNodes); err != nil {
		t.Fatalf("decode published StoryGraph nodes: %v", err)
	}
	if err = json.Unmarshal(graphVersion.Edges, &graphEdges); err != nil {
		t.Fatalf("decode published StoryGraph edges: %v", err)
	}
	typeCounts := map[storygraph.NodeType]int{}
	nodeTypes := map[string]storygraph.NodeType{}
	keys := make([]string, len(graphNodes))
	identityKey := ""
	sceneKey := ""
	for index, node := range graphNodes {
		typeCounts[node.NodeType]++
		nodeTypes[node.StoryNodeKey] = node.NodeType
		keys[index] = node.StoryNodeKey
		if identityKey == "" && node.NodeType == storygraph.NodeTypeAssetIdentity {
			identityKey = node.StoryNodeKey
		}
		if node.OwnerRef.OwnerVersionID == "" || node.OwnerRef.OwnerRevision < 1 || len(node.OwnerRef.ContentHash) != 64 || len(node.ContentHash) != 64 {
			t.Fatalf("StoryGraph node lost exact Owner reference: %#v", node)
		}
	}
	for _, edge := range graphEdges {
		if edge.EdgeType == storygraph.EdgeTypeAnchorsOccurrence && nodeTypes[edge.FromNodeKey] == storygraph.NodeTypeScene {
			sceneKey = edge.FromNodeKey
			break
		}
	}
	if _, err = storygraph.TopologicalOrder(keys, graphEdges); err != nil {
		t.Fatalf("published StoryGraph is not a DAG: %v", err)
	}
	if graphVersion.VersionNo != 1 || graphVersion.Status != "published" || graphVersion.ContentHash != storyGraphOutput.Bindings[0].ContentHash ||
		graphHead.CurrentVersionID != graphVersion.ID || graphHead.CurrentContentHash != graphVersion.ContentHash || graphHead.Revision != 1 ||
		typeCounts[storygraph.NodeTypeEpisode] != len(publishedEpisodes) || typeCounts[storygraph.NodeTypeSourceEvidence] == 0 ||
		typeCounts[storygraph.NodeTypeAssetIdentity] < 2 || typeCounts[storygraph.NodeTypeCharacterSpecification] < 2 ||
		typeCounts[storygraph.NodeTypeAssetState] < 2 || typeCounts[storygraph.NodeTypeProductionBinding] < 2 ||
		typeCounts[storygraph.NodeTypeWorldRule] == 0 || typeCounts[storygraph.NodeTypeStoryArc] == 0 ||
		typeCounts[storygraph.NodeTypeRelationshipClaim] == 0 ||
		typeCounts[storygraph.NodeTypeScene] == 0 ||
		typeCounts[storygraph.NodeTypeNarrativeBeat] != typeCounts[storygraph.NodeTypeScene] ||
		typeCounts[storygraph.NodeTypeOccurrence] != typeCounts[storygraph.NodeTypeScene] ||
		typeCounts[storygraph.NodeTypeCausalClaim] != typeCounts[storygraph.NodeTypeScene] {
		t.Fatalf("published multi-Episode StoryGraph is incomplete: version=%#v head=%#v types=%v", graphVersion, graphHead, typeCounts)
	}
	if identityKey == "" || sceneKey == "" {
		t.Fatalf("published StoryGraph has no queryable identity or occurrence anchor: identity=%q scene=%q", identityKey, sceneKey)
	}
	queryService := storygraphapp.NewQueryService(storygraphgorm.New(database))
	queryActor := storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	impact, err := queryService.Lens(ctx, queryActor, storygraphapp.LensQuery{
		ProjectID: fixture.projectID.String(), VersionRef: graphVersion.ID.String(), Lens: "impact",
		ScopeKind: storygraphapp.ScopeStoryNode, ScopeID: identityKey, Depth: 1, Limit: 200,
	})
	if err != nil || impact.VersionID != graphVersion.ID.String() || impact.ContentHash != graphVersion.ContentHash ||
		len(impact.ResultHash) != 64 || !containsStoryGraphNodeType(impact.Nodes, storygraph.NodeTypeRelationshipClaim) {
		t.Fatalf("published StoryGraph Impact Lens is not queryable: result=%#v err=%v", impact, err)
	}
	downstream, err := queryService.Trace(ctx, queryActor, storygraphapp.TraceQuery{
		ProjectID: fixture.projectID.String(), VersionRef: graphVersion.ID.String(), StoryNodeKey: sceneKey,
		Direction: storygraphapp.DirectionDownstream, Depth: 1, Limit: 200,
	})
	if err != nil || downstream.VersionID != graphVersion.ID.String() || len(downstream.ResultHash) != 64 ||
		!containsStoryGraphNodeType(downstream.Nodes, storygraph.NodeTypeOccurrence) ||
		!containsStoryGraphNodeType(downstream.Nodes, storygraph.NodeTypeCausalClaim) {
		t.Fatalf("published StoryGraph downstream trace is incomplete: result=%#v err=%v", downstream, err)
	}
	var graphReceipt model.CommandReceipt
	if err = database.Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?", fixture.workspaceID, "storygraph.compile_owner_set", graphVersion.ID,
	).First(&graphReceipt).Error; err != nil {
		t.Fatal(err)
	}
	var graphReceiptCount, graphEventCount int64
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?", fixture.workspaceID, "storygraph.compile_owner_set", graphVersion.ID,
	).Count(&graphReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.OutboxEvent{}).Where(
		"project_id = ? AND event_type = ? AND aggregate_id = ?", fixture.projectID, "StoryGraphVersionPublished", fixture.projectID.String(),
	).Count(&graphEventCount).Error; err != nil {
		t.Fatal(err)
	}
	if graphReceiptCount != 1 || graphEventCount != 1 {
		t.Fatalf("StoryGraph publication boundary counts: receipts=%d events=%d", graphReceiptCount, graphEventCount)
	}
	requiredPlanningOwners := make([]storygraph.OwnerHeadRef, len(planningSet.Structures))
	for index, reference := range planningSet.Structures {
		requiredPlanningOwners[index] = storygraph.OwnerHeadRef{
			OwnerKind: "production/planning", OwnerLogicalID: reference.EpisodeID,
			OwnerVersionID: reference.StructureID, OwnerRevision: int64(reference.Revision), ContentHash: reference.ResultHash,
		}
	}
	graphCompileCommand := storygraphapp.CompileOwnerSetCommand{
		ProjectID: fixture.projectID.String(), OwnerSetID: planningSet.ID, OwnerSetHash: planningSet.ContentHash,
		RequiredBibleVersionID: planningSet.BibleVersionID, RequiredBibleHash: planningSet.BibleContentHash,
		RequiredOwners: requiredPlanningOwners, IdempotencyKey: graphReceipt.IdempotencyKey,
	}
	replayedGraph, err := storyGraphService.CompileOwnerSet(ctx, storygraphapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, graphCompileCommand)
	if err != nil || replayedGraph.Version.ID != graphVersion.ID.String() ||
		replayedGraph.Version.ContentHash != graphVersion.ContentHash || replayedGraph.Receipt.ID != graphReceipt.ID.String() {
		t.Fatalf("StoryGraph unknown-result replay diverged: result=%#v receipt=%#v err=%v", replayedGraph, graphReceipt, err)
	}
	driftedGraphCommand := graphCompileCommand
	driftedGraphCommand.OwnerSetHash = bibledomain.SourceTextHash("different StoryGraph owner set")
	_, err = storyGraphService.CompileOwnerSet(ctx, storygraphapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, driftedGraphCommand)
	var graphConflict *storygraphapp.Error
	if !errors.As(err, &graphConflict) || graphConflict.Code != "resource_conflict" {
		t.Fatalf("StoryGraph idempotency mismatch error=%#v", err)
	}
	if err = database.Model(&model.StoryGraphVersion{}).Where("project_id = ?", fixture.projectID).Count(&graphReceiptCount).Error; err != nil || graphReceiptCount != 1 {
		t.Fatalf("StoryGraph replay or conflict created another Version: count=%d err=%v", graphReceiptCount, err)
	}
	var storyboardNode model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "storyboard").First(&storyboardNode).Error; err != nil {
		t.Fatalf("load Storyboard Draft NodeRun: %v", err)
	}
	storyboardOutput, _, storyboardOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(storyboardNode.Output))
	if err != nil || storyboardNode.Status != "SUCCEEDED" || storyboardNode.OutputHash == nil ||
		*storyboardNode.OutputHash != storyboardOutputHash || len(storyboardOutput.Bindings) != 1 ||
		storyboardOutput.Bindings[0].Port != "candidate" ||
		storyboardOutput.Bindings[0].ValueType != "storyboard_intent_candidate_set" ||
		storyboardOutput.Bindings[0].ReferenceVersion != "1" || len(storyboardOutput.Bindings[0].ContentHash) != 64 {
		t.Fatalf("Storyboard Draft output=%#v node=%#v err=%v", storyboardOutput, storyboardNode, err)
	}
	var draftSet model.StoryboardDraftSet
	if err = database.First(&draftSet, "node_run_id = ?", storyboardNode.ID).Error; err != nil {
		t.Fatalf("load Storyboard Draft set: %v", err)
	}
	if draftSet.Status != "needs_asset" || draftSet.Revision != 2 || draftSet.WorkflowRunID.String() != run.ID ||
		draftSet.NodeRunID != storyboardNode.ID || draftSet.GraphVersionID != graphVersion.ID ||
		draftSet.GraphVersionNo != graphVersion.VersionNo || draftSet.GraphContentHash != graphVersion.ContentHash ||
		draftSet.CandidateRevisionID == nil || draftSet.CandidateRevisionHash == nil || draftSet.ResultHash == nil ||
		storyboardOutput.Bindings[0].ReferenceID != draftSet.CandidateRevisionID.String() ||
		storyboardOutput.Bindings[0].ContentHash != *draftSet.CandidateRevisionHash ||
		*draftSet.ResultHash != *draftSet.CandidateRevisionHash {
		t.Fatalf("Storyboard Draft set lost exact workflow or StoryGraph ownership: %#v", draftSet)
	}
	var draftManifest model.ShardManifest
	if err = database.First(&draftManifest, "id = ? AND version = ?", draftSet.ManifestID, draftSet.ManifestVersion).Error; err != nil {
		t.Fatalf("load Storyboard Draft manifest: %v", err)
	}
	var draftShards []storyboarddomain.DraftManifestShard
	if err = json.Unmarshal(draftManifest.Shards, &draftShards); err != nil ||
		draftManifest.Stage != "draft_storyboard" || draftManifest.WorkflowRunID.String() != run.ID ||
		draftManifest.NodeRunID != storyboardNode.ID || draftManifest.ManifestHash != draftSet.ManifestHash ||
		len(draftShards) != typeCounts[storygraph.NodeTypeScene] {
		t.Fatalf("Storyboard Draft manifest does not cover every Scene: manifest=%#v shards=%#v err=%v", draftManifest, draftShards, err)
	}
	var storyboardBatches []model.StoryboardDraftBatch
	if err = database.Where("node_run_id = ?", storyboardNode.ID).Order("scene_story_node_key").Find(&storyboardBatches).Error; err != nil {
		t.Fatalf("load Storyboard Scene batches: %v", err)
	}
	var storyboardInvocations []model.AgentInvocation
	if err = database.Where("node_run_id = ? AND request_type = ?", storyboardNode.ID, "storyboard_scene_draft").
		Order("shard_key").Find(&storyboardInvocations).Error; err != nil {
		t.Fatalf("load Storyboard Scene invocations: %v", err)
	}
	if len(storyboardBatches) != len(draftShards) || len(storyboardInvocations) != len(draftShards) {
		t.Fatalf("Storyboard Draft is not one durable shard per Scene: shards=%d batches=%d invocations=%d",
			len(draftShards), len(storyboardBatches), len(storyboardInvocations))
	}
	leafRevisionIDs := make(map[string]struct{}, len(storyboardInvocations))
	for _, invocation := range storyboardInvocations {
		request, requestErr := agentgorm.StageInvocation(invocation)
		if requestErr != nil || request.Payload.BaseStoryGraphVersionID != graphVersion.ID.String() ||
			request.Payload.BaseStoryGraphHash != graphVersion.ContentHash || len(request.Payload.SourceRefs) != 2 ||
			len(request.Payload.UpstreamCandidates) != 0 || request.Payload.Shard.Kind != "story_scene" ||
			request.Payload.ShardManifestRef.ManifestID != draftManifest.ID.String() ||
			request.Payload.ShardManifestRef.Version != draftManifest.Version ||
			request.Payload.ShardManifestRef.Hash != draftManifest.ManifestHash {
			t.Fatalf("Storyboard Scene invocation lost exact immutable inputs: invocation=%#v request=%#v err=%v", invocation, request, requestErr)
		}
		var stageInput agentcontract.StoryboardDraftStageInput
		if err = json.Unmarshal(request.Payload.StageInput, &stageInput); err != nil || len(stageInput.Beats) == 0 ||
			len(stageInput.Occurrences) == 0 || len(stageInput.AssetVersions) != 0 ||
			stageInput.EffectiveStyleSnapshot.VisualStyle != "cinematic noir" ||
			stageInput.EffectiveStyleSnapshot.AspectRatio != "9:16" {
			t.Fatalf("Storyboard Scene input is not exact or did not preserve needs_asset: input=%#v err=%v", stageInput, err)
		}
		var leaf model.StageCandidateRevision
		if err = database.First(&leaf, "source_invocation_id = ?", invocation.ID).Error; err != nil {
			t.Fatalf("load Storyboard Scene CandidateRevision: %v", err)
		}
		candidate, candidateErr := storyboarddomain.DecodeAndValidateCandidate(json.RawMessage(leaf.Candidate), request.Payload.StageInput)
		if candidateErr != nil || leaf.OriginKind != "invocation" || candidate.AssetReadiness != "needs_asset" ||
			candidate.SceneStoryNodeKey != stageInput.Scene.StoryNodeKey || len(candidate.ShotIntents) == 0 {
			t.Fatalf("Storyboard Scene Candidate is not reviewable: revision=%#v candidate=%#v err=%v", leaf, candidate, candidateErr)
		}
		for _, intent := range candidate.ShotIntents {
			for _, requirement := range intent.VisualRequirements {
				if requirement.AssetReadiness != "needs_asset" || requirement.AssetVersionRef != nil {
					t.Fatalf("missing exact AssetVersion did not remain explicit needs_asset: %#v", requirement)
				}
			}
		}
		leafRevisionIDs[leaf.ID.String()] = struct{}{}
	}
	var storyboardAggregateRevision model.StageCandidateRevision
	if err = database.First(&storyboardAggregateRevision, "id = ?", storyboardOutput.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatalf("load Storyboard aggregate CandidateRevision: %v", err)
	}
	var candidateSet storyboarddomain.CandidateSet
	var aggregateOrigin agentcontract.AggregateCandidateOrigin
	if err = json.Unmarshal(storyboardAggregateRevision.Candidate, &candidateSet); err != nil {
		t.Fatalf("decode Storyboard aggregate Candidate Set: %v", err)
	}
	if err = json.Unmarshal(storyboardAggregateRevision.AggregateOrigin, &aggregateOrigin); err != nil {
		t.Fatalf("decode Storyboard aggregate origin: %v", err)
	}
	if storyboardAggregateRevision.OriginKind != "aggregate" || storyboardAggregateRevision.CandidateRevisionHash != storyboardOutput.Bindings[0].ContentHash ||
		candidateSet.SchemaVersion != "storyboard-intent-candidate-set-v1" || candidateSet.AssetReadiness != "needs_asset" ||
		candidateSet.DraftSetID != draftSet.ID.String() || candidateSet.DraftSetRevision != draftSet.Revision ||
		candidateSet.GraphVersionID != graphVersion.ID.String() || candidateSet.GraphContentHash != graphVersion.ContentHash ||
		candidateSet.ManifestID != draftManifest.ID.String() || candidateSet.ManifestHash != draftManifest.ManifestHash ||
		len(candidateSet.Scenes) != len(draftShards) || len(aggregateOrigin.LeafCandidates) != len(draftShards) {
		t.Fatalf("Storyboard aggregate Candidate Set is incomplete: revision=%#v candidate=%#v origin=%#v",
			storyboardAggregateRevision, candidateSet, aggregateOrigin)
	}
	for _, leaf := range aggregateOrigin.LeafCandidates {
		if _, exists := leafRevisionIDs[leaf.CandidateRevisionID]; !exists {
			t.Fatalf("Storyboard aggregate references a non-Scene CandidateRevision: %#v", leaf)
		}
	}
	var storyboardTaskCount, storyboardShotCount, storyboardImageBindingCount, generationTargetCount, generationIntentCount, providerJobCount int64
	var costEstimateCount, costReservationCount, quotaReservationCount, storyboardArtifactCount, storyboardGraphVersionCount int64
	for target, destination := range map[any]*int64{
		&model.StoryboardShot{}:                    &storyboardShotCount,
		&model.StoryboardShotImageBindingVersion{}: &storyboardImageBindingCount,
		&model.GenerationTarget{}:                  &generationTargetCount,
		&model.GenerationIntent{}:                  &generationIntentCount,
		&model.GenerationProviderJob{}:             &providerJobCount,
		&model.CostEstimate{}:                      &costEstimateCount,
		&model.CostReservation{}:                   &costReservationCount,
		&model.QuotaReservation{}:                  &quotaReservationCount,
		&model.Artifact{}:                          &storyboardArtifactCount,
		&model.StoryGraphVersion{}:                 &storyboardGraphVersionCount,
	} {
		if err = database.Model(target).Where("workspace_id = ?", fixture.workspaceID).Count(destination).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = database.Model(&model.WorkflowTask{}).Where(
		"workspace_id = ? AND request_type = ?", fixture.workspaceID, "storyboard_scene_draft",
	).Count(&storyboardTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if storyboardTaskCount != 0 || storyboardShotCount != 0 || storyboardImageBindingCount != 0 || generationTargetCount != 0 ||
		generationIntentCount != 0 || providerJobCount != 0 || costEstimateCount != 0 || costReservationCount != 0 ||
		quotaReservationCount != 0 || storyboardArtifactCount != 0 || storyboardGraphVersionCount != 1 {
		t.Fatalf("Storyboard intent drafting crossed its candidate-only boundary: tasks=%d shots=%d bindings=%d targets=%d intents=%d jobs=%d estimates=%d reservations=%d quotas=%d artifacts=%d graph_versions=%d",
			storyboardTaskCount, storyboardShotCount, storyboardImageBindingCount, generationTargetCount, generationIntentCount, providerJobCount,
			costEstimateCount, costReservationCount, quotaReservationCount, storyboardArtifactCount, storyboardGraphVersionCount)
	}

	var storyboardTaskCandidateIDs []string
	if err = json.Unmarshal(storyboardTask.CandidateIDs, &storyboardTaskCandidateIDs); err != nil || len(storyboardTaskCandidateIDs) != 1 ||
		storyboardTask.SubjectType != "storyboard_intent_candidate" ||
		storyboardTask.SubjectID.String() != storyboardAggregateRevision.ID.String() || storyboardTask.SubjectRevision != 1 ||
		storyboardTask.SubjectHash != storyboardAggregateRevision.CandidateRevisionHash ||
		storyboardTaskCandidateIDs[0] != storyboardAggregateRevision.ID.String() {
		t.Fatalf("Storyboard Intent HumanTask did not freeze the aggregate Candidate: task=%#v candidates=%v err=%v",
			storyboardTask, storyboardTaskCandidateIDs, err)
	}
	storyboardClaim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: storyboardTask.ID.String(), ExpectedRevision: storyboardTask.Revision,
		IdempotencyKey: "storyboard-intent-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Storyboard Intent HumanTask: %v", err)
	}
	storyboardDecision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: storyboardClaim.Task.ID, ClaimToken: storyboardClaim.ClaimToken,
		ExpectedTaskRevision:    storyboardClaim.Task.Revision,
		ExpectedSubjectRevision: storyboardClaim.Task.SubjectRevision,
		ExpectedSubjectHash:     storyboardClaim.Task.SubjectHash, Decision: "approved",
		IdempotencyKey: "storyboard-intent-review-decision",
	})
	if err != nil {
		t.Fatalf("approve Storyboard Intent HumanTask: %v", err)
	}
	storyboardSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: run.ID, NodeRunID: storyboardGateNode.ID.String(),
		HumanTaskID: storyboardDecision.Task.ID, ReviewDecisionID: storyboardDecision.Decision.ID,
		SubjectRevision: storyboardDecision.Decision.SubjectRevision, Decision: storyboardDecision.Decision.Decision,
		IdempotencyKey: "storyboard-intent-review-signal",
	}
	func() {
		driftDatabase := database.Begin()
		if driftDatabase.Error != nil {
			t.Fatalf("begin Storyboard baseline drift transaction: %v", driftDatabase.Error)
		}
		defer func() { _ = driftDatabase.Rollback().Error }()
		if updateErr := driftDatabase.Model(&model.StoryboardDraftSet{}).Where("id = ?", draftSet.ID).
			Update("revision", draftSet.Revision+1).Error; updateErr != nil {
			t.Fatalf("inject Storyboard Draft Set baseline drift: %v", updateErr)
		}
		driftWorkflowStore := workflowgorm.New(driftDatabase)
		driftSignalService := workflowapp.NewSignalService(driftWorkflowStore, temporalRuntime, workflowapp.SignalConfig{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
			Owner: workflowproduction.New(
				bibleapp.NewService(biblegorm.New(driftDatabase), bibleapp.Config{Now: time.Now, NewID: uuid.NewString}),
				planningapp.NewService(planninggorm.New(driftDatabase), planningapp.Config{Now: time.Now, NewID: uuid.NewString}),
				planningapp.NewEpisodePlanningService(planninggorm.New(driftDatabase), planningapp.Config{Now: time.Now, NewID: uuid.NewString}),
				storyboardapp.NewService(storyboardgorm.New(driftDatabase), storyboardapp.Config{Now: time.Now, NewID: uuid.NewString}),
			),
		})
		driftCommand := storyboardSignalCommand
		driftCommand.IdempotencyKey = "storyboard-intent-drift-signal"
		_, driftErr := driftSignalService.SignalHumanGate(ctx, workflowapp.Actor{
			UserID: fixture.userID.String(), TokenVersion: 1,
		}, driftCommand)
		var typedDrift *workflowapp.Error
		if !errors.As(driftErr, &typedDrift) || typedDrift.Status != 409 {
			t.Fatalf("Storyboard baseline drift did not stop the approved Decision: %v", driftErr)
		}
		var decisionCount, freezeCount int64
		var conflictApply model.WorkflowHumanGateApplyReceipt
		if queryErr := driftDatabase.Model(&model.ReviewDecision{}).Where("id = ?", storyboardDecision.Decision.ID).
			Count(&decisionCount).Error; queryErr != nil {
			t.Fatal(queryErr)
		}
		if queryErr := driftDatabase.Model(&model.CommandReceipt{}).Where(
			"workspace_id = ? AND operation = ?", fixture.workspaceID, "storyboard.freeze_intent_set",
		).Count(&freezeCount).Error; queryErr != nil {
			t.Fatal(queryErr)
		}
		if queryErr := driftDatabase.Where("review_decision_id = ?", storyboardDecision.Decision.ID).
			First(&conflictApply).Error; queryErr != nil || conflictApply.Status != "conflict" ||
			conflictApply.ConflictCode == nil || conflictApply.OwnerReceiptID != nil || decisionCount != 1 || freezeCount != 0 {
			t.Fatalf("Storyboard baseline drift evidence is incomplete: decision=%d freeze=%d apply=%#v err=%v",
				decisionCount, freezeCount, conflictApply, queryErr)
		}
	}()
	if err = database.First(&draftSet, "id = ?", draftSet.ID).Error; err != nil || draftSet.Revision != 2 || draftSet.Status != "needs_asset" {
		t.Fatalf("Storyboard baseline drift test leaked state: set=%#v err=%v", draftSet, err)
	}
	storyboardUnknownSignaler := &unknownOnceWorkflowSignaler{delegate: temporalRuntime}
	storyboardSignalService := workflowapp.NewSignalService(workflowStore, storyboardUnknownSignaler, workflowapp.SignalConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Owner: workflowproduction.New(bibleService, planningService, episodePlanningService, storyboardService),
	})
	unknownStoryboardSignal, err := storyboardSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, storyboardSignalCommand)
	if err != nil || unknownStoryboardSignal.Status != "unknown" || unknownStoryboardSignal.AttemptNo != 1 {
		t.Fatalf("persist unknown Storyboard Intent Signal: intent=%#v err=%v", unknownStoryboardSignal, err)
	}
	var intentFreezeReceipt model.CommandReceipt
	if err = database.Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "storyboard.freeze_intent_set", draftSet.ID,
	).First(&intentFreezeReceipt).Error; err != nil {
		t.Fatalf("load Storyboard Intent freeze Receipt after unknown Signal: %v", err)
	}
	var approvedIntents storyboarddomain.ApprovedIntentSet
	if err = json.Unmarshal(intentFreezeReceipt.Result, &approvedIntents); err != nil ||
		approvedIntents.SchemaVersion != "approved-storyboard-intents-v1" ||
		approvedIntents.ID != intentFreezeReceipt.ID.String() || approvedIntents.DraftSetID != draftSet.ID.String() ||
		approvedIntents.DraftSetRevision != 2 ||
		approvedIntents.CandidateRevisionID != storyboardAggregateRevision.ID.String() ||
		approvedIntents.CandidateRevisionHash != storyboardAggregateRevision.CandidateRevisionHash ||
		approvedIntents.CandidateRevision != 1 || approvedIntents.ReviewDecisionID != storyboardDecision.Decision.ID ||
		len(approvedIntents.Scenes) != len(candidateSet.Scenes) || len(approvedIntents.VisualRequirementsHash) != 64 ||
		len(approvedIntents.ContentHash) != 64 {
		t.Fatalf("Storyboard approved Intent Receipt is incomplete: approved=%#v err=%v", approvedIntents, err)
	}
	approvedIntentCount, approvedRequirementCount := 0, 0
	for _, scene := range approvedIntents.Scenes {
		if len(scene.ShotIntents) == 0 || scene.AssetReadiness != "needs_asset" {
			t.Fatalf("Storyboard approved Scene lost accepted Intents or readiness: %#v", scene)
		}
		approvedIntentCount += len(scene.ShotIntents)
		for _, intent := range scene.ShotIntents {
			approvedRequirementCount += len(intent.VisualRequirements)
		}
	}
	if approvedIntentCount == 0 || approvedRequirementCount == 0 {
		t.Fatalf("Storyboard Intent freeze did not preserve visual requirements: intents=%d requirements=%d",
			approvedIntentCount, approvedRequirementCount)
	}
	if err = database.First(&draftSet, "id = ?", draftSet.ID).Error; err != nil {
		t.Fatal(err)
	}
	if draftSet.Status != "intent_frozen" || draftSet.Revision != approvedIntents.DraftSetRevision+1 ||
		draftSet.ResultHash == nil || *draftSet.ResultHash != approvedIntents.ContentHash ||
		draftSet.CandidateRevisionID == nil || draftSet.CandidateRevisionID.String() != approvedIntents.CandidateRevisionID ||
		draftSet.CandidateRevisionHash == nil || *draftSet.CandidateRevisionHash != approvedIntents.CandidateRevisionHash {
		t.Fatalf("Storyboard Draft Set did not preserve its Candidate while freezing Intents: %#v", draftSet)
	}
	var storyboardApplyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.Where("review_decision_id = ?", storyboardDecision.Decision.ID).First(&storyboardApplyReceipt).Error; err != nil ||
		storyboardApplyReceipt.Status != "completed" || storyboardApplyReceipt.OwnerReceiptID == nil ||
		*storyboardApplyReceipt.OwnerReceiptID != intentFreezeReceipt.ID || storyboardApplyReceipt.OwnerOperation == nil ||
		*storyboardApplyReceipt.OwnerOperation != "storyboard.freeze_intent_set" {
		t.Fatalf("Storyboard Workflow Apply Receipt is incomplete: receipt=%#v err=%v", storyboardApplyReceipt, err)
	}
	replayedIntentFreeze, err := storyboardService.FreezeIntentSet(ctx, storyboardapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, storyboardapp.FreezeIntentSetCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		CandidateRevisionID:       storyboardAggregateRevision.ID.String(),
		CandidateRevisionHash:     storyboardAggregateRevision.CandidateRevisionHash,
		ExpectedCandidateRevision: int64(storyboardTask.SubjectRevision),
		ReviewDecisionID:          storyboardDecision.Decision.ID,
		IdempotencyKey:            "workflow-review:" + storyboardDecision.Decision.ID,
	})
	var intentFreezeCount int64
	if countErr := database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "storyboard.freeze_intent_set", draftSet.ID,
	).Count(&intentFreezeCount).Error; countErr != nil {
		t.Fatal(countErr)
	}
	if err != nil || replayedIntentFreeze.Receipt.ID != intentFreezeReceipt.ID.String() ||
		replayedIntentFreeze.Approved.ContentHash != approvedIntents.ContentHash ||
		replayedIntentFreeze.Set.Revision != draftSet.Revision || intentFreezeCount != 1 {
		t.Fatalf("Storyboard Intent owner replay is not identical: replay=%#v receipts=%d err=%v",
			replayedIntentFreeze, intentFreezeCount, err)
	}
	completedStoryboardSignal, err := storyboardSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, storyboardSignalCommand)
	if err != nil || completedStoryboardSignal.ID != unknownStoryboardSignal.ID ||
		completedStoryboardSignal.Status != "completed" || completedStoryboardSignal.AttemptNo != 2 {
		t.Fatalf("recover unknown Storyboard Intent Signal: intent=%#v err=%v", completedStoryboardSignal, err)
	}
	replayedStoryboardSignal, err := storyboardSignalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, storyboardSignalCommand)
	if err != nil || replayedStoryboardSignal.ID != completedStoryboardSignal.ID || replayedStoryboardSignal.Status != "completed" {
		t.Fatalf("replay completed Storyboard Intent Signal: intent=%#v err=%v", replayedStoryboardSignal, err)
	}
	completionDeadline = time.Now().Add(20 * time.Second)
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("reload completed Storyboard Intent Workflow Run: %v", err)
		}
		if persistedRun.Status == "SUCCEEDED" {
			break
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(completionDeadline) {
			t.Fatalf("Source Evidence Workflow did not complete after Storyboard Intent approval: %#v", persistedRun)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = database.First(&storyboardGateNode, "id = ?", storyboardGateNode.ID).Error; err != nil {
		t.Fatal(err)
	}
	intentOutput, _, intentOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(storyboardGateNode.Output))
	if err != nil || storyboardGateNode.Status != "SUCCEEDED" || storyboardGateNode.OutputHash == nil ||
		*storyboardGateNode.OutputHash != intentOutputHash || len(intentOutput.Bindings) != 1 ||
		intentOutput.Bindings[0].Port != "intents" || intentOutput.Bindings[0].ValueType != "approved_storyboard_intents" ||
		intentOutput.Bindings[0].ReferenceID != approvedIntents.ID || intentOutput.Bindings[0].ReferenceVersion != "1" ||
		intentOutput.Bindings[0].ContentHash != approvedIntents.ContentHash {
		t.Fatalf("Storyboard Intent Gate output=%#v node=%#v err=%v", intentOutput, storyboardGateNode, err)
	}
	referenceTargetBuilder := generationapp.NewReferenceTargetBuilderService(
		generationgorm.New(database),
		generationapp.ReferenceTargetBuilderConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	referenceTargetCommand := generationapp.BuildReferenceTargetsCommand{
		ApprovedIntentSetID: approvedIntents.ID, ExpectedContentHash: approvedIntents.ContentHash,
		IdempotencyKey: "workflow-reference-targets:" + approvedIntents.ID,
	}
	referenceTargets, err := referenceTargetBuilder.BuildReferenceTargets(ctx, generationapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, referenceTargetCommand)
	if err != nil || len(referenceTargets.Targets) == 0 ||
		referenceTargets.Receipt.Operation != "generation.reference_targets.build" ||
		referenceTargets.Receipt.ResourceID != approvedIntents.ID {
		t.Fatalf("build reference targets from approved Storyboard Intents: result=%#v err=%v", referenceTargets, err)
	}
	for _, target := range referenceTargets.Targets {
		if generationdomain.ValidateGenerationTarget(target) != nil ||
			target.SourceOwnerRef.ID != approvedIntents.ID || target.SourceOwnerRef.ContentHash != approvedIntents.ContentHash ||
			target.PolicySnapshotRef.Owner != "preset" || target.ReferenceAsset == nil ||
			target.ReferenceAsset.AssetKind != "character" || target.ReferenceAsset.OutputKind != "reference_sheet" ||
			!slices.Equal(target.ReferenceAsset.RequiredViewRoles, []string{"front", "profile", "back"}) {
			t.Fatalf("approved reference target is incomplete: %#v", target)
		}
	}
	replayedReferenceTargets, err := referenceTargetBuilder.BuildReferenceTargets(ctx, generationapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, referenceTargetCommand)
	if err != nil || replayedReferenceTargets.Receipt.ID != referenceTargets.Receipt.ID ||
		len(replayedReferenceTargets.Targets) != len(referenceTargets.Targets) {
		t.Fatalf("replay approved reference targets: result=%#v err=%v", replayedReferenceTargets, err)
	}
	func() {
		driftDatabase := database.Begin()
		if driftDatabase.Error != nil {
			t.Fatalf("begin reference target drift transaction: %v", driftDatabase.Error)
		}
		defer func() { _ = driftDatabase.Rollback().Error }()
		specificationID := referenceTargets.Targets[0].ReferenceAsset.SpecificationVersionRef.ID
		if updateErr := driftDatabase.Model(&model.ProductionBibleSpecificationVersion{}).
			Where("id = ?", specificationID).UpdateColumn("content_hash", strings.Repeat("0", 64)).Error; updateErr != nil {
			t.Fatalf("inject reference target Specification drift: %v", updateErr)
		}
		driftBuilder := generationapp.NewReferenceTargetBuilderService(
			generationgorm.New(driftDatabase),
			generationapp.ReferenceTargetBuilderConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
		)
		driftCommand := referenceTargetCommand
		driftCommand.IdempotencyKey = "workflow-reference-targets-drift:" + approvedIntents.ID
		_, driftErr := driftBuilder.BuildReferenceTargets(ctx, generationapp.Actor{
			UserID: fixture.userID.String(), TokenVersion: 1,
		}, driftCommand)
		var typedDrift *generationapp.Error
		if !errors.As(driftErr, &typedDrift) || typedDrift.Code != "state_conflict" {
			t.Fatalf("drifted approved Specification reached target persistence: %T %v", driftErr, driftErr)
		}
		var driftReceiptCount int64
		if countErr := driftDatabase.Model(&model.CommandReceipt{}).Where(
			"workspace_id = ? AND operation = ? AND idempotency_key = ?",
			fixture.workspaceID, "generation.reference_targets.build", driftCommand.IdempotencyKey,
		).Count(&driftReceiptCount).Error; countErr != nil || driftReceiptCount != 0 {
			t.Fatalf("drifted approved Specification wrote a target receipt: count=%d err=%v", driftReceiptCount, countErr)
		}
	}()
	var intentFreezeReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		fixture.workspaceID, "storyboard.freeze_intent_set", draftSet.ID,
	).Count(&intentFreezeReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	for target, destination := range map[any]*int64{
		&model.StoryboardShot{}:                    &storyboardShotCount,
		&model.StoryboardShotImageBindingVersion{}: &storyboardImageBindingCount,
		&model.GenerationTarget{}:                  &generationTargetCount,
		&model.GenerationIntent{}:                  &generationIntentCount,
		&model.GenerationProviderJob{}:             &providerJobCount,
		&model.CostEstimate{}:                      &costEstimateCount,
		&model.CostReservation{}:                   &costReservationCount,
		&model.QuotaReservation{}:                  &quotaReservationCount,
		&model.Artifact{}:                          &storyboardArtifactCount,
		&model.StoryGraphVersion{}:                 &storyboardGraphVersionCount,
	} {
		if err = database.Model(target).Where("workspace_id = ?", fixture.workspaceID).Count(destination).Error; err != nil {
			t.Fatal(err)
		}
	}
	if intentFreezeReceiptCount != 1 || storyboardShotCount != 0 || storyboardImageBindingCount != 0 ||
		generationTargetCount != int64(len(referenceTargets.Targets)) ||
		generationIntentCount != 0 || providerJobCount != 0 || costEstimateCount != 0 || costReservationCount != 0 ||
		quotaReservationCount != 0 || storyboardArtifactCount != 0 || storyboardGraphVersionCount != 1 {
		t.Fatalf("Approved target build crossed the paid/formal boundary: receipts=%d shots=%d bindings=%d targets=%d intents=%d jobs=%d estimates=%d reservations=%d quotas=%d artifacts=%d graph_versions=%d",
			intentFreezeReceiptCount, storyboardShotCount, storyboardImageBindingCount, generationTargetCount, generationIntentCount, providerJobCount,
			costEstimateCount, costReservationCount, quotaReservationCount, storyboardArtifactCount, storyboardGraphVersionCount)
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
	var aggregateRevisions []model.StageCandidateRevision
	if err = database.Where("workspace_id = ? AND origin_kind = ?", fixture.workspaceID, "aggregate").
		Find(&aggregateRevisions).Error; err != nil {
		t.Fatal(err)
	}
	var aggregateRevision model.StageCandidateRevision
	for _, candidateRevision := range aggregateRevisions {
		var origin agentcontract.AggregateCandidateOrigin
		if err = json.Unmarshal(candidateRevision.AggregateOrigin, &origin); err != nil {
			t.Fatal(err)
		}
		if origin.ShardManifestID == manifest.ID.String() {
			aggregateRevision = candidateRevision
			aggregateRevisionCount++
		}
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
	if aggregateRevision.ID == uuid.Nil {
		t.Fatal("Source Evidence aggregate Candidate Revision is missing")
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
	episodeAnalysisUnknown  bool
	episodeAnalysisID       string
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
	if invocation.Payload.Stage == "draft_storyboard" {
		return storyboardDraftFixtureResult(invocation)
	}
	if invocation.Payload.Stage == planningdomain.AnalyzeEpisodeStage {
		agent.mutex.Lock()
		if agent.episodeAnalysisID == "" {
			agent.episodeAnalysisID = invocation.InvocationID
			agent.mutex.Unlock()
			return agentcontract.StageResult{}, errors.New("injected Episode analysis transport outcome unknown")
		}
		if agent.episodeAnalysisID == invocation.InvocationID && !agent.episodeAnalysisUnknown {
			agent.episodeAnalysisUnknown = true
		}
		agent.mutex.Unlock()
		return episodeAnalysisFixtureResult(invocation)
	}
	if invocation.Payload.Stage == planningdomain.ReconcileEpisodeStage {
		return episodeReconciliationFixtureResult(invocation)
	}
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

func storyboardDraftFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var input agentcontract.StoryboardDraftStageInput
	if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
		return agentcontract.StageResult{}, err
	}
	beatKeys := make([]string, len(input.Beats))
	for index, beat := range input.Beats {
		beatKeys[index] = beat.StoryNodeKey
	}
	visualRequirements := make([]storyboarddomain.VisualRequirement, len(input.Occurrences))
	for index, occurrence := range input.Occurrences {
		role := "subject"
		viewRoles := []string{"front", "profile", "back"}
		switch occurrence.AssetKind {
		case "location":
			role, viewRoles = "environment", []string{"environment"}
		case "prop":
			role, viewRoles = "prop", []string{"prop"}
		}
		visualRequirements[index] = storyboarddomain.VisualRequirement{
			OccurrenceStoryNodeKey:    occurrence.StoryNodeKey,
			IdentityStoryNodeKey:      occurrence.IdentityStoryNodeKey,
			SpecificationStoryNodeKey: occurrence.SpecificationStoryNodeKey,
			AssetStateStoryNodeKey:    occurrence.AssetStateStoryNodeKey,
			AssetID:                   occurrence.AssetID, SpecificationVersionID: occurrence.SpecificationVersionID,
			AssetStateID: occurrence.AssetStateID, AssetRole: role, RequiredViewRoles: viewRoles,
			AssetReadiness: "needs_asset",
		}
	}
	candidate, err := json.Marshal(storyboarddomain.Candidate{
		SceneStoryNodeKey: input.Scene.StoryNodeKey,
		ShotIntents: []storyboarddomain.ShotIntent{{
			ShotKey: "intent:" + input.Scene.StoryNodeKey, IntentOrder: 1,
			SourceBeatStoryNodeKeys: beatKeys,
			SourceEvidence: []storyboarddomain.EvidenceRef{{
				DocumentRevisionID: input.Beats[0].Evidence[0].DocumentRevisionID,
				AbsoluteStart:      input.Beats[0].Evidence[0].AbsoluteStart,
				AbsoluteEnd:        input.Beats[0].Evidence[0].AbsoluteEnd,
				TextHash:           input.Beats[0].Evidence[0].TextHash,
			}},
			Purpose: "建立场景动作", ProposedDurationMS: 2500,
			Camera: storyboarddomain.CameraIntent{
				Scale: "medium", Angle: "eye_level", Movement: "static", Composition: "centered",
			},
			ActionIntent: "人物完成场景动作", SoundIntent: "保持场景环境声", PerformanceIntent: "克制",
			ContinuityIn: "承接上一场", ContinuityOut: "保持动作轴线",
			FrameIntent:        storyboarddomain.FrameIntent{First: "场景建立", Key: "动作发生", Last: "动作完成"},
			VisualRequirements: visualRequirements, RiskCodes: []string{"reference_asset_missing"},
			ReviewIssues: []storyboarddomain.ReviewIssue{},
		}},
		AssetReadiness: "needs_asset",
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
		Status: "succeeded", CandidateType: "storyboard_row_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, ResultHash: &resultHash, Issues: []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "storyboard-draft-v1", Model: "deterministic-fixture"},
	}, nil
}

func episodeAnalysisFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var input agentcontract.EpisodeAnalysisStageInput
	if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
		return agentcontract.StageResult{}, err
	}
	contextRunes := []rune(input.ContextText)
	localStart, localEnd := input.LogicalStart-input.ContextStart, input.LogicalEnd-input.ContextStart
	if localStart < 0 || localEnd > len(contextRunes) || localEnd <= localStart {
		return agentcontract.StageResult{}, errors.New("fixture Episode analysis received an invalid logical range")
	}
	anchor := string(contextRunes[localStart:localEnd])
	episodeNumber := input.EpisodePosition
	fragments := []planningdomain.EpisodeStructureFragment{}
	claims := []planningdomain.EpisodeClaimCandidate{}
	if strings.TrimSpace(anchor) != "" {
		sceneKey := "scene:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-")
		evidence := bibledomain.Evidence{
			SourceStart: input.LogicalStart, SourceEnd: input.LogicalEnd,
			TextHash: bibledomain.SourceTextHash(anchor), ExactAnchor: anchor, EpisodeNumber: &episodeNumber,
		}
		fragments = append(fragments, planningdomain.EpisodeStructureFragment{
			TemporaryKey: sceneKey,
			Kind:         "scene", SourceKeys: []string{"episode:" + input.EpisodeID},
			SourceStart: input.LogicalStart, SourceEnd: input.LogicalEnd, Summary: "冻结分片场景",
			Evidence: []bibledomain.Evidence{evidence},
			Attributes: planningdomain.EpisodeStructureAttributes{
				ParticipantKeys: []string{}, ContinuityNotes: []string{},
			},
		})
		action := "人物完成冻结分片中的叙事动作"
		beatKey := "beat:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-")
		fragments = append(fragments, planningdomain.EpisodeStructureFragment{
			TemporaryKey: beatKey, Kind: "beat", SourceKeys: []string{"episode:" + input.EpisodeID},
			SourceStart: input.LogicalStart, SourceEnd: input.LogicalEnd, Summary: action,
			Evidence: []bibledomain.Evidence{evidence},
			Attributes: planningdomain.EpisodeStructureAttributes{
				SceneKey: &sceneKey, Action: &action, ParticipantKeys: []string{}, ContinuityNotes: []string{},
			},
		})
		if len(input.KnownIdentities) >= 2 {
			occurrenceKey := "occurrence:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-")
			baseState := "base"
			fragments = append(fragments, planningdomain.EpisodeStructureFragment{
				TemporaryKey: occurrenceKey, Kind: "occurrence", SourceKeys: []string{"episode:" + input.EpisodeID},
				SourceStart: input.LogicalStart, SourceEnd: input.LogicalEnd, Summary: "角色以基础状态出现",
				Evidence: []bibledomain.Evidence{evidence},
				Attributes: planningdomain.EpisodeStructureAttributes{
					SceneKey: &sceneKey, ParticipantKeys: []string{},
					OccurrenceEntityKey: &input.KnownIdentities[0].EntityKey,
					StateKey:            &baseState, ContinuityNotes: []string{},
				},
			})
			claims = append(claims, planningdomain.EpisodeClaimCandidate{
				ClaimKey: "claim:" + strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-"), ClaimType: "causal",
				ParticipantKeys: []string{input.KnownIdentities[0].EntityKey, input.KnownIdentities[1].EntityKey},
				AnchorKeys:      []string{sceneKey, beatKey, occurrenceKey}, Scope: "episode:" + input.EpisodeID,
				Polarity: "positive", Status: "proposed", Evidence: []bibledomain.Evidence{evidence},
			})
		}
	}
	slices.SortFunc(fragments, func(left, right planningdomain.EpisodeStructureFragment) int {
		if left.SourceStart != right.SourceStart {
			return left.SourceStart - right.SourceStart
		}
		if left.SourceEnd != right.SourceEnd {
			return left.SourceEnd - right.SourceEnd
		}
		return strings.Compare(left.TemporaryKey, right.TemporaryKey)
	})
	candidate, err := json.Marshal(planningdomain.EpisodeAnalysisCandidate{
		EpisodeID: input.EpisodeID, ScriptVersionID: input.ScriptVersionID,
		LogicalStart: input.LogicalStart, LogicalEnd: input.LogicalEnd,
		Fragments: fragments,
		Claims:    claims, ReviewIssues: []bibledomain.ReviewIssue{},
	})
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	return episodeFixtureStageResult(invocation, "episode_analysis_candidate", candidate)
}

func episodeReconciliationFixtureResult(invocation agentcontract.StageInvocation) (agentcontract.StageResult, error) {
	var input agentcontract.EpisodeReconciliationStageInput
	if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
		return agentcontract.StageResult{}, err
	}
	fragments := make([]planningdomain.EpisodeStructureFragment, 0)
	claims := make([]planningdomain.EpisodeClaimCandidate, 0)
	for _, child := range input.Candidates {
		switch input.CandidateType {
		case "episode_analysis_candidate":
			var candidate planningdomain.EpisodeAnalysisCandidate
			if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
				return agentcontract.StageResult{}, err
			}
			fragments = append(fragments, candidate.Fragments...)
			claims = append(claims, candidate.Claims...)
		case "episode_reconciliation_candidate":
			var candidate planningdomain.EpisodeReconciliationCandidate
			if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
				return agentcontract.StageResult{}, err
			}
			fragments = append(fragments, candidate.OrderedFragments...)
			claims = append(claims, candidate.Claims...)
		default:
			return agentcontract.StageResult{}, errors.New("fixture Episode reconciliation received an invalid Candidate type")
		}
	}
	slices.SortFunc(fragments, func(left, right planningdomain.EpisodeStructureFragment) int {
		if left.SourceStart != right.SourceStart {
			return left.SourceStart - right.SourceStart
		}
		if left.SourceEnd != right.SourceEnd {
			return left.SourceEnd - right.SourceEnd
		}
		return strings.Compare(left.TemporaryKey, right.TemporaryKey)
	})
	slices.SortFunc(claims, func(left, right planningdomain.EpisodeClaimCandidate) int {
		return strings.Compare(left.ClaimKey, right.ClaimKey)
	})
	candidate, err := json.Marshal(planningdomain.EpisodeReconciliationCandidate{
		EpisodeID: input.EpisodeID, ScriptVersionID: input.ScriptVersionID,
		SourceStart: input.EpisodeSourceStart, SourceEnd: input.EpisodeSourceEnd,
		OrderedFragments: fragments, Claims: claims,
		Conflicts: []bibledomain.ReviewIssue{}, ReviewIssues: []bibledomain.ReviewIssue{},
	})
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	return episodeFixtureStageResult(invocation, "episode_reconciliation_candidate", candidate)
}

func episodeFixtureStageResult(
	invocation agentcontract.StageInvocation,
	candidateType string,
	candidate json.RawMessage,
) (agentcontract.StageResult, error) {
	resultHash, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return agentcontract.StageResult{}, err
	}
	return agentcontract.StageResult{
		InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
		WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
		Status: "succeeded", CandidateType: candidateType, Candidate: candidate,
		InputHash: invocation.InputHash, ResultHash: &resultHash, Issues: []agentcontract.StageIssue{},
		Executor: agentcontract.Executor{Name: "test-agent", Version: "episode-analysis-v1", Model: "deterministic-fixture"},
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
		semanticSuffix := strings.ReplaceAll(invocation.Payload.ShardKey, ".", "-")
		entityKey := "character:" + invocation.Payload.ShardKey
		peerEntityKey := "character:peer-" + invocation.Payload.ShardKey
		value, err := json.Marshal(bibledomain.StoryAnalysisCandidate{
			Entities: []bibledomain.StoryEntityCandidate{
				{
					EntityKey: entityKey, Kind: "character",
					CanonicalName: "分片角色", NormalizedName: "分片角色", Aliases: []string{},
					StableSpec: storyFixtureAssetSpec(), EpisodeNumbers: []int{},
					Evidence: evidence.Observations[0].Evidence,
					States:   []bibledomain.StoryEntityStateCandidate{}, Ambiguities: []string{},
				},
				{
					EntityKey: peerEntityKey, Kind: "character",
					CanonicalName: "分片搭档", NormalizedName: "分片搭档", Aliases: []string{},
					StableSpec: storyFixtureAssetSpec(), EpisodeNumbers: []int{},
					Evidence: evidence.Observations[0].Evidence,
					States:   []bibledomain.StoryEntityStateCandidate{}, Ambiguities: []string{},
				},
			},
			WorldEntries: []bibledomain.StoryWorldEntryCandidate{{
				EntryKey: "world:" + semanticSuffix, Category: "rule", Title: "分片世界规则",
				Facts: []string{"规则来自原稿"}, Rules: []string{"角色遵循规则"}, EntityKeys: []string{entityKey},
				EpisodeNumbers: []int{}, Evidence: evidence.Observations[0].Evidence, Ambiguities: []string{},
			}},
			Claims: []bibledomain.StoryClaimCandidate{{
				ClaimKey: "relationship:" + semanticSuffix, ClaimType: "relationship",
				ParticipantKeys: []string{entityKey, peerEntityKey}, AnchorKeys: []string{entityKey},
				Scope: "project", Polarity: "positive", Status: "proposed",
				Evidence: evidence.Observations[0].Evidence,
			}},
			Arcs: []bibledomain.StoryArcCandidate{{
				ArcKey: "arc:" + semanticSuffix, Title: "分片故事弧", Summary: "故事弧来自原稿",
				Evidence: evidence.Observations[0].Evidence,
			}},
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

func containsStoryGraphNodeType(nodes []storygraph.Node, nodeType storygraph.NodeType) bool {
	return slices.ContainsFunc(nodes, func(node storygraph.Node) bool { return node.NodeType == nodeType })
}
