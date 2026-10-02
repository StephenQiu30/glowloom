package db_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// The supplied database must be empty. Every probe rolls back its own changes.
func TestSchemaInitializesEmptyDatabaseAndRejectsReplay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_SCHEMA_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_SCHEMA_DB_DSN to an empty disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open schema test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pool, err := conn.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		t.Fatalf("initialize empty database from schema.sql: %v", err)
	}
	for _, parent := range []string{"audit.audit_log", "infra.outbox", "operation.provider_call"} {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_inherits WHERE inhparent = $1::regclass`, parent).Scan(&count); err != nil || count != 5 {
			t.Fatalf("%s partitions = %d, error = %v; want current and three future UTC months plus default", parent, count, err)
		}
	}
	var canAppend, canRead, canUpdate, canDelete, canTruncate bool
	if err := tx.QueryRowContext(ctx, `SELECT
		has_table_privilege('lanverse_app', 'audit.audit_log', 'INSERT'),
		has_table_privilege('lanverse_app', 'audit.audit_log', 'SELECT'),
		has_table_privilege('lanverse_app', 'audit.audit_log', 'UPDATE'),
		has_table_privilege('lanverse_app', 'audit.audit_log', 'DELETE'),
		has_table_privilege('lanverse_app', 'audit.audit_log', 'TRUNCATE')
	`).Scan(&canAppend, &canRead, &canUpdate, &canDelete, &canTruncate); err != nil {
		t.Fatal(err)
	}
	if !canAppend || !canRead || canUpdate || canDelete || canTruncate {
		t.Fatalf("audit privileges insert/select/update/delete/truncate = %t/%t/%t/%t/%t", canAppend, canRead, canUpdate, canDelete, canTruncate)
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT schema_replay"); err != nil {
		t.Fatal(err)
	}
	_, replayErr := tx.ExecContext(ctx, string(schema))
	if replayErr == nil || !strings.Contains(replayErr.Error(), "requires an empty business database") {
		t.Fatalf("schema replay error = %v; want explicit nonempty database rejection", replayErr)
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT schema_replay"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var remains bool
	if err := pool.QueryRowContext(ctx, "SELECT to_regnamespace('audit') IS NOT NULL").Scan(&remains); err != nil || remains {
		t.Fatalf("rollback left schema objects = %t, error = %v", remains, err)
	}
}
