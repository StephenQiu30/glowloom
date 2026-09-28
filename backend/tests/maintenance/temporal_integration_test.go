package maintenance_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestScheduledMaintenanceRunsOnLocalTemporal(t *testing.T) {
	dsn := os.Getenv("LV_TEST_MAINTENANCE_DB_DSN")
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if dsn == "" || addr == "" || namespace == "" {
		t.Skip("set LV_TEST_MAINTENANCE_DB_DSN, LV_TEST_TEMPORAL_ADDR, and LV_TEST_TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	markerID := uuid.NewString()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO infra.processed_event(id, consumer, event_id, create_time)
		VALUES (?::uuid, ?, ?::uuid, ?)
	`, markerID, "maintenance-live-test", uuid.NewString(), time.Now().UTC().Add(-31*24*time.Hour)).Error; err != nil {
		t.Fatalf("insert expired marker: %v", err)
	}
	outboxExpiredID := uuid.NewString()
	outboxPendingID := uuid.NewString()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO infra.outbox(id, topic, partition_key, payload, published_at)
		VALUES (?::uuid, 'maintenance.test', 'expired', '{}', ?),
		       (?::uuid, 'maintenance.test', 'pending', '{}', NULL)
	`, outboxExpiredID, time.Now().UTC().Add(-8*24*time.Hour), outboxPendingID).Error; err != nil {
		t.Fatalf("insert Outbox events: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.DB.Exec("DELETE FROM infra.processed_event WHERE id = ?::uuid", markerID).Error; err != nil {
			t.Errorf("remove test marker: %v", err)
		}
		if err := conn.DB.Exec("DELETE FROM infra.outbox WHERE id IN (?::uuid, ?::uuid)", outboxExpiredID, outboxPendingID).Error; err != nil {
			t.Errorf("remove test Outbox events: %v", err)
		}
	})

	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(workflowClient.Close)
	queue := "maintenance-test-" + uuid.NewString()
	flowWorker := worker.New(workflowClient, queue, worker.Options{})
	service := application.NewService(pgoutbox.NewPartitionStore(conn.DB), pginbox.NewStore(conn.DB))
	maintenanceflow.Register(flowWorker, maintenanceflow.NewActivities(service, 2))
	if err := flowWorker.Start(); err != nil {
		t.Fatalf("start maintenance worker: %v", err)
	}
	t.Cleanup(flowWorker.Stop)

	awaitPeriodicScheduledWorkflow(ctx, t, workflowClient, queue, maintenanceflow.ProcessedEventCleanupWorkflow)
	awaitPeriodicScheduledWorkflow(ctx, t, workflowClient, queue, maintenanceflow.OutboxCleanupWorkflow)

	var count int64
	if err := conn.DB.WithContext(ctx).Raw(
		"SELECT count(*) FROM infra.processed_event WHERE id = ?::uuid", markerID,
	).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired marker after scheduled workflow = %d, error = %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(
		"SELECT count(*) FROM infra.outbox WHERE id = ?::uuid", outboxExpiredID,
	).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired Outbox event after scheduled workflow = %d, error = %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(
		"SELECT count(*) FROM infra.outbox WHERE id = ?::uuid", outboxPendingID,
	).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("pending Outbox event after scheduled workflow = %d, error = %v", count, err)
	}
}

func awaitPeriodicScheduledWorkflow(ctx context.Context, t *testing.T, workflowClient client.Client, queue string, workflow any) {
	t.Helper()
	scheduleID := "maintenance-test-" + uuid.NewString()
	handle, err := workflowClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID: scheduleID,
		Spec: client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{
			{Every: 3 * time.Second},
		}},
		Action: &client.ScheduleWorkflowAction{
			ID:        scheduleID,
			Workflow:  workflow,
			TaskQueue: queue,
		},
	})
	if err != nil {
		t.Fatalf("create test schedule: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := handle.Delete(cleanupCtx); err != nil {
			t.Errorf("delete test schedule: %v", err)
		}
	})
	var workflowID, runID string
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for workflowID == "" {
		description, err := handle.Describe(ctx)
		if err != nil {
			t.Fatalf("describe test schedule: %v", err)
		}
		for _, action := range description.Info.RecentActions {
			if action.StartWorkflowResult != nil {
				workflowID = action.StartWorkflowResult.WorkflowID
				runID = action.StartWorkflowResult.FirstExecutionRunID
			}
		}
		if workflowID != "" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for automatic scheduled run: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	if err := workflowClient.GetWorkflow(ctx, workflowID, runID).Get(ctx, nil); err != nil {
		t.Fatalf("scheduled workflow %s: %v", workflowID, err)
	}
}
