package outbox_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestEnsurePartitionsMovesDefaultRowsBeforeAttach(t *testing.T) {
	dsn := os.Getenv("LV_TEST_OUTBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_OUTBOX_DB_DSN to a disposable database with Outbox migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	current := time.Now().UTC()
	month := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 4, 0)
	rowTime := month.Add(24 * time.Hour)
	partitionName := "outbox_" + month.Format("200601")
	id := uuid.NewString()
	if err := conn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload, create_time) VALUES (?::uuid, ?, ?, ?::jsonb, ?)",
		id, "lanverse.operation.status_changed.v1", uuid.NewString(), `{"event_id":"`+id+`"}`, rowTime,
	).Error; err != nil {
		t.Fatalf("insert overflow event: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", id).Error; err != nil {
			t.Errorf("remove test event: %v", err)
		}
	})
	var before string
	if err := conn.DB.WithContext(ctx).Raw("SELECT tableoid::regclass::text FROM infra.outbox WHERE id = ?::uuid", id).Scan(&before).Error; err != nil || before != "infra.outbox_default" {
		t.Fatalf("before maintenance table = %q, error = %v", before, err)
	}
	store := pgoutbox.NewPartitionStore(conn.DB)
	maintenanceTime := month.AddDate(0, -3, 0)
	errors := make(chan error, 2)
	for range 2 {
		go func() { errors <- store.EnsurePartitions(ctx, maintenanceTime) }()
	}
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("concurrent maintenance: %v", err)
		}
	}
	if err := store.EnsurePartitions(ctx, maintenanceTime); err != nil {
		t.Fatalf("repeat maintenance: %v", err)
	}
	var after string
	if err := conn.DB.WithContext(ctx).Raw("SELECT tableoid::regclass::text FROM infra.outbox WHERE id = ?::uuid", id).Scan(&after).Error; err != nil || after != "infra."+partitionName {
		t.Fatalf("after maintenance table = %q, error = %v", after, err)
	}
	var pending bool
	if err := conn.DB.WithContext(ctx).Raw("SELECT published_at IS NULL FROM infra.outbox WHERE id = ?::uuid", id).Scan(&pending).Error; err != nil || !pending {
		t.Fatalf("moved event pending = %t, error = %v", pending, err)
	}
	var payloadID string
	if err := conn.DB.WithContext(ctx).Raw("SELECT payload->>'event_id' FROM infra.outbox WHERE id = ?::uuid AND create_time = ?", id, rowTime).Scan(&payloadID).Error; err != nil || payloadID != id {
		t.Fatalf("moved event payload ID = %q, error = %v", payloadID, err)
	}
}
