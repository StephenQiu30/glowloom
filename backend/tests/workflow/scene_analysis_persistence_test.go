package workflow_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	eventingdomain "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
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
	worldgorm "github.com/StephenQiu30/lanverse/backend/internal/production/world/adapter/gormdb"
	worldapp "github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestSceneAnalysisWorkflowPersistsStructureIdentityReviewAndReplays(t *testing.T) {
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
	if len(plan.Nodes) != 11 || plan.Nodes[0].Executor != "workflow.input.script_source" ||
		plan.Nodes[1].Executor != "activity.script_span_proposal" ||
		plan.Nodes[2].Executor != "activity.scene_fact_extraction" ||
		plan.Nodes[3].Executor != "activity.identity_resolution" ||
		plan.Nodes[4].Executor != "activity.structure_identity_review" ||
		plan.Nodes[5].Executor != "gate.structure_identity_review" ||
		plan.Nodes[6].Executor != "activity.production_entity_derivation" ||
		plan.Nodes[7].Executor != "activity.scene_occurrence_binding" ||
		plan.Nodes[8].Executor != "activity.interaction_continuity_reconciliation" ||
		plan.Nodes[9].Executor != "activity.production_world_assembly" ||
		plan.Nodes[10].Executor != "gate.production_world_review" {
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
	nodeExecutor := workflowproduction.NewNodeExecutor(
		scriptapp.NewService(scriptStore, nil, scriptapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{
			Sources: sourceService, Candidates: sceneService, StructureIdentities: structureIdentityQuery,
			ProductionWorld: productionWorldService,
		},
	)
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 15 * time.Minute,
	})
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return now }, NewID: uuid.NewString, Executor: nodeExecutor,
		HumanTasks: workflowreview.New(reviewService),
	})
	var final workflow.NodeActivityResult
	var spanOutput workflow.NodeActivityResult
	var factOutput workflow.NodeActivityResult
	var identityOutput workflow.NodeActivityResult
	for _, node := range plan.Nodes[:5] {
		final, err = runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
			WorkflowRunID: started.ID, NodeRunID: node.NodeRunID, NodeID: node.NodeID,
			Executor: node.Executor, Attempt: 1,
		})
		if err != nil || final.Status != "SUCCEEDED" {
			t.Fatalf("execute %s: calls=%d result=%#v err=%v", node.Executor, agentRuntime.calls, final, err)
		}
		switch node.Executor {
		case "activity.script_span_proposal":
			spanOutput = final
		case "activity.scene_fact_extraction":
			factOutput = final
		case "activity.identity_resolution":
			identityOutput = final
		}
	}
	if len(final.Output.Bindings) != 1 || final.Output.Bindings[0].ValueType != "structure_identity_review_candidate" {
		t.Fatalf("StructureIdentityReview output = %#v", final.Output)
	}
	candidate, err := sceneService.GetCandidate(ctx, fixture.projectID.String(), final.Output.Bindings[0].ReferenceID)
	if err != nil || candidate.CandidateRevisionHash != final.Output.Bindings[0].ContentHash {
		t.Fatalf("query persisted StructureIdentityReview Candidate: candidate=%#v err=%v", candidate, err)
	}
	var reviewInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&reviewInvocation, "id = ?", candidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query StructureIdentityReview invocation: %v", err)
	}
	var reviewPayload contract.SceneAnalysisPayload
	var reviewInput contract.StructureIdentityReviewInput
	if err = json.Unmarshal(reviewInvocation.Payload, &reviewPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(reviewPayload.StageInput, &reviewInput); err != nil ||
		contract.ValidateStructureIdentityReviewCandidate(candidate.Candidate, reviewInput) != nil {
		t.Fatalf("validate persisted StructureIdentityReview Candidate: %v", err)
	}
	var reviewReads []model.SceneAnalysisInvocationRead
	if err = database.Where("invocation_id = ?", reviewInvocation.ID).
		Order("position ASC").Find(&reviewReads).Error; err != nil || len(reviewReads) != 3 {
		t.Fatalf("query StructureIdentityReview read set: reads=%#v err=%v", reviewReads, err)
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
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[4].NodeRunID, NodeID: plan.Nodes[4].NodeID,
		Executor: plan.Nodes[4].Executor, Attempt: 2,
	})
	if err != nil || replayed.OutputHash != final.OutputHash || agentRuntime.calls != 4 {
		t.Fatalf("replay StructureIdentityReview node: calls=%d result=%#v err=%v", agentRuntime.calls, replayed, err)
	}
	gate := plan.Nodes[5]
	gateCommand := workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: gate.NodeRunID, NodeID: gate.NodeID,
		Executor: gate.Executor, Attempt: 1,
	}
	if err = runtimeService.OpenHumanGate(ctx, gateCommand); err != nil {
		t.Fatalf("open StructureIdentity HumanTask: %v", err)
	}
	var gateInput model.WorkflowHumanGateInput
	if err = database.First(&gateInput, "node_run_id = ?", gate.NodeRunID).Error; err != nil {
		t.Fatalf("query StructureIdentity Gate input: %v", err)
	}
	decodedGateInput, _, decodeGateErr := workflow.DecodeStructureIdentityGateInput(json.RawMessage(gateInput.Input))
	if decodeGateErr != nil || decodedGateInput.InputHash != gateInput.InputHash ||
		decodedGateInput.Subject.SourceVersion.VersionID != fixture.revisionID.String() ||
		decodedGateInput.Subject.SpanCandidate.CandidateRevisionID != spanOutput.Output.Bindings[0].ReferenceID ||
		decodedGateInput.Subject.SceneFactCandidate.CandidateRevisionID != factOutput.Output.Bindings[0].ReferenceID ||
		decodedGateInput.Subject.IdentityCandidate.CandidateRevisionID != identityOutput.Output.Bindings[0].ReferenceID ||
		decodedGateInput.Subject.ReviewCandidate.CandidateRevisionID != final.Output.Bindings[0].ReferenceID {
		t.Fatalf("persisted StructureIdentity Gate input = %#v err=%v", decodedGateInput, decodeGateErr)
	}
	var humanTask model.HumanTask
	if err = database.First(&humanTask, "node_run_id = ?", gate.NodeRunID).Error; err != nil {
		t.Fatalf("query StructureIdentity HumanTask: %v", err)
	}
	var taskCandidateIDs []string
	expectedCandidateIDs := []string{
		spanOutput.Output.Bindings[0].ReferenceID, factOutput.Output.Bindings[0].ReferenceID,
		identityOutput.Output.Bindings[0].ReferenceID, final.Output.Bindings[0].ReferenceID,
	}
	slices.Sort(expectedCandidateIDs)
	if err = json.Unmarshal(humanTask.CandidateIDs, &taskCandidateIDs); err != nil ||
		!slices.Equal(taskCandidateIDs, expectedCandidateIDs) ||
		humanTask.SubjectType != "structure_identity_gate_input" || humanTask.SubjectID != gateInput.ID ||
		humanTask.SubjectRevision != 1 || humanTask.SubjectHash != gateInput.InputHash {
		t.Fatalf("StructureIdentity HumanTask = %#v candidates=%v err=%v", humanTask, taskCandidateIDs, err)
	}
	decisionID := uuid.New()
	if err = database.Model(&model.HumanTask{}).Where("id = ?", humanTask.ID).Updates(map[string]any{
		"status": "COMPLETED", "revision": humanTask.Revision + 1, "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&model.ReviewDecision{
		ID: decisionID, WorkspaceID: fixture.workspaceID, HumanTaskID: humanTask.ID,
		Decision: "approved", SubjectRevision: humanTask.SubjectRevision, SubjectHash: humanTask.SubjectHash,
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
		CreatedBy:           fixture.userID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	ownerApplication, err := workflowStore.ResolveHumanGateOwnerApplication(ctx, workflow.HumanGateDecisionRequest{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: gate.NodeRunID,
		HumanTaskID: humanTask.ID.String(), ReviewDecisionID: decisionID.String(),
		SubjectRevision: humanTask.SubjectRevision, Decision: "approved", DecisionPayloadHash: emptyReviewDecisionPayloadHash,
	})
	if err != nil {
		t.Fatalf("resolve Structure Identity owner application: %v", err)
	}
	ownerMaterial, err := workflow.DecodeStructureIdentityOwnerMaterial(ownerApplication.OwnerMaterial)
	if err != nil || ownerApplication.Candidate.ReferenceID != final.Output.Bindings[0].ReferenceID ||
		ownerApplication.OutputPort != "identities" || ownerApplication.OutputValueType != "structure_identity_set_version" ||
		ownerMaterial.GateInputID != gateInput.ID.String() || ownerMaterial.GateInput.InputHash != gateInput.InputHash ||
		ownerMaterial.SpanIndexID != accepted.SpanIndexID || len(ownerMaterial.Candidates) != 4 {
		t.Fatalf("Structure Identity owner material = %#v application=%#v err=%v", ownerMaterial, ownerApplication, err)
	}
	if err = runtimeService.OpenHumanGate(ctx, gateCommand); err != nil {
		t.Fatalf("replay StructureIdentity HumanTask open: %v", err)
	}
	var gateInputCount, humanTaskCount int64
	if err = database.Model(&model.WorkflowHumanGateInput{}).Where("node_run_id = ?", gate.NodeRunID).
		Count(&gateInputCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.HumanTask{}).Where("node_run_id = ?", gate.NodeRunID).
		Count(&humanTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if gateInputCount != 1 || humanTaskCount != 1 {
		t.Fatalf("Gate replay facts: inputs=%d tasks=%d", gateInputCount, humanTaskCount)
	}
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	productionWorldConfirmation := worldapp.NewConfirmationService(
		worldgorm.NewStore(database), func() time.Time { return now }, uuid.NewString,
	)
	signalService := workflowapp.NewSignalService(
		workflowStore, &acceptingStructureIdentitySignaler{}, workflowapp.SignalConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			Owner: workflowproduction.New(nil, bibleService, projectService, nil, nil, nil, productionWorldConfirmation),
		},
	)
	signalIntent, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: gate.NodeRunID,
		HumanTaskID: humanTask.ID.String(), ReviewDecisionID: decisionID.String(),
		SubjectRevision: humanTask.SubjectRevision, Decision: "approved", DecisionPayloadHash: emptyReviewDecisionPayloadHash,
		IdempotencyKey: "structure-identity-signal:" + decisionID.String(),
	})
	if err != nil {
		t.Fatalf("signal Structure Identity owner chain: %v", err)
	}
	var applyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.First(&applyReceipt, "review_decision_id = ?", decisionID).Error; err != nil {
		t.Fatal(err)
	}
	ownerOutput, _, ownerOutputHash, err := workflow.ParseNodeOutput(json.RawMessage(applyReceipt.Output))
	if err != nil || signalIntent.Status != "completed" || applyReceipt.Status != "completed" ||
		applyReceipt.DecisionPayloadHash != emptyReviewDecisionPayloadHash ||
		signalIntent.DecisionPayloadHash != emptyReviewDecisionPayloadHash ||
		applyReceipt.OwnerOperation == nil || *applyReceipt.OwnerOperation != "production_bible.confirm_structure_identity_set" ||
		applyReceipt.OutputHash == nil || *applyReceipt.OutputHash != ownerOutputHash || len(ownerOutput.Bindings) != 1 ||
		ownerOutput.Bindings[0].ValueType != "structure_identity_set_version" {
		t.Fatalf("Structure Identity owner signal: intent=%#v apply=%#v output=%#v err=%v", signalIntent, applyReceipt, ownerOutput, err)
	}
	var structureVersion model.StructureIdentitySetVersion
	if err = database.First(&structureVersion, "id = ?", ownerOutput.Bindings[0].ReferenceID).Error; err != nil {
		t.Fatal(err)
	}
	var projectReceipt, collectionReceipt, outboxCount int64
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ?", fixture.workspaceID, "project.confirm_episode_lifecycle",
	).Count(&projectReceipt).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StructureIdentityCollectionReceipt{}).Where(
		"version_id = ?", structureVersion.ID,
	).Count(&collectionReceipt).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.OutboxEvent{}).Where(
		"aggregate_id = ? AND event_type = ?", structureVersion.ID.String(), "StructureIdentitySetPublished",
	).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if structureVersion.ReviewDecisionID != decisionID || structureVersion.GateInputID != gateInput.ID ||
		projectReceipt != 1 || collectionReceipt != 1 || outboxCount != 1 {
		t.Fatalf("Structure Identity SOP facts: version=%#v project_receipts=%d collection_receipts=%d outbox=%d",
			structureVersion, projectReceipt, collectionReceipt, outboxCount)
	}
	if applyReceipt.OwnerReceiptID == nil || applyReceipt.OutputHash == nil {
		t.Fatalf("Structure Identity apply receipt has no owner evidence: %#v", applyReceipt)
	}
	if err = runtimeService.ApplyHumanGate(ctx, workflow.ApplyHumanGateCommand{
		WorkflowRunID: started.ID, NodeRunID: gate.NodeRunID, NodeID: gate.NodeID,
		SignalIntentID: signalIntent.ID, Decision: "APPROVED",
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
		OwnerReceiptID:      applyReceipt.OwnerReceiptID.String(), Output: ownerOutput, OutputHash: *applyReceipt.OutputHash,
	}); err != nil {
		t.Fatalf("apply Structure Identity Gate output: %v", err)
	}
	productionEntityResult, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[6].NodeRunID, NodeID: plan.Nodes[6].NodeID,
		Executor: plan.Nodes[6].Executor, Attempt: 1,
	})
	if err != nil || productionEntityResult.Status != "SUCCEEDED" ||
		len(productionEntityResult.Output.Bindings) != 1 ||
		productionEntityResult.Output.Bindings[0].ValueType != "production_entity_fragment_candidate" {
		t.Fatalf("derive Production Entity Candidate: result=%#v err=%v", productionEntityResult, err)
	}
	productionEntityCandidate, err := sceneService.GetCandidate(
		ctx, fixture.projectID.String(), productionEntityResult.Output.Bindings[0].ReferenceID,
	)
	if err != nil {
		t.Fatalf("query persisted Production Entity Candidate: %v", err)
	}
	var productionEntityInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&productionEntityInvocation, "id = ?", productionEntityCandidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query Production Entity invocation: %v", err)
	}
	var productionEntityPayload contract.SceneAnalysisPayload
	var productionEntityInput contract.ProductionEntityDerivationInput
	if err = json.Unmarshal(productionEntityInvocation.Payload, &productionEntityPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(productionEntityPayload.StageInput, &productionEntityInput); err != nil ||
		productionEntityInput.StructureIdentitySetVersionID != ownerOutput.Bindings[0].ReferenceID ||
		productionEntityInput.StructureIdentitySetVersionHash != ownerOutput.Bindings[0].ContentHash ||
		contract.ValidateProductionEntityFragmentCandidate(productionEntityCandidate.Candidate, productionEntityInput) != nil {
		t.Fatalf("validate persisted Production Entity Candidate: input=%#v err=%v", productionEntityInput, err)
	}
	var productionEntityReads []model.SceneAnalysisInvocationRead
	if err = database.Where("invocation_id = ?", productionEntityInvocation.ID).
		Order("position ASC").Find(&productionEntityReads).Error; err != nil || len(productionEntityReads) != 1 ||
		productionEntityReads[0].CandidateRevisionID.String() != factOutput.Output.Bindings[0].ReferenceID {
		t.Fatalf("Production Entity exact SceneFact read set: reads=%#v err=%v", productionEntityReads, err)
	}
	sceneBindingResult, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[7].NodeRunID, NodeID: plan.Nodes[7].NodeID,
		Executor: plan.Nodes[7].Executor, Attempt: 1,
	})
	if err != nil || sceneBindingResult.Status != "SUCCEEDED" ||
		len(sceneBindingResult.Output.Bindings) != 1 ||
		sceneBindingResult.Output.Bindings[0].ValueType != "scene_binding_fragment_candidate" {
		t.Fatalf("bind Scene occurrences: result=%#v err=%v", sceneBindingResult, err)
	}
	sceneBindingCandidate, err := sceneService.GetCandidate(
		ctx, fixture.projectID.String(), sceneBindingResult.Output.Bindings[0].ReferenceID,
	)
	if err != nil {
		t.Fatalf("query persisted Scene binding Candidate: %v", err)
	}
	var sceneBindingInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&sceneBindingInvocation, "id = ?", sceneBindingCandidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query Scene binding invocation: %v", err)
	}
	var sceneBindingPayload contract.SceneAnalysisPayload
	var sceneBindingInput contract.SceneOccurrenceBindingInput
	if err = json.Unmarshal(sceneBindingInvocation.Payload, &sceneBindingPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(sceneBindingPayload.StageInput, &sceneBindingInput); err != nil ||
		sceneBindingInput.StructureIdentitySetVersionID != ownerOutput.Bindings[0].ReferenceID ||
		sceneBindingInput.ProductionEntityCandidateRevisionID != productionEntityCandidate.ID ||
		contract.ValidateSceneBindingFragmentCandidate(sceneBindingCandidate.Candidate, sceneBindingInput) != nil {
		t.Fatalf("validate persisted Scene binding Candidate: input=%#v err=%v", sceneBindingInput, err)
	}
	var sceneBindingReads []model.SceneAnalysisInvocationRead
	if err = database.Where("invocation_id = ?", sceneBindingInvocation.ID).
		Order("position ASC").Find(&sceneBindingReads).Error; err != nil || len(sceneBindingReads) != 2 ||
		sceneBindingReads[0].StageKey != "extract_scene_facts" ||
		sceneBindingReads[1].StageKey != "derive_production_entities" {
		t.Fatalf("Scene binding exact read set: reads=%#v err=%v", sceneBindingReads, err)
	}
	continuityResult, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[8].NodeRunID, NodeID: plan.Nodes[8].NodeID,
		Executor: plan.Nodes[8].Executor, Attempt: 1,
	})
	if err != nil || continuityResult.Status != "SUCCEEDED" ||
		len(continuityResult.Output.Bindings) != 1 ||
		continuityResult.Output.Bindings[0].ValueType != "continuity_fragment_candidate" {
		t.Fatalf("reconcile Interaction/Continuity: result=%#v err=%v", continuityResult, err)
	}
	continuityCandidate, err := sceneService.GetCandidate(
		ctx, fixture.projectID.String(), continuityResult.Output.Bindings[0].ReferenceID,
	)
	if err != nil {
		t.Fatalf("query persisted Interaction/Continuity Candidate: %v", err)
	}
	var continuityInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&continuityInvocation, "id = ?", continuityCandidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query Interaction/Continuity invocation: %v", err)
	}
	var continuityPayload contract.SceneAnalysisPayload
	var continuityInput contract.InteractionContinuityInput
	if err = json.Unmarshal(continuityInvocation.Payload, &continuityPayload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(continuityPayload.StageInput, &continuityInput); err != nil ||
		continuityInput.SceneBindingCandidateRevisionID != sceneBindingCandidate.ID ||
		contract.ValidateInteractionContinuityCandidate(
			continuityCandidate.Candidate, continuityInput,
		) != nil {
		t.Fatalf("validate persisted Interaction/Continuity Candidate: input=%#v err=%v", continuityInput, err)
	}
	var continuityValue contract.InteractionContinuityCandidate
	if err = json.Unmarshal(continuityCandidate.Candidate, &continuityValue); err != nil ||
		len(continuityValue.Interactions) != 1 || len(continuityValue.Continuity) != 1 {
		t.Fatalf("Interaction/Continuity facts: candidate=%#v err=%v", continuityValue, err)
	}
	var continuityReads []model.SceneAnalysisInvocationRead
	if err = database.Where("invocation_id = ?", continuityInvocation.ID).
		Order("position ASC").Find(&continuityReads).Error; err != nil || len(continuityReads) != 3 ||
		continuityReads[0].StageKey != "extract_scene_facts" ||
		continuityReads[1].StageKey != "derive_production_entities" ||
		continuityReads[2].StageKey != "bind_scene_occurrences" {
		t.Fatalf("Interaction/Continuity exact read set: reads=%#v err=%v", continuityReads, err)
	}
	productionWorldResult, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[9].NodeRunID, NodeID: plan.Nodes[9].NodeID,
		Executor: plan.Nodes[9].Executor, Attempt: 1,
	})
	if err != nil || productionWorldResult.Status != "SUCCEEDED" ||
		len(productionWorldResult.Output.Bindings) != 1 ||
		productionWorldResult.Output.Bindings[0].ValueType != "production_world_candidate" {
		t.Fatalf("assemble Production World Candidate: result=%#v err=%v", productionWorldResult, err)
	}
	var productionWorldRevision model.StageCandidateRevision
	if err = database.First(
		&productionWorldRevision,
		"id = ?",
		productionWorldResult.Output.Bindings[0].ReferenceID,
	).Error; err != nil {
		t.Fatalf("query Production World Candidate revision: %v", err)
	}
	productionWorld, _, err := worlddomain.DecodeProductionWorldCandidate(json.RawMessage(productionWorldRevision.Candidate))
	if err != nil || productionWorld.ContentHash != productionWorldRevision.CandidateContentHash ||
		productionWorld.UpstreamCandidates.ProductionEntity.CandidateRevisionID != productionEntityCandidate.ID ||
		productionWorld.UpstreamCandidates.SceneOccurrence.CandidateRevisionID != sceneBindingCandidate.ID ||
		productionWorld.UpstreamCandidates.InteractionContinuity.CandidateRevisionID != continuityCandidate.ID {
		t.Fatalf("persisted Production World Candidate: candidate=%#v revision=%#v err=%v", productionWorld, productionWorldRevision, err)
	}
	var productionWorldHead model.StageCandidateHead
	if err = database.First(
		&productionWorldHead,
		"stage_instance_key = ?",
		productionWorldRevision.StageInstanceKey,
	).Error; err != nil || productionWorldHead.CurrentRevisionID != productionWorldRevision.ID ||
		productionWorldHead.CurrentCandidateRevisionHash != productionWorldRevision.CandidateRevisionHash {
		t.Fatalf("Production World Candidate head: head=%#v err=%v", productionWorldHead, err)
	}
	var productionWorldManifest model.ShardManifest
	if err = database.First(
		&productionWorldManifest,
		"workflow_run_id = ? AND node_run_id = ? AND stage = ?",
		uuid.MustParse(started.ID), uuid.MustParse(plan.Nodes[9].NodeRunID), "assemble_production_world",
	).Error; err != nil || productionWorldManifest.RootInputHash == "" {
		t.Fatalf("Production World aggregate manifest: manifest=%#v err=%v", productionWorldManifest, err)
	}
	directCommand := workflowapp.ProductionWorldAssemblyCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[9].NodeRunID,
		InputHash: productionWorldManifest.RootInputHash,
		Draft: worlddomain.ProductionWorldCandidateDraft{
			WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
			SourceVersion: contract.ScriptSourceVersionIdentity{
				OwnerKind: accepted.Identity.OwnerKind, LogicalID: accepted.Identity.LogicalID,
				VersionID: accepted.Identity.VersionID, Revision: accepted.Identity.Revision,
				ContentHash: accepted.Identity.ContentHash, CreatedAt: accepted.Identity.CreatedAt,
			},
			FrozenInput:                        continuityInput,
			ProductionEntityCandidate:          sceneAnalysisCandidateIdentity(productionEntityCandidate),
			SceneOccurrenceCandidate:           sceneAnalysisCandidateIdentity(sceneBindingCandidate),
			InteractionContinuityCandidate:     sceneAnalysisCandidateIdentity(continuityCandidate),
			InteractionContinuityCandidateBody: continuityCandidate.Candidate,
		},
		Leaves: []contract.AggregateLeafCandidateRef{
			sceneAnalysisAggregateLeaf(productionEntityCandidate),
			sceneAnalysisAggregateLeaf(sceneBindingCandidate),
			sceneAnalysisAggregateLeaf(continuityCandidate),
		},
	}
	directReplay, err := productionWorldService.AssembleProductionWorld(ctx, directCommand)
	if err != nil || directReplay.ID != productionWorldRevision.ID.String() ||
		directReplay.CandidateRevisionHash != productionWorldRevision.CandidateRevisionHash {
		t.Fatalf("idempotent Production World persistence: revision=%#v err=%v", directReplay, err)
	}
	staleReadNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: staleReadNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: uuid.MustParse(started.ID),
		NodeID: "production-world-stale-read", DefinitionKey: "production.production_world_assembly",
		DefinitionVersion: "1.0.0", Executor: "activity.production_world_assembly",
		RiskLevel: "low", Status: "QUEUED", Attempt: 0, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create Production World stale-read NodeRun: %v", err)
	}
	if err = database.Model(&model.SceneAnalysisCandidateHead{}).
		Where("stage_instance_key = ?", continuityCandidate.StageInstanceKey).
		Update("current_candidate_revision_hash", strings.Repeat("e", 64)).Error; err != nil {
		t.Fatalf("drift Production World leaf Head: %v", err)
	}
	staleReadCommand := directCommand
	staleReadCommand.NodeRunID = staleReadNodeRunID.String()
	_, staleReadErr := productionWorldService.AssembleProductionWorld(ctx, staleReadCommand)
	if err = database.Model(&model.SceneAnalysisCandidateHead{}).
		Where("stage_instance_key = ?", continuityCandidate.StageInstanceKey).
		Update("current_candidate_revision_hash", continuityCandidate.CandidateRevisionHash).Error; err != nil {
		t.Fatalf("restore Production World leaf Head: %v", err)
	}
	if staleReadErr == nil || !strings.Contains(staleReadErr.Error(), "read set is stale") {
		t.Fatalf("Production World stale read error = %v", staleReadErr)
	}
	var staleReadManifestCount int64
	if err = database.Model(&model.ShardManifest{}).
		Where("node_run_id = ? AND stage = ?", staleReadNodeRunID, "assemble_production_world").
		Count(&staleReadManifestCount).Error; err != nil {
		t.Fatal(err)
	}
	if staleReadManifestCount != 0 {
		t.Fatalf("Production World stale read persisted %d manifests", staleReadManifestCount)
	}
	replayedProductionWorld, err := runtimeService.ExecuteNode(ctx, workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: plan.Nodes[9].NodeRunID, NodeID: plan.Nodes[9].NodeID,
		Executor: plan.Nodes[9].Executor, Attempt: 2,
	})
	if err != nil || replayedProductionWorld.OutputHash != productionWorldResult.OutputHash {
		t.Fatalf("replay Production World aggregate: result=%#v err=%v", replayedProductionWorld, err)
	}
	var productionWorldRevisionCount, productionWorldManifestCount int64
	if err = database.Model(&model.StageCandidateRevision{}).
		Where("stage_instance_key = ?", productionWorldRevision.StageInstanceKey).
		Count(&productionWorldRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.ShardManifest{}).
		Where("workflow_run_id = ? AND node_run_id = ? AND stage = ?",
			uuid.MustParse(started.ID), uuid.MustParse(plan.Nodes[9].NodeRunID), "assemble_production_world").
		Count(&productionWorldManifestCount).Error; err != nil {
		t.Fatal(err)
	}
	if productionWorldRevisionCount != 1 || productionWorldManifestCount != 1 {
		t.Fatalf("Production World replay facts: revisions=%d manifests=%d", productionWorldRevisionCount, productionWorldManifestCount)
	}
	productionWorldGate := plan.Nodes[10]
	productionWorldGateCommand := workflow.NodeActivityCommand{
		WorkflowRunID: started.ID, NodeRunID: productionWorldGate.NodeRunID, NodeID: productionWorldGate.NodeID,
		Executor: productionWorldGate.Executor, Attempt: 1,
	}
	if err = database.Model(&model.StructureIdentityScopeHead{}).
		Where("project_id = ?", fixture.projectID).
		Update("head_hash", strings.Repeat("f", 64)).Error; err != nil {
		t.Fatalf("drift StructureIdentity Head before Gate 2: %v", err)
	}
	staleGateErr := runtimeService.OpenHumanGate(ctx, productionWorldGateCommand)
	if err = database.Model(&model.StructureIdentityScopeHead{}).
		Where("project_id = ?", fixture.projectID).
		Update("head_hash", structureVersion.ContentHash).Error; err != nil {
		t.Fatalf("restore StructureIdentity Head before Gate 2: %v", err)
	}
	if staleGateErr == nil || !strings.Contains(staleGateErr.Error(), "StructureIdentitySet Head has drifted") {
		t.Fatalf("Gate 2 stale formal read set error = %v", staleGateErr)
	}
	var staleGateInputCount, staleGateTaskCount int64
	if err = database.Model(&model.WorkflowHumanGateInput{}).
		Where("node_run_id = ?", productionWorldGate.NodeRunID).Count(&staleGateInputCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.HumanTask{}).
		Where("node_run_id = ?", productionWorldGate.NodeRunID).Count(&staleGateTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if staleGateInputCount != 0 || staleGateTaskCount != 0 {
		t.Fatalf("Gate 2 stale read persisted inputs=%d tasks=%d", staleGateInputCount, staleGateTaskCount)
	}
	if err = runtimeService.OpenHumanGate(ctx, productionWorldGateCommand); err != nil {
		t.Fatalf("open Production World HumanTask: %v", err)
	}
	var productionWorldGateInput model.WorkflowHumanGateInput
	if err = database.First(&productionWorldGateInput, "node_run_id = ?", productionWorldGate.NodeRunID).Error; err != nil {
		t.Fatalf("query Production World Gate input: %v", err)
	}
	decodedProductionWorldGate, _, gateDecodeErr := workflow.DecodeProductionWorldGateInput(json.RawMessage(productionWorldGateInput.Input))
	if gateDecodeErr != nil || decodedProductionWorldGate.InputHash != productionWorldGateInput.InputHash ||
		decodedProductionWorldGate.Subject.ProductionWorldCandidate.CandidateRevisionID != productionWorldRevision.ID.String() ||
		decodedProductionWorldGate.Subject.SceneOccurrenceCandidate.CandidateRevisionID != sceneBindingCandidate.ID ||
		decodedProductionWorldGate.Subject.InteractionCandidate.Candidate.CandidateRevisionID != continuityCandidate.ID ||
		decodedProductionWorldGate.Subject.ContinuityCandidate.Candidate.CandidateRevisionID != continuityCandidate.ID ||
		decodedProductionWorldGate.Subject.InteractionCandidate.ProjectionHash == decodedProductionWorldGate.Subject.ContinuityCandidate.ProjectionHash ||
		len(decodedProductionWorldGate.Subject.ExpectedHeads) != 4 {
		t.Fatalf("persisted Production World Gate input = %#v err=%v", decodedProductionWorldGate, gateDecodeErr)
	}
	for _, expectedHead := range decodedProductionWorldGate.Subject.ExpectedHeads {
		if expectedHead.Revision != 0 || expectedHead.ContentHash != "" {
			t.Fatalf("initial Production World expected Head = %#v", expectedHead)
		}
	}
	var productionWorldTask model.HumanTask
	if err = database.First(&productionWorldTask, "node_run_id = ?", productionWorldGate.NodeRunID).Error; err != nil {
		t.Fatalf("query Production World HumanTask: %v", err)
	}
	var productionWorldCandidateIDs []string
	wantProductionWorldCandidateIDs := []string{
		productionWorldRevision.ID.String(),
		productionEntityCandidate.ID,
		sceneBindingCandidate.ID,
		continuityCandidate.ID,
	}
	slices.Sort(wantProductionWorldCandidateIDs)
	if err = json.Unmarshal(productionWorldTask.CandidateIDs, &productionWorldCandidateIDs); err != nil ||
		!slices.Equal(productionWorldCandidateIDs, wantProductionWorldCandidateIDs) ||
		productionWorldTask.SubjectType != "production_world_gate_input" ||
		productionWorldTask.SubjectID != productionWorldGateInput.ID ||
		productionWorldTask.SubjectRevision != 1 ||
		productionWorldTask.SubjectHash != productionWorldGateInput.InputHash {
		t.Fatalf("Production World HumanTask = %#v candidates=%v err=%v", productionWorldTask, productionWorldCandidateIDs, err)
	}
	productionWorldDetail, err := reviewService.GetTask(ctx, reviewapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, productionWorldTask.ID.String())
	if err != nil {
		t.Fatalf("query Production World review detail: %v", err)
	}
	productionWorldReview, _, detailErr := workflow.DecodeProductionWorldReviewDetail(productionWorldDetail.Subject)
	if detailErr != nil || productionWorldReview.InputHash != productionWorldGateInput.InputHash ||
		productionWorldReview.CandidateRevision.CandidateRevisionID != productionWorldRevision.ID.String() ||
		productionWorldReview.Views.CharacterAppearances == nil || productionWorldReview.Views.Locations == nil ||
		productionWorldReview.Views.PropStates == nil || productionWorldReview.Views.SceneOccurrences == nil ||
		productionWorldReview.Views.Interactions == nil || productionWorldReview.Views.Continuity.Claims == nil ||
		productionWorldReview.Views.Continuity.Ledger == nil {
		t.Fatalf("Production World six-view detail = %#v err=%v", productionWorldReview, detailErr)
	}
	if err = runtimeService.OpenHumanGate(ctx, productionWorldGateCommand); err != nil {
		t.Fatalf("replay Production World HumanTask open: %v", err)
	}
	var productionWorldGateInputCount, productionWorldTaskCount int64
	if err = database.Model(&model.WorkflowHumanGateInput{}).
		Where("node_run_id = ?", productionWorldGate.NodeRunID).Count(&productionWorldGateInputCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.HumanTask{}).
		Where("node_run_id = ?", productionWorldGate.NodeRunID).Count(&productionWorldTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if productionWorldGateInputCount != 1 || productionWorldTaskCount != 1 {
		t.Fatalf("Production World Gate replay facts: inputs=%d tasks=%d", productionWorldGateInputCount, productionWorldTaskCount)
	}
	productionWorldDecisionID := uuid.New()
	if err = database.Model(&model.HumanTask{}).Where("id = ?", productionWorldTask.ID).Updates(map[string]any{
		"status": "COMPLETED", "revision": productionWorldTask.Revision + 1, "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&model.ReviewDecision{
		ID: productionWorldDecisionID, WorkspaceID: fixture.workspaceID, HumanTaskID: productionWorldTask.ID,
		Decision: "approved", SubjectRevision: productionWorldTask.SubjectRevision,
		SubjectHash: productionWorldTask.SubjectHash, DecisionPayloadHash: emptyReviewDecisionPayloadHash,
		CreatedBy: fixture.userID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	productionWorldOwnerApplication, err := workflowStore.ResolveHumanGateOwnerApplication(ctx, workflow.HumanGateDecisionRequest{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: productionWorldGate.NodeRunID,
		HumanTaskID: productionWorldTask.ID.String(), ReviewDecisionID: productionWorldDecisionID.String(),
		SubjectRevision: productionWorldTask.SubjectRevision, Decision: "approved",
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
	})
	productionWorldOwnerMaterial, materialErr := workflow.DecodeProductionWorldOwnerMaterial(
		productionWorldOwnerApplication.OwnerMaterial,
	)
	if err != nil || materialErr != nil ||
		productionWorldOwnerApplication.Candidate.ReferenceID != productionWorldRevision.ID.String() ||
		productionWorldOwnerApplication.OutputPort != "world" ||
		productionWorldOwnerApplication.OutputValueType != "production_world_owner_set" ||
		productionWorldOwnerMaterial.GateInputID != productionWorldGateInput.ID.String() ||
		productionWorldOwnerMaterial.GateInput.InputHash != productionWorldGateInput.InputHash ||
		productionWorldOwnerMaterial.Candidate.ContentHash != productionWorld.ContentHash {
		t.Fatalf(
			"Production World owner material = %#v application=%#v resolve_err=%v decode_err=%v",
			productionWorldOwnerMaterial, productionWorldOwnerApplication, err, materialErr,
		)
	}
	expectedProductionWorldHeads := make([]worldapp.ExpectedHead, len(productionWorldOwnerMaterial.GateInput.Subject.ExpectedHeads))
	for index, head := range productionWorldOwnerMaterial.GateInput.Subject.ExpectedHeads {
		expectedProductionWorldHeads[index] = worldapp.ExpectedHead{
			OwnerKind: head.OwnerKind, VersionFamily: head.VersionFamily, ScopeKind: head.ScopeKind,
			ScopeKey: head.ScopeKey, Revision: head.Revision, ContentHash: head.ContentHash,
		}
	}
	confirmationCommand := worldapp.ConfirmProductionWorldCommand{
		CommandID:   uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:confirm-production-world:"+productionWorldDecisionID.String())).String(),
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		ActorID: fixture.userID.String(), GateInputID: productionWorldOwnerMaterial.GateInputID,
		GateInputHash:    productionWorldOwnerMaterial.GateInput.InputHash,
		ReviewDecisionID: productionWorldDecisionID.String(), CandidateRevisionID: productionWorldRevision.ID.String(),
		CandidateRevision: productionWorldRevision.RevisionNo, CandidateRevisionHash: productionWorldRevision.CandidateRevisionHash,
		IdempotencyKey: "workflow-production-world:" + productionWorldDecisionID.String(),
		ExpectedHeads:  expectedProductionWorldHeads, Candidate: productionWorld,
	}
	failedCommand := confirmationCommand
	failedCommand.CommandID = uuid.NewString()
	failedCommand.IdempotencyKey = "confirm-production-world-rollback:" + productionWorldDecisionID.String()
	fixedID := uuid.NewString()
	failedService := worldapp.NewConfirmationService(
		worldgorm.NewStore(database), func() time.Time { return now }, func() string { return fixedID },
	)
	if _, rollbackErr := failedService.ConfirmProductionWorld(ctx, failedCommand); rollbackErr == nil {
		t.Fatal("Production World confirmation with colliding immutable IDs unexpectedly succeeded")
	}
	rollbackChecks := []struct {
		model any
		query string
		args  []any
	}{
		{&model.ProductionWorldCommandDedup{}, "command_id = ?", []any{failedCommand.CommandID}},
		{&model.Asset{}, "project_id = ?", []any{fixture.projectID}},
		{&model.ProductionWorldBibleVersion{}, "project_id = ?", []any{fixture.projectID}},
		{&model.ProductionWorldPlanningEpisodeHead{}, "project_id = ?", []any{fixture.projectID}},
		{&model.ProductionWorldCollectionReceipt{}, "command_id = ?", []any{failedCommand.CommandID}},
		{&model.OutboxEvent{}, "aggregate_id = ?", []any{failedCommand.CommandID}},
	}
	for _, check := range rollbackChecks {
		var count int64
		if countErr := database.Model(check.model).Where(check.query, check.args...).Count(&count).Error; countErr != nil || count != 0 {
			t.Fatalf("Production World rollback %T count=%d err=%v", check.model, count, countErr)
		}
	}
	productionWorldSignalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: started.ID, NodeRunID: productionWorldGate.NodeRunID,
		HumanTaskID: productionWorldTask.ID.String(), ReviewDecisionID: productionWorldDecisionID.String(),
		SubjectRevision: productionWorldTask.SubjectRevision, Decision: "approved",
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
		IdempotencyKey:      "production-world-signal:" + productionWorldDecisionID.String(),
	}
	productionWorldSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, productionWorldSignalCommand)
	if err != nil || productionWorldSignal.Status != "completed" {
		t.Fatalf("signal Production World owner chain: intent=%#v err=%v", productionWorldSignal, err)
	}
	var productionWorldApplyReceipt model.WorkflowHumanGateApplyReceipt
	if err = database.First(&productionWorldApplyReceipt, "review_decision_id = ?", productionWorldDecisionID).Error; err != nil {
		t.Fatal(err)
	}
	productionWorldOutput, _, productionWorldOutputHash, outputErr := workflow.ParseNodeOutput(
		json.RawMessage(productionWorldApplyReceipt.Output),
	)
	if outputErr != nil || productionWorldApplyReceipt.Status != "completed" ||
		productionWorldApplyReceipt.OwnerReceiptID == nil || productionWorldApplyReceipt.OwnerOperation == nil ||
		*productionWorldApplyReceipt.OwnerOperation != worlddomain.ConfirmProductionWorldOperation ||
		productionWorldApplyReceipt.OutputHash == nil || *productionWorldApplyReceipt.OutputHash != productionWorldOutputHash ||
		len(productionWorldOutput.Bindings) != 1 || productionWorldOutput.Bindings[0].ValueType != "production_world_owner_set" ||
		productionWorldOutput.Bindings[0].ReferenceID != productionWorldApplyReceipt.OwnerReceiptID.String() {
		t.Fatalf("Production World owner signal: apply=%#v output=%#v err=%v", productionWorldApplyReceipt, productionWorldOutput, outputErr)
	}
	applyProductionWorldCommand := workflow.ApplyHumanGateCommand{
		WorkflowRunID: started.ID, NodeRunID: productionWorldGate.NodeRunID, NodeID: productionWorldGate.NodeID,
		SignalIntentID: productionWorldSignal.ID, Decision: "APPROVED",
		DecisionPayloadHash: productionWorldSignal.DecisionPayloadHash,
		OwnerReceiptID:      productionWorldApplyReceipt.OwnerReceiptID.String(),
		Output:              productionWorldOutput, OutputHash: productionWorldOutputHash,
	}
	if err = runtimeService.ApplyHumanGate(ctx, applyProductionWorldCommand); err != nil {
		t.Fatalf("apply Production World owner output to Workflow node: %v", err)
	}
	if err = runtimeService.ApplyHumanGate(ctx, applyProductionWorldCommand); err != nil {
		t.Fatalf("replay Production World Workflow node application: %v", err)
	}
	var appliedProductionWorldGate model.NodeRunProjection
	if err = database.First(&appliedProductionWorldGate, "id = ?", productionWorldGate.NodeRunID).Error; err != nil {
		t.Fatal(err)
	}
	if appliedProductionWorldGate.Status != "SUCCEEDED" || appliedProductionWorldGate.OutputHash == nil ||
		*appliedProductionWorldGate.OutputHash != productionWorldOutputHash {
		t.Fatalf("applied Production World Workflow node = %#v", appliedProductionWorldGate)
	}
	replayedProductionWorldSignal, err := signalService.SignalHumanGate(ctx, workflowapp.Actor{
		UserID: fixture.userID.String(), TokenVersion: 1,
	}, productionWorldSignalCommand)
	if err != nil || replayedProductionWorldSignal.ID != productionWorldSignal.ID ||
		replayedProductionWorldSignal.InputHash != productionWorldSignal.InputHash ||
		replayedProductionWorldSignal.Status != productionWorldSignal.Status ||
		replayedProductionWorldSignal.AttemptNo != productionWorldSignal.AttemptNo ||
		replayedProductionWorldSignal.Revision != productionWorldSignal.Revision ||
		!replayedProductionWorldSignal.CreatedAt.Equal(productionWorldSignal.CreatedAt) ||
		!replayedProductionWorldSignal.UpdatedAt.Equal(productionWorldSignal.UpdatedAt) {
		t.Fatalf("replay Production World owner signal: got=%#v want=%#v err=%v", replayedProductionWorldSignal, productionWorldSignal, err)
	}
	confirmedWorld, err := productionWorldConfirmation.ConfirmProductionWorld(ctx, confirmationCommand)
	if err != nil || confirmedWorld.CommandID != confirmationCommand.CommandID ||
		confirmedWorld.CommandContractID != worlddomain.ConfirmProductionWorldContract ||
		confirmedWorld.CommandReceiptID == "" || confirmedWorld.ReceiptContentHash == "" ||
		len(confirmedWorld.OrderedCollectionReceiptRefs) != 2+len(productionWorld.SharedProof.PlanningEpisodeScopes) {
		t.Fatalf("confirm Production World atomically: result=%#v err=%v", confirmedWorld, err)
	}
	replayedWorld, err := productionWorldConfirmation.ConfirmProductionWorld(ctx, confirmationCommand)
	if err != nil || !reflect.DeepEqual(replayedWorld, confirmedWorld) {
		t.Fatalf("replay Production World confirmation: got=%#v want=%#v err=%v", replayedWorld, confirmedWorld, err)
	}
	driftedConfirmation := confirmationCommand
	driftedConfirmation.CommandID = uuid.NewString()
	if _, conflictErr := productionWorldConfirmation.ConfirmProductionWorld(ctx, driftedConfirmation); !errors.Is(conflictErr, worldapp.ErrProductionWorldConfirmationConflict) {
		t.Fatalf("Production World idempotency drift error = %v", conflictErr)
	}
	var collectionReceiptCount, commandReceiptCount, confirmationOutboxCount, rebaseHeadCount int64
	checks := []struct {
		model any
		query string
		args  []any
		want  int64
	}{
		{&model.ProductionWorldCollectionReceipt{}, "command_id = ?", []any{confirmationCommand.CommandID}, int64(len(confirmedWorld.OrderedCollectionReceiptRefs))},
		{&model.CommandReceipt{}, "id = ? AND operation = ?", []any{confirmedWorld.CommandReceiptID, worlddomain.ConfirmProductionWorldOperation}, 1},
		{&model.OutboxEvent{}, "source_receipt_id = ? AND event_type = ?", []any{confirmedWorld.CommandReceiptID, worlddomain.ProductionWorldConfirmedEvent}, 1},
		{&model.ProductionWorldPlanningRebaseHead{}, "project_id = ? AND member_count = 0", []any{fixture.projectID}, 1},
	}
	counts := []*int64{&collectionReceiptCount, &commandReceiptCount, &confirmationOutboxCount, &rebaseHeadCount}
	for index, check := range checks {
		if countErr := database.Model(check.model).Where(check.query, check.args...).Count(counts[index]).Error; countErr != nil || *counts[index] != check.want {
			t.Fatalf("Production World confirmation %T count=%d want=%d err=%v", check.model, *counts[index], check.want, countErr)
		}
	}
	var confirmationOutbox model.OutboxEvent
	if err = database.First(&confirmationOutbox, "source_receipt_id = ?", confirmedWorld.CommandReceiptID).Error; err != nil {
		t.Fatal(err)
	}
	if _, envelopeErr := eventingdomain.NewEnvelope(eventingdomain.OutboxEvent{
		ID: confirmationOutbox.ID.String(), EventType: confirmationOutbox.EventType,
		EventVersion: confirmationOutbox.EventVersion, WorkspaceID: confirmationOutbox.WorkspaceID.String(),
		ProjectID: confirmationOutbox.ProjectID.String(), AggregateKind: confirmationOutbox.AggregateKind,
		AggregateID: confirmationOutbox.AggregateID, AggregateRevision: confirmationOutbox.AggregateRevision,
		SourceReceiptID: confirmationOutbox.SourceReceiptID.String(), Payload: json.RawMessage(confirmationOutbox.Payload),
		PayloadHash: confirmationOutbox.PayloadHash, OccurredAt: confirmationOutbox.OccurredAt,
	}, eventingdomain.TraceContext{RequestID: uuid.NewString()}); envelopeErr != nil {
		t.Fatalf("Production World confirmation outbox envelope: %v", envelopeErr)
	}
	assetInputs := make([]assetapp.ProductionWorldAssetIdentityInput, len(productionWorld.Asset.Identities))
	for identityIndex, identity := range productionWorld.Asset.Identities {
		states := make([]assetapp.ProductionWorldAssetStateInput, len(identity.States))
		for stateIndex, state := range identity.States {
			snapshot, marshalErr := json.Marshal(state)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			states[stateIndex] = assetapp.ProductionWorldAssetStateInput{StateKey: state.StateKey, Snapshot: snapshot}
		}
		assetInputs[identityIndex] = assetapp.ProductionWorldAssetIdentityInput{
			IdentityKey: identity.IdentityKey, Kind: identity.Kind, States: states,
		}
	}
	assetOwner := assetapp.NewProductionWorldAssetOwner(func() time.Time { return now }, uuid.NewString)
	assetRepository := assetgorm.NewProductionWorldRepository(database)
	currentAssetHead, err := assetRepository.GetIdentityStateHead(
		ctx, fixture.workspaceID.String(), fixture.projectID.String(), false,
	)
	if err != nil {
		t.Fatalf("load confirmed Production World Asset Head: %v", err)
	}
	assetResult, err := assetOwner.ApplyProductionWorldAssets(ctx, assetRepository, assetapp.ApplyProductionWorldAssetsCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(), ActorID: fixture.userID.String(),
		ExpectedHeadRevision: currentAssetHead.HeadRevision, ExpectedHeadHash: currentAssetHead.HeadContentHash,
		ExpectedBusinessKeyRoot: productionWorldBusinessKeyRoot(t, productionWorld, "asset"), Identities: assetInputs,
	})
	if err != nil || len(assetResult.Assets) != len(productionWorld.Asset.Identities) || len(assetResult.States) == 0 {
		t.Fatalf("apply Production World Asset owner: result=%#v err=%v", assetResult, err)
	}
	bibleSpecifications := make([]bibleapp.ProductionWorldSpecificationInput, len(productionWorld.Bible.Specifications))
	for index, specification := range productionWorld.Bible.Specifications {
		bibleSpecifications[index] = bibleapp.ProductionWorldSpecificationInput{
			SpecificationKey: specification.SpecificationKey, IdentityKey: specification.IdentityKey,
			Kind: specification.Kind, Slots: specification.SpecificationSlots, Basis: specification.Basis,
		}
	}
	bibleClaims := make([]bibleapp.ProductionWorldClaimInput, len(productionWorld.Bible.WorldClaims))
	for index, claim := range productionWorld.Bible.WorldClaims {
		bibleClaims[index] = bibleapp.ProductionWorldClaimInput{
			ClaimKey: claim.ClaimKey, ClaimType: claim.ClaimType, Statement: claim.Statement,
			SubjectIdentityKeys: claim.SubjectIdentityKeys, Basis: claim.Basis,
		}
	}
	bibleOwner := bibleapp.NewProductionWorldBibleOwner(func() time.Time { return now }, uuid.NewString)
	bibleRepository := biblegorm.NewProductionWorldRepository(database)
	currentBibleHead, _, err := bibleRepository.GetProductionWorldBibleHead(
		ctx, fixture.workspaceID.String(), fixture.projectID.String(), false,
	)
	if err != nil {
		t.Fatalf("load confirmed Production World Bible Head: %v", err)
	}
	bibleCommand := bibleapp.ApplyProductionWorldBibleCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(), ActorID: fixture.userID.String(),
		ReviewDecisionID: productionWorldDecisionID.String(), ExpectedBusinessKeyRoot: productionWorldBusinessKeyRoot(t, productionWorld, "bible"),
		ExpectedHeadRevision: currentBibleHead.HeadRevision, ExpectedHeadHash: currentBibleHead.HeadContentHash,
		PartitionHash: productionWorld.PartitionRoots.Bible,
		StructureIdentitySet: bibledomain.ProductionWorldOwnerRef{
			OwnerKind: productionWorld.StructureIdentitySetVersion.OwnerKind, LogicalID: productionWorld.StructureIdentitySetVersion.LogicalID,
			VersionID: productionWorld.StructureIdentitySetVersion.VersionID, Revision: productionWorld.StructureIdentitySetVersion.Revision,
			ContentHash: productionWorld.StructureIdentitySetVersion.ContentHash,
		},
		Candidate: bibledomain.ProductionWorldOwnerRef{
			OwnerKind: "agent", LogicalID: fixture.projectID.String(), VersionID: productionWorldRevision.ID.String(),
			Revision: productionWorldRevision.RevisionNo, ContentHash: productionWorldRevision.CandidateRevisionHash,
		},
		Specifications: bibleSpecifications, Claims: bibleClaims, Assets: assetResult.Assets, States: assetResult.States,
	}
	bibleResult, err := bibleOwner.ApplyProductionWorldBible(ctx, bibleRepository, bibleCommand)
	if err != nil || bibleResult.Head.HeadRevision != 1 || bibleResult.Head.ScopeRevision != 1 ||
		bibleResult.Head.ScopeKey != "project:"+fixture.projectID.String() || bibleResult.Head.MemberCount != 1 ||
		bibleResult.Head.ScopeContentHash == "" || bibleResult.Head.MembersHash == "" || bibleResult.Head.CollectionRootHash == "" ||
		len(bibleResult.Specifications) != len(bibleSpecifications) ||
		len(bibleResult.Bindings) != len(assetResult.Assets) || bibleResult.Version.StructureIdentitySet.VersionID != structureVersion.ID.String() {
		t.Fatalf("apply Production World Bible owner: result=%#v err=%v", bibleResult, err)
	}
	staleBibleCommand := bibleCommand
	staleBibleCommand.ExpectedHeadRevision, staleBibleCommand.ExpectedHeadHash = 0, ""
	if _, staleErr := bibleOwner.ApplyProductionWorldBible(ctx, bibleRepository, staleBibleCommand); !errors.Is(staleErr, bibleapp.ErrProductionWorldBibleConflict) {
		t.Fatalf("stale Production World Bible Head error = %v", staleErr)
	}
	bibleCommand.ExpectedHeadRevision = bibleResult.Head.HeadRevision
	bibleCommand.ExpectedHeadHash = bibleResult.Head.HeadContentHash
	replayedBible, err := bibleOwner.ApplyProductionWorldBible(ctx, bibleRepository, bibleCommand)
	if err != nil || replayedBible.Version.ID != bibleResult.Version.ID || replayedBible.Head.HeadContentHash != bibleResult.Head.HeadContentHash {
		t.Fatalf("replay Production World Bible owner: result=%#v err=%v", replayedBible, err)
	}
	for record, expected := range map[any]int64{
		&model.ProductionWorldBibleVersion{}: 1, &model.ProductionWorldBibleScopeHead{}: 1,
		&model.ProductionWorldEvidence{}:      int64(len(bibleSpecifications) + len(bibleClaims)),
		&model.ProductionWorldSpecification{}: int64(len(bibleSpecifications)),
		&model.ProductionWorldClaim{}:         int64(len(bibleClaims)),
		&model.ProductionWorldBinding{}:       int64(len(assetResult.Assets)),
	} {
		var count int64
		if countErr := database.Model(record).Where("project_id = ?", fixture.projectID).Count(&count).Error; countErr != nil || count != expected {
			t.Fatalf("Production World Bible %T count=%d want=%d err=%v", record, count, expected, countErr)
		}
	}
	planningHeads := make([]planningapp.ExpectedProductionWorldPlanningHead, len(productionWorld.SharedProof.PlanningEpisodeScopes))
	planningRepository := planninggorm.NewProductionWorldRepository(database)
	for index, episodeScope := range productionWorld.SharedProof.PlanningEpisodeScopes {
		currentHead, loadErr := planningRepository.GetProductionWorldPlanningHead(
			ctx, fixture.workspaceID.String(), fixture.projectID.String(), episodeScope.EpisodeID, false,
		)
		if loadErr != nil {
			t.Fatalf("load confirmed Production World Planning Head: %v", loadErr)
		}
		planningHeads[index] = planningapp.ExpectedProductionWorldPlanningHead{
			EpisodeID: episodeScope.EpisodeID, Revision: currentHead.HeadRevision, ContentHash: currentHead.HeadContentHash,
		}
	}
	planningOwner := planningapp.NewProductionWorldPlanningOwner(func() time.Time { return now }, uuid.NewString)
	planningCommand := planningapp.ApplyProductionWorldPlanningCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(), ActorID: fixture.userID.String(),
		ExpectedBusinessKeyRoot: productionWorldBusinessKeyRoot(t, productionWorld, "planning"),
		ExpectedHeads:           planningHeads, EpisodeScopes: productionWorld.SharedProof.PlanningEpisodeScopes,
		Planning: productionWorld.Planning, Assets: assetResult.Assets, States: assetResult.States,
		Specifications: bibleResult.Specifications, Bindings: bibleResult.Bindings,
	}
	planningResult, err := planningOwner.ApplyProductionWorldPlanning(ctx, planningRepository, planningCommand)
	if err != nil || len(planningResult.Heads) != len(productionWorld.SharedProof.PlanningEpisodeScopes) || len(planningResult.Facts) == 0 {
		t.Fatalf("apply Production World Planning owner: result=%#v err=%v", planningResult, err)
	}
	for _, head := range planningResult.Heads {
		if head.HeadRevision != 1 || head.MemberCount == 0 || head.ScopeKey != "episode:"+head.EpisodeID ||
			head.ScopeContentHash == "" || head.MembersHash == "" || head.CollectionRootHash == "" {
			t.Fatalf("Production World Planning Head = %#v", head)
		}
	}
	stalePlanningCommand := planningCommand
	stalePlanningCommand.ExpectedHeads = make([]planningapp.ExpectedProductionWorldPlanningHead, len(planningCommand.ExpectedHeads))
	for index, head := range planningCommand.ExpectedHeads {
		stalePlanningCommand.ExpectedHeads[index].EpisodeID = head.EpisodeID
	}
	if _, staleErr := planningOwner.ApplyProductionWorldPlanning(ctx, planningRepository, stalePlanningCommand); !errors.Is(staleErr, planningapp.ErrProductionWorldPlanningConflict) {
		t.Fatalf("stale Production World Planning Head error = %v", staleErr)
	}
	for index, head := range planningResult.Heads {
		planningCommand.ExpectedHeads[index] = planningapp.ExpectedProductionWorldPlanningHead{
			EpisodeID: head.EpisodeID, Revision: head.HeadRevision, ContentHash: head.HeadContentHash,
		}
		reloadedHead, reloadErr := planningRepository.GetProductionWorldPlanningHead(
			ctx, fixture.workspaceID.String(), fixture.projectID.String(), head.EpisodeID, false,
		)
		if reloadErr != nil || !reflect.DeepEqual(reloadedHead, head) {
			t.Fatalf("reload Production World Planning Head: got=%#v want=%#v err=%v", reloadedHead, head, reloadErr)
		}
	}
	replayedPlanning, err := planningOwner.ApplyProductionWorldPlanning(ctx, planningRepository, planningCommand)
	if err != nil || !reflect.DeepEqual(replayedPlanning.Heads, planningResult.Heads) || len(replayedPlanning.Facts) != len(planningResult.Facts) {
		t.Fatalf("replay Production World Planning owner: result=%#v err=%v", replayedPlanning, err)
	}
	planningExpectedCounts := map[any]int64{
		&model.ProductionWorldPlanningScene{}:       int64(len(productionWorld.Planning.Scenes)),
		&model.ProductionWorldPlanningDialogue{}:    int64(productionWorldDialogueCount(productionWorld.Planning.Scenes)),
		&model.ProductionWorldPlanningBeat{}:        int64(productionWorldBeatCount(productionWorld.Planning.Scenes)),
		&model.ProductionWorldPlanningOccurrence{}:  int64(productionWorldOccurrenceCount(productionWorld.Planning.Scenes)),
		&model.ProductionWorldPlanningClaim{}:       int64(len(productionWorld.Planning.Interactions) + len(productionWorld.Planning.Continuity)),
		&model.ProductionWorldPlanningMembership{}:  int64(len(planningResult.Facts)),
		&model.ProductionWorldPlanningEpisodeHead{}: int64(len(productionWorld.SharedProof.PlanningEpisodeScopes)),
	}
	for record, expected := range planningExpectedCounts {
		var count int64
		if countErr := database.Model(record).Where("project_id = ?", fixture.projectID).Count(&count).Error; countErr != nil || count != expected {
			t.Fatalf("Production World Planning %T count=%d want=%d err=%v", record, count, expected, countErr)
		}
	}
	assetByIdentity := make(map[string]assetdomain.Asset, len(assetResult.Assets))
	stateByKey := make(map[string]assetdomain.AssetState, len(assetResult.States))
	specificationByIdentity := make(map[string]bibledomain.ProductionWorldSpecification, len(bibleResult.Specifications))
	bindingByIdentity := make(map[string]bibledomain.ProductionWorldBinding, len(bibleResult.Bindings))
	for _, asset := range assetResult.Assets {
		assetByIdentity[asset.IdentityKey] = asset
	}
	for _, state := range assetResult.States {
		stateByKey[state.StateKey] = state
	}
	for _, specification := range bibleResult.Specifications {
		specificationByIdentity[specification.IdentityKey] = specification
	}
	for _, binding := range bibleResult.Bindings {
		bindingByIdentity[binding.IdentityKey] = binding
	}
	var occurrenceRecords []model.ProductionWorldPlanningOccurrence
	if err = database.Where("project_id = ?", fixture.projectID).Find(&occurrenceRecords).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range occurrenceRecords {
		var payload planningdomain.OccurrenceFactPayload
		if json.Unmarshal(record.Payload, &payload) != nil {
			t.Fatalf("decode Planning Occurrence %s", record.ID)
		}
		var fragment contract.SceneOccurrenceFragment
		if json.Unmarshal(payload.Fragment, &fragment) != nil {
			t.Fatalf("decode Planning Occurrence fragment %s", record.ID)
		}
		asset, state := assetByIdentity[fragment.IdentityKey], stateByKey[fragment.StateKey]
		specification, binding := specificationByIdentity[fragment.IdentityKey], bindingByIdentity[fragment.IdentityKey]
		if record.AssetID.String() != asset.ID || record.AssetStateID.String() != state.ID ||
			record.SpecificationID.String() != specification.ID || record.ProductionBindingID.String() != binding.ID ||
			record.AssetContentHash != asset.ContentHash || record.StateContentHash != state.ContentHash ||
			record.SpecificationHash != specification.ContentHash || record.BindingHash != binding.ContentHash {
			t.Fatalf("Planning Occurrence exact refs drifted: record=%#v payload=%#v", record, payload)
		}
	}
	episodeBySceneScope := make(map[string]string)
	for _, scope := range productionWorld.SharedProof.PlanningEpisodeScopes {
		for _, sceneScopeKey := range scope.SceneScopeKeys {
			episodeBySceneScope[sceneScopeKey] = scope.EpisodeID
		}
	}
	var planningClaimRecords []model.ProductionWorldPlanningClaim
	if err = database.Where("project_id = ?", fixture.projectID).Find(&planningClaimRecords).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range planningClaimRecords {
		var payload planningdomain.PlanningClaimFactPayload
		var targetScene model.ProductionWorldPlanningScene
		if json.Unmarshal(record.Payload, &payload) != nil ||
			database.First(&targetScene, "id = ?", record.TargetSceneID).Error != nil ||
			record.EpisodeID.String() != episodeBySceneScope[targetScene.SceneScopeKey] {
			t.Fatalf("Planning Claim target Episode assignment drifted: record=%#v payload=%#v", record, payload)
		}
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

	identityCandidate, err := sceneService.GetCandidate(
		ctx, fixture.projectID.String(), identityOutput.Output.Bindings[0].ReferenceID,
	)
	if err != nil {
		t.Fatalf("query persisted IdentityResolution Candidate: %v", err)
	}
	var identityInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&identityInvocation, "id = ?", identityCandidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query IdentityResolution invocation: %v", err)
	}
	var identityRead model.SceneAnalysisInvocationRead
	if err = database.First(
		&identityRead, "invocation_id = ? AND stage_key = ?", identityInvocation.ID, "extract_scene_facts",
	).Error; err != nil {
		t.Fatal("IdentityResolution invocation has no upstream SceneFact Candidate")
	}
	factCandidate, err := sceneService.GetCandidate(
		ctx,
		fixture.projectID.String(),
		identityRead.CandidateRevisionID.String(),
	)
	if err != nil {
		t.Fatalf("query upstream SceneFact Candidate: %v", err)
	}
	var sceneFactInvocation model.SceneAnalysisInvocationRecord
	if err = database.First(&sceneFactInvocation, "id = ?", factCandidate.SourceInvocationID).Error; err != nil {
		t.Fatalf("query SceneFact invocation: %v", err)
	}
	var sceneFactRead model.SceneAnalysisInvocationRead
	if err = database.First(
		&sceneFactRead, "invocation_id = ? AND stage_key = ?", sceneFactInvocation.ID, "propose_script_spans",
	).Error; err != nil {
		t.Fatal("Scene Fact invocation has no upstream Script Span Candidate")
	}
	spanCandidate, err := sceneService.GetCandidate(
		ctx,
		fixture.projectID.String(),
		sceneFactRead.CandidateRevisionID.String(),
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
	upstreamReadSetDriftCommand.Upstreams = []agentapp.Candidate{spanCandidate}
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
	multiReadDriftNodeRunID := uuid.New()
	if err = database.Create(&model.NodeRunProjection{
		ID: multiReadDriftNodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: parsedWorkflowRunID,
		NodeID: "review-upstream-read-set-drift", DefinitionKey: "agent.structure_identity_review",
		DefinitionVersion: "1.0.0", Executor: "activity.structure_identity_review",
		RiskLevel: "external_ai", Status: "QUEUED", Attempt: 0, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create review read-set drift NodeRun: %v", err)
	}
	multiReadDriftRuntime := &readSetDriftSceneAnalysisRuntime{
		delegate: deterministicSceneAnalysisRuntime{now: now},
		beforeResult: func() error {
			return database.Model(&model.SceneAnalysisInvocationRecord{}).
				Where("id = ?", identityCandidate.SourceInvocationID).
				Update("shard_key", "script:drifted").Error
		},
	}
	multiReadDriftService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), multiReadDriftRuntime, dispatchSigner,
		agentapp.SceneAnalysisConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + fmt.Sprintf("%064d", 7),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	multiReadDriftCommand := unknownOutcomeCommand
	multiReadDriftCommand.NodeRunID = multiReadDriftNodeRunID.String()
	multiReadDriftCommand.StageKey = "review_candidate"
	multiReadDriftCommand.Upstreams = []agentapp.Candidate{spanCandidate, factCandidate, identityCandidate}
	multiReadDriftCommand.DeterministicIssues = []contract.CandidateReviewIssue{}
	_, multiReadDriftErr := multiReadDriftService.Execute(ctx, multiReadDriftCommand)
	if err = database.Model(&model.SceneAnalysisInvocationRecord{}).
		Where("id = ?", identityCandidate.SourceInvocationID).
		Update("shard_key", "script:full").Error; err != nil {
		t.Fatalf("restore review upstream read-set fixture: %v", err)
	}
	if agentapp.ErrorCode(multiReadDriftErr) != "stale_read_set" {
		t.Fatalf("review read-set drift execution error = %v", multiReadDriftErr)
	}
	var multiReadDriftedCandidateCount int64
	if err = database.Model(&model.SceneAnalysisCandidateRevision{}).
		Where("source_invocation_id IN (?)", database.Model(&model.SceneAnalysisInvocationRecord{}).
			Select("id").Where("node_run_id = ?", multiReadDriftNodeRunID)).
		Count(&multiReadDriftedCandidateCount).Error; err != nil {
		t.Fatal(err)
	}
	if multiReadDriftedCandidateCount != 0 {
		t.Fatalf("review read-set drift published %d Candidate revisions", multiReadDriftedCandidateCount)
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
		{&model.SceneAnalysisRelease{}, 3}, {&model.SceneAnalysisControlHead{}, 3},
		{&model.SceneAnalysisInvocationRecord{}, 3}, {&model.SceneAnalysisAttempt{}, 3},
		{&model.SceneAnalysisDispatchAuthorization{}, 3},
		{&model.SceneAnalysisResult{}, 3}, {&model.SceneAnalysisCandidateRevision{}, 3},
		{&model.SceneAnalysisCandidateHead{}, 3},
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

type acceptingStructureIdentitySignaler struct{}

func (*acceptingStructureIdentitySignaler) Signal(
	_ context.Context,
	request workflow.SignalRequest,
) (workflow.SignalObservation, error) {
	return workflow.SignalObservation{Outcome: workflow.SignalOutcomeSignaled, ObservedInputHash: request.InputHash}, nil
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

func sceneAnalysisCandidateIdentity(value agentapp.Candidate) contract.SceneAnalysisCandidateRevisionIdentity {
	return contract.SceneAnalysisCandidateRevisionIdentity{
		StageKey: value.StageKey, ShardKey: "script:full", CandidateRevisionID: value.ID,
		CandidateRevisionHash: value.CandidateRevisionHash, SourceInvocationID: value.SourceInvocationID,
		SourceResultHash: value.SourceResultHash,
	}
}

func sceneAnalysisAggregateLeaf(value agentapp.Candidate) contract.AggregateLeafCandidateRef {
	return contract.AggregateLeafCandidateRef{
		StageInstanceKey: value.StageInstanceKey, ShardKey: "script:full",
		CandidateRevisionID: value.ID, CandidateRevisionHash: value.CandidateRevisionHash,
	}
}

func sceneAnalysisGraph(revisionID string) authoring.Graph {
	return authoring.Graph{
		Nodes: []authoring.Node{
			{ID: "source", DefinitionKey: "input.script_source", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"` + revisionID + `"}`)},
			{ID: "spans", DefinitionKey: "agent.script_span_proposal", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "facts", DefinitionKey: "agent.scene_fact_extraction", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "identities", DefinitionKey: "agent.identity_resolution", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "review", DefinitionKey: "agent.structure_identity_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "structure-identity-gate", DefinitionKey: "human.structure_identity_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "production-entities", DefinitionKey: "agent.production_entity_derivation", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "scene-bindings", DefinitionKey: "agent.scene_occurrence_binding", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "interaction-continuity", DefinitionKey: "agent.interaction_continuity_reconciliation", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "production-world", DefinitionKey: "production.production_world_assembly", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "production-world-gate", DefinitionKey: "human.production_world_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
		},
		Edges: []authoring.Edge{
			{ID: "source-spans", FromNodeID: "source", FromPort: "source", ToNodeID: "spans", ToPort: "source"},
			{ID: "source-facts", FromNodeID: "source", FromPort: "source", ToNodeID: "facts", ToPort: "source"},
			{ID: "spans-facts", FromNodeID: "spans", FromPort: "candidate", ToNodeID: "facts", ToPort: "spans"},
			{ID: "source-identities", FromNodeID: "source", FromPort: "source", ToNodeID: "identities", ToPort: "source"},
			{ID: "facts-identities", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "identities", ToPort: "facts"},
			{ID: "source-review", FromNodeID: "source", FromPort: "source", ToNodeID: "review", ToPort: "source"},
			{ID: "spans-review", FromNodeID: "spans", FromPort: "candidate", ToNodeID: "review", ToPort: "spans"},
			{ID: "facts-review", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "review", ToPort: "facts"},
			{ID: "identities-review", FromNodeID: "identities", FromPort: "candidate", ToNodeID: "review", ToPort: "identities"},
			{ID: "source-structure-identity-gate", FromNodeID: "source", FromPort: "source", ToNodeID: "structure-identity-gate", ToPort: "source"},
			{ID: "spans-structure-identity-gate", FromNodeID: "spans", FromPort: "candidate", ToNodeID: "structure-identity-gate", ToPort: "spans"},
			{ID: "facts-structure-identity-gate", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "structure-identity-gate", ToPort: "facts"},
			{ID: "identities-structure-identity-gate", FromNodeID: "identities", FromPort: "candidate", ToNodeID: "structure-identity-gate", ToPort: "identities"},
			{ID: "review-structure-identity-gate", FromNodeID: "review", FromPort: "candidate", ToNodeID: "structure-identity-gate", ToPort: "review"},
			{ID: "source-production-entities", FromNodeID: "source", FromPort: "source", ToNodeID: "production-entities", ToPort: "source"},
			{ID: "facts-production-entities", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "production-entities", ToPort: "facts"},
			{ID: "identities-production-entities", FromNodeID: "structure-identity-gate", FromPort: "identities", ToNodeID: "production-entities", ToPort: "identities"},
			{ID: "source-scene-bindings", FromNodeID: "source", FromPort: "source", ToNodeID: "scene-bindings", ToPort: "source"},
			{ID: "facts-scene-bindings", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "scene-bindings", ToPort: "facts"},
			{ID: "identities-scene-bindings", FromNodeID: "structure-identity-gate", FromPort: "identities", ToNodeID: "scene-bindings", ToPort: "identities"},
			{ID: "entities-scene-bindings", FromNodeID: "production-entities", FromPort: "candidate", ToNodeID: "scene-bindings", ToPort: "entities"},
			{ID: "source-interaction-continuity", FromNodeID: "source", FromPort: "source", ToNodeID: "interaction-continuity", ToPort: "source"},
			{ID: "facts-interaction-continuity", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "interaction-continuity", ToPort: "facts"},
			{ID: "identities-interaction-continuity", FromNodeID: "structure-identity-gate", FromPort: "identities", ToNodeID: "interaction-continuity", ToPort: "identities"},
			{ID: "entities-interaction-continuity", FromNodeID: "production-entities", FromPort: "candidate", ToNodeID: "interaction-continuity", ToPort: "entities"},
			{ID: "bindings-interaction-continuity", FromNodeID: "scene-bindings", FromPort: "candidate", ToNodeID: "interaction-continuity", ToPort: "bindings"},
			{ID: "source-production-world", FromNodeID: "source", FromPort: "source", ToNodeID: "production-world", ToPort: "source"},
			{ID: "facts-production-world", FromNodeID: "facts", FromPort: "candidate", ToNodeID: "production-world", ToPort: "facts"},
			{ID: "identities-production-world", FromNodeID: "structure-identity-gate", FromPort: "identities", ToNodeID: "production-world", ToPort: "identities"},
			{ID: "entities-production-world", FromNodeID: "production-entities", FromPort: "candidate", ToNodeID: "production-world", ToPort: "entities"},
			{ID: "bindings-production-world", FromNodeID: "scene-bindings", FromPort: "candidate", ToNodeID: "production-world", ToPort: "bindings"},
			{ID: "continuity-production-world", FromNodeID: "interaction-continuity", FromPort: "candidate", ToNodeID: "production-world", ToPort: "continuity"},
			{ID: "production-world-gate-input", FromNodeID: "production-world", FromPort: "candidate", ToNodeID: "production-world-gate", ToPort: "candidate"},
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
	now                   time.Time
	calls                 int
	reviewIssue           bool
	spanCandidate         json.RawMessage
	factCandidate         json.RawMessage
	repair                *contract.StructureIdentityRepairDirective
	repairStage           string
	repairHasNote         bool
	productionRepair      *contract.ProductionWorldRepairDirective
	productionRepairStage string
	productionRepairNote  bool
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
		if input.Repair != nil {
			runtime.repair = input.Repair
			runtime.repairStage = invocation.Payload.Variant.StageKey
			runtime.repairHasNote = jsonContainsKey(invocation.Payload.StageInput, "user_note")
		}
		runtime.spanCandidate = append([]byte(nil), candidate...)
	} else if invocation.Payload.Variant.StageKey == "extract_scene_facts" {
		var input contract.SceneFactExtractionInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildSceneFactCandidate(input)
		runtime.factCandidate = append([]byte(nil), candidate...)
	} else if invocation.Payload.Variant.StageKey == "resolve_identities" {
		var input contract.IdentityResolutionInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildIdentityResolutionCandidate(input)
		if input.Repair != nil {
			runtime.repair = input.Repair
			runtime.repairStage = invocation.Payload.Variant.StageKey
			runtime.repairHasNote = jsonContainsKey(invocation.Payload.StageInput, "user_note")
		}
	} else if invocation.Payload.Variant.StageKey == "derive_production_entities" {
		var input contract.ProductionEntityDerivationInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildProductionEntityCandidate(input)
	} else if invocation.Payload.Variant.StageKey == "bind_scene_occurrences" {
		var input contract.SceneOccurrenceBindingInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildSceneBindingCandidate(input)
	} else if invocation.Payload.Variant.StageKey == "reconcile_interaction_continuity" {
		var input contract.InteractionContinuityInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildInteractionContinuityCandidate(input)
		if invocation.Payload.ProductionRepair != nil {
			runtime.productionRepair = invocation.Payload.ProductionRepair
			runtime.productionRepairStage = invocation.Payload.Variant.StageKey
			runtime.productionRepairNote = jsonContainsKey(invocation.Payload.StageInput, "user_note") ||
				jsonContainsKey(mustSceneJSON(invocation.Payload.ProductionRepair), "user_note")
			candidate = buildRepairedInteractionContinuityCandidate(
				candidate,
				*invocation.Payload.ProductionRepair,
			)
		}
	} else {
		var input contract.StructureIdentityReviewInput
		if err := json.Unmarshal(invocation.Payload.StageInput, &input); err != nil {
			return contract.SceneAnalysisAttemptResult{}, err
		}
		candidate = buildStructureIdentityReviewCandidate(input, runtime.reviewIssue)
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
		Status: "accepted",
		CandidateType: map[string]string{
			"propose_script_spans":             "script_span_candidate",
			"extract_scene_facts":              "scene_fact_candidate",
			"resolve_identities":               "identity_resolution_candidate",
			"review_candidate":                 "structure_identity_review_candidate",
			"derive_production_entities":       "production_entity_fragment_candidate",
			"bind_scene_occurrences":           "scene_binding_fragment_candidate",
			"reconcile_interaction_continuity": "continuity_fragment_candidate",
		}[invocation.Payload.Variant.StageKey],
		Candidate: candidate, InputHash: invocation.InputHash, OutputHash: &outputHash,
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

func buildRepairedInteractionContinuityCandidate(
	base json.RawMessage,
	directive contract.ProductionWorldRepairDirective,
) json.RawMessage {
	var candidate contract.InteractionContinuityCandidate
	if err := json.Unmarshal(base, &candidate); err != nil {
		panic("decode deterministic Interaction/Continuity repair fixture: " + err.Error())
	}
	if directive.ChangeSpec.Operation != contract.ProductionWorldRepairReviseInteraction ||
		len(directive.ChangeSpec.TargetKeys) != 1 {
		panic("unsupported deterministic Production World repair directive")
	}
	target := directive.ChangeSpec.TargetKeys[0]
	for index := range candidate.Interactions {
		if candidate.Interactions[index].InteractionKey != target {
			continue
		}
		candidate.Interactions[index].Hand = "left"
		evidence := candidate.Interactions[index].Evidence
		candidate.Interactions[index].GeometryEvidence.Hand = &evidence
		return mustSceneJSON(candidate)
	}
	panic("deterministic Production World repair target is missing")
}

func jsonContainsKey(value json.RawMessage, key string) bool {
	return strings.Contains(string(value), `"`+key+`"`)
}

func buildStructureIdentityReviewCandidate(input contract.StructureIdentityReviewInput, withReviewIssue bool) json.RawMessage {
	issues := any(input.DeterministicIssues)
	suggestions := any([]any{})
	if withReviewIssue {
		text := []rune(input.NormalizedText)
		anchor := string(text[0:1])
		issues = []any{map[string]any{
			"issue_key": "issue_source_interpretation_0001", "code": "source_interpretation_needs_confirmation",
			"severity": "warning", "scope": "script_source", "summary": "首段原文语义需要人工确认",
			"evidence": []any{map[string]any{
				"source_start": 0, "source_end": 1,
				"text_hash": fmt.Sprintf("%x", sha256.Sum256([]byte(anchor))), "exact_anchor": anchor,
			}},
		}}
		suggestions = []any{map[string]any{
			"issue_key": "issue_source_interpretation_0001", "action": "inspect_source",
			"target_keys": []string{"script_source"}, "rationale": "重新核对首段原文。",
		}}
	}
	return mustSceneJSON(map[string]any{
		"profile_key":       "structure_identity",
		"source_version_id": input.SourceVersionID, "source_hash": input.SourceHash,
		"span_candidate_revision_id":         input.SpanCandidateRevisionID,
		"span_candidate_revision_hash":       input.SpanCandidateRevisionHash,
		"scene_fact_candidate_revision_id":   input.SceneFactCandidateRevisionID,
		"scene_fact_candidate_revision_hash": input.SceneFactCandidateRevisionHash,
		"identity_candidate_revision_id":     input.IdentityCandidateRevisionID,
		"identity_candidate_revision_hash":   input.IdentityCandidateRevisionHash,
		"review_issues":                      issues, "suggestions": suggestions,
	})
}

func buildProductionEntityCandidate(input contract.ProductionEntityDerivationInput) json.RawMessage {
	type identityMaterial struct {
		kind, canonicalName string
		evidence            []contract.SourceEvidenceSpan
		scopes              []string
	}
	scenes := make(map[string]string, len(input.StructureIdentitySet.SceneRefs))
	for _, scene := range input.StructureIdentitySet.SceneRefs {
		scenes[scene.TemporarySceneID] = scene.ScopeKey
	}
	materials := make(map[string]*identityMaterial, len(input.StructureIdentitySet.Identities))
	for _, identity := range input.StructureIdentitySet.Identities {
		materials[identity.IdentityKey] = &identityMaterial{kind: identity.Kind, canonicalName: identity.CanonicalName}
	}
	for _, mapping := range input.StructureIdentitySet.MentionMappings {
		if mapping.IdentityKey == nil {
			continue
		}
		material := materials[*mapping.IdentityKey]
		material.evidence = append(material.evidence, contract.SourceEvidenceSpan{
			SourceStart: mapping.SourceStart, SourceEnd: mapping.SourceEnd,
			TextHash: mapping.TextHash, ExactAnchor: mapping.ExactAnchor,
		})
		material.scopes = append(material.scopes, scenes[mapping.TemporarySceneID])
	}
	identityKeys := make([]string, 0, len(materials))
	for identityKey := range materials {
		identityKeys = append(identityKeys, identityKey)
	}
	slices.Sort(identityKeys)
	entities := make([]contract.ProductionEntityFragment, 0, len(identityKeys))
	for _, identityKey := range identityKeys {
		material := materials[identityKey]
		slices.Sort(material.scopes)
		material.scopes = slices.Compact(material.scopes)
		basis := contract.ProductionSourceBasis{
			Provenance: "source_explicit", Evidence: material.evidence,
		}
		value := material.canonicalName
		entities = append(entities, contract.ProductionEntityFragment{
			IdentityKey: identityKey, Kind: material.kind,
			SpecificationKey: "specification_" + strings.ReplaceAll(identityKey, "-", "_"),
			SpecificationSlots: []contract.ProductionSemanticSlot{{
				SlotKey: "canonical_name", Resolution: "known", Value: &value,
			}},
			Basis: basis,
			States: []contract.ProductionStateFragment{{
				StateKey: "state_" + strings.ReplaceAll(identityKey, "-", "_") + "_initial",
				StateKind: map[string]string{
					"character": "character_appearance", "location": "location_state", "prop": "prop_state",
				}[material.kind],
				CompleteSlots: []contract.ProductionSemanticSlot{{
					SlotKey: "canonical_name", Resolution: "known", Value: &value,
				}},
				ApplicableSceneScopeKeys: material.scopes,
				EntryReason:              "该实体首次出现在冻结场景事实中。",
				ExitReason:               "当前冻结场景范围结束。",
				Basis:                    basis,
			}},
		})
	}
	worldClaims := []contract.ProductionWorldClaimFragment{}
	if len(entities) > 0 {
		worldClaims = append(worldClaims, contract.ProductionWorldClaimFragment{
			ClaimKey: "claim_primary_identity_exists", ClaimType: "world_rule",
			SubjectIdentityKeys: []string{entities[0].IdentityKey}, Statement: "主要叙事身份存在于当前制作世界。",
			Basis: entities[0].Basis,
		})
	}
	return mustSceneJSON(contract.ProductionEntityFragmentCandidate{
		SourceVersionID: input.SourceVersionID, SourceHash: input.SourceHash,
		StructureIdentitySetVersionID:   input.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash: input.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:    input.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:  input.SceneFactCandidateRevisionHash,
		Entities:                        entities, WorldClaims: worldClaims,
		DesignGaps: []contract.ProductionDesignGap{}, ReviewIssues: []contract.CandidateReviewIssue{},
	})
}

func buildSceneBindingCandidate(input contract.SceneOccurrenceBindingInput) json.RawMessage {
	var facts contract.SceneFactCandidate
	var production contract.ProductionEntityFragmentCandidate
	_ = json.Unmarshal(input.SceneFactCandidate, &facts)
	_ = json.Unmarshal(input.ProductionEntityCandidate, &production)
	factByScene := make(map[string]contract.SceneFact, len(facts.Scenes))
	for _, fact := range facts.Scenes {
		factByScene[fact.TemporarySceneID] = fact
	}
	stateByIdentity := make(map[string]string, len(production.Entities))
	for _, entity := range production.Entities {
		stateByIdentity[entity.IdentityKey] = entity.States[0].StateKey
	}
	mappingsByScene := make(map[string][]contract.FrozenStructureIdentityMentionMapping)
	for _, mapping := range input.StructureIdentitySet.MentionMappings {
		if mapping.Resolution == "resolved" {
			mappingsByScene[mapping.TemporarySceneID] = append(
				mappingsByScene[mapping.TemporarySceneID], mapping,
			)
		}
	}
	for sceneID := range mappingsByScene {
		slices.SortFunc(mappingsByScene[sceneID], func(left, right contract.FrozenStructureIdentityMentionMapping) int {
			return strings.Compare(sceneBindingMappingTestKey(left), sceneBindingMappingTestKey(right))
		})
	}
	scenes := make([]contract.SceneBindingFragment, 0, len(input.StructureIdentitySet.SceneRefs))
	for _, formal := range input.StructureIdentitySet.SceneRefs {
		fact := factByScene[formal.TemporarySceneID]
		beats := make([]contract.SceneBeatFragment, len(fact.Actions))
		for index, action := range fact.Actions {
			beats[index] = contract.SceneBeatFragment{
				BeatKey: fmt.Sprintf("beat_%s_%04d", formal.TemporarySceneID, index+1),
				Order:   index + 1, Text: action.Text, Evidence: action.Evidence,
			}
		}
		dialogues := make([]contract.SceneDialogueFragment, len(fact.Dialogues))
		for index, dialogue := range fact.Dialogues {
			dialogues[index] = contract.SceneDialogueFragment{
				DialogueKey: fmt.Sprintf("dialogue_%s_%04d", formal.TemporarySceneID, index+1),
				Order:       index + 1, Text: dialogue.Text, Evidence: dialogue.Evidence,
			}
		}
		mappings := mappingsByScene[formal.TemporarySceneID]
		occurrences := make([]contract.SceneOccurrenceFragment, len(mappings))
		for index, mapping := range mappings {
			occurrences[index] = contract.SceneOccurrenceFragment{
				OccurrenceKey: fmt.Sprintf("occurrence_%s_%04d", formal.TemporarySceneID, index+1),
				Order:         index + 1, SubjectKind: mapping.Kind, IdentityKey: *mapping.IdentityKey,
				StateKey: stateByIdentity[*mapping.IdentityKey], OccurrenceRole: mapping.OccurrenceRole,
				Evidence: contract.SourceEvidenceSpan{
					SourceStart: mapping.SourceStart, SourceEnd: mapping.SourceEnd,
					TextHash: mapping.TextHash, ExactAnchor: mapping.ExactAnchor,
				},
			}
		}
		scenes = append(scenes, contract.SceneBindingFragment{
			SceneScopeKey: formal.ScopeKey, SceneOwnerLogicalID: formal.SceneOwnerLogicalID,
			TemporarySceneID: formal.TemporarySceneID,
			SourceStart:      formal.SourceStart, SourceEnd: formal.SourceEnd,
			Dialogues: dialogues, Beats: beats, Occurrences: occurrences,
		})
	}
	return mustSceneJSON(contract.SceneBindingFragmentCandidate{
		SourceVersionID: input.SourceVersionID, SourceHash: input.SourceHash,
		StructureIdentitySetVersionID:         input.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash:       input.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:          input.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:        input.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   input.ProductionEntityCandidateRevisionID,
		ProductionEntityCandidateRevisionHash: input.ProductionEntityCandidateRevisionHash,
		Scenes:                                scenes, ReviewIssues: []contract.CandidateReviewIssue{},
	})
}

func buildInteractionContinuityCandidate(input contract.InteractionContinuityInput) json.RawMessage {
	var bindings contract.SceneBindingFragmentCandidate
	var facts contract.SceneFactCandidate
	_ = json.Unmarshal(input.SceneBindingCandidate, &bindings)
	_ = json.Unmarshal(input.SceneFactCandidate, &facts)
	first, second := bindings.Scenes[0], bindings.Scenes[1]
	var actor, prop, firstLocation contract.SceneOccurrenceFragment
	for _, occurrence := range first.Occurrences {
		switch occurrence.SubjectKind {
		case "character":
			actor = occurrence
		case "prop":
			prop = occurrence
		case "location":
			firstLocation = occurrence
		}
	}
	var nextActor, secondLocation contract.SceneOccurrenceFragment
	for _, occurrence := range second.Occurrences {
		switch occurrence.SubjectKind {
		case "character":
			nextActor = occurrence
		case "location":
			secondLocation = occurrence
		}
	}
	holder := actor.IdentityKey
	firstLocationIdentity := firstLocation.IdentityKey
	secondLocationIdentity := secondLocation.IdentityKey
	transitionKey := "interaction_scene_0001_0001"
	candidate := mustSceneJSON(contract.InteractionContinuityCandidate{
		SourceVersionID: input.SourceVersionID, SourceHash: input.SourceHash,
		StructureIdentitySetVersionID:         input.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash:       input.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:          input.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:        input.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   input.ProductionEntityCandidateRevisionID,
		ProductionEntityCandidateRevisionHash: input.ProductionEntityCandidateRevisionHash,
		SceneBindingCandidateRevisionID:       input.SceneBindingCandidateRevisionID,
		SceneBindingCandidateRevisionHash:     input.SceneBindingCandidateRevisionHash,
		SceneStoryTimes: []contract.SceneStoryTimeFragment{
			{SceneScopeKey: first.SceneScopeKey, StoryTimeKey: "storytime:00000001"},
			{SceneScopeKey: second.SceneScopeKey, StoryTimeKey: "storytime:00000002"},
		},
		Interactions: []contract.InteractionFragment{{
			InteractionKey: transitionKey,
			ClaimSeriesKey: "interaction_series_door_handle_hold", ClaimRevision: 1,
			SceneScopeKey: first.SceneScopeKey, BeatKey: &first.Beats[0].BeatKey,
			StoryTimeKey: "storytime:00000001", Predicate: "hold", Hand: "unspecified",
			ActorOccurrenceKey: actor.OccurrenceKey, PropOccurrenceKey: prop.OccurrenceKey,
			HolderAfterIdentityKey: &holder,
			PropStateBeforeKey:     prop.StateKey, PropStateAfterKey: prop.StateKey,
			Evidence: facts.Scenes[0].Actions[0].Evidence,
		}},
		ContinuityLedger: []contract.ContinuityLedgerEntry{
			{
				LedgerKey: "ledger_character_linzhou_scene_0001", SubjectKind: "character",
				IdentityKey: actor.IdentityKey, SceneScopeKey: first.SceneScopeKey,
				StoryTimeKey: "storytime:00000001", StateKey: actor.StateKey,
				LocationIdentityKey: &firstLocationIdentity,
				Evidence:            []contract.SourceEvidenceSpan{firstLocation.Evidence, actor.Evidence},
			},
			{
				LedgerKey: "ledger_prop_door_handle_scene_0001", SubjectKind: "prop",
				IdentityKey: prop.IdentityKey, SceneScopeKey: first.SceneScopeKey,
				StoryTimeKey: "storytime:00000001", StateKey: prop.StateKey,
				HolderIdentityKey: &holder, LocationIdentityKey: &firstLocationIdentity,
				TransitionInteractionKey: &transitionKey,
				Evidence:                 []contract.SourceEvidenceSpan{firstLocation.Evidence, prop.Evidence},
			},
			{
				LedgerKey: "ledger_character_linzhou_scene_0002", SubjectKind: "character",
				IdentityKey: nextActor.IdentityKey, SceneScopeKey: second.SceneScopeKey,
				StoryTimeKey: "storytime:00000002", StateKey: nextActor.StateKey,
				LocationIdentityKey: &secondLocationIdentity,
				Evidence:            []contract.SourceEvidenceSpan{secondLocation.Evidence, nextActor.Evidence},
			},
		},
		Continuity: []contract.ContinuityFragment{{
			ContinuityKey:  "continuity_character_linzhou_0001",
			ClaimSeriesKey: "continuity_series_character_linzhou", ClaimRevision: 1,
			SubjectKind: "character",
			IdentityKey: actor.IdentityKey, FromSceneScopeKey: first.SceneScopeKey,
			ToSceneScopeKey: second.SceneScopeKey, BeforeStateKey: actor.StateKey,
			AfterStateKey: nextActor.StateKey, Transition: "state_persists",
			StoryTimeStart: "storytime:00000001", StoryTimeEnd: "storytime:00000002",
			Evidence: []contract.SourceEvidenceSpan{actor.Evidence, nextActor.Evidence},
		}},
		ReviewIssues: []contract.CandidateReviewIssue{},
	})
	if err := contract.ValidateInteractionContinuityCandidate(candidate, input); err != nil {
		panic("invalid deterministic Interaction/Continuity fixture: " + err.Error())
	}
	return candidate
}

func buildSpanCandidate(input contract.ScriptSpanProposalInput) json.RawMessage {
	text := []rune(input.NormalizedText)
	second := runeIndex(input.NormalizedText, "第二场")
	return mustSceneJSON(map[string]any{
		"source_version_id": input.SourceVersionID, "source_hash": input.SourceHash,
		"codepoint_count": len(text),
		"coverage":        map[string]any{"source_hash": input.SourceHash, "codepoint_start": 0, "codepoint_end": len(text), "covered_codepoints": len(text)},
		"episodes": []any{map[string]any{
			"temporary_episode_id": "episode_0001", "position": 1,
			"codepoint_start": 0, "codepoint_end": len(text), "heading": nil, "evidence": nil,
			"scene_span_ids": []string{"span_0001", "span_0002"},
		}},
		"spans": []any{
			map[string]any{"temporary_span_id": "span_0001", "episode_span_id": "episode_0001", "kind": "scene", "codepoint_start": 0, "codepoint_end": second, "heading": "第一场 夜 内", "evidence": sceneEvidence(input.NormalizedText, 0, runeIndex(input.NormalizedText, "\n"))},
			map[string]any{"temporary_span_id": "span_0002", "episode_span_id": "episode_0001", "kind": "scene", "codepoint_start": second, "codepoint_end": len(text), "heading": "第二场 日 外", "evidence": sceneEvidence(input.NormalizedText, second, second+runeIndex(string(text[second:]), "\n"))},
		},
		"review_issues": []any{},
	})
}

func buildSceneFactCandidate(input contract.SceneFactExtractionInput) json.RawMessage {
	var spans contract.ScriptSpanCandidate
	_ = json.Unmarshal(input.SpanCandidate, &spans)
	scenes := make([]any, len(spans.Spans))
	for index, span := range spans.Spans {
		timeText, locationText, actionText := "夜", "内", "林舟握住门把。"
		if index == 1 {
			timeText, locationText, actionText = "日", "外", "林舟离开。"
		}
		timeStart := runeIndexFrom(input.NormalizedText, timeText, span.CodepointStart)
		locationStart := runeIndexFrom(input.NormalizedText, locationText, timeStart+1)
		nameStart := runeIndexFrom(input.NormalizedText, "林舟", span.CodepointStart)
		actionStart := runeIndexFrom(input.NormalizedText, actionText, span.CodepointStart)
		rawProps := []any{}
		if index == 0 {
			propStart := runeIndexFrom(input.NormalizedText, "门把", actionStart)
			rawProps = []any{map[string]any{
				"text": "门把", "occurrence_role": "actual",
				"evidence": sceneEvidence(input.NormalizedText, propStart, propStart+2),
			}}
		}
		scenes[index] = map[string]any{
			"temporary_scene_id": fmt.Sprintf("scene_%04d", index+1), "span_id": span.TemporarySpanID,
			"source_start": span.CodepointStart, "source_end": span.CodepointEnd,
			"location": map[string]any{"text": locationText, "evidence": sceneEvidence(input.NormalizedText, locationStart, locationStart+1)},
			"time":     map[string]any{"text": timeText, "evidence": sceneEvidence(input.NormalizedText, timeStart, timeStart+1)},
			"actions": []any{map[string]any{
				"text":     actionText,
				"evidence": sceneEvidence(input.NormalizedText, actionStart, actionStart+len([]rune(actionText))),
			}}, "dialogues": []any{},
			"raw_character_mentions": []any{map[string]any{"text": "林舟", "occurrence_role": "actual", "evidence": sceneEvidence(input.NormalizedText, nameStart, nameStart+2)}},
			"raw_prop_mentions":      rawProps,
		}
	}
	return mustSceneJSON(map[string]any{
		"source_version_id": input.SourceVersionID, "source_hash": input.SourceHash,
		"span_candidate_revision_id":   input.SpanCandidateRevisionID,
		"span_candidate_revision_hash": input.SpanCandidateRevisionHash,
		"scenes":                       scenes, "review_issues": []any{},
	})
}

func buildIdentityResolutionCandidate(input contract.IdentityResolutionInput) json.RawMessage {
	var facts contract.SceneFactCandidate
	_ = json.Unmarshal(input.SceneFactCandidate, &facts)
	characterRefs := make([]contract.IdentityMentionRef, 0, len(facts.Scenes))
	characterEvidence := make([]contract.SourceEvidenceSpan, 0, len(facts.Scenes))
	locationClusters := make([]contract.IdentityCluster, 0, len(facts.Scenes))
	propClusters := make([]contract.IdentityCluster, 0)
	universe := make([]contract.IdentityMentionRef, 0, len(facts.Scenes)*3)
	for _, scene := range facts.Scenes {
		for _, mention := range scene.RawCharacterMentions {
			ref := contract.IdentityMentionRef{
				Kind: "character", OccurrenceRole: mention.OccurrenceRole,
				TemporarySceneID: scene.TemporarySceneID,
				SourceStart:      mention.Evidence.SourceStart, SourceEnd: mention.Evidence.SourceEnd,
				TextHash: mention.Evidence.TextHash, ExactAnchor: mention.Evidence.ExactAnchor,
			}
			characterRefs = append(characterRefs, ref)
			characterEvidence = append(characterEvidence, mention.Evidence)
			universe = append(universe, ref)
		}
		if scene.Location != nil {
			ref := contract.IdentityMentionRef{
				Kind: "location", OccurrenceRole: "actual", TemporarySceneID: scene.TemporarySceneID,
				SourceStart: scene.Location.Evidence.SourceStart, SourceEnd: scene.Location.Evidence.SourceEnd,
				TextHash: scene.Location.Evidence.TextHash, ExactAnchor: scene.Location.Evidence.ExactAnchor,
			}
			universe = append(universe, ref)
			locationClusters = append(locationClusters, contract.IdentityCluster{
				TemporaryIdentityKey: "identity_location_" + scene.TemporarySceneID,
				Kind:                 "location", Resolution: "new", CanonicalName: scene.Location.Text,
				Aliases: []string{scene.Location.Text}, MentionRefs: []contract.IdentityMentionRef{ref},
				SupportingEvidence:    []contract.SourceEvidenceSpan{scene.Location.Evidence},
				ContradictingEvidence: []contract.SourceEvidenceSpan{}, ConfidenceBasisPoints: 10000,
				Rationale: "地点属性来自当前场景的精确原文证据。",
			})
		}
		for _, mention := range scene.RawPropMentions {
			ref := contract.IdentityMentionRef{
				Kind: "prop", OccurrenceRole: mention.OccurrenceRole,
				TemporarySceneID: scene.TemporarySceneID,
				SourceStart:      mention.Evidence.SourceStart, SourceEnd: mention.Evidence.SourceEnd,
				TextHash: mention.Evidence.TextHash, ExactAnchor: mention.Evidence.ExactAnchor,
			}
			universe = append(universe, ref)
			propClusters = append(propClusters, contract.IdentityCluster{
				TemporaryIdentityKey: "identity_prop_door_handle", Kind: "prop", Resolution: "new",
				CanonicalName: mention.Text, Aliases: []string{mention.Text},
				MentionRefs:           []contract.IdentityMentionRef{ref},
				SupportingEvidence:    []contract.SourceEvidenceSpan{mention.Evidence},
				ContradictingEvidence: []contract.SourceEvidenceSpan{}, ConfidenceBasisPoints: 10000,
				Rationale: "道具提及来自当前场景的精确原文证据。",
			})
		}
	}
	slices.SortFunc(universe, func(left, right contract.IdentityMentionRef) int {
		return strings.Compare(identityMentionTestKey(left), identityMentionTestKey(right))
	})
	universeHash, _ := contract.ProductionCanonicalHash(mustSceneJSON(universe))
	clusters := []contract.IdentityCluster{{
		TemporaryIdentityKey: "identity_character_linzhou", Kind: "character", Resolution: "new",
		CanonicalName: "林舟", Aliases: []string{"林舟"}, MentionRefs: characterRefs,
		SupportingEvidence: characterEvidence, ContradictingEvidence: []contract.SourceEvidenceSpan{},
		ConfidenceBasisPoints: 9800, Rationale: "两个场景中的同名角色提及没有相互矛盾的证据。",
	}}
	clusters = append(clusters, locationClusters...)
	clusters = append(clusters, propClusters...)
	return mustSceneJSON(contract.IdentityResolutionCandidate{
		SourceVersionID: input.SourceVersionID, SourceHash: input.SourceHash,
		SceneFactCandidateRevisionID:   input.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash: input.SceneFactCandidateRevisionHash,
		ResolvedClusters:               clusters,
		AmbiguousMentions:              []contract.AmbiguousIdentityMention{},
		RejectedMentions:               []contract.RejectedIdentityMention{},
		Coverage: contract.IdentityResolutionCoverage{
			MentionCount: len(universe), ResolvedCount: len(universe),
			MentionUniverseHash: universeHash,
		},
		ReviewIssues: []contract.CandidateReviewIssue{},
	})
}

func productionWorldBusinessKeyRoot(t *testing.T, value worlddomain.ProductionWorldCandidate, partition string) string {
	t.Helper()
	for _, root := range value.SharedProof.ExpectedBusinessKeyRoots {
		if root.Partition == partition {
			return root.Root
		}
	}
	t.Fatalf("Production World business key root %q not found", partition)
	return ""
}

func productionWorldOccurrenceCount(scenes []contract.SceneBindingFragment) int {
	count := 0
	for _, scene := range scenes {
		count += len(scene.Occurrences)
	}
	return count
}

func productionWorldDialogueCount(scenes []contract.SceneBindingFragment) int {
	count := 0
	for _, scene := range scenes {
		count += len(scene.Dialogues)
	}
	return count
}

func productionWorldBeatCount(scenes []contract.SceneBindingFragment) int {
	count := 0
	for _, scene := range scenes {
		count += len(scene.Beats)
	}
	return count
}

func identityMentionTestKey(value contract.IdentityMentionRef) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%020d\x00%020d\x00%s\x00%s", value.Kind, value.OccurrenceRole, value.TemporarySceneID,
		value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor)
}

func sceneBindingMappingTestKey(value contract.FrozenStructureIdentityMentionMapping) string {
	identityKey := ""
	if value.IdentityKey != nil {
		identityKey = *value.IdentityKey
	}
	return fmt.Sprintf("%020d\x00%020d\x00%s\x00%s", value.SourceStart, value.SourceEnd, value.Kind, identityKey)
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
