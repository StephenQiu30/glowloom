package workflow_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type referenceTargetBuilderStub struct {
	result generationapp.BuildReferenceTargetsResult
	calls  []referenceTargetBuildCall
}

type referenceTargetBuildCall struct {
	actor   generationapp.Actor
	command generationapp.BuildReferenceTargetsCommand
}

func (stub *referenceTargetBuilderStub) BuildReferenceTargets(
	_ context.Context,
	actor generationapp.Actor,
	command generationapp.BuildReferenceTargetsCommand,
) (generationapp.BuildReferenceTargetsResult, error) {
	stub.calls = append(stub.calls, referenceTargetBuildCall{actor: actor, command: command})
	return stub.result, nil
}

type imagePreparationStub struct {
	results []generationapp.PreparationResult
	calls   []imagePreparationCall
}

type imagePreparationCall struct {
	actor   generationapp.Actor
	command generationapp.PrepareImageGenerationCommand
}

func (stub *imagePreparationStub) PrepareImageGeneration(
	_ context.Context,
	actor generationapp.Actor,
	command generationapp.PrepareImageGenerationCommand,
) (generationapp.PreparationResult, error) {
	stub.calls = append(stub.calls, imagePreparationCall{actor: actor, command: command})
	index := len(stub.calls) - 1
	if index >= len(stub.results) {
		index = len(stub.results) - 1
	}
	return stub.results[index], nil
}

type executionClaimStub struct {
	results []generationapp.ExecutionClaimResult
	calls   []generationapp.AcquireExecutionClaimCommand
}

func (stub *executionClaimStub) AcquireExecutionClaim(
	_ context.Context,
	command generationapp.AcquireExecutionClaimCommand,
) (generationapp.ExecutionClaimResult, error) {
	stub.calls = append(stub.calls, command)
	index := len(stub.calls) - 1
	if index >= len(stub.results) {
		index = len(stub.results) - 1
	}
	return stub.results[index], nil
}

type imageProviderStub struct {
	submitResult    generationapp.ProviderExecutionResult
	reconcileResult generationapp.ProviderExecutionResult
	submitCalls     []providerSubmitCall
	reconcileCalls  []generationapp.ReconcileProviderJobCommand
}

type providerSubmitCall struct {
	authorization generationdomain.ExecutionAuthorization
	command       generationapp.SubmitImageRequestCommand
}

func (stub *imageProviderStub) SubmitImageRequest(
	_ context.Context,
	authorization generationdomain.ExecutionAuthorization,
	command generationapp.SubmitImageRequestCommand,
) (generationapp.ProviderExecutionResult, error) {
	stub.submitCalls = append(stub.submitCalls, providerSubmitCall{authorization: authorization, command: command})
	return stub.submitResult, nil
}

func (stub *imageProviderStub) ReconcileProviderJob(
	_ context.Context,
	command generationapp.ReconcileProviderJobCommand,
) (generationapp.ProviderExecutionResult, error) {
	stub.reconcileCalls = append(stub.reconcileCalls, command)
	return stub.reconcileResult, nil
}

