package inbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestProcessOnceRollsBackAndDeduplicates(t *testing.T) {
	dsn := os.Getenv("LV_TEST_INBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_INBOX_DB_DSN to a disposable database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TABLE inbox_test_effect (
			consumer text NOT NULL,
			event_id uuid NOT NULL,
			PRIMARY KEY (consumer, event_id)
		)
	`).Error; err != nil {
		t.Fatalf("create test effect table: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.DB.Exec("DROP TABLE inbox_test_effect").Error; err != nil {
			t.Errorf("drop test effect table: %v", err)
		}
	})

	eventID := uuid.NewString()
	t.Cleanup(func() {
		if err := conn.DB.Exec("DELETE FROM infra.processed_event WHERE consumer IN (?, ?) AND event_id = ?::uuid", "realtime", "audit", eventID).Error; err != nil {
			t.Errorf("remove processed event: %v", err)
		}
	})
	store := pginbox.NewStore(conn.DB)
	wantErr := errors.New("side effect failed")
	failedHandler := func(ctx context.Context, tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Exec(
			"INSERT INTO inbox_test_effect(consumer, event_id) VALUES (?, ?::uuid)", "realtime", eventID,
		).Error; err != nil {
			return err
		}
		return wantErr
	}
	if applied, err := store.ProcessOnce(ctx, "realtime", eventID, failedHandler); applied || !errors.Is(err, wantErr) {
		t.Fatalf("failed handling = (%t, %v)", applied, err)
	}
	var effects, processed int64
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM inbox_test_effect").Scan(&effects).Error; err != nil || effects != 0 {
		t.Fatalf("effects after rollback = %d, error = %v", effects, err)
	}
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = ? AND event_id = ?::uuid", "realtime", eventID).Scan(&processed).Error; err != nil || processed != 0 {
		t.Fatalf("processed markers after rollback = %d, error = %v", processed, err)
	}

	handleCalls := 0
	handler := func(ctx context.Context, tx *gorm.DB) error {
		handleCalls++
		return tx.WithContext(ctx).Exec(
			"INSERT INTO inbox_test_effect(consumer, event_id) VALUES (?, ?::uuid)", "realtime", eventID,
		).Error
	}
	if applied, err := store.ProcessOnce(ctx, "realtime", eventID, handler); !applied || err != nil {
		t.Fatalf("retry = (%t, %v)", applied, err)
	}
	if applied, err := store.ProcessOnce(ctx, "realtime", eventID, handler); applied || err != nil {
		t.Fatalf("duplicate = (%t, %v)", applied, err)
	}
	if handleCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handleCalls)
	}
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM inbox_test_effect").Scan(&effects).Error; err != nil || effects != 1 {
		t.Fatalf("effects after retry = %d, error = %v", effects, err)
	}
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = ? AND event_id = ?::uuid", "realtime", eventID).Scan(&processed).Error; err != nil || processed != 1 {
		t.Fatalf("processed markers after retry = %d, error = %v", processed, err)
	}
	if applied, err := store.ProcessOnce(ctx, "audit", eventID, func(ctx context.Context, tx *gorm.DB) error {
		return tx.WithContext(ctx).Exec(
			"INSERT INTO inbox_test_effect(consumer, event_id) VALUES (?, ?::uuid)", "audit", eventID,
		).Error
	}); !applied || err != nil {
		t.Fatalf("same event for another consumer = (%t, %v)", applied, err)
	}
}
