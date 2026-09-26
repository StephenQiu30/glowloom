package outbox_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestDropEmptyPastPartitionsPreservesUnpublishedEvents(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Second)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	past := first.AddDate(0, -3, 0)
	store := pgoutbox.NewPartitionStore(conn.DB)
	if err := store.EnsurePartitions(ctx, past); err != nil {
		t.Fatalf("create past partitions: %v", err)
	}
	oldPublished := now.Add(-8 * 24 * time.Hour)
	recentPublished := now.Add(-time.Hour)
	rows := []struct {
		id        string
		created   time.Time
		published *time.Time
	}{
		{uuid.NewString(), past.Add(24 * time.Hour), nil},
		{uuid.NewString(), past.AddDate(0, 1, 0).Add(24 * time.Hour), &recentPublished},
	}
	t.Cleanup(func() {
		for _, row := range rows {
			if err := conn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", row.id).Error; err != nil {
				t.Errorf("remove event: %v", err)
			}
		}
	})
	for _, row := range rows {
		if err := conn.DB.WithContext(ctx).Exec(`
			INSERT INTO infra.outbox(id, topic, partition_key, payload, create_time, published_at)
			VALUES (?::uuid, ?, ?, ?::jsonb, ?, ?)
		`, row.id, "lanverse.operation.status_changed.v1", uuid.NewString(), `{}`, row.created, row.published).Error; err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	if dropped, err := store.DropEmptyPastPartitions(ctx, now); err != nil || dropped != 1 {
		t.Fatalf("first cleanup = (%d, %v), want (1, nil)", dropped, err)
	}
	assertPartitionExists(ctx, t, conn.DB, past, true)
	assertPartitionExists(ctx, t, conn.DB, past.AddDate(0, 1, 0), true)
	assertPartitionExists(ctx, t, conn.DB, past.AddDate(0, 2, 0), false)
	assertPartitionExists(ctx, t, conn.DB, first, true)

	if err := conn.DB.WithContext(ctx).Exec(
		"UPDATE infra.outbox SET published_at = ? WHERE id IN (?::uuid, ?::uuid)",
		oldPublished, rows[0].id, rows[1].id,
	).Error; err != nil {
		t.Fatalf("mark old events published: %v", err)
	}
	if deleted, err := store.PrunePublished(ctx, now, 10); err != nil || deleted != 2 {
		t.Fatalf("prune published = (%d, %v), want (2, nil)", deleted, err)
	}
	if dropped, err := store.DropEmptyPastPartitions(ctx, now); err != nil || dropped != 2 {
		t.Fatalf("second cleanup = (%d, %v), want (2, nil)", dropped, err)
	}
	for offset := range 3 {
		assertPartitionExists(ctx, t, conn.DB, past.AddDate(0, offset, 0), false)
	}
	assertPartitionExists(ctx, t, conn.DB, first, true)
	if dropped, err := store.DropEmptyPastPartitions(ctx, now); err != nil || dropped != 0 {
		t.Fatalf("repeat cleanup = (%d, %v), want (0, nil)", dropped, err)
	}
}

func assertPartitionExists(ctx context.Context, t *testing.T, db *gorm.DB, month time.Time, want bool) {
	t.Helper()
	var exists bool
	name := "infra.outbox_" + month.Format("200601")
	if err := db.WithContext(ctx).Raw("SELECT to_regclass(?) IS NOT NULL", name).Scan(&exists).Error; err != nil {
		t.Fatalf("check partition %s: %v", name, err)
	}
	if exists != want {
		t.Fatalf("partition %s exists = %t, want %t", name, exists, want)
	}
}
