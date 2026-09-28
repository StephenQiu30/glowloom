package operation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestBatchWorkflowStartsSelectedChildAndFinishes(t *testing.T) {
	batchID, projectID, operationID := uuid.New(), uuid.New(), uuid.New()
	snapshot := operationapp.WorkflowBatch{
		Batch: domain.Batch{
			ID: batchID, ProjectID: projectID, Kind: "mixed", Scope: json.RawMessage(`{}`),
			Status: domain.BatchStatusConfirmed, TotalCount: 1,
		},
		Items: []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusConfirmed}},
	}
	running := snapshot
	running.Batch.Status = domain.BatchStatusRunning
	finishedItem := running
	finishedItem.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCompleted}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	id := batchID.String()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(snapshot, nil).Once()
	env.OnActivity("flow.MarkBatchRunning", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).Once()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.MatchedBy(func(input operationflow.BatchLaunchInput) bool {
		return input.BatchID == id && input.OperationID == operationID.String()
	})).Return(true, nil).Once()
	env.OnWorkflow(operationflow.OperationWorkflow, mock.Anything, mock.MatchedBy(func(input operationflow.OperationInput) bool {
		return input.OperationID == operationID.String() && input.BatchID == id
	})).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(finishedItem, nil).Once()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("batch workflow: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowChildRunsWithParentIdentity(t *testing.T) {
	loaded := workflowFixture()
	batchID := uuid.New()
	loaded.Operation.BatchID = &batchID
	sourceID := uuid.New()
	zero := int64(0)
	loaded.Operation.ReusedFromID = &sourceID
	loaded.Operation.QuoteMicros = &zero
	loaded.Provider = operationapp.WorkflowProvider{}
	id, operationID := batchID.String(), loaded.Operation.ID.String()
	snapshot := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: loaded.Operation.ProjectID,
			Kind: "mixed", Scope: json.RawMessage(`{}`), Status: domain.BatchStatusConfirmed,
			TotalCount: 1},
		Items: []operationapp.WorkflowBatchItem{{OperationID: loaded.Operation.ID, Status: domain.StatusConfirmed}},
	}
	running := snapshot
	running.Batch.Status = domain.BatchStatusRunning
	completed := running
	completed.Items = []operationapp.WorkflowBatchItem{{OperationID: loaded.Operation.ID, Status: domain.StatusCompleted}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "batch/" + id, TaskQueue: "flow"})
	env.RegisterWorkflow(operationflow.OperationWorkflow)
	registerOperationMockActivities(env)
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(snapshot, nil).Once()
	env.OnActivity("flow.MarkBatchRunning", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).Once()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnActivity("flow.LoadOperation", mock.Anything, operationID).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, operationID).Return(nil).Once()
	env.OnActivity("flow.CompleteFromReuse", mock.Anything, operationID).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(completed, nil).Maybe()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("batch child parent identity: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowContinuesAfterOneHundredStarts(t *testing.T) {
	batchID := uuid.New()
	id := batchID.String()
	items := make([]operationapp.WorkflowBatchItem, 100)
	for i := range items {
		items[i] = operationapp.WorkflowBatchItem{OperationID: uuid.New(), Status: domain.StatusConfirmed}
	}
	snapshot := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 100},
		Items: items,
	}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(snapshot, nil).Maybe()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(true, nil).Times(100)
	env.OnWorkflow(operationflow.OperationWorkflow, mock.Anything, mock.Anything).Return(nil).Times(100)
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if !workflow.IsContinueAsNewError(env.GetWorkflowError()) {
		t.Fatalf("100 child starts did not continue as new: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowReloadsAlreadyStartedChild(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	snapshot := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		Items: []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusConfirmed}},
	}
	completed := snapshot
	completed.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCompleted}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(snapshot, nil).Twice()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnWorkflow(operationflow.OperationWorkflow, mock.Anything, mock.Anything).
		Return(serviceerror.NewWorkflowExecutionAlreadyStarted("existing child", "operation/"+operationID.String(), "run"))
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(completed, nil).Maybe()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("already started child replay: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowWaitsForProviderCapacity(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	snapshot := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		Items: []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusConfirmed}},
	}
	completed := snapshot
	completed.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCompleted}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(snapshot, nil).Times(3)
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(false, nil).Once()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnWorkflow(operationflow.OperationWorkflow, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(completed, nil).Maybe()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("provider capacity wait: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchFailureStreakUsesTerminalOrderAndProviderCodes(t *testing.T) {
	base := time.Now().UTC()
	items := make([]operationapp.WorkflowBatchItem, 0, 7)
	for i := range 5 {
		items = append(items, operationapp.WorkflowBatchItem{
			OperationID: uuid.New(), Status: domain.StatusFailed,
			FailureCode: "provider:rate_limited", FinishedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	items = append(items, operationapp.WorkflowBatchItem{
		OperationID: uuid.New(), Status: domain.StatusConfirmed,
	})
	if got := operationflow.BatchFailureStreak(items); got != "provider:rate_limited" {
		t.Fatalf("five provider failures: %q", got)
	}
	items[2].FailureCode = "provider:invalid_request"
	if got := operationflow.BatchFailureStreak(items); got != "" {
		t.Fatalf("different provider failure interrupted streak: %q", got)
	}
	items[2].FailureCode = "provider:rate_limited"
	items[4].FailureCode = "provider_result_invalid"
	if got := operationflow.BatchFailureStreak(items); got != "" {
		t.Fatalf("internal failure triggered provider breaker: %q", got)
	}
	items[4].FailureCode = "provider:rate_limited"
	items = append(items, operationapp.WorkflowBatchItem{
		OperationID: uuid.New(), Status: domain.StatusCompleted,
		FinishedAt: base.Add(2500 * time.Millisecond),
	})
	if got := operationflow.BatchFailureStreak(items); got != "" {
		t.Fatalf("completion order ignored: %q", got)
	}
}

func TestBatchWorkflowPausesUntilResumeSignal(t *testing.T) {
	batchID, pendingID := uuid.New(), uuid.New()
	id := batchID.String()
	base := time.Now().UTC()
	items := make([]operationapp.WorkflowBatchItem, 0, 6)
	for i := range 5 {
		items = append(items, operationapp.WorkflowBatchItem{
			OperationID: uuid.New(), Status: domain.StatusFailed,
			FailureCode: "provider:rate_limited", FinishedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	items = append(items, operationapp.WorkflowBatchItem{OperationID: pendingID, Status: domain.StatusConfirmed})
	running := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 6},
		Items: items,
	}
	paused := running
	paused.PausedReason = "provider:rate_limited"
	finished := running
	finished.Items = append([]operationapp.WorkflowBatchItem(nil), items...)
	finished.Items[5].Status = domain.StatusCompleted
	finished.Items[5].FinishedAt = base.Add(6 * time.Second)
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).Twice()
	env.OnActivity("flow.PauseBatch", mock.Anything, id, "provider:rate_limited").Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(paused, nil).Twice()
	env.OnActivity("flow.ResumeBatch", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).Once()
	env.OnActivity("flow.AcquireBatchLaunch", mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnWorkflow(operationflow.OperationWorkflow, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(finished, nil).Maybe()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("resume", nil) }, time.Second)
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("batch pause and resume: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowCancelsUnlaunchedChild(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	cancelling := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		CancelRequested: true,
		Items:           []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusConfirmed}},
	}
	cancelled := cancelling
	cancelled.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCancelled}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelling, nil).Twice()
	env.OnActivity("flow.CancelQueuedBatchChild", mock.Anything, mock.MatchedBy(func(input operationflow.BatchLaunchInput) bool {
		return input.BatchID == id && input.OperationID == operationID.String()
	})).Return(true, nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelled, nil).Once()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("cancel queued child: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowSignalsSubmittedChildCancellationOnce(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	cancelling := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		CancelRequested: true,
		Items:           []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusSubmitted}},
	}
	cancelled := cancelling
	cancelled.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCancelled}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelling, nil).Twice()
	env.OnSignalExternalWorkflow(mock.Anything, "operation/"+operationID.String(), "", "cancel", mock.Anything).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelled, nil).Once()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("signal submitted child: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowAcceptsChildCompletionBeforeCancelSignal(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	cancelling := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		CancelRequested: true,
		Items:           []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusSubmitted}},
	}
	completed := cancelling
	completed.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCompleted}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelling, nil).Twice()
	env.OnSignalExternalWorkflow(mock.Anything, "operation/"+operationID.String(), "", "cancel", mock.Anything).
		Return(serviceerror.NewNotFound("child already closed")).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(completed, nil).Twice()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("child completed before cancel signal: %v", err)
	}
	env.AssertExpectations(t)
}

