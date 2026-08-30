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
	submitErr       error
	reconcileErr    error
	submitCalls     []providerSubmitCall
	reconcileCalls  []generationapp.ReconcileProviderJobCommand
}

type providerSubmitCall struct {
	authorization generationdomain.ExecutionAuthorization
	command       generationapp.SubmitImageRequestCommand
}

type providerOutputMaterializerStub struct {
	result generationapp.OutputMaterializationResult
	err    error
	calls  []providerOutputMaterializationCall
}

type providerOutputMaterializationCall struct {
	actor   generationapp.Actor
	command generationapp.MaterializeProviderOutputsCommand
}

func (stub *providerOutputMaterializerStub) MaterializeSucceededOutputs(
	_ context.Context,
	actor generationapp.Actor,
	command generationapp.MaterializeProviderOutputsCommand,
) (generationapp.OutputMaterializationResult, error) {
	stub.calls = append(stub.calls, providerOutputMaterializationCall{actor: actor, command: command})
	return stub.result, stub.err
}

func (stub *imageProviderStub) SubmitImageRequest(
	_ context.Context,
	authorization generationdomain.ExecutionAuthorization,
	command generationapp.SubmitImageRequestCommand,
) (generationapp.ProviderExecutionResult, error) {
	stub.submitCalls = append(stub.submitCalls, providerSubmitCall{authorization: authorization, command: command})
	return stub.submitResult, stub.submitErr
}

