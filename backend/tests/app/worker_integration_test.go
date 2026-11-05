package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestWorkerRoleRunsMaintenanceOnDatabase(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_WORKER_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable worker db")
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

	storageConfig := businessStorageConfig(t)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		workerDone <- app.RunWorker(workerCtx, config.Config{
			Env:              "local",
			ObjectStorage:    &storageConfig,
			DBDSN:            dsn,
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

	if err := taskruntime.Enqueue(ctx, dbConn.DB, maintenanceflow.Inbox, markerID, time.Now()); err != nil {
		t.Fatal(err)
	}
	for {
		var ran bool
		if err := dbConn.DB.Raw("SELECT next_run_at>clock_timestamp() AND epoch>0 FROM infra.task_work WHERE kind=? AND business_id=?", maintenanceflow.Inbox, markerID).Scan(&ran).Error; err != nil {
			t.Fatal(err)
		}
		if ran {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	var count int64
	if err := dbConn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE id = ?::uuid", markerID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired marker after worker run = %d, error = %v", count, err)
	}
}
