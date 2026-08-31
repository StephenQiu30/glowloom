package workflow_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestSceneAnalysisWorkflowPersistsTwoStrictCandidatesAndReplays(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Scene Analysis workflow journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	rootDatabase := database
	t.Cleanup(func() { _ = platformdatabase.Close(rootDatabase) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	database = database.Begin()
	if database.Error != nil {
		t.Fatalf("begin isolated Scene Analysis journey: %v", database.Error)
	}
	t.Cleanup(func() { _ = database.Rollback().Error })
	now := time.Date(2026, time.August, 31, 8, 0, 0, 0, time.UTC)
	fixture := seedSceneAnalysisProject(t, func(value any) error { return database.Create(value).Error }, now)
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatal(err)
	}

	scriptStore := scriptgorm.New(database)
	sourceService := scriptapp.NewSourceService(scriptStore, scriptapp.SourceConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	actor := scriptapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	accepted, err := sourceService.Accept(ctx, actor, scriptapp.AcceptSourceCommand{
		ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(),
		ExpectedHeadRevision: 0, IdempotencyKey: "scene-analysis-source-" + fixture.projectID.String(),
	})
	if err != nil {
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
		IdempotencyKey: "scene-analysis-authoring-" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("create Scene Analysis authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "scene-analysis-publish-" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("publish Scene Analysis authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	starter := &immediateSceneAnalysisStarter{}
	startService := workflowapp.NewStartService(compiler, workflowStore, starter, workflowapp.StartConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	started, err := startService.Start(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "scene-analysis-run-" + fixture.projectID.String(),
	})
	if err != nil {
		t.Fatalf("start Scene Analysis workflow: %v", err)
	}
	plan, err := workflowapp.NewRuntimeService(workflowStore).LoadExecutionPlan(ctx, starter.request)
	if err != nil {
		t.Fatalf("load Scene Analysis plan: %v", err)
	}
	if len(plan.Nodes) != 3 || plan.Nodes[0].Executor != "workflow.input.script_source" ||
		plan.Nodes[1].Executor != "activity.script_span_proposal" ||
		plan.Nodes[2].Executor != "activity.scene_fact_extraction" {
		t.Fatalf("Scene Analysis plan = %#v", plan.Nodes)
	}

	agentRuntime := &deterministicSceneAnalysisRuntime{now: now}
	dispatchSigner, err := agentgrant.NewSigner(
		"scene-analysis-persistence-test-secret-value",
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	sceneService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), agentRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
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
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString, Executor: nodeExecutor,
	})
	var final workflow.NodeActivityResult
	for _, node := range plan.Nodes {
		final, err = runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
			WorkflowRunID: started.ID, NodeRunID: node.NodeRunID, NodeID: node.NodeID,
			Executor: node.Executor, Attempt: 1,
		})
		if err != nil || final.Status != "SUCCEEDED" {
			t.Fatalf("execute %s: result=%#v err=%v", node.Executor, final, err)
		}
	}
	if len(final.Output.Bindings) != 1 || final.Output.Bindings[0].ValueType != "scene_fact_candidate" {
		t.Fatalf("SceneFact output = %#v", final.Output)
	}
	candidate, err := sceneService.GetCandidate(ctx, fixture.projectID.String(), final.Output.Bindings[0].ReferenceID)
	if err != nil || candidate.CandidateRevisionHash != final.Output.Bindings[0].ContentHash ||
		contract.ValidateSceneFactCandidate(candidate.Candidate, fixture.text, agentRuntime.spanCandidate) != nil {
		t.Fatalf("query persisted SceneFact Candidate: candidate=%#v err=%v", candidate, err)
	}
	if candidate.SourceResultHash == candidate.CandidateContentHash {
		t.Fatal("Candidate lineage reused output_hash as source_result_hash")
	}
	var acceptedResult model.SceneAnalysisResult
	if err = database.First(&acceptedResult, "id = ?", candidate.SourceResultID).Error; err != nil {
		t.Fatalf("query accepted Scene Analysis Result: %v", err)
	}
	var acceptedResultContract contract.SceneAnalysisAttemptResult
	if err = json.Unmarshal(acceptedResult.Result, &acceptedResultContract); err != nil ||
		acceptedResultContract.ResultHash != candidate.SourceResultHash {
		t.Fatalf(
			"Candidate source Result Hash drifted: candidate=%s result=%s err=%v",
			candidate.SourceResultHash, acceptedResultContract.ResultHash, err,
		)
	}
	replayed, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[2].NodeRunID, NodeID: plan.Nodes[2].NodeID,
		Executor: plan.Nodes[2].Executor, Attempt: 2,
	})
	if err != nil || replayed.OutputHash != final.OutputHash || agentRuntime.calls != 2 {
		t.Fatalf("replay SceneFact node: calls=%d result=%#v err=%v", agentRuntime.calls, replayed, err)
	}
	dispatchFailureNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: dispatchFailureNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: uuid.MustParse(started.ID),
		NodeID: "span-dispatch-authorization-failure", DefinitionKey: "agent.script_span_proposal",
		DefinitionVersion: "1.0.0", Executor: "activity.script_span_proposal",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create dispatch authorization failure NodeRun: %v", err)
	}
	dispatchFailureRuntime := &deterministicSceneAnalysisRuntime{now: now}
	dispatchFailureAuthorizer := &rejectingSceneAnalysisDispatchAuthorizer{
		observeAttempt: func(attemptID string) bool {
			var count int64
			return database.Model(&model.SceneAnalysisAttempt{}).
				Where("id = ?", attemptID).Count(&count).Error == nil && count == 1
		},
	}
	dispatchFailureService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), dispatchFailureRuntime,
		dispatchFailureAuthorizer,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dispatchFailureService.Execute(ctx, agentapp.ExecuteCommand{
		WorkflowRunID: started.ID, NodeRunID: dispatchFailureNodeRunID.String(),
		StageKey: "propose_script_spans",
		Source: agentapp.SourceInput{
			WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
			OwnerKind: accepted.Identity.OwnerKind, LogicalID: accepted.Identity.LogicalID,
			VersionID: accepted.Identity.VersionID, Revision: accepted.Identity.Revision,
			ContentHash: accepted.Identity.ContentHash, CreatedAt: accepted.Identity.CreatedAt,
			NormalizedText: fixture.text, NewlineNormalization: accepted.NewlineNormalization,
			CodepointIndexRule: accepted.CodepointIndexRule,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "dispatch authorization signing unavailable") ||
		dispatchFailureAuthorizer.calls != 1 || !dispatchFailureAuthorizer.sawAttempt ||
		dispatchFailureRuntime.calls != 0 {
		t.Fatalf(
			"dispatch authorization failure: err=%v authorizer=%#v runtime_calls=%d",
			err, dispatchFailureAuthorizer, dispatchFailureRuntime.calls,
		)
	}
	var rolledBackInvocations int64
	if err = database.Model(&model.SceneAnalysisInvocationRecord{}).
		Where("node_run_id = ?", dispatchFailureNodeRunID).Count(&rolledBackInvocations).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBackInvocations != 0 {
		t.Fatalf("dispatch authorization failure left %d invocation facts", rolledBackInvocations)
	}
	parsedWorkflowRunID, err := uuid.Parse(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	unknownOutcomeNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: unknownOutcomeNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: parsedWorkflowRunID,
		NodeID: "span-outcome-retry", DefinitionKey: "agent.script_span_proposal",
		DefinitionVersion: "1.0.0", Executor: "activity.script_span_proposal",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create outcome-unknown NodeRun: %v", err)
	}
	unknownOutcomeRuntime := &failOnceSceneAnalysisRuntime{
		delegate: deterministicSceneAnalysisRuntime{now: now},
	}
	unknownOutcomeService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), unknownOutcomeRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	unknownOutcomeCommand := agentapp.ExecuteCommand{
		WorkflowRunID: started.ID, NodeRunID: unknownOutcomeNodeRunID.String(), StageKey: "propose_script_spans",
		Source: agentapp.SourceInput{
			WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
			OwnerKind: accepted.Identity.OwnerKind, LogicalID: accepted.Identity.LogicalID,
			VersionID: accepted.Identity.VersionID, Revision: accepted.Identity.Revision,
			ContentHash: accepted.Identity.ContentHash, CreatedAt: accepted.Identity.CreatedAt,
			NormalizedText: fixture.text, NewlineNormalization: accepted.NewlineNormalization,
			CodepointIndexRule: accepted.CodepointIndexRule,
		},
	}
	if _, err = unknownOutcomeService.Execute(ctx, unknownOutcomeCommand); agentapp.ErrorCode(err) != "agent_outcome_unknown" {
		t.Fatalf("first execution error = %v", err)
	}
	var unknownOutcomeInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where("node_run_id = ?", unknownOutcomeNodeRunID).First(&unknownOutcomeInvocation).Error; err != nil {
		t.Fatalf("query outcome-unknown invocation: %v", err)
	}
	if unknownOutcomeInvocation.Status != "outcome_unknown" {
		t.Fatalf("invocation status = %q", unknownOutcomeInvocation.Status)
	}
	var unknownOutcomeAttempt model.SceneAnalysisAttempt
	if err = database.Where("invocation_id = ?", unknownOutcomeInvocation.ID).
		Order("claim_version ASC").First(&unknownOutcomeAttempt).Error; err != nil {
		t.Fatalf("query first attempt: %v", err)
	}
	if unknownOutcomeAttempt.Status != "completed" || unknownOutcomeAttempt.CompletedAt == nil {
		t.Fatalf("first attempt = %#v", unknownOutcomeAttempt)
	}
	var unknownOutcomeResult model.SceneAnalysisResult
	if err = database.Where("attempt_id = ?", unknownOutcomeAttempt.ID).First(&unknownOutcomeResult).Error; err != nil {
		t.Fatalf("query first result: %v", err)
	}
	if unknownOutcomeResult.Status != "outcome_unknown" ||
		strings.Contains(string(unknownOutcomeResult.Result), "sensitive transport detail") {
		t.Fatalf("persisted safe result = %s", unknownOutcomeResult.Result)
	}
	var unknownAuthorization model.SceneAnalysisDispatchAuthorization
	if err = database.First(&unknownAuthorization, "attempt_id = ?", unknownOutcomeAttempt.ID).Error; err != nil {
		t.Fatalf("query dispatch authorization: %v", err)
	}
	var persistedUnknown contract.SceneAnalysisAttemptResult
	if err = json.Unmarshal(unknownOutcomeResult.Result, &persistedUnknown); err != nil {
		t.Fatalf("decode persisted outcome-unknown result: %v", err)
	}
	if persistedUnknown.ClaimVersion != unknownOutcomeAttempt.ClaimVersion ||
		persistedUnknown.DispatchAuthorizationHash != unknownAuthorization.AuthorizationHash ||
		!unknownAuthorization.ExpiresAt.After(unknownAuthorization.IssuedAt) {
		t.Fatalf(
			"dispatch evidence drifted: result=%#v attempt=%#v authorization=%#v",
			persistedUnknown,
			unknownOutcomeAttempt,
			unknownAuthorization,
		)
	}
	retryCandidate, err := unknownOutcomeService.Execute(ctx, unknownOutcomeCommand)
	if err != nil || retryCandidate.CandidateType != "script_span_candidate" {
		t.Fatalf("retry candidate=%#v err=%v", retryCandidate, err)
	}
	var retryAttemptCount, retryResultCount int64
	if err = database.Model(&model.SceneAnalysisAttempt{}).
		Where("invocation_id = ?", unknownOutcomeInvocation.ID).Count(&retryAttemptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.SceneAnalysisResult{}).
		Joins("JOIN agt_scene_analysis_attempts ON agt_scene_analysis_attempts.id = agt_scene_analysis_results.attempt_id").
		Where("agt_scene_analysis_attempts.invocation_id = ?", unknownOutcomeInvocation.ID).Count(&retryResultCount).Error; err != nil {
		t.Fatal(err)
	}
	if retryAttemptCount != 2 || retryResultCount != 2 || unknownOutcomeRuntime.calls != 2 {
		t.Fatalf(
			"retry persistence: attempts=%d results=%d calls=%d",
			retryAttemptCount, retryResultCount, unknownOutcomeRuntime.calls,
		)
	}

	readSetDriftNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: readSetDriftNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: parsedWorkflowRunID,
		NodeID: "span-read-set-drift", DefinitionKey: "agent.script_span_proposal",
		DefinitionVersion: "1.0.0", Executor: "activity.script_span_proposal",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create read-set drift NodeRun: %v", err)
	}
	readSetDriftRuntime := &readSetDriftSceneAnalysisRuntime{
		delegate: deterministicSceneAnalysisRuntime{now: now},
		beforeResult: func() error {
			return database.Model(&model.DocumentRevision{}).Where("id = ?", fixture.revisionID).
				Update("normalized_hash", strings.Repeat("f", 64)).Error
		},
	}
	readSetDriftService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), readSetDriftRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	readSetDriftCommand := unknownOutcomeCommand
	readSetDriftCommand.NodeRunID = readSetDriftNodeRunID.String()
	_, readSetDriftErr := readSetDriftService.Execute(ctx, readSetDriftCommand)
	if err = database.Model(&model.DocumentRevision{}).Where("id = ?", fixture.revisionID).
		Update("normalized_hash", fixture.sourceHash).Error; err != nil {
		t.Fatalf("restore Source read-set fixture: %v", err)
	}
	if agentapp.ErrorCode(readSetDriftErr) != "stale_read_set" {
		t.Fatalf("read-set drift execution error = %v", readSetDriftErr)
	}
	var driftedCandidateCount int64
	if err = database.Model(&model.SceneAnalysisCandidateRevision{}).
		Where("source_invocation_id IN (?)", database.Model(&model.SceneAnalysisInvocationRecord{}).
			Select("id").Where("node_run_id = ?", readSetDriftNodeRunID)).
		Count(&driftedCandidateCount).Error; err != nil {
		t.Fatal(err)
	}
	if driftedCandidateCount != 0 {
		t.Fatalf("read-set drift published %d Candidate revisions", driftedCandidateCount)
	}

	var sceneFactInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&sceneFactInvocation, "id = ?", candidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query Scene Fact invocation: %v", err)
	}
	if sceneFactInvocation.UpstreamCandidateRevisionID == nil {
		t.Fatal("Scene Fact invocation has no upstream Script Span Candidate")
	}
	spanCandidate, err := sceneService.GetCandidate(
		ctx,
		fixture.projectID.String(),
		sceneFactInvocation.UpstreamCandidateRevisionID.String(),
	)
	if err != nil {
		t.Fatalf("query upstream Script Span Candidate: %v", err)
	}
	upstreamReadSetDriftNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: upstreamReadSetDriftNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: parsedWorkflowRunID,
		NodeID: "fact-upstream-read-set-drift", DefinitionKey: "agent.scene_fact_extraction",
		DefinitionVersion: "1.0.0", Executor: "activity.scene_fact_extraction",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create upstream read-set drift NodeRun: %v", err)
	}
	upstreamReadSetDriftRuntime := &readSetDriftSceneAnalysisRuntime{
		delegate: deterministicSceneAnalysisRuntime{now: now},
		beforeResult: func() error {
			return database.Model(&model.SceneAnalysisInvocationRecord{}).
				Where("id = ?", spanCandidate.SourceInvocationID).
				Update("shard_key", "script:drifted").Error
		},
	}
	upstreamReadSetDriftService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), upstreamReadSetDriftRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	upstreamReadSetDriftCommand := unknownOutcomeCommand
	upstreamReadSetDriftCommand.NodeRunID = upstreamReadSetDriftNodeRunID.String()
	upstreamReadSetDriftCommand.StageKey = "extract_scene_facts"
	upstreamReadSetDriftCommand.Upstream = &spanCandidate
	_, upstreamReadSetDriftErr := upstreamReadSetDriftService.Execute(ctx, upstreamReadSetDriftCommand)
	if err = database.Model(&model.SceneAnalysisInvocationRecord{}).
		Where("id = ?", spanCandidate.SourceInvocationID).
		Update("shard_key", "script:full").Error; err != nil {
		t.Fatalf("restore upstream read-set fixture: %v", err)
	}
	if agentapp.ErrorCode(upstreamReadSetDriftErr) != "stale_read_set" {
		t.Fatalf("upstream read-set drift execution error = %v", upstreamReadSetDriftErr)
	}
	var upstreamDriftedCandidateCount int64
	if err = database.Model(&model.SceneAnalysisCandidateRevision{}).
		Where("source_invocation_id IN (?)", database.Model(&model.SceneAnalysisInvocationRecord{}).
			Select("id").Where("node_run_id = ?", upstreamReadSetDriftNodeRunID)).
		Count(&upstreamDriftedCandidateCount).Error; err != nil {
		t.Fatal(err)
	}
	if upstreamDriftedCandidateCount != 0 {
		t.Fatalf("upstream read-set drift published %d Candidate revisions", upstreamDriftedCandidateCount)
	}

	bundleUnavailableNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: bundleUnavailableNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: parsedWorkflowRunID,
		NodeID: "span-bundle-unavailable", DefinitionKey: "agent.script_span_proposal",
		DefinitionVersion: "1.0.0", Executor: "activity.script_span_proposal",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create bundle-unavailable NodeRun: %v", err)
	}
	bundleUnavailableRuntime := &bundleUnavailableSceneAnalysisRuntime{}
	bundleUnavailableService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), bundleUnavailableRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	bundleUnavailableCommand := unknownOutcomeCommand
	bundleUnavailableCommand.NodeRunID = bundleUnavailableNodeRunID.String()
	if _, err = bundleUnavailableService.Execute(ctx, bundleUnavailableCommand); agentapp.ErrorCode(err) != "skill_bundle_unavailable" {
		t.Fatalf("bundle-unavailable execution error = %v", err)
	}
	var bundleUnavailableInvocation model.SceneAnalysisInvocationRecord
	if err = database.Where("node_run_id = ?", bundleUnavailableNodeRunID).
		First(&bundleUnavailableInvocation).Error; err != nil {
		t.Fatalf("query bundle-unavailable invocation: %v", err)
	}
	var bundleUnavailableResult model.SceneAnalysisResult
	if err = database.Joins(
		"JOIN agt_scene_analysis_attempts ON agt_scene_analysis_attempts.id = agt_scene_analysis_results.attempt_id",
	).Where("agt_scene_analysis_attempts.invocation_id = ?", bundleUnavailableInvocation.ID).
		First(&bundleUnavailableResult).Error; err != nil {
		t.Fatalf("query bundle-unavailable result: %v", err)
	}
	var persistedBundleUnavailable contract.SceneAnalysisAttemptResult
	if err = json.Unmarshal(bundleUnavailableResult.Result, &persistedBundleUnavailable); err != nil ||
		persistedBundleUnavailable.Status != "outcome_unknown" || persistedBundleUnavailable.Error == nil ||
		persistedBundleUnavailable.Error.Code != "skill_bundle_unavailable" ||
		persistedBundleUnavailable.Error.RetryClass != "same_release" || bundleUnavailableRuntime.calls != 1 {
		t.Fatalf(
			"typed bundle-unavailable result: result=%#v calls=%d err=%v",
			persistedBundleUnavailable,
			bundleUnavailableRuntime.calls,
			err,
		)
	}

	for _, assertion := range []struct {
		value any
		want  int64
	}{
		{&model.SceneAnalysisRelease{}, 2}, {&model.SceneAnalysisControlHead{}, 2},
		{&model.SceneAnalysisInvocationRecord{}, 2}, {&model.SceneAnalysisAttempt{}, 2},
		{&model.SceneAnalysisDispatchAuthorization{}, 2},
		{&model.SceneAnalysisResult{}, 2}, {&model.SceneAnalysisCandidateRevision{}, 2},
		{&model.SceneAnalysisCandidateHead{}, 2},
	} {
		var count int64
		query := database.Model(assertion.value)
		switch assertion.value.(type) {
		case *model.SceneAnalysisRelease, *model.SceneAnalysisControlHead,
			*model.SceneAnalysisAttempt, *model.SceneAnalysisDispatchAuthorization,
			*model.SceneAnalysisResult:
		default:
			query = query.Where("project_id = ?", fixture.projectID)
		}
		if err = query.Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count < assertion.want {
			t.Fatalf("%T count=%d want at least %d", assertion.value, count, assertion.want)
		}
	}
	if accepted.Identity.VersionID != fixture.revisionID.String() {
		t.Fatalf("accepted Source identity = %#v", accepted.Identity)
	}
}