func (stub *imageProviderStub) ReconcileProviderJob(
	_ context.Context,
	command generationapp.ReconcileProviderJobCommand,
) (generationapp.ProviderExecutionResult, error) {
	stub.reconcileCalls = append(stub.reconcileCalls, command)
	return stub.reconcileResult, stub.reconcileErr
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
	preparedIntent := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, second)
	claimedIntent, authorization := claimReferenceIntent(preparedIntent, "workflow-node:"+nodeRunID)
	executingIntent := executeReferenceIntent(claimedIntent)
	succeededIntent := completeReferenceIntent(executingIntent, generationdomain.IntentPartialSucceeded)
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{
		{IntentView: generationapp.IntentView{Intent: preparedIntent}},
		{IntentView: generationapp.IntentView{Intent: executingIntent}},
		{IntentView: generationapp.IntentView{Intent: succeededIntent}},
		{IntentView: generationapp.IntentView{Intent: succeededIntent}},
	}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{
		{Intent: claimedIntent, Authorization: authorization},
		{Intent: executingIntent, Authorization: authorization},
	}}
	submitResult := referenceProviderResult(executingIntent, second, generationdomain.ProviderJobRunning)
	succeededProviderResult := referenceProviderResult(succeededIntent, second, generationdomain.ProviderJobPartialSucceeded)
	for index := range succeededProviderResult.Calls {
		succeededProviderResult.Calls[index].ID = submitResult.Calls[index].ID
		succeededProviderResult.Calls[index].CallKey = submitResult.Calls[index].CallKey
		succeededProviderResult.Calls[index].RemoteRequestID = submitResult.Calls[index].RemoteRequestID
		succeededProviderResult.Calls[index].RemoteJobID = submitResult.Calls[index].RemoteJobID
		succeededProviderResult.Receipts[index].CallID = submitResult.Calls[index].ID
	}
	providerReceiptSetHash := strings.Repeat("6", 64)
	candidateSet := generationdomain.CandidateSet{
		ID: executingIntent.ProviderJobID, WorkspaceID: workspaceID, ProjectID: projectID,
		ProviderReceiptSetHash: providerReceiptSetHash,
		Revision:               1, ContentHash: strings.Repeat("9", 64),
		Candidates: []generationdomain.CandidateReference{{
			ID: uuid.NewString(), Revision: 1, ArtifactID: uuid.NewString(), ArtifactRevision: 2,
			ArtifactSHA256: strings.Repeat("7", 64), QCReportID: uuid.NewString(), QCReportHash: strings.Repeat("8", 64),
		}},
	}
	providers := newReferenceImageProviderStub()
	providers.submitResult, providers.reconcileResult = submitResult, succeededProviderResult
	materializer := &providerOutputMaterializerStub{result: generationapp.OutputMaterializationResult{
		ProviderReceiptSetHash: providerReceiptSetHash, CandidateSet: candidateSet,
	}}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, materializer)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		second.ReferenceAsset.AssetID, second.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil || result.Status != "RETRYING" || result.Output.SchemaVersion != "" || len(result.Output.Bindings) != 0 {
		t.Fatalf("prepare reference asset Workflow node: result=%#v err=%v", result, err)
	}
	if len(providers.submitCalls) != 1 || len(providers.reconcileCalls) != 0 {
		t.Fatalf("CLAIMED activity crossed more than the Submit boundary: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
	command.Attempt = 2
	command.IdempotencyKey = "workflow-node-redelivery:" + nodeRunID
	result, err = executor.Execute(context.Background(), command)
	if err != nil || result.Status != "SUCCEEDED" || result.Output.SchemaVersion != workflow.NodeOutputSchemaVersion ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0] != (workflow.NodeOutputBinding{
		Port: "candidates", ValueType: "generation_candidate_set", ReferenceID: executingIntent.ProviderJobID,
		ReferenceVersion: "1", ContentHash: candidateSet.ContentHash,
	}) {
		t.Fatalf("complete reference asset Workflow node: result=%#v err=%v", result, err)
	}
	if len(providers.submitCalls) != 1 || len(providers.reconcileCalls) != 1 {
		t.Fatalf("EXECUTING activity crossed a boundary other than Reconcile: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
	completedOutput := result.Output
	command.Attempt = 3
	command.IdempotencyKey = "workflow-node-redelivery-3:" + nodeRunID
	result, err = executor.Execute(context.Background(), command)
	if err != nil || result.Status != "SUCCEEDED" || result.Output.SchemaVersion != completedOutput.SchemaVersion ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0] != completedOutput.Bindings[0] {
		t.Fatalf("replay completed reference asset Workflow node: result=%#v err=%v", result, err)
	}
	if len(providers.submitCalls) != 1 || len(providers.reconcileCalls) != 1 {
		t.Fatalf("terminal replay crossed a remote Provider boundary: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
	if len(builder.calls) != 3 || len(preparations.calls) != 3 {
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
			prepare.command.TargetHash != second.TargetHash ||
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
		len(providers.reconcileCalls) != 1 || providers.reconcileCalls[0].ProviderJobID != executingIntent.ProviderJobID ||
		providers.reconcileCalls[0].IdempotencyKey != "workflow-node:"+nodeRunID+":provider-reconcile:4" {
		t.Fatalf("reference asset Provider calls drifted: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
	if len(submitResult.Calls) == 0 || len(succeededProviderResult.Calls) == 0 ||
		submitResult.Calls[0].ID != succeededProviderResult.Calls[0].ID ||
		submitResult.Calls[0].CallKey != succeededProviderResult.Calls[0].CallKey ||
		submitResult.Calls[0].RemoteRequestID != succeededProviderResult.Calls[0].RemoteRequestID ||
		submitResult.Calls[0].RemoteJobID != succeededProviderResult.Calls[0].RemoteJobID ||
		succeededProviderResult.Receipts[0].CallID != submitResult.Calls[0].ID {
		t.Fatalf("Reconcile did not recover the exact submitted remote task: submit=%#v reconcile=%#v", submitResult.Calls, succeededProviderResult)
	}
	if len(materializer.calls) != 2 {
		t.Fatalf("reference asset materialization calls drifted: %#v", materializer.calls)
	}
	for _, call := range materializer.calls {
		if call.actor != (generationapp.Actor{UserID: userID, TokenVersion: 3}) ||
			call.command.ProviderJobID != executingIntent.ProviderJobID {
			t.Fatalf("reference asset materialization binding drifted: %#v", call)
		}
	}
	materializer.result.CandidateSet.ProjectID = uuid.NewString()
	command.Attempt = 4
	command.IdempotencyKey = "workflow-node-redelivery-4:" + nodeRunID
	if _, err = executor.Execute(context.Background(), command); err == nil ||
		!strings.Contains(err.Error(), "CandidateSet source has drifted") {
		t.Fatalf("reference asset Workflow node accepted a drifted materialized CandidateSet: %v", err)
	}
	if len(providers.submitCalls) != 1 || len(providers.reconcileCalls) != 1 {
		t.Fatalf("drifted terminal replay crossed a remote Provider boundary: submit=%#v reconcile=%#v", providers.submitCalls, providers.reconcileCalls)
	}
}

func TestReferenceAssetExecutorRejectsClaimOwnedByAnotherNodeBeforeProviderSubmission(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("c", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
	claimed, authorization := claimReferenceIntent(prepared, "workflow-node:"+uuid.NewString())
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: prepared},
	}}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{
		Intent: claimed, Authorization: authorization,
	}}}
	providers := newReferenceImageProviderStub()
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, &providerOutputMaterializerStub{})
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
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{}}}
	providers := newReferenceImageProviderStub()
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, &providerOutputMaterializerStub{})
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

