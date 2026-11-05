package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func batchTaskFixture() *stateStore {
	s := newStateStore()
	s.batch = application.WorkflowBatch{Batch: domain.Batch{ID: uuid.New(), ProjectID: s.loaded.Operation.ProjectID, Status: domain.BatchStatusRunning}, Items: []application.WorkflowBatchItem{{OperationID: uuid.New(), Status: domain.StatusConfirmed}}}
	return s
}
func batchStep(t *testing.T, s *stateStore) {
	t.Helper()
	_, err := s.handlers()["operation.batch"](t.Context(), taskruntime.Job{BusinessID: s.batch.Batch.ID.String()})
	require.NoError(t, err)
}
func TestBatchTaskStartsSelectedChildAndFinishes(t *testing.T) {
	s := batchTaskFixture()
	batchStep(t, s)
	require.Equal(t, []uuid.UUID{s.batch.Items[0].OperationID}, s.acquired)
	require.Equal(t, []string{"operation.run"}, s.scheduled)
	s.batch.Items[0].Status = domain.StatusCompleted
	batchStep(t, s)
	require.Equal(t, domain.BatchStatusFinished, s.batch.Batch.Status)
}
func TestBatchTaskReloadsAlreadyStartedChild(t *testing.T) {
	s := batchTaskFixture()
	s.batch.Items[0].Status = domain.StatusSubmitted
	batchStep(t, s)
	require.Empty(t, s.acquired)
	require.Equal(t, []string{"operation.run"}, s.scheduled)
}
func TestBatchTaskWaitsForProviderCapacity(t *testing.T) {
	s := batchTaskFixture()
	s.capacity = false
	batchStep(t, s)
	require.Empty(t, s.acquired)
	require.Empty(t, s.scheduled)
}
func TestBatchTaskPausesUntilDurableResume(t *testing.T) {
	s := batchTaskFixture()
	s.batch.PausedReason = "provider:rate_limited"
	batchStep(t, s)
	require.Empty(t, s.scheduled)
	require.NoError(t, s.ResumeWorkflowBatch(t.Context(), s.batch.Batch.ID))
	batchStep(t, s)
	require.Len(t, s.scheduled, 1)
}
func TestBatchTaskCancelsUnlaunchedChild(t *testing.T) {
	s := batchTaskFixture()
	s.batch.CancelRequested = true
	batchStep(t, s)
	require.Equal(t, []uuid.UUID{s.batch.Items[0].OperationID}, s.cancelled)
	require.Empty(t, s.acquired)
	require.Empty(t, s.scheduled)
}
func TestBatchTaskAcceptsChildCompletionBeforeCancel(t *testing.T) {
	s := batchTaskFixture()
	s.batch.CancelRequested = true
	s.batch.Items[0].Status = domain.StatusCompleted
	batchStep(t, s)
	require.Empty(t, s.cancelled)
	require.Equal(t, domain.BatchStatusFinished, s.batch.Batch.Status)
}
func TestBatchFailureStreakUsesTerminalOrderAndProviderCodes(t *testing.T) {
	base := time.Now().UTC()
	items := make([]application.WorkflowBatchItem, 0, 7)
	for i := range 5 {
		items = append(items, application.WorkflowBatchItem{
			OperationID: uuid.New(), Status: domain.StatusFailed,
			FailureCode: "provider:rate_limited", FinishedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	items = append(items, application.WorkflowBatchItem{
		OperationID: uuid.New(), Status: domain.StatusConfirmed,
	})
	if got := task.BatchFailureStreak(items); got != "provider:rate_limited" {
		t.Fatalf("five provider failures: %q", got)
	}
	items[2].FailureCode = "provider:invalid_request"
	if got := task.BatchFailureStreak(items); got != "" {
		t.Fatalf("different provider failure interrupted streak: %q", got)
	}
	items[2].FailureCode = "provider:rate_limited"
	items[4].FailureCode = "provider_result_invalid"
	if got := task.BatchFailureStreak(items); got != "" {
		t.Fatalf("internal failure triggered provider breaker: %q", got)
	}
	items[4].FailureCode = "provider:rate_limited"
	items = append(items, application.WorkflowBatchItem{
		OperationID: uuid.New(), Status: domain.StatusCompleted,
		FinishedAt: base.Add(2500 * time.Millisecond),
	})
	if got := task.BatchFailureStreak(items); got != "" {
		t.Fatalf("completion order ignored: %q", got)
	}
}
