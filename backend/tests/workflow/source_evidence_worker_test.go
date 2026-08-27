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
			},
			Edges: []authoring.Edge{
				{ID: "script-to-evidence", FromNodeID: "script", FromPort: "script", ToNodeID: "evidence", ToPort: "script"},
				{ID: "evidence-to-story", FromNodeID: "evidence", FromPort: "evidence", ToNodeID: "story", ToPort: "evidence"},
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
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		evidenceService,
		storyAnalysisService,
		bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString),
		planningapp.NewService(planninggorm.New(database), planningapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		storyboardapp.NewService(storyboardgorm.New(database), storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil, nil,
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
	bibleWorker := bibleapp.NewWorker(
		bibleStore, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, agentLogger,
	)
	go agentWorker.Run(agentContext)
	go agentWorker.Run(agentContext)
	go storyWorker.Run(agentContext)
	go storyWorker.Run(agentContext)
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

	deadline := time.Now().Add(25 * time.Second)
	var persistedRun model.WorkflowRun
	for {
		if err = database.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
			t.Fatalf("load Source Evidence Workflow Run: %v", err)
		}
		if persistedRun.Status == "SUCCEEDED" {
			break
		}
		if persistedRun.Status == "FAILED" || persistedRun.Status == "CANCELLED" || time.Now().After(deadline) {
			t.Fatalf("Source Evidence Workflow did not complete: %#v", persistedRun)
		}
		time.Sleep(50 * time.Millisecond)
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
	beforeReplay := len(storyInvocations)
	replayed, err := storyAnalysisService.Ensure(ctx, bibleapp.StoryAnalysisCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		WorkflowRunID: run.ID, NodeRunID: storyNode.ID.String(),
		EvidenceCandidateRevisionID:   output.Bindings[0].ReferenceID,
		EvidenceCandidateRevisionHash: output.Bindings[0].ContentHash,
	})
	if err != nil || replayed.Status != "ready" || replayed.CandidateRevisionID != storyOutput.Bindings[0].ReferenceID {
		t.Fatalf("Story analysis replay drifted: state=%#v err=%v", replayed, err)
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

type recoveringSourceEvidenceAgent struct {
	mutex                   sync.Mutex
	failed                  map[string]bool
	budget                  bool
	storyMapBudget          bool
	storyReduceBudget       bool
	storyDeadline           bool
	storyDeadlineID         string
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
