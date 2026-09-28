package operation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestProductionFlowWorkerCancelsBatchOnLocalTemporal(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	dsn := os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN")
	if addr == "" || namespace == "" || dsn == "" {
		t.Skip("set isolated PostgreSQL and local Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	database := operationStoreDB(t)
	actor, projectID, batchID, items := seedConfirmableBatch(t, database, 1)
	operationID := items[0].Operation.ID
	store := pgoperation.NewStore(database)
	if _, err := store.ConfirmBatchQuote(ctx, actor, operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatalf("confirm real batch before worker start: %v", err)
	}
	if err := store.MarkWorkflowBatchRunning(ctx, batchID); err != nil {
		t.Fatalf("claim real confirmed batch for cancellation: %v", err)
	}
	if err := store.MarkWorkflowBatchCancelRequested(ctx, batchID); err != nil {
		t.Fatal(err)
	}
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- app.RunWorker(workerCtx, config.Config{
			DBDSN: dsn, TemporalAddr: addr, TemporalNamespace: namespace,
			WorkerHealthAddr: "127.0.0.1:0",
		}, zap.NewNop(), "flow")
	}()
	t.Cleanup(func() {
		stopWorker()
		select {
		case err := <-workerDone:
			if err != nil {
				t.Errorf("stop production flow worker: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("production flow worker did not stop")
		}
	})
	if err := operationflow.NewStarter(workflowClient).StartBatch(ctx, batchID); err != nil {
		t.Fatalf("start confirmed batch: %v", err)
	}
	if err := workflowClient.GetWorkflow(ctx, "batch/"+batchID.String(), "").Get(ctx, nil); err != nil {
		t.Fatalf("production batch workflow: %v", err)
	}
	assertCancelledBatchMember(t, database, operationID)
	var status string
	if err := database.WithContext(ctx).Raw(`SELECT status FROM operation.batch WHERE id = ?::uuid`,
		batchID.String()).Scan(&status).Error; err != nil || status != "cancelled" {
		t.Fatalf("batch status = %q, %v", status, err)
	}
	assertBatchHistoryReplays(ctx, t, workflowClient, namespace, batchID)
}

func TestBatchCancelResumesAfterProductionFlowWorkerRestart(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	dsn := os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN")
	if addr == "" || namespace == "" || dsn == "" {
		t.Skip("set isolated PostgreSQL and local Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	database, store, batchID, operationID := confirmedOneItemBatch(t)
	if err := database.WithContext(ctx).Exec(`
		UPDATE operation.batch SET paused_reason = 'provider:restart_test'
		WHERE id = ?::uuid
	`, batchID.String()).Error; err != nil {
		t.Fatal(err)
	}
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	workerConfig := config.Config{
		DBDSN: dsn, TemporalAddr: addr, TemporalNamespace: namespace,
		WorkerHealthAddr: "127.0.0.1:0",
	}
	firstCtx, stopFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { firstDone <- app.RunWorker(firstCtx, workerConfig, zap.NewNop(), "flow") }()
	firstRunning := true
	t.Cleanup(func() {
		if firstRunning {
			stopFirst()
			select {
			case err := <-firstDone:
				if err != nil {
					t.Errorf("stop first flow worker: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("first flow worker did not stop")
			}
		}
	})
	if err := operationflow.NewStarter(workflowClient).StartBatch(ctx, batchID); err != nil {
		t.Fatal(err)
	}
	workflowID := "batch/" + batchID.String()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		completed := 0
		iter := workflowClient.GetWorkflowHistory(ctx, workflowID, "", false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
		for iter.HasNext() {
			event, err := iter.Next()
			if err != nil {
				t.Fatalf("read batch workflow history: %v", err)
			}
			if event.GetEventType() == enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED {
				completed++
			}
		}
		if completed >= 2 {
			break
		}
		select {
		case err := <-firstDone:
			firstRunning = false
			t.Fatalf("first flow worker stopped before pause: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for paused batch workflow: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	stopFirst()
	select {
	case err := <-firstDone:
		firstRunning = false
		if err != nil {
			t.Fatalf("stop first flow worker: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("first flow worker did not stop")
	}
	if err := store.MarkWorkflowBatchCancelRequested(ctx, batchID); err != nil {
		t.Fatalf("persist cancellation during outage: %v", err)
	}
	secondCtx, stopSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- app.RunWorker(secondCtx, workerConfig, zap.NewNop(), "flow") }()
	t.Cleanup(func() {
		stopSecond()
		select {
		case err := <-secondDone:
			if err != nil {
				t.Errorf("stop restarted flow worker: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("restarted flow worker did not stop")
		}
	})
	if err := workflowClient.GetWorkflow(ctx, workflowID, "").Get(ctx, nil); err != nil {
		t.Fatalf("batch cancellation after worker restart: %v", err)
	}
	assertCancelledBatchMember(t, database, operationID)
	var providerCalls int64
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM operation.provider_call WHERE operation_id = ?::uuid
	`, operationID.String()).Scan(&providerCalls).Error; err != nil || providerCalls != 0 {
		t.Fatalf("cancelled paused batch provider calls = %d, %v", providerCalls, err)
	}
	assertBatchHistoryReplays(ctx, t, workflowClient, namespace, batchID)
}

func assertBatchHistoryReplays(ctx context.Context, t *testing.T, workflowClient client.Client,
	namespace string, batchID uuid.UUID,
) {
	t.Helper()
	closed, err := workflowClient.DescribeWorkflowExecution(ctx, "batch/"+batchID.String(), "")
	if err != nil {
		t.Fatalf("describe completed batch for replay: %v", err)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(operationflow.BatchWorkflow, workflow.RegisterOptions{Name: "BatchWorkflow"})
	if err := replayer.ReplayWorkflowExecution(ctx, workflowClient.WorkflowService(), nil, namespace,
		workflow.Execution{ID: closed.WorkflowExecutionInfo.Execution.WorkflowId,
			RunID: closed.WorkflowExecutionInfo.Execution.RunId}); err != nil {
		t.Fatalf("replay completed batch history: %v", err)
	}
}
