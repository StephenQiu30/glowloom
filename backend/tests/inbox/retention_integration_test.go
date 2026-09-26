package inbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestPruneExpiredRejectsUnboundedBatch(t *testing.T) {
	store := pginbox.NewStore(nil)
	if _, err := store.PruneExpired(t.Context(), time.Now(), 0); !errors.Is(err, pginbox.ErrInvalidPruneBatchSize) {
		t.Fatalf("zero batch size error = %v", err)
	}
}

func TestPruneExpiredProcessesOnlyOldMarkersInBatches(t *testing.T) {
	dsn := os.Getenv("LV_TEST_INBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_INBOX_DB_DSN to a disposable database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	now := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour)
	markers := []struct {
		id      string
		created time.Time
	}{
		{uuid.NewString(), cutoff.Add(-time.Second)},
		{uuid.NewString(), cutoff.Add(-time.Minute)},
		{uuid.NewString(), cutoff},
		{uuid.NewString(), cutoff.Add(time.Second)},
	}
	for _, marker := range markers {
		if err := conn.DB.WithContext(ctx).Exec(`
			INSERT INTO infra.processed_event(id, consumer, event_id, create_time)
			VALUES (?::uuid, ?, ?::uuid, ?)
		`, marker.id, "retention-test", uuid.NewString(), marker.created).Error; err != nil {
			t.Fatalf("insert marker: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, marker := range markers {
			if err := conn.DB.Exec("DELETE FROM infra.processed_event WHERE id = ?::uuid", marker.id).Error; err != nil {
				t.Errorf("remove marker: %v", err)
			}
		}
	})

	store := pginbox.NewStore(conn.DB)
	for _, want := range []int64{1, 1, 0} {
		deleted, err := store.PruneExpired(ctx, now, 1)
		if err != nil || deleted != want {
			t.Fatalf("prune expired = (%d, %v), want (%d, nil)", deleted, err, want)
		}
	}
	for i, marker := range markers {
		var count int64
		if err := conn.DB.WithContext(ctx).Raw(
			"SELECT count(*) FROM infra.processed_event WHERE id = ?::uuid", marker.id,
		).Scan(&count).Error; err != nil {
			t.Fatalf("count marker %d: %v", i, err)
		}
		want := int64(1)
		if i < 2 {
			want = 0
		}
		if count != want {
			t.Fatalf("marker %d count = %d, want %d", i, count, want)
		}
	}
}