func TestBatchWorkflowCancelSignalPersistsBeforeChildAdmission(t *testing.T) {
	batchID, operationID := uuid.New(), uuid.New()
	id := batchID.String()
	running := operationapp.WorkflowBatch{
		Batch: domain.Batch{ID: batchID, ProjectID: uuid.New(), Kind: "mixed",
			Scope: json.RawMessage(`{}`), Status: domain.BatchStatusRunning, TotalCount: 1},
		Items: []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusConfirmed}},
	}
	cancelling := running
	cancelling.CancelRequested = true
	cancelled := cancelling
	cancelled.Items = []operationapp.WorkflowBatchItem{{OperationID: operationID, Status: domain.StatusCancelled}}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerBatchMockActivities(env)
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).
		Run(func(mock.Arguments) { env.SignalWorkflow("cancel", nil) }).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(running, nil).Once()
	env.OnActivity("flow.MarkBatchCancelRequested", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelling, nil).Once()
	env.OnActivity("flow.CancelQueuedBatchChild", mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnActivity("flow.LoadBatch", mock.Anything, id).Return(cancelled, nil).Once()
	env.OnActivity("flow.FinishBatch", mock.Anything, id).Return(nil).Once()
	env.ExecuteWorkflow(operationflow.BatchWorkflow, operationflow.BatchInput{BatchID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("batch cancel signal: %v", err)
	}
	env.AssertExpectations(t)
}

func registerBatchMockActivities(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(func(context.Context, string) (operationapp.WorkflowBatch, error) {
		return operationapp.WorkflowBatch{}, nil
	}, activity.RegisterOptions{Name: "flow.LoadBatch"})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "flow.MarkBatchRunning"})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "flow.MarkBatchCancelRequested"})
	env.RegisterActivityWithOptions(func(context.Context, operationflow.BatchLaunchInput) (bool, error) {
		return false, nil
	}, activity.RegisterOptions{Name: "flow.CancelQueuedBatchChild"})
	env.RegisterActivityWithOptions(func(context.Context, operationflow.BatchLaunchInput) (bool, error) {
		return false, nil
	}, activity.RegisterOptions{Name: "flow.AcquireBatchLaunch"})
	env.RegisterActivityWithOptions(func(context.Context, string, string) error { return nil },
		activity.RegisterOptions{Name: "flow.PauseBatch"})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "flow.ResumeBatch"})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "flow.FinishBatch"})
}