type sceneAnalysisFixture struct {
	userID, workspaceID, projectID, documentID, revisionID uuid.UUID
	text, sourceHash                                       string
}

func seedSceneAnalysisProject(t *testing.T, create func(any) error, now time.Time) sceneAnalysisFixture {
	t.Helper()
	fixture := sceneAnalysisFixture{
		userID: uuid.New(), workspaceID: uuid.New(), projectID: uuid.New(), documentID: uuid.New(), revisionID: uuid.New(),
		text: "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。",
	}
	fixture.sourceHash = sceneTextHash(fixture.text)
	records := []any{
		&model.UserAccount{ID: fixture.userID, EmailNormalized: fixture.userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Scene Analysis", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: fixture.workspaceID, Name: "Scene Analysis", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: fixture.userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: fixture.projectID, WorkspaceID: fixture.workspaceID, Name: "Scene Analysis", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: fixture.documentID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, Title: "多场剧本", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: fixture.revisionID, WorkspaceID: fixture.workspaceID, DocumentID: fixture.documentID, VersionNo: 1, SourceType: "text", RawText: fixture.text, RawHash: fixture.sourceHash, NormalizedText: fixture.text, NormalizedHash: fixture.sourceHash, NormalizerVersion: "line-ending-lf", NormalizationMap: []byte(`{"newline":"lf"}`), CodepointCount: utf8.RuneCountInString(fixture.text), AnalysisStatus: "deterministic", AnalyzerVersion: "scene-analysis-test", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: fixture.userID, CreatedAt: now},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return fixture
}

func sceneAnalysisGraph(revisionID string) authoring.Graph {
	return authoring.Graph{
		Nodes: []authoring.Node{
			{ID: "source", DefinitionKey: "input.script_source", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"` + revisionID + `"}`)},
			{ID: "spans", DefinitionKey: "agent.script_span_proposal", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "facts", DefinitionKey: "agent.scene_fact_extraction", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
		},
		Edges: []authoring.Edge{
			{ID: "source-spans", FromNodeID: "source", FromPort: "source", ToNodeID: "spans", ToPort: "source"},
			{ID: "source-facts", FromNodeID: "source", FromPort: "source", ToNodeID: "facts", ToPort: "source"},
			{ID: "spans-facts", FromNodeID: "spans", FromPort: "candidate", ToNodeID: "facts", ToPort: "spans"},
		},
	}
}

type immediateSceneAnalysisStarter struct {
	request workflow.StartRequest
}

func (starter *immediateSceneAnalysisStarter) Start(_ context.Context, request workflow.StartRequest) (workflow.StartObservation, error) {
	starter.request = request
	return workflow.StartObservation{Outcome: workflow.StartOutcomeStarted, ObservedInputHash: request.InputHash}, nil
}

type deterministicSceneAnalysisRuntime struct {
	now           time.Time
	calls         int
	spanCandidate json.RawMessage
}

type failOnceSceneAnalysisRuntime struct {
	calls    int
	delegate deterministicSceneAnalysisRuntime
}

type bundleUnavailableSceneAnalysisRuntime struct{ calls int }

type readSetDriftSceneAnalysisRuntime struct {
	delegate     deterministicSceneAnalysisRuntime
	beforeResult func() error
}

type rejectingSceneAnalysisDispatchAuthorizer struct {
	calls          int
	sawAttempt     bool
	observeAttempt func(string) bool
}

func (authorizer *rejectingSceneAnalysisDispatchAuthorizer) IssueSceneAnalysisDispatchAuthorization(
	invocation contract.SceneAnalysisInvocation,
	_ int64,
) (contract.SceneAnalysisDispatchAuthorization, error) {
	authorizer.calls++
	authorizer.sawAttempt = authorizer.observeAttempt != nil && authorizer.observeAttempt(invocation.AttemptID)
	return contract.SceneAnalysisDispatchAuthorization{}, errors.New("dispatch authorization signing unavailable")
}

func (runtime *failOnceSceneAnalysisRuntime) InvokeSceneAnalysis(
	ctx context.Context,
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.SceneAnalysisAttemptResult, error) {
	runtime.calls++
	if runtime.calls == 1 {
		return contract.SceneAnalysisAttemptResult{}, errors.New("sensitive transport detail")
	}
	return runtime.delegate.InvokeSceneAnalysis(ctx, invocation, authorization)
}

func (runtime *bundleUnavailableSceneAnalysisRuntime) InvokeSceneAnalysis(
	context.Context,
	contract.SceneAnalysisInvocation,
	contract.SceneAnalysisDispatchAuthorization,
) (contract.SceneAnalysisAttemptResult, error) {
	runtime.calls++
	return contract.SceneAnalysisAttemptResult{}, contract.ErrSkillBundleUnavailable
}

func (runtime *readSetDriftSceneAnalysisRuntime) InvokeSceneAnalysis(
	ctx context.Context,
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.SceneAnalysisAttemptResult, error) {
	result, err := runtime.delegate.InvokeSceneAnalysis(ctx, invocation, authorization)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	if runtime.beforeResult != nil {
		if err = runtime.beforeResult(); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
	}
	return result, nil
}

func (runtime *deterministicSceneAnalysisRuntime) InvokeSceneAnalysis(
	_ context.Context,
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.SceneAnalysisAttemptResult, error) {
	runtime.calls++
	var candidate json.RawMessage
	if invocation.Payload.Variant.StageKey == "propose_script_spans" {
		var input contract.ScriptSpanProposalInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildSpanCandidate(input)
		runtime.spanCandidate = append([]byte(nil), candidate...)
	} else {
		var input contract.SceneFactExtractionInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildSceneFactCandidate(input)
	}
	outputHash, err := contract.ProductionCanonicalHash(candidate)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(json.RawMessage(`[]`))
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	result := contract.SceneAnalysisAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID, Kind: "storygraph_stage",
		WireSchemaVersion: invocation.WireSchemaVersion, Variant: invocation.Payload.Variant,
		StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status:        "accepted",
		CandidateType: map[string]string{"propose_script_spans": "script_span_candidate", "extract_scene_facts": "scene_fact_candidate"}[invocation.Payload.Variant.StageKey],
		Candidate:     candidate, InputHash: invocation.InputHash, OutputHash: &outputHash,
		Diagnostics: []contract.SceneAnalysisDiagnostic{}, DiagnosticHash: diagnosticHash, CompletedAt: runtime.now,
		Executor: contract.SceneAnalysisExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "scene-analysis-harness", Model: "deterministic-contract-runtime",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}

func buildSpanCandidate(input contract.ScriptSpanProposalInput) json.RawMessage {
	text := []rune(input.NormalizedText)
	second := runeIndex(input.NormalizedText, "第二场")
	return mustSceneJSON(map[string]any{
		"source_version_id": input.SourceVersionID, "source_hash": input.SourceHash,
		"codepoint_count": len(text),
		"coverage":        map[string]any{"source_hash": input.SourceHash, "codepoint_start": 0, "codepoint_end": len(text), "covered_codepoints": len(text)},
		"spans": []any{
			map[string]any{"temporary_span_id": "span_0001", "kind": "scene", "codepoint_start": 0, "codepoint_end": second, "heading": "第一场 夜 内", "evidence": sceneEvidence(input.NormalizedText, 0, runeIndex(input.NormalizedText, "\n"))},
			map[string]any{"temporary_span_id": "span_0002", "kind": "scene", "codepoint_start": second, "codepoint_end": len(text), "heading": "第二场 日 外", "evidence": sceneEvidence(input.NormalizedText, second, second+runeIndex(string(text[second:]), "\n"))},
		},
		"review_issues": []any{},
	})
}

func buildSceneFactCandidate(input contract.SceneFactExtractionInput) json.RawMessage {
	var spans contract.ScriptSpanCandidate
	_ = json.Unmarshal(input.SpanCandidate, &spans)
	scenes := make([]any, len(spans.Spans))
	for index, span := range spans.Spans {
		timeText, locationText := "夜", "内"
		if index == 1 {
			timeText, locationText = "日", "外"
		}
		timeStart := runeIndexFrom(input.NormalizedText, timeText, span.CodepointStart)
		locationStart := runeIndexFrom(input.NormalizedText, locationText, timeStart+1)
		nameStart := runeIndexFrom(input.NormalizedText, "林舟", span.CodepointStart)
		scenes[index] = map[string]any{
			"temporary_scene_id": fmt.Sprintf("scene_%04d", index+1), "span_id": span.TemporarySpanID,
			"source_start": span.CodepointStart, "source_end": span.CodepointEnd,
			"location": map[string]any{"text": locationText, "evidence": sceneEvidence(input.NormalizedText, locationStart, locationStart+1)},
			"time":     map[string]any{"text": timeText, "evidence": sceneEvidence(input.NormalizedText, timeStart, timeStart+1)},
			"actions":  []any{}, "dialogues": []any{},
			"raw_character_mentions": []any{map[string]any{"text": "林舟", "evidence": sceneEvidence(input.NormalizedText, nameStart, nameStart+2)}},
			"raw_prop_mentions":      []any{},
		}
	}
	return mustSceneJSON(map[string]any{
		"source_version_id": input.SourceVersionID, "source_hash": input.SourceHash,
		"span_candidate_revision_id":   input.SpanCandidateRevisionID,
		"span_candidate_revision_hash": input.SpanCandidateRevisionHash,
		"scenes":                       scenes, "review_issues": []any{},
	})
}

func sceneEvidence(text string, start, end int) map[string]any {
	anchor := string([]rune(text)[start:end])
	return map[string]any{"source_start": start, "source_end": end, "text_hash": sceneTextHash(anchor), "exact_anchor": anchor}
}

func runeIndex(text, substring string) int { return runeIndexFrom(text, substring, 0) }

func runeIndexFrom(text, substring string, offset int) int {
	runes := []rune(text)
	target := []rune(substring)
	for index := offset; index+len(target) <= len(runes); index++ {
		if string(runes[index:index+len(target)]) == substring {
			return index
		}
	}
	return -1
}

func sceneTextHash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func mustSceneJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