func TestReferenceAssetExecutorEscalatesOutcomeUnknownWithoutRemoteBoundary(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("6", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
	claimed, _ := claimReferenceIntent(prepared, "workflow-node:"+nodeRunID)
	unknown := executeReferenceIntent(claimed)
	unknown.Status = generationdomain.IntentOutcomeUnknown
	unknown.Revision = 5
	unknown.ContentHash = strings.Repeat("5", 64)
	unknown.UpdatedAt = unknown.UpdatedAt.Add(time.Minute)
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: unknown},
	}}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{}}}
	providers := newReferenceImageProviderStub()
	materializer := &providerOutputMaterializerStub{}
	executor := workflowgeneration.NewNodeExecutor(
		nil, builder, preparations, claims, providers, materializer,
	)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil || result.Status != workflow.NodeActivityNeedsAttention ||
		result.ErrorCode != workflow.ProviderOutcomeUnknownErrorCode ||
		result.NextAction != workflow.ManualProviderReconciliationNextAction {
		t.Fatalf("OUTCOME_UNKNOWN did not escalate for reconciliation: result=%#v err=%v", result, err)
	}
	if len(claims.calls) != 0 || len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 0 || len(materializer.calls) != 0 {
		t.Fatalf("OUTCOME_UNKNOWN crossed a local or remote execution boundary: claim=%#v submit=%#v reconcile=%#v materialize=%#v",
			claims.calls, providers.submitCalls, providers.reconcileCalls, materializer.calls)
	}
}

func TestReferenceAssetExecutorEscalatesNewProviderOutcomeUnknown(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("6", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
	claimed, authorization := claimReferenceIntent(prepared, "workflow-node:"+nodeRunID)
	unknown := executeReferenceIntent(claimed)
	unknown.Status = generationdomain.IntentOutcomeUnknown
	unknown.Revision = 5
	unknown.ContentHash = strings.Repeat("5", 64)
	unknown.UpdatedAt = unknown.UpdatedAt.Add(time.Minute)
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: prepared},
	}}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{
		Intent: claimed, Authorization: authorization,
	}}}
	providers := newReferenceImageProviderStub()
	providers.submitResult = referenceProviderResult(unknown, target, generationdomain.ProviderJobOutcomeUnknown)
	materializer := &providerOutputMaterializerStub{}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, materializer)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil || result.Status != workflow.NodeActivityNeedsAttention ||
		result.ErrorCode != workflow.ProviderOutcomeUnknownErrorCode ||
		result.NextAction != workflow.ManualProviderReconciliationNextAction {
		t.Fatalf("new Provider OUTCOME_UNKNOWN did not escalate: result=%#v err=%v", result, err)
	}
	if len(providers.submitCalls) != 1 || len(providers.reconcileCalls) != 0 || len(materializer.calls) != 0 {
		t.Fatalf("OUTCOME_UNKNOWN crossed an extra Provider boundary: submit=%#v reconcile=%#v materialize=%#v",
			providers.submitCalls, providers.reconcileCalls, materializer.calls)
	}
}

