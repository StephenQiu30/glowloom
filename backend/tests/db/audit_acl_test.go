package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditApplicationRoleOnlyAppendsThroughParent(t *testing.T) {
	dsn := os.Getenv("LV_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_DB_DSN to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pool, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL pool: %v", err)
	}

	// The role, schema, grants and records are all created inside this transaction.
	// Rollback cleans them up without touching another database's role or grants.
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin ACL test transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleExists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lanverse_app')").Scan(&roleExists); err != nil {
		t.Fatalf("check application role: %v", err)
	}
	if !roleExists {
		if _, err := tx.ExecContext(ctx, "CREATE ROLE lanverse_app NOLOGIN"); err != nil {
			t.Fatalf("create test application role (requires CREATEROLE): %v", err)
		}
	}
	applyAuditACLMigration(ctx, t, tx, "202609270930_create_audit_log.up.sql")
	applyAuditACLMigration(ctx, t, tx, "202609271040_grant_audit_app_role.up.sql")

	var canLogin bool
	if err := tx.QueryRowContext(ctx, "SELECT rolcanlogin FROM pg_roles WHERE rolname = 'lanverse_app'").Scan(&canLogin); err != nil {
		t.Fatalf("read application role attributes: %v", err)
	}
	if canLogin {
		t.Fatal("lanverse_app must be NOLOGIN")
	}

	now := time.Now().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	futureMonth := currentMonth.AddDate(3, 1, 0)
	futurePartition := "audit.audit_log_acl_" + futureMonth.Format("200601")
	createPartition := fmt.Sprintf(
		"CREATE TABLE %s PARTITION OF audit.audit_log FOR VALUES FROM ('%s') TO ('%s')",
		futurePartition, futureMonth.Format(time.RFC3339), futureMonth.AddDate(0, 1, 0).Format(time.RFC3339),
	)
	if _, err := tx.ExecContext(ctx, createPartition); err != nil {
		t.Fatalf("create new future partition as migration owner: %v", err)
	}

	var appOwnsParent, appOwnsNewPartition bool
	if err := tx.QueryRowContext(ctx, `
		SELECT
		  (SELECT relowner = 'lanverse_app'::regrole FROM pg_class WHERE oid = 'audit.audit_log'::regclass),
		  (SELECT relowner = 'lanverse_app'::regrole FROM pg_class WHERE oid = $1::regclass)
	`, futurePartition).Scan(&appOwnsParent, &appOwnsNewPartition); err != nil {
		t.Fatalf("check audit table owners: %v", err)
	}
	if appOwnsParent || appOwnsNewPartition {
		t.Fatal("lanverse_app must not own the audit parent or a new partition")
	}

	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE lanverse_app"); err != nil {
		t.Fatalf("assume application role (requires migration owner or superuser): %v", err)
	}
	var activeRole string
	if err := tx.QueryRowContext(ctx, "SELECT current_user").Scan(&activeRole); err != nil || activeRole != "lanverse_app" {
		t.Fatalf("current_user = %q, error = %v, want lanverse_app", activeRole, err)
	}

	var canUseSchema, canInsert, canSelect, canUpdate, canDelete, canTruncate bool
	if err := tx.QueryRowContext(ctx, `
		SELECT
		  has_schema_privilege('audit', 'USAGE'),
		  has_table_privilege('audit.audit_log', 'INSERT'),
		  has_table_privilege('audit.audit_log', 'SELECT'),
		  has_table_privilege('audit.audit_log', 'UPDATE'),
		  has_table_privilege('audit.audit_log', 'DELETE'),
		  has_table_privilege('audit.audit_log', 'TRUNCATE')
	`).Scan(&canUseSchema, &canInsert, &canSelect, &canUpdate, &canDelete, &canTruncate); err != nil {
		t.Fatalf("read effective audit privileges: %v", err)
	}
	if !canUseSchema || !canInsert || !canSelect || canUpdate || canDelete || canTruncate {
		t.Fatalf("application privileges: schema=%t insert=%t select=%t update=%t delete=%t truncate=%t; want only schema usage and table INSERT/SELECT",
			canUseSchema, canInsert, canSelect, canUpdate, canDelete, canTruncate)
	}

	for _, row := range []struct {
		name      string
		created   time.Time
		partition string
	}{
		{"current", now, "audit.audit_log_" + currentMonth.Format("200601")},
		{"new future", futureMonth.Add(24 * time.Hour), futurePartition},
	} {
		t.Run(row.name, func(t *testing.T) {
			id := uuid.NewString()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, create_time)
				VALUES ($1::uuid, $2::uuid, 'system', 'acl.checked', 'audit', $3, $4)
			`, id, uuid.NewString(), id, row.created); err != nil {
				t.Fatalf("insert through audit parent: %v", err)
			}
			var actualPartition string
			if err := tx.QueryRowContext(ctx,
				"SELECT tableoid::regclass::text FROM audit.audit_log WHERE id = $1::uuid", id,
			).Scan(&actualPartition); err != nil || actualPartition != row.partition {
				t.Fatalf("parent SELECT partition = %q, error = %v, want %q", actualPartition, err, row.partition)
			}
		})
	}

	for _, stmt := range []string{
		"UPDATE audit.audit_log SET after = '{}'::jsonb",
		"DELETE FROM audit.audit_log",
		"TRUNCATE audit.audit_log",
		"ALTER TABLE audit.audit_log ADD COLUMN acl_probe text",
		"ALTER TABLE audit.audit_log OWNER TO lanverse_app",
	} {
		assertAuditACLPermissionDenied(ctx, t, tx, stmt)
	}
	if _, err := tx.ExecContext(ctx, "RESET ROLE"); err != nil {
		t.Fatalf("restore migration role: %v", err)
	}
	applyAuditACLMigration(ctx, t, tx, "202609271040_grant_audit_app_role.down.sql")
	if err := tx.QueryRowContext(ctx, `
		SELECT
		  has_schema_privilege('lanverse_app', 'audit', 'USAGE'),
		  has_table_privilege('lanverse_app', 'audit.audit_log', 'INSERT'),
		  has_table_privilege('lanverse_app', 'audit.audit_log', 'SELECT')
	`).Scan(&canUseSchema, &canInsert, &canSelect); err != nil {
		t.Fatalf("read rolled back audit privileges: %v", err)
	}
	if canUseSchema || canInsert || canSelect {
		t.Fatalf("ACL down left privileges: schema=%t insert=%t select=%t", canUseSchema, canInsert, canSelect)
	}
}

func applyAuditACLMigration(ctx context.Context, t *testing.T, tx *sql.Tx, filename string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", filename))
	if err != nil {
		t.Fatalf("read audit migration %s: %v", filename, err)
	}
	if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
		t.Fatalf("apply audit migration %s: %v", filename, err)
	}
}

func assertAuditACLPermissionDenied(ctx context.Context, t *testing.T, tx *sql.Tx, stmt string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, "SAVEPOINT audit_acl_probe"); err != nil {
		t.Fatalf("savepoint for %q: %v", stmt, err)
	}
	_, execErr := tx.ExecContext(ctx, stmt)
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT audit_acl_probe"); err != nil {
		t.Fatalf("rollback attempted %q: %v", stmt, err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT audit_acl_probe"); err != nil {
		t.Fatalf("release savepoint for %q: %v", stmt, err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(execErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("%q error = %v, want permission denied SQLSTATE 42501", stmt, execErr)
	}
}
