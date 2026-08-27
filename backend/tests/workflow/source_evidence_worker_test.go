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

func TestSourceEvidenceWorkflowIsDefinitionFirstAndRecoversTheSameShardIdentity(t *testing.T) {
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
			},
			Edges: []authoring.Edge{{ID: "script-to-evidence", FromNodeID: "script", FromPort: "script", ToNodeID: "evidence", ToPort: "script"}},
		},
		Layout: json.RawMessage(`{"guided":{"step":2}}`), FrozenInputs: []authoring.FrozenReference{{
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
	activities, err := bootstrap.NewWorkflowRuntime(
		workflowStore,
		scriptapp.NewService(scriptgorm.New(database), nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		evidenceService,
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
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(lateRelease) }) })
	agent := &recoveringSourceEvidenceAgent{failed: map[string]bool{}, lateRelease: lateRelease}
	agentWorker := bibleapp.NewSourceEvidenceWorker(
		bibleStore, evidenceService, agent, func() time.Time { return time.Now().UTC() },
		time.Millisecond, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	go agentWorker.Run(agentContext)
	go agentWorker.Run(agentContext)
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
	if invocationRevisionCount != int64(len(invocations)+1) || aggregateRevisionCount != 1 {
		t.Fatalf("Source Evidence revisions: invocation=%d aggregate=%d", invocationRevisionCount, aggregateRevisionCount)
	}
	output, _, outputHash, err := workflow.ParseNodeOutput(json.RawMessage(evidenceNode.Output))
	if err != nil || outputHash != *evidenceNode.OutputHash || len(output.Bindings) != 1 ||
		output.Bindings[0].Port != "evidence" || output.Bindings[0].ValueType != "source_evidence_candidate" {
		t.Fatalf("Source Evidence Node output=%#v hash=%s err=%v", output, outputHash, err)
	}
}

type recoveringSourceEvidenceAgent struct {
	mutex         sync.Mutex
	failed        map[string]bool
	budget        bool
	originalCalls int
	lateRelease   <-chan struct{}
}

func (agent *recoveringSourceEvidenceAgent) Invoke(
	_ context.Context,
	invocation agentcontract.StageInvocation,
	_ int,
	_ int64,
) (agentcontract.StageResult, error) {
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