func TestReferenceAssetExecutorMapsTemporaryProviderQueryFailureToRetrying(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("6", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
	claimed, authorization := claimReferenceIntent(prepared, "workflow-node:"+nodeRunID)
	executing := executeReferenceIntent(claimed)
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: executing},
	}}}
	claims := &executionClaimStub{results: []generationapp.ExecutionClaimResult{{
		Intent: executing, Authorization: authorization,
	}}}
	providers := newReferenceImageProviderStub()
	const downstreamText = "provider response body contained a downstream credential"
	providers.reconcileErr = &generationapp.Error{
		Code: "provider_query_temporarily_unavailable", Message: downstreamText,
		Status: 503, NextAction: "retry_provider_query",
	}
	materializer := &providerOutputMaterializerStub{}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, materializer)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil {
		if strings.Contains(err.Error(), downstreamText) {
			t.Fatalf("temporary Provider query leaked downstream text: %v", err)
		}
		t.Fatalf("temporary Provider query returned an error: %v", err)
	}
	if result.Status != "RETRYING" || result.Output.SchemaVersion != "" ||
		len(result.Output.Bindings) != 0 || result.ErrorCode != "" || result.NextAction != "" {
		t.Fatalf("temporary Provider query failure leaked instead of retrying: result=%#v err=%v", result, err)
	}
	if len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 1 || len(materializer.calls) != 0 {
		t.Fatalf("temporary Provider query crossed an extra boundary: submit=%#v reconcile=%#v materialize=%#v",
			providers.submitCalls, providers.reconcileCalls, materializer.calls)
	}
}

func TestReferenceAssetExecutorKeepsTemporaryOutputMaterializationRetryable(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	approvedID, approvedHash := uuid.NewString(), strings.Repeat("6", 64)
	target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
	prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
	claimed, _ := claimReferenceIntent(prepared, "workflow-node:"+nodeRunID)
	terminal := completeReferenceIntent(executeReferenceIntent(claimed), generationdomain.IntentSucceeded)
	builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
		Targets: []generationdomain.GenerationTarget{target},
	}}
	preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
		IntentView: generationapp.IntentView{Intent: terminal},
	}}}
	claims := &executionClaimStub{}
	providers := newReferenceImageProviderStub()
	const downstreamText = "object storage response contained a downstream credential"
	materializer := &providerOutputMaterializerStub{err: &generationapp.Error{
		Code: "dependency_unavailable", Message: downstreamText, Status: 503, NextAction: "retry",
	}}
	executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, materializer)
	command := referenceAssetExecutorCommand(
		t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
		target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
	)

	result, err := executor.Execute(context.Background(), command)
	if err != nil {
		if strings.Contains(err.Error(), downstreamText) {
			t.Fatalf("temporary materialization leaked downstream text: %v", err)
		}
		t.Fatalf("temporary materialization returned an error: %v", err)
	}
	if result.Status != "RETRYING" || result.Output.SchemaVersion != "" ||
		len(result.Output.Bindings) != 0 || result.ErrorCode != "" || result.NextAction != "" {
		t.Fatalf("temporary materialization became terminal instead of retrying: result=%#v", result)
	}
	if len(materializer.calls) != 1 || len(claims.calls) != 0 ||
		len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 0 {
		t.Fatalf("temporary materialization crossed an execution boundary: materialize=%#v claim=%#v submit=%#v reconcile=%#v",
			materializer.calls, claims.calls, providers.submitCalls, providers.reconcileCalls)
	}

	materializer.err = &generationapp.Error{
		Code: "artifact_not_ready", Message: "Provider output is quarantined", Status: 409, NextAction: "wait_or_replace",
	}
	command.Attempt = 2
	command.IdempotencyKey = "workflow-node-redelivery:" + nodeRunID
	result, err = executor.Execute(context.Background(), command)
	if !generationapp.IsCode(err, "artifact_not_ready") || result.Status != "" {
		t.Fatalf("quarantined materialization became retryable: result=%#v err=%v", result, err)
	}
	if len(materializer.calls) != 2 || len(claims.calls) != 0 ||
		len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 0 {
		t.Fatalf("quarantined materialization crossed an execution boundary: materialize=%#v claim=%#v submit=%#v reconcile=%#v",
			materializer.calls, claims.calls, providers.submitCalls, providers.reconcileCalls)
	}
}

