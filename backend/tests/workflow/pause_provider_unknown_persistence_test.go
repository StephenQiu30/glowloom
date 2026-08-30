package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestPauseWinsProviderOutcomeUnknownProjectionAndResumeReusesTheFrozenIntent(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL pause/provider-unknown race journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
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
	now := time.Date(2026, time.August, 30, 13, 0, 0, 0, time.UTC)
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "pause-provider-unknown-authoring-create-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "pause-provider-unknown-authoring-publish-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(
		compiler, workflowStore, &scriptedWorkflowStarter{outcomes: []string{"started"}},
		workflowapp.StartConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	run, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "pause-provider-unknown-start-" + uuid.NewString(),
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start pausable workflow: run=%#v err=%v", run, err)
	}
	var node model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "script").First(&node).Error; err != nil {
		t.Fatalf("load reference generation node: %v", err)
	}
	command := workflow.NodeActivityCommand{
		WorkflowRunID: run.ID, NodeRunID: node.ID.String(), NodeID: node.NodeID,
		Executor: node.Executor, Attempt: 1,
	}
	executor := newFrozenOutcomeUnknownExecutor()
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, Executor: executor,
	})
	firstExecution := make(chan struct {
		result workflow.NodeActivityResult
		err    error
	}, 1)
	go func() {
		result, executeErr := runtimeService.ExecuteNode(ctx, command)
		firstExecution <- struct {
			result workflow.NodeActivityResult
			err    error
		}{result: result, err: executeErr}
	}()
	select {
	case <-executor.firstDispatchEntered:
	case <-ctx.Done():
		t.Fatalf("provider dispatch did not enter before pause: %v", ctx.Err())
	}

	var claimedRun model.WorkflowRun
	if err = database.First(&claimedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load claimed workflow before pause: %v", err)
	}
	controller := newBlockingAppliedPauseController()
	controlService := workflowapp.NewControlService(workflowStore, controller, workflowapp.ControlConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	pauseResult := make(chan struct {
		intent workflow.ControlIntent
		err    error
	}, 1)
	go func() {
		intent, pauseErr := controlService.Pause(ctx, actor, workflowapp.PauseCommand{
			WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: claimedRun.Revision,
			IdempotencyKey: "pause-provider-unknown-control-" + uuid.NewString(),
		})
		pauseResult <- struct {
			intent workflow.ControlIntent
			err    error
		}{intent: intent, err: pauseErr}
	}()
	select {
	case <-controller.pauseEntered:
	case <-ctx.Done():
		t.Fatalf("pause control did not reach its pending boundary: %v", ctx.Err())
	}
	var pendingPauseCount int64
	if err = database.Model(&model.WorkflowControlIntent{}).
		Where("workflow_run_id = ? AND action = ? AND status = ?", run.ID, workflow.ControlActionPause, "pending").
		Count(&pendingPauseCount).Error; err != nil || pendingPauseCount != 1 {
		t.Fatalf("pending pause control count=%d err=%v", pendingPauseCount, err)
	}
	close(executor.releaseFirstDispatch)
	first := <-firstExecution
	if first.err != nil || first.result.Status != "RETRYING" || first.result.ErrorCode != "" || first.result.NextAction != "" {
		t.Fatalf("provider unknown did not yield to pending pause: result=%#v err=%v", first.result, first.err)
	}
	var yieldedNode model.NodeRunProjection
	if err = database.First(&yieldedNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load pause-winning node projection: %v", err)
	}
	if yieldedNode.Status != "RETRYING" || yieldedNode.ActiveClaimToken != nil ||
		yieldedNode.Revision != node.Revision+2 {
		t.Fatalf("pause-winning node projection = %#v initial revision=%d", yieldedNode, node.Revision)
	}
	close(controller.releasePause)
	paused := <-pauseResult
	if paused.err != nil || paused.intent.Status != "completed" {
		t.Fatalf("finalize pause after node yielded: intent=%#v err=%v", paused.intent, paused.err)
	}
	var pausedRun model.WorkflowRun
	if err = database.First(&pausedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load paused workflow: %v", err)
	}
	if pausedRun.Status != "PAUSED" || pausedRun.PausedFromStatus == nil ||
		*pausedRun.PausedFromStatus != "RUNNING" || pausedRun.NextAction == nil || *pausedRun.NextAction != "resume_workflow" {
		t.Fatalf("pause did not remain authoritative: %#v", pausedRun)
	}

	resumed, err := controlService.Resume(ctx, actor, workflowapp.ResumeCommand{
		WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: pausedRun.Revision,
		IdempotencyKey: "resume-provider-unknown-control-" + uuid.NewString(),
	})
	if err != nil || resumed.Status != "completed" {
		t.Fatalf("resume pause-winning workflow: intent=%#v err=%v", resumed, err)
	}
	attention, err := runtimeService.ExecuteNode(ctx, command)
	if err != nil || attention.Status != workflow.NodeActivityNeedsAttention ||
		attention.ErrorCode != workflow.ProviderOutcomeUnknownErrorCode ||
		attention.NextAction != workflow.ManualProviderReconciliationNextAction {
		t.Fatalf("resume did not project frozen unknown intent: result=%#v err=%v", attention, err)
	}
	var attentionRun model.WorkflowRun
	var attentionNode model.NodeRunProjection
	if err = database.First(&attentionRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load attention workflow: %v", err)
	}
	if err = database.First(&attentionNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load attention node: %v", err)
	}
	if attentionRun.Status != "NEEDS_ATTENTION" || attentionNode.Status != "FAILED" ||
		attentionNode.ActiveClaimToken != nil || attentionNode.Attempt != 2 {
		t.Fatalf("resumed attention projection: run=%#v node=%#v", attentionRun, attentionNode)
	}
	calls, submits, queries, idempotencyKeys := executor.Counts()
	if calls != 2 || submits != 1 || queries != 0 || len(idempotencyKeys) != 2 ||
		idempotencyKeys[0] == "" || idempotencyKeys[0] != idempotencyKeys[1] {
		t.Fatalf("frozen Provider intent crossed an extra paid boundary: calls=%d submits=%d queries=%d keys=%v",
			calls, submits, queries, idempotencyKeys)
	}

	terminalController := &scriptedController{outcomes: []workflow.ControlObservation{{
		Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request",
	}}}
	terminalControlService := workflowapp.NewControlService(workflowStore, terminalController, workflowapp.ControlConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	_, err = terminalControlService.Cancel(ctx, actor, workflowapp.CancelCommand{
		WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: attentionRun.Revision,
		IdempotencyKey: "cancel-terminal-provider-unknown-" + uuid.NewString(),
	})
	var controlError *workflowapp.Error
	if !errors.As(err, &controlError) || controlError.Code != "resource_conflict" ||
		len(terminalController.Requests()) != 0 {
		t.Fatalf("terminal Provider attention accepted Workflow control: err=%v requests=%#v", err, terminalController.Requests())
	}
	var terminalControlCount int64
	if err = database.Model(&model.WorkflowControlIntent{}).
		Where("workflow_run_id = ? AND action = ?", run.ID, workflow.ControlActionCancel).
		Count(&terminalControlCount).Error; err != nil || terminalControlCount != 0 {
		t.Fatalf("terminal Provider attention persisted a Cancel intent: count=%d err=%v", terminalControlCount, err)
	}
}

func TestControlConflictReleasesLateProviderUnknownClaimAndFencesTemporalFailure(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL control-conflict/provider-unknown journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
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
	now := time.Date(2026, time.August, 30, 14, 0, 0, 0, time.UTC)
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "control-conflict-provider-unknown-create-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "control-conflict-provider-unknown-publish-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(
		compiler, workflowStore, &scriptedWorkflowStarter{outcomes: []string{"started"}},
		workflowapp.StartConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	run, err := startService.Start(ctx, actor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "control-conflict-provider-unknown-start-" + uuid.NewString(),
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start controllable workflow: run=%#v err=%v", run, err)
	}
	var node model.NodeRunProjection
	if err = database.Where("workflow_run_id = ? AND node_id = ?", run.ID, "script").First(&node).Error; err != nil {
		t.Fatalf("load Provider node: %v", err)
	}
	command := workflow.NodeActivityCommand{
		WorkflowRunID: run.ID, NodeRunID: node.ID.String(), NodeID: node.NodeID,
		Executor: node.Executor, Attempt: 1,
	}
	executor := newFrozenOutcomeUnknownExecutor()
	runtimeService := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, Executor: executor,
	})
	executionResult := make(chan struct {
		result workflow.NodeActivityResult
		err    error
	}, 1)
	go func() {
		result, executeErr := runtimeService.ExecuteNode(ctx, command)
		executionResult <- struct {
			result workflow.NodeActivityResult
			err    error
		}{result: result, err: executeErr}
	}()
	select {
	case <-executor.firstDispatchEntered:
	case <-ctx.Done():
		t.Fatalf("Provider dispatch did not enter before control conflict: %v", ctx.Err())
	}

	var claimedRun model.WorkflowRun
	if err = database.First(&claimedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load claimed workflow before control conflict: %v", err)
	}
	controlService := workflowapp.NewControlService(
		workflowStore,
		&scriptedController{outcomes: []workflow.ControlObservation{{
			Outcome: workflow.ControlOutcomeConflict, ObservedInputHash: strings.Repeat("f", 64),
		}}},
		workflowapp.ControlConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	conflictedIntent, err := controlService.Pause(ctx, actor, workflowapp.PauseCommand{
		WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: claimedRun.Revision,
		IdempotencyKey: "control-conflict-provider-unknown-pause-" + uuid.NewString(),
	})
	if err != nil || conflictedIntent.Status != "conflict" {
		t.Fatalf("persist control conflict: intent=%#v err=%v", conflictedIntent, err)
	}
	var committedAttention model.WorkflowRun
	var activeNode model.NodeRunProjection
	if err = database.First(&committedAttention, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load committed control attention: %v", err)
	}
	if err = database.First(&activeNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load active node after control conflict: %v", err)
	}
	if committedAttention.Status != "NEEDS_ATTENTION" || committedAttention.ProgressStage != "pause_conflict" ||
		committedAttention.NextAction == nil || *committedAttention.NextAction != "inspect_workflow_control" ||
		len(committedAttention.Error) == 0 || activeNode.Status != "RUNNING" || activeNode.ActiveClaimToken == nil {
		t.Fatalf("control conflict facts: run=%#v node=%#v", committedAttention, activeNode)
	}

	close(executor.releaseFirstDispatch)
	lateUnknown := <-executionResult
	if lateUnknown.err != nil || lateUnknown.result.Status != "RETRYING" ||
		lateUnknown.result.ErrorCode != "" || lateUnknown.result.NextAction != "" {
		t.Fatalf("late Provider unknown did not yield to control conflict: result=%#v err=%v",
			lateUnknown.result, lateUnknown.err)
	}
	var releasedNode model.NodeRunProjection
	var preservedAttention model.WorkflowRun
	if err = database.First(&releasedNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load released Provider node: %v", err)
	}
	if err = database.First(&preservedAttention, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload preserved control attention: %v", err)
	}
	if releasedNode.Status != "RETRYING" || releasedNode.ActiveClaimToken != nil ||
		releasedNode.Revision != node.Revision+2 || releasedNode.OutputHash != nil || len(releasedNode.Output) != 0 {
		t.Fatalf("late Provider unknown leaked its claim: %#v initial revision=%d", releasedNode, node.Revision)
	}
	assertControlAttentionUnchanged(t, committedAttention, preservedAttention)

	if err = runtimeService.FailRun(ctx, workflow.FailRunCommand{
		WorkflowRunID: run.ID, NodeRunID: node.ID.String(), NodeID: node.NodeID,
		FailureCode: "node_activity_failed",
	}); err != nil {
		t.Fatalf("Temporal failure did not reconcile the committed control attention: %v", err)
	}
	var afterTemporalFailure model.WorkflowRun
	var afterTemporalFailureNode model.NodeRunProjection
	if err = database.First(&afterTemporalFailure, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load control attention after Temporal failure: %v", err)
	}
	if err = database.First(&afterTemporalFailureNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load released node after Temporal failure: %v", err)
	}
	assertControlAttentionUnchanged(t, committedAttention, afterTemporalFailure)
	if afterTemporalFailureNode.Status != releasedNode.Status ||
		afterTemporalFailureNode.Revision != releasedNode.Revision || afterTemporalFailureNode.ActiveClaimToken != nil {
		t.Fatalf("Temporal failure rewrote the released node: before=%#v after=%#v",
			releasedNode, afterTemporalFailureNode)
	}
	calls, submits, queries, _ := executor.Counts()
	if calls != 1 || submits != 1 || queries != 0 {
		t.Fatalf("control conflict crossed an extra Provider boundary: calls=%d submits=%d queries=%d", calls, submits, queries)
	}
	comparisonController := &scriptedController{outcomes: []workflow.ControlObservation{{
		Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: "match_request",
	}}}
	comparisonControlService := workflowapp.NewControlService(workflowStore, comparisonController, workflowapp.ControlConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	comparisonCancel, err := comparisonControlService.Cancel(ctx, actor, workflowapp.CancelCommand{
		WorkspaceID: run.WorkspaceID, WorkflowRunID: run.ID, ExpectedRevision: preservedAttention.Revision,
		IdempotencyKey: "cancel-control-conflict-attention-" + uuid.NewString(),
	})
	if err != nil || comparisonCancel.Status != "completed" || len(comparisonController.Requests()) != 1 {
		t.Fatalf("non-Provider NEEDS_ATTENTION lost its valid Cancel path: intent=%#v requests=%#v err=%v",
			comparisonCancel, comparisonController.Requests(), err)
	}
}

func assertControlAttentionUnchanged(t *testing.T, before model.WorkflowRun, after model.WorkflowRun) {
	t.Helper()
	if after.Status != before.Status || after.ProgressStage != before.ProgressStage || after.Revision != before.Revision ||
		!after.UpdatedAt.Equal(before.UpdatedAt) || string(after.Error) != string(before.Error) ||
		(after.NextAction == nil) != (before.NextAction == nil) ||
		(after.NextAction != nil && *after.NextAction != *before.NextAction) {
		t.Fatalf("committed control attention was overwritten: before=%#v after=%#v", before, after)
	}
}

type frozenOutcomeUnknownExecutor struct {
	mu                   sync.Mutex
	unknown              bool
	calls                int
	submits              int
	queries              int
	idempotencyKeys      []string
	firstDispatchEntered chan struct{}
	releaseFirstDispatch chan struct{}
}

func newFrozenOutcomeUnknownExecutor() *frozenOutcomeUnknownExecutor {
	return &frozenOutcomeUnknownExecutor{
		firstDispatchEntered: make(chan struct{}), releaseFirstDispatch: make(chan struct{}),
	}
}

func (executor *frozenOutcomeUnknownExecutor) Execute(
	ctx context.Context,
	command workflow.NodeExecutorCommand,
) (workflow.NodeExecutorResult, error) {
	executor.mu.Lock()
	executor.calls++
	executor.idempotencyKeys = append(executor.idempotencyKeys, command.IdempotencyKey)
	firstDispatch := !executor.unknown
	if firstDispatch {
		executor.unknown = true
		executor.submits++
		close(executor.firstDispatchEntered)
	}
	executor.mu.Unlock()
	if firstDispatch {
		select {
		case <-executor.releaseFirstDispatch:
		case <-ctx.Done():
			return workflow.NodeExecutorResult{}, ctx.Err()
		}
	}
	return workflow.NodeExecutorResult{
		Status: workflow.NodeActivityNeedsAttention, ErrorCode: workflow.ProviderOutcomeUnknownErrorCode,
		NextAction: workflow.ManualProviderReconciliationNextAction,
	}, nil
}

func (executor *frozenOutcomeUnknownExecutor) Counts() (int, int, int, []string) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return executor.calls, executor.submits, executor.queries, append([]string(nil), executor.idempotencyKeys...)
}

type blockingAppliedPauseController struct {
	pauseEntered chan struct{}
	releasePause chan struct{}
}

func newBlockingAppliedPauseController() *blockingAppliedPauseController {
	return &blockingAppliedPauseController{pauseEntered: make(chan struct{}), releasePause: make(chan struct{})}
}

func (controller *blockingAppliedPauseController) Control(
	ctx context.Context,
	request workflow.ControlRequest,
) (workflow.ControlObservation, error) {
	if request.Action == workflow.ControlActionPause {
		close(controller.pauseEntered)
		select {
		case <-controller.releasePause:
		case <-ctx.Done():
			return workflow.ControlObservation{}, ctx.Err()
		}
	}
	return workflow.ControlObservation{
		Outcome: workflow.ControlOutcomeApplied, ObservedInputHash: request.InputHash,
	}, nil
}