func TestReferenceAssetExecutorSelectsOneApprovedTargetAndPreparesItOncePerNode(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("a", 64)
	first := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	second := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{first, second},
	}}
	preparedIntent := generationdomain.Intent{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, TargetID: second.ID,
		TargetHash: second.TargetHash, Units: 4, Status: generationdomain.IntentPrepared,
		CostReservationID: uuid.NewString(), QuotaReservationID: uuid.NewString(),
	}
	claimant, claimToken, claimExpiresAt := "workflow-node:"+nodeRunID, uuid.NewString(), time.Now().UTC().Add(5*time.Minute)
	claimedIntent := preparedIntent
	claimedIntent.Status, claimedIntent.Revision = generationdomain.IntentClaimed, 3
	claimedIntent.Claimant = &claimant
	claimedIntent.ClaimToken, claimedIntent.ClaimExpiresAt = &claimToken, &claimExpiresAt
	claimedIntent.ClaimFencingVersion = 1
	providerRequestID, providerJobID := uuid.NewString(), uuid.NewString()
	unknownIntent := claimedIntent
	unknownIntent.Status, unknownIntent.Revision = generationdomain.IntentOutcomeUnknown, 5
	unknownIntent.GenerationRequestID, unknownIntent.ProviderJobID = providerRequestID, providerJobID
	authorization := generationdomain.ExecutionAuthorization{
		IntentID: claimedIntent.ID, ClaimToken: claimToken, TargetID: second.ID, TargetHash: second.TargetHash,
		CostReservationID: claimedIntent.CostReservationID, QuotaReservationID: claimedIntent.QuotaReservationID,
		ClaimFencingVersion: 1, IntentRevision: 3, Units: 4, ExpiresAt: claimExpiresAt,
	}
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{
		{IntentView: generationapp.IntentView{Intent: preparedIntent}},
		{IntentView: generationapp.IntentView{Intent: unknownIntent}},
	}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{
		{Intent: claimedIntent, Authorization: authorization},
		{Intent: unknownIntent, Authorization: authorization},
	}}
	providerResult := generationapp.ProviderExecutionResult{
		Intent: unknownIntent,
		Request: generationdomain.GenerationRequest{
			ID: providerRequestID, IntentID: unknownIntent.ID, TargetID: second.ID, TargetHash: second.TargetHash,
		},
		Job: generationdomain.ProviderJob{
			ID: providerJobID, IntentID: unknownIntent.ID, RequestID: providerRequestID,
			Status: generationdomain.ProviderJobUnknown, Revision: 2,
		},
	}
	providers := &imageProviderStub{submitResult: providerResult, reconcileResult: providerResult}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		second.ReferenceAsset.AssetID, second.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil || result.Status != "RETRYING" || result.Output.SchemaVersion != "" || len(result.Output.Bindings) != 0 {
		t.Fatalf("prepare reference asset Workflow node: result=%#v err=%v", result, err)
	}
	command.Attempt = 2
	command.IdempotencyKey = "workflow-node-redelivery:" + nodeRunID
	result, err = executor.Execute(context.Background(), command)
	if err != nil || result.Status != "RETRYING" {
		t.Fatalf("replay reference asset Workflow node: result=%#v err=%v", result, err)
	}
	if len(builder.calls) != 2 || len(preparations.calls) != 2 {
		t.Fatalf("reference asset call counts: builder=%d preparation=%d", len(builder.calls), len(preparations.calls))
	}
	for index := range builder.calls {
		build := builder.calls[index]
		prepare := preparations.calls[index]
		if build.actor.UserID != userID || build.actor.TokenVersion != 3 ||
			build.command.ApprovedIntentSetID != approvedID || build.command.ExpectedContentHash != approvedHash ||
			build.command.IdempotencyKey != "workflow-run:"+workflowRunID+":reference-targets" ||
			prepare.actor != build.actor || prepare.command.WorkspaceID != workspaceID || prepare.command.ProjectID != projectID ||
			prepare.command.WorkflowRunID != workflowRunID || prepare.command.NodeRunID != nodeRunID ||
			prepare.command.WorkflowInputHash != command.InputHash || prepare.command.TargetID != second.ID ||
			prepare.command.TargetHash != second.TargetHash || prepare.command.Units != 4 ||
			prepare.command.IdempotencyKey != "workflow-node:"+nodeRunID+":generation-prepare" {
			t.Fatalf("reference asset execution binding drifted: build=%#v prepare=%#v", build, prepare)
		}
	}
	if len(claims.calls) != 2 || claims.calls[0].IntentID != preparedIntent.ID ||
		claims.calls[0].Claimant != "workflow-node:"+nodeRunID ||
		claims.calls[0].IdempotencyKey != "workflow-node:"+nodeRunID+":generation-claim" ||
		claims.calls[1] != claims.calls[0] {
		t.Fatalf("reference asset execution claim calls drifted: %#v", claims.calls)
	}
	if len(providers.submitCalls) != 1 || providers.submitCalls[0].authorization != authorization ||
		providers.submitCalls[0].command.IntentID != preparedIntent.ID ||
		providers.submitCalls[0].command.IdempotencyKey != "workflow-node:"+nodeRunID+":provider-submit" ||
		len(providers.reconcileCalls) != 1 || providers.reconcileCalls[0].ProviderJobID != providerJobID ||
		providers.reconcileCalls[0].IdempotencyKey != "workflow-node:"+nodeRunID+":provider-reconcile:5" {
		t.Fatalf("reference asset Provider calls drifted: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
}

func TestReferenceAssetExecutorRejectsClaimOwnedByAnotherNodeBeforeProviderSubmission(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("c", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	prepared := generationdomain.Intent{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, TargetID: target.ID,
		TargetHash: target.TargetHash, Units: int64(target.ReferenceAsset.NumberResults),
		Status: generationdomain.IntentPrepared, CostReservationID: uuid.NewString(), QuotaReservationID: uuid.NewString(),
	}
	wrongClaimant, claimToken, claimExpiresAt := "workflow-node:"+uuid.NewString(), uuid.NewString(), time.Now().UTC().Add(5*time.Minute)
	claimed := prepared
	claimed.Status, claimed.Revision = generationdomain.IntentClaimed, 3
	claimed.Claimant, claimed.ClaimToken, claimed.ClaimExpiresAt = &wrongClaimant, &claimToken, &claimExpiresAt
	claimed.ClaimFencingVersion = 1
	authorization := generationdomain.ExecutionAuthorization{
		IntentID: claimed.ID, ClaimToken: claimToken, TargetID: target.ID, TargetHash: target.TargetHash,
		CostReservationID: claimed.CostReservationID, QuotaReservationID: claimed.QuotaReservationID,
		ClaimFencingVersion: 1, IntentRevision: 3, Units: claimed.Units, ExpiresAt: claimExpiresAt,
	}
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: prepared},
	}}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{
		Intent: claimed, Authorization: authorization,
	}}}
	providers := &imageProviderStub{}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)

	if _, err := executor.Execute(context.Background(), command); err == nil ||
		!strings.Contains(err.Error(), "execution claim returned drifted facts") {
		t.Fatalf("reference asset Workflow node accepted another node's claim: %v", err)
	}
	if len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 0 {
		t.Fatalf("drifted execution claim reached Provider: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
}

func TestReferenceAssetExecutorRejectsAmbiguousOrUnselectedTargetsBeforeCostPreparation(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("b", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	duplicate := target
	duplicate.ID = uuid.NewString()
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target, duplicate},
	}}
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{}}}
	claims, providers := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{}}}, &imageProviderStub{}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, uuid.NewString(), uuid.NewString(), approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)
	if _, err := executor.Execute(context.Background(), command); err == nil {
		t.Fatal("reference asset Workflow node accepted two targets for one NodeRun")
	}
	if len(preparations.calls) != 0 {
		t.Fatalf("ambiguous target reached Cost/Quota preparation: %#v", preparations.calls)
	}

	builder.result.Targets = []generationdomain.GenerationTarget{target}
	command = referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, uuid.NewString(), uuid.NewString(), approvedID, approvedHash,
		uuid.NewString(), uuid.NewString(),
	)
	if _, err := executor.Execute(context.Background(), command); err == nil {
		t.Fatal("reference asset Workflow node accepted a selector absent from approved targets")
	}
	if len(preparations.calls) != 0 {
		t.Fatalf("missing target reached Cost/Quota preparation: %#v", preparations.calls)
	}

	command = referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, uuid.NewString(), uuid.NewString(), approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)
	command.Input.Config = json.RawMessage(`{"asset_id":"` + target.ReferenceAsset.AssetID +
		`","asset_state_id":"` + target.ReferenceAsset.AssetStateRef.ID + `","target_id":"` + target.ID + `"}`)
	normalizedInput, _, invalidConfigHash, buildErr := workflow.BuildNodeInput(command.Input)
	if buildErr != nil {
		t.Fatalf("build strict reference config input: %v", buildErr)
	}
	command.Input, command.InputHash = normalizedInput, invalidConfigHash
	buildCallsBeforeInvalidConfig := len(builder.calls)
	if _, err := executor.Execute(context.Background(), command); err == nil {
		t.Fatal("reference asset Workflow node accepted an undeclared config field")
	}
	if len(builder.calls) != buildCallsBeforeInvalidConfig || len(preparations.calls) != 0 {
		t.Fatalf("invalid reference config reached owners: builder=%d preparation=%d", len(builder.calls), len(preparations.calls))
	}
}

