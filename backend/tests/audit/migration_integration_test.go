package audit_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditLogPartitionsAndAppendOnly(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN to a disposable database with audit migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	now := time.Now().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows := []struct {
		name      string
		created   time.Time
		partition string
	}{
		{"current", now, "audit.audit_log_" + currentMonth.Format("200601")},
		{"overflow", currentMonth.AddDate(0, 5, 0), "audit.audit_log_default"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			id := uuid.NewString()
			if err := conn.DB.WithContext(ctx).Exec(`
				INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, before, after, create_time)
				VALUES (?::uuid, ?::uuid, 'user', 'budget.changed', 'budget', ?, '{"old":1}', '{"new":2}', ?)
			`, id, uuid.NewString(), uuid.NewString(), row.created).Error; err != nil {
				t.Fatalf("insert audit record: %v", err)
			}
			var partition string
			if err := conn.DB.WithContext(ctx).Raw(
				"SELECT tableoid::regclass::text FROM audit.audit_log WHERE id = ?::uuid", id,
			).Scan(&partition).Error; err != nil || partition != row.partition {
				t.Fatalf("record partition = %q, error = %v, want %q", partition, err, row.partition)
			}
			updateErr := conn.DB.WithContext(ctx).Exec(
				"UPDATE audit.audit_log SET after = '{}' WHERE id = ?::uuid", id,
			).Error
			assertSQLState(t, updateErr, "42501")
			deleteErr := conn.DB.WithContext(ctx).Exec(
				"DELETE FROM audit.audit_log WHERE id = ?::uuid", id,
			).Error
			assertSQLState(t, deleteErr, "42501")
			var count int64
			if err := conn.DB.WithContext(ctx).Raw(
				"SELECT count(*) FROM audit.audit_log WHERE id = ?::uuid AND after->>'new' = '2'", id,
			).Scan(&count).Error; err != nil || count != 1 {
				t.Fatalf("audit record after rejected mutations = %d, error = %v", count, err)
			}
		})
	}
	logicalDeleteErr := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, is_delete)
		VALUES (?::uuid, ?::uuid, 'system', 'test.deleted', 'test', '1', true)
	`, uuid.NewString(), uuid.NewString()).Error
	assertSQLState(t, logicalDeleteErr, "23514")
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("PostgreSQL error = %v, want SQLSTATE %s", err, want)
	}
}
