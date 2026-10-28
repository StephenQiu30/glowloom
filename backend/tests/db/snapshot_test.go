package db_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestReadSnapshotRetriesWholeTransactionAfterConcurrentUpdate(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	table := fmt.Sprintf("snapshot_retry_%d", os.Getpid())
	if err := conn.DB.Exec("CREATE TABLE " + table + " (id int PRIMARY KEY, value int); INSERT INTO " + table + " VALUES (1,1)").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.DB.Exec("DROP TABLE " + table).Error; err != nil {
			t.Error(err)
		}
	})
	calls := 0
	var got int
	err = db.ReadSnapshot(t.Context(), conn.DB, func(tx *gorm.DB) error {
		calls++
		var value int
		if err := tx.Raw("SELECT value FROM " + table + " WHERE id=1").Scan(&value).Error; err != nil {
			return err
		}
		if calls == 1 {
			if err := conn.DB.Exec("UPDATE " + table + " SET value=2 WHERE id=1").Error; err != nil {
				return err
			}
		}
		err := tx.Raw("SELECT value FROM " + table + " WHERE id=1 FOR SHARE").Scan(&value).Error
		got = value
		return err
	})
	if err != nil || got != 2 || calls != 2 {
		t.Fatalf("snapshot=%d attempts=%d error=%v", got, calls, err)
	}
}

func TestReadSnapshotBoundsRetriesAndDoesNotRetryOtherErrors(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for _, code := range []string{"40001", "40P01", "23505"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			want := &pgconn.PgError{Code: code}
			err := db.ReadSnapshot(t.Context(), conn.DB, func(_ *gorm.DB) error { calls++; return fmt.Errorf("read: %w", want) })
			attempts := 1
			if code == "40001" {
				attempts = 3
			}
			if calls != attempts || !errors.Is(err, want) {
				t.Fatalf("attempts=%d error=%v", calls, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	err = db.ReadSnapshot(ctx, conn.DB, func(_ *gorm.DB) error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled attempts=%d error=%v", calls, err)
	}
}
