package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestWorkerRoleRunsMaintenanceOnLocalTemporal(t *testing.T) {
	dsn := os.Getenv("LV_TEST_WORKER_DB_DSN")
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if dsn == "" || addr == "" || namespace == "" {
		t.Skip("set disposable LV_TEST_WORKER_DB_DSN, LV_TEST_TEMPORAL_ADDR, and LV_TEST_TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	markerID := uuid.NewString()
	if err := dbConn.DB.WithContext(ctx).Exec(`
		INSERT INTO infra.processed_event(id, consumer, event_id, create_time)
		VALUES (?::uuid, 'worker-role-test', ?::uuid, ?)
	`, markerID, uuid.NewString(), time.Now().UTC().Add(-31*24*time.Hour)).Error; err != nil {
		t.Fatalf("insert expired marker: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.DB.Exec("DELETE FROM infra.processed_event WHERE id = ?::uuid", markerID).Error })

	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		workerDone <- app.RunWorker(workerCtx, config.Config{
			DBDSN: dsn, TemporalAddr: addr, TemporalNamespace: namespace,
			WorkerHealthAddr: healthAddr,
		}, zap.NewNop(), "flow")
	}()
	t.Cleanup(func() {
		stopWorker()
		select {
		case err := <-workerDone:
			if err != nil {
				t.Errorf("stop worker: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("worker did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)

	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(workflowClient.Close)
	run, err := workflowClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: "worker-role-test-" + uuid.NewString(), TaskQueue: maintenanceflow.FlowTaskQueue,
	}, maintenanceflow.ProcessedEventCleanupWorkflow)
	if err != nil {
		t.Fatalf("start cleanup workflow: %v", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("cleanup workflow: %v", err)
	}
	var count int64
	if err := dbConn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE id = ?::uuid", markerID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired marker after worker run = %d, error = %v", count, err)
	}
}
