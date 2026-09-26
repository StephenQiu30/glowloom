package outbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestPrunePublishedRejectsInvalidBatch(t *testing.T) {
	store := pgoutbox.NewPartitionStore(nil)
	if _, err := store.PrunePublished(t.Context(), time.Now(), 0); !errors.Is(err, pgoutbox.ErrInvalidPruneBatchSize) {
		t.Fatalf("zero batch size error = %v", err)
	}
}

func TestPrunePublishedPreservesPendingAndRecentEvents(t *testing.T) {
	dsn := os.Getenv("LV_TEST_OUTBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_OUTBOX_DB_DSN to a disposable database with Outbox migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-7 * 24 * time.Hour)
	oldPublished := cutoff.Add(-time.Second)
	recentPublished := now.Add(-time.Hour)
	rows := []struct {
		id        string
		created   time.Time
		published *time.Time
		keep      bool
	}{
		{uuid.NewString(), now.Add(-40 * 24 * time.Hour), &oldPublished, false},
		{uuid.NewString(), now.Add(-10 * 24 * time.Hour), &oldPublished, false},
		{uuid.NewString(), now.Add(-40 * 24 * time.Hour), nil, true},
		{uuid.NewString(), now.Add(-40 * 24 * time.Hour), &recentPublished, true},
		{uuid.NewString(), now.Add(-10 * 24 * time.Hour), &cutoff, true},
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

	store := pgoutbox.NewPartitionStore(conn.DB)
	lock := conn.DB.WithContext(ctx).Begin()
	if lock.Error != nil {
		t.Fatalf("begin row lock: %v", lock.Error)
	}
	defer func() { _ = lock.Rollback().Error }()
	if err := lock.Exec("UPDATE infra.outbox SET update_time = update_time WHERE id = ?::uuid", rows[0].id).Error; err != nil {
		t.Fatalf("lock old event: %v", err)
	}
	for _, want := range []int64{1, 0} {
		deleted, err := store.PrunePublished(ctx, now, 1)
		if err != nil || deleted != want {
			t.Fatalf("prune while one event locked = (%d, %v), want (%d, nil)", deleted, err, want)
		}
	}
	if err := lock.Commit().Error; err != nil {
		t.Fatalf("release row lock: %v", err)
	}
	for _, want := range []int64{1, 0} {
		deleted, err := store.PrunePublished(ctx, now, 1)
		if err != nil || deleted != want {
			t.Fatalf("prune published = (%d, %v), want (%d, nil)", deleted, err, want)
		}
	}
	for i, row := range rows {
		var count int64
		if err := conn.DB.WithContext(ctx).Raw(
			"SELECT count(*) FROM infra.outbox WHERE id = ?::uuid", row.id,
		).Scan(&count).Error; err != nil {
			t.Fatalf("count event %d: %v", i, err)
		}
		want := int64(0)
		if row.keep {
			want = 1
		}
		if count != want {
			t.Fatalf("event %d count = %d, want %d", i, count, want)
		}
	}
}