func TestReferenceAssetExecutorMaterializesBothSucceededAndPartialTerminalIntents(t *testing.T) {
	for _, status := range []string{generationdomain.IntentSucceeded, generationdomain.IntentPartialSucceeded} {
		t.Run(status, func(t *testing.T) {
			workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
			approvedID, approvedHash := uuid.NewString(), strings.Repeat("6", 64)
			target := newReferenceExecutorTarget(t, workspaceID, projectID, userID, approvedID, approvedHash)
			prepared := newReferencePreparedIntent(workspaceID, projectID, userID, workflowRunID, nodeRunID, target)
			claimed, _ := claimReferenceIntent(prepared, "workflow-node:"+nodeRunID)
			terminal := completeReferenceIntent(executeReferenceIntent(claimed), status)
			receiptSetHash := strings.Repeat("a", 64)
			candidateSet := generationdomain.CandidateSet{
				ID: terminal.ProviderJobID, WorkspaceID: workspaceID, ProjectID: projectID,
				ProviderReceiptSetHash: receiptSetHash, Revision: 1, ContentHash: strings.Repeat("b", 64),
				Candidates: []generationdomain.CandidateReference{{
					ID: uuid.NewString(), Revision: 1, ArtifactID: uuid.NewString(), ArtifactRevision: 1,
					ArtifactSHA256: strings.Repeat("c", 64), QCReportID: uuid.NewString(), QCReportHash: strings.Repeat("d", 64),
				}},
			}
			builder := &referenceTargetBuilderStub{result: generationapp.BuildReferenceTargetsResult{
				Targets: []generationdomain.GenerationTarget{target},
			}}
			preparations := &imagePreparationStub{results: []generationapp.PreparationResult{{
				IntentView: generationapp.IntentView{Intent: terminal},
			}}}
			claims := &executionClaimStub{}
			providers := newReferenceImageProviderStub()
			materializer := &providerOutputMaterializerStub{result: generationapp.OutputMaterializationResult{
				ProviderReceiptSetHash: receiptSetHash, CandidateSet: candidateSet,
			}}
			executor := workflowgeneration.NewNodeExecutor(nil, builder, preparations, claims, providers, materializer)
			command := referenceAssetExecutorCommand(
				t, workspaceID, projectID, userID, workflowRunID, nodeRunID, approvedID, approvedHash,
				target.ReferenceAsset.AssetID, target.ReferenceAsset.AssetStateRef.ID,
			)

			result, err := executor.Execute(context.Background(), command)
			if err != nil || result.Status != "SUCCEEDED" || len(result.Output.Bindings) != 1 ||
				result.Output.Bindings[0].ReferenceID != terminal.ProviderJobID {
				t.Fatalf("terminal %s intent did not materialize: result=%#v err=%v", status, result, err)
			}
			if len(materializer.calls) != 1 || len(claims.calls) != 0 ||
				len(providers.submitCalls) != 0 || len(providers.reconcileCalls) != 0 {
				t.Fatalf("terminal %s materialization crossed an execution boundary: materialize=%#v claim=%#v submit=%#v reconcile=%#v",
					status, materializer.calls, claims.calls, providers.submitCalls, providers.reconcileCalls)
			}
		})
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
			PromptVersion: "character-reference-sheet", PositivePrompt: "character reference sheet",
			NegativePrompt: "identity drift", Width: 1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
		},
		Revision: 1, CreatedBy: userID, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("build reference asset Target fixture: %v", err)
	}
	return target
}

