package maintenance_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestScheduledMaintenanceRunsOnPostgres(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_MAINTENANCE_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated maintenance database")
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
		if err := conn.DB.Exec("DELETE FROM infra.task_work WHERE business_id = ?", markerID).Error; err != nil {
			t.Errorf("remove test maintenance tasks: %v", err)
		}
		if err := conn.DB.Exec("DELETE FROM infra.processed_event WHERE id = ?::uuid", markerID).Error; err != nil {
			t.Errorf("remove test marker: %v", err)
		}
		if err := conn.DB.Exec("DELETE FROM infra.outbox WHERE id IN (?::uuid, ?::uuid)", outboxExpiredID, outboxPendingID).Error; err != nil {
			t.Errorf("remove test Outbox events: %v", err)
		}
	})

	service := application.NewService(pgoutbox.NewPartitionStore(conn.DB), pginbox.NewStore(conn.DB))
	handlers := maintenanceflow.Handlers(service)
	for _, kind := range []string{maintenanceflow.Inbox, maintenanceflow.Outbox} {
		if err := taskruntime.Enqueue(ctx, conn.DB, kind, markerID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate an old partition task left leased by a stopped worker. Recovery
	// must complete its original execution rather than issue DDL or schedule again.
	legacyDue := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if err := taskruntime.Enqueue(ctx, conn.DB, maintenanceflow.Partitions, markerID, legacyDue); err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE infra.task_work
		SET owner = 'stopped-maintenance-worker', epoch = 1, attempt = 1,
		    lease_until = clock_timestamp() - interval '1 second', failures = 2
		WHERE kind = ? AND business_id = ?
	`, maintenanceflow.Partitions, markerID).Error; err != nil {
		t.Fatalf("simulate interrupted partition task: %v", err)
	}
	taskfixture.Start(t, conn.DB, handlers)
	taskfixture.WaitWork(t, conn.DB, maintenanceflow.Partitions, markerID, 5*time.Second)
	var completed int64
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.task_work
		WHERE kind = ? AND business_id = ? AND next_run_at = ?
		  AND completed_at IS NOT NULL AND failed_at IS NULL
		  AND owner IS NULL AND lease_until IS NULL
		  AND epoch = 2 AND attempt = 2 AND failures = 2
	`, maintenanceflow.Partitions, markerID, legacyDue).Scan(&completed).Error; err != nil || completed != 1 {
		t.Fatalf("legacy partition task must complete without retry or reschedule: count=%d err=%v", completed, err)
	}
	for {
		var count int
		if err := conn.DB.Raw("SELECT count(*) FROM infra.task_work WHERE business_id=? AND next_run_at>clock_timestamp() AND epoch>0", markerID).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
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