func referenceAssetExecutorCommand(
	t *testing.T,
	workspaceID, projectID, userID, workflowRunID, nodeRunID string,
	approvedID, approvedHash, assetID, assetStateID string,
) workflow.NodeExecutorCommand {
	t.Helper()
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{"asset_id":"` + assetID + `","asset_state_id":"` + assetStateID + `"}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "intents", ValueType: "approved_storyboard_intents", SourceKind: workflow.NodeInputSourceNodeOutput,
			SourceNodeID: "intent-review", SourcePort: "intents", ReferenceID: approvedID,
			ReferenceVersion: "1", ContentHash: approvedHash,
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("f", 64),
		}},
	})
	if err != nil {
		t.Fatalf("build reference asset node input: %v", err)
	}
	return workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, NodeID: "reference-character",
			Executor: "activity.reference_asset_generation", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID, InitiatorUserID: userID, InitiatorTokenVersion: 3,
		IdempotencyKey: "workflow-node:" + nodeRunID + ":attempt:1", Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidates", ValueType: "generation_candidate_set", Required: true}},
	}
}

func newReferenceExecutorTarget(
	t *testing.T,
	workspaceID, projectID, userID, approvedID, approvedHash string,
) generationdomain.GenerationTarget {
	t.Helper()
	target, err := generationdomain.NewGenerationTarget(generationdomain.GenerationTargetInput{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		Kind: generationdomain.GenerationTargetReferenceAsset,
		SourceOwnerRef: generationdomain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents", ID: approvedID,
			Revision: 1, ContentHash: approvedHash,
		},
		PolicySnapshotRef: generationdomain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot", ID: uuid.NewString(),
			Revision: 1, ContentHash: strings.Repeat("c", 64),
		},
		ReferenceAsset: &generationdomain.ReferenceAssetTarget{
			AssetID: uuid.NewString(), AssetKind: "character",
			SpecificationVersionRef: generationdomain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("d", 64),
			},
			AssetStateRef: generationdomain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("e", 64),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"front", "profile", "back"},
			PromptVersion: "character-reference-sheet-v1", PositivePrompt: "character reference sheet",
			NegativePrompt: "identity drift", Width: 1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
		},
		Revision: 1, CreatedBy: userID, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("build reference asset Target fixture: %v", err)
	}
	return target
}