func newReferencePreparedIntent(
	workspaceID, projectID, userID, workflowRunID, nodeRunID string,
	target generationdomain.GenerationTarget,
) generationdomain.Intent {
	createdAt := time.Now().UTC().Add(-time.Minute)
	return generationdomain.Intent{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, TargetID: target.ID, TargetHash: target.TargetHash,
		BindingVersionID: uuid.NewString(), BindingRevision: 1, BindingContentHash: strings.Repeat("1", 64),
		ConnectionVersionID: uuid.NewString(), CredentialVersionID: uuid.NewString(),
		ModelProfileVersionID: uuid.NewString(), ModelProfileRevision: 1,
		ModelProfileContentHash: strings.Repeat("2", 64), PriceQuoteID: uuid.NewString(), PriceQuoteRevision: 1,
		PriceQuoteContentHash: strings.Repeat("3", 64), BillingMetric: "generation.image.call",
		EstimatedUnits: int64(target.ReferenceAsset.NumberResults), CostEstimateID: uuid.NewString(),
		CostReservationID: uuid.NewString(), QuotaReservationID: uuid.NewString(),
		CostEstimateReceiptID: uuid.NewString(), CostReservationReceiptID: uuid.NewString(),
		QuotaReservationReceiptID: uuid.NewString(), Status: generationdomain.IntentPrepared,
		ContentHash: strings.Repeat("4", 64), CreatedBy: userID, InitiatorTokenVersion: 3,
		Revision: 2, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func claimReferenceIntent(
	prepared generationdomain.Intent,
	claimant string,
) (generationdomain.Intent, generationdomain.ExecutionAuthorization) {
	claimToken := uuid.NewString()
	claimExpiresAt := prepared.UpdatedAt.Add(10 * time.Minute)
	claimed := prepared
	claimed.Status, claimed.Revision = generationdomain.IntentClaimed, 3
	claimed.Claimant, claimed.ClaimToken, claimed.ClaimExpiresAt = &claimant, &claimToken, &claimExpiresAt
	claimed.ClaimFencingVersion = 1
	claimed.ContentHash = strings.Repeat("5", 64)
	claimed.UpdatedAt = prepared.UpdatedAt.Add(time.Minute)
	return claimed, generationdomain.ExecutionAuthorization{
		IntentID: claimed.ID, ClaimToken: claimToken, TargetID: claimed.TargetID, TargetHash: claimed.TargetHash,
		BindingVersionID: claimed.BindingVersionID, BindingRevision: claimed.BindingRevision,
		BindingContentHash: claimed.BindingContentHash, ConnectionVersionID: claimed.ConnectionVersionID,
		CredentialVersionID: claimed.CredentialVersionID, ModelProfileVersionID: claimed.ModelProfileVersionID,
		ModelProfileRevision: claimed.ModelProfileRevision, ModelProfileContentHash: claimed.ModelProfileContentHash,
		PriceQuoteID: claimed.PriceQuoteID, PriceQuoteRevision: claimed.PriceQuoteRevision,
		PriceQuoteContentHash: claimed.PriceQuoteContentHash, BillingMetric: claimed.BillingMetric,
		CostReservationID: claimed.CostReservationID, QuotaReservationID: claimed.QuotaReservationID,
		ClaimFencingVersion: 1, IntentRevision: 3, EstimatedUnits: claimed.EstimatedUnits, ExpiresAt: claimExpiresAt,
	}
}

func executeReferenceIntent(claimed generationdomain.Intent) generationdomain.Intent {
	executing := claimed
	executing.Status, executing.Revision = generationdomain.IntentExecuting, 4
	executing.GenerationRequestID, executing.ProviderJobID = uuid.NewString(), uuid.NewString()
	executing.ProviderCallSetHash = strings.Repeat("7", 64)
	executing.ContentHash = strings.Repeat("8", 64)
	executing.UpdatedAt = claimed.UpdatedAt.Add(time.Minute)
	return executing
}

func completeReferenceIntent(executing generationdomain.Intent, status string) generationdomain.Intent {
	completed := executing
	completed.Status, completed.Revision = status, 5
	completed.CostSettlementReceiptID = uuid.NewString()
	completed.QuotaConsumptionReceiptID = uuid.NewString()
	completed.ContentHash = strings.Repeat("9", 64)
	completed.UpdatedAt = executing.UpdatedAt.Add(time.Minute)
	return completed
}

func referenceProviderResult(
	intent generationdomain.Intent,
	target generationdomain.GenerationTarget,
	jobStatus string,
) generationapp.ProviderExecutionResult {
	createdAt := intent.CreatedAt.Add(2 * time.Minute)
	request := generationdomain.GenerationRequest{
		ID: intent.GenerationRequestID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
		TargetID: target.ID, BindingID: intent.BindingVersionID, BindingRevision: intent.BindingRevision,
		BindingContentHash: intent.BindingContentHash, Purpose: generationdomain.ProviderPurposeReferenceAsset,
		ProviderKey: "controlled-image", ExternalModelID: "image-model", ConnectionVersionID: intent.ConnectionVersionID,
		CredentialVersionID: intent.CredentialVersionID, ModelProfileVersionID: intent.ModelProfileVersionID,
		ModelProfileRevision: intent.ModelProfileRevision, ModelProfileContentHash: intent.ModelProfileContentHash,
		PriceQuoteID: intent.PriceQuoteID, PriceQuoteRevision: intent.PriceQuoteRevision,
		PriceQuoteContentHash: intent.PriceQuoteContentHash, BillingMetric: intent.BillingMetric,
		RequestKey: "generation-request:" + intent.GenerationRequestID, TargetHash: target.TargetHash,
		EstimatedUnits: intent.EstimatedUnits, ContentHash: strings.Repeat("a", 64), CreatedBy: intent.CreatedBy,
		CreatedAt: createdAt,
	}
	job := generationdomain.ProviderJob{
		ID: intent.ProviderJobID, WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, IntentID: intent.ID,
		RequestID: request.ID, ProviderKey: request.ProviderKey, RequestKey: request.RequestKey, Status: jobStatus,
		CallSetHash: intent.ProviderCallSetHash, CallCount: int(intent.EstimatedUnits), Revision: 2,
		ContentHash: strings.Repeat("b", 64), CreatedAt: createdAt, UpdatedAt: intent.UpdatedAt,
	}
	switch jobStatus {
	case generationdomain.ProviderJobRunning:
		job.DispatchedCallCount = 1
	case generationdomain.ProviderJobSucceeded:
		job.DispatchedCallCount, job.SucceededCallCount = job.CallCount, job.CallCount
	case generationdomain.ProviderJobPartialSucceeded:
		job.DispatchedCallCount, job.SucceededCallCount, job.FailedCallCount = job.CallCount, job.CallCount-1, 1
	case generationdomain.ProviderJobOutcomeUnknown:
		job.DispatchedCallCount = 1
	}
	calls := make([]generationdomain.ProviderCall, job.CallCount)
	receipts := make([]generationdomain.ProviderResultReceipt, job.CallCount)
	for index := range calls {
		callStatus := generationdomain.ProviderCallPending
		if jobStatus == generationdomain.ProviderJobRunning && index == 0 {
			callStatus = generationdomain.ProviderCallSubmitted
		} else if jobStatus == generationdomain.ProviderJobSucceeded ||
			(jobStatus == generationdomain.ProviderJobPartialSucceeded && index < job.SucceededCallCount) {
			callStatus = generationdomain.ProviderCallSucceeded
		} else if jobStatus == generationdomain.ProviderJobPartialSucceeded {
			callStatus = generationdomain.ProviderCallFailed
		}
		calls[index] = generationdomain.ProviderCall{
			ID: uuid.NewString(), WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, JobID: job.ID,
			CandidateIndex: index, CallKey: "provider-call:" + job.ID + ":" + string(rune('0'+index)),
			RequestHash: strings.Repeat("c", 64), RequestedOutputCount: 1, Status: callStatus,
			RemoteRequestID: "remote-request-" + string(rune('0'+index)), RemoteJobID: "remote-job-" + string(rune('0'+index)),
			Revision: 2, ContentHash: strings.Repeat("d", 64), CreatedAt: createdAt, UpdatedAt: intent.UpdatedAt,
		}
		receipts[index] = generationdomain.ProviderResultReceipt{
			ID: uuid.NewString(), WorkspaceID: intent.WorkspaceID, ProjectID: intent.ProjectID, CallID: calls[index].ID,
			ProviderEventID: "provider-event-" + string(rune('0'+index)), Status: generationdomain.ProviderResultSucceeded,
			OutputCount: 1, ProviderUsageHash: strings.Repeat("e", 64), ContentHash: strings.Repeat("f", 64),
			OccurredAt: intent.UpdatedAt, ReceivedAt: intent.UpdatedAt,
		}
	}
	return generationapp.ProviderExecutionResult{Intent: intent, Request: request, Job: job, Calls: calls, Receipts: receipts}
}

func newReferenceImageProviderStub() *imageProviderStub {
	return &imageProviderStub{}
}
