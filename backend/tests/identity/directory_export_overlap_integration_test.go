package identity_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// This overlap uses only locks and transaction IDs on an existing settings row.
// No actor/session/export fixtures, DDL, new database or persistent writes occur.
func TestDirectoryExportWaitingTransactionGetsFreshReceiptSnapshot(t *testing.T) {
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		t.Skip("existing database connection absent; overlap not verified")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open approved existing database")
	}
	defer func() { _ = connection.Close() }()
	pool, err := connection.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	first, err := pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback() }()
	if _, err := first.ExecContext(ctx, `SELECT singleton FROM identity.platform_settings WHERE singleton FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	var priorXID int64
	if err := first.QueryRowContext(ctx, `SELECT txid_current()`).Scan(&priorXID); err != nil {
		t.Fatal(err)
	}
	second, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = first.Rollback()
		_ = second.Close()
	}()
	var secondPID int
	if err := second.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&secondPID); err != nil {
		t.Fatal(err)
	}
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: second}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	type observed struct {
		Isolation    string
		PriorVisible bool
	}
	result := make(chan error, 1)
	observation := make(chan observed, 1)
	go func() {
		result <- pgidentity.NewDirectoryExportStore(gdb).Transaction(ctx, func(_ application.ExportTx) error {
			var row observed
			err := second.QueryRowContext(ctx, `SELECT current_setting('transaction_isolation'),txid_visible_in_snapshot($1::bigint,txid_current_snapshot())`, priorXID).Scan(&row.Isolation, &row.PriorVisible)
			observation <- row
			return err
		})
	}()
	waiting := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err := pool.QueryRowContext(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, secondPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !waiting {
		t.Fatal("second transaction never overlapped the first governance lock")
	}
	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	row := <-observation
	if row.Isolation != "read committed" || !row.PriorVisible {
		t.Fatalf("waiting creator retained stale pre-lock snapshot: %+v", row)
	}
}
