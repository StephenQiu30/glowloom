package audit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditEnsurePartitionsMovesDefaultRowsWithoutChangingRecords(t *testing.T) {
	dsn := os.Getenv("LV_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_DB_DSN to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// The migration, test roles, records and partitions live only in this transaction.
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin partition test transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "202609270930_create_audit_log.up.sql"))
	if err != nil {
		t.Fatalf("read audit migration: %v", err)
	}
	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatalf("apply audit migration: %v", err)
	}

	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	maintenanceMonth := first.AddDate(0, 4, 0)
	remainingMonth := first.AddDate(0, 8, 0)
	oldMonth := first.AddDate(-3, -1, 0)
	rows := []struct {
		id      string
		created time.Time
	}{
		{uuid.NewString(), maintenanceMonth.Add(24 * time.Hour)},
		{uuid.NewString(), remainingMonth.Add(24 * time.Hour)},
		{uuid.NewString(), oldMonth.Add(24 * time.Hour)},
	}
	orgID := uuid.NewString()
	for _, row := range rows {
		if err := tx.Exec(`
			INSERT INTO audit.audit_log (
				id, org_id, actor_kind, action, object_type, object_id,
				before, after, request_id, trace_id, ip, create_time
			) VALUES (
				?::uuid, ?::uuid, 'system', 'partition.checked', 'audit', ?,
				'{"old":1}'::jsonb, '{"new":2}'::jsonb, 'req-partition', 'trace-partition',
				'127.0.0.1'::inet, ?
			)
		`, row.id, orgID, row.id, row.created).Error; err != nil {
			t.Fatalf("insert default record: %v", err)
		}
		assertAuditPartition(t, tx, row.id, "audit.audit_log_default")
	}

	store := pgaudit.NewPartitionStore(tx)
	role := "audit_partition_test_" + uuid.NewString()[:8]
	if err := tx.Exec("CREATE ROLE " + role + " NOLOGIN").Error; err != nil {
		t.Fatalf("create transaction-local application role: %v", err)
	}
	if err := tx.Exec("GRANT USAGE ON SCHEMA audit TO " + role).Error; err != nil {
		t.Fatalf("grant audit schema usage: %v", err)
	}
	if err := tx.Exec("GRANT INSERT, SELECT ON audit.audit_log TO " + role).Error; err != nil {
		t.Fatalf("grant append/read access: %v", err)
	}
	if err := tx.Exec("SET LOCAL ROLE " + role).Error; err != nil {
		t.Fatalf("assume application role: %v", err)
	}
	if err := store.EnsurePartitions(ctx, maintenanceMonth); err == nil {
		t.Fatal("application role unexpectedly maintained audit partitions")
	}
	if err := tx.Exec("RESET ROLE").Error; err != nil {
		t.Fatalf("restore migration role: %v", err)
	}
	assertAuditPartition(t, tx, rows[0].id, "audit.audit_log_default")
	assertAuditMutationRejected(t, tx, rows[0].id, "UPDATE")
	assertAuditMutationRejected(t, tx, rows[0].id, "DELETE")

	if err := store.EnsurePartitions(ctx, maintenanceMonth); err != nil {
		t.Fatalf("maintain current and next three UTC months: %v", err)
	}
	for offset := range 4 {
		month := maintenanceMonth.AddDate(0, offset, 0)
		var attached bool
		if err := tx.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM pg_inherits
				WHERE inhparent = 'audit.audit_log'::regclass
				  AND inhrelid = ?::regclass
			)
		`, "audit.audit_log_"+month.Format("200601")).Scan(&attached).Error; err != nil || !attached {
			t.Fatalf("month %s attached = %t, error = %v", month.Format("200601"), attached, err)
		}
	}
	assertAuditPartition(t, tx, rows[0].id, "audit.audit_log_"+maintenanceMonth.Format("200601"))
	assertAuditRecordUnchanged(t, tx, rows[0].id, rows[0].created)
	assertAuditMutationRejected(t, tx, rows[0].id, "UPDATE")
	assertAuditMutationRejected(t, tx, rows[0].id, "DELETE")
	assertAuditPartition(t, tx, rows[1].id, "audit.audit_log_default")
	assertAuditPartition(t, tx, rows[2].id, "audit.audit_log_default")
	if err := tx.Exec("SET LOCAL ROLE " + role).Error; err != nil {
		t.Fatalf("assume application role after maintenance: %v", err)
	}
	appRowID := uuid.NewString()
	if err := tx.Exec(`
		INSERT INTO audit.audit_log (id, org_id, actor_kind, action, object_type, object_id, create_time)
		VALUES (?::uuid, ?::uuid, 'system', 'partition.checked', 'audit', ?, ?)
	`, appRowID, orgID, appRowID, maintenanceMonth.Add(48*time.Hour)).Error; err != nil {
		t.Fatalf("application insert through newly attached parent partition: %v", err)
	}
	assertAuditPartition(t, tx, appRowID, "audit.audit_log_"+maintenanceMonth.Format("200601"))
	if err := tx.Exec("RESET ROLE").Error; err != nil {
		t.Fatalf("restore migration role after application read: %v", err)
	}
	if err := store.EnsurePartitions(ctx, maintenanceMonth); err != nil {
		t.Fatalf("repeat audit partition maintenance: %v", err)
	}
	assertAuditRecordUnchanged(t, tx, rows[0].id, rows[0].created)

	// The first missing month has rows to move; a conflicting second table must
	// roll back the first move and its attachment, including trigger changes.
	conflictMonth := remainingMonth.AddDate(0, 1, 0)
	conflictName := "audit.audit_log_" + conflictMonth.Format("200601")
	if err := tx.Exec("CREATE TABLE " + conflictName + " (conflict_marker integer)").Error; err != nil {
		t.Fatalf("create conflicting month table: %v", err)
	}
	if err := store.EnsurePartitions(ctx, remainingMonth); err == nil {
		t.Fatal("conflicting month table unexpectedly allowed maintenance")
	}
	var exists bool
	if err := tx.Raw("SELECT to_regclass(?) IS NOT NULL", "audit.audit_log_"+remainingMonth.Format("200601")).Scan(&exists).Error; err != nil || exists {
		t.Fatalf("first month table after rollback exists = %t, error = %v", exists, err)
	}
	assertAuditPartition(t, tx, rows[1].id, "audit.audit_log_default")
	assertAuditRecordUnchanged(t, tx, rows[1].id, rows[1].created)
	assertAuditMutationRejected(t, tx, rows[1].id, "UPDATE")
	assertAuditMutationRejected(t, tx, rows[1].id, "DELETE")
	assertAuditPartition(t, tx, rows[2].id, "audit.audit_log_default")
	assertAuditRecordUnchanged(t, tx, rows[2].id, rows[2].created)
}

func assertAuditPartition(t *testing.T, tx *gorm.DB, id, want string) {
	t.Helper()
	var partition string
	if err := tx.Raw("SELECT tableoid::regclass::text FROM audit.audit_log WHERE id = ?::uuid", id).Scan(&partition).Error; err != nil || partition != want {
		t.Fatalf("record %s partition = %q, error = %v, want %q", id, partition, err, want)
	}
}

func assertAuditRecordUnchanged(t *testing.T, tx *gorm.DB, id string, created time.Time) {
	t.Helper()
	var record struct {
		Count   int64
		Before  string
		After   string
		Request string
		Trace   string
		IP      string
	}
	if err := tx.Raw(`
		SELECT count(*) AS count,
		       max(before->>'old') AS before,
		       max(after->>'new') AS after,
		       max(request_id) AS request,
		       max(trace_id) AS trace,
		       max(ip::text) AS ip
		FROM audit.audit_log WHERE id = ?::uuid AND create_time = ?
	`, id, created).Scan(&record).Error; err != nil {
		t.Fatalf("read preserved audit record: %v", err)
	}
	if record.Count != 1 || record.Before != "1" || record.After != "2" ||
		record.Request != "req-partition" || record.Trace != "trace-partition" || record.IP != "127.0.0.1/32" {
		t.Fatalf("moved audit record changed: %+v", record)
	}
}

func assertAuditMutationRejected(t *testing.T, tx *gorm.DB, id, operation string) {
	t.Helper()
	if err := tx.Exec("SAVEPOINT audit_mutation_probe").Error; err != nil {
		t.Fatalf("savepoint before %s: %v", operation, err)
	}
	var stmt string
	switch operation {
	case "UPDATE":
		stmt = "UPDATE audit.audit_log SET after = '{}'::jsonb WHERE id = ?::uuid"
	case "DELETE":
		stmt = "DELETE FROM audit.audit_log WHERE id = ?::uuid"
	default:
		t.Fatalf("unsupported mutation %q", operation)
	}
	mutationErr := tx.Exec(stmt, id).Error
	if err := tx.Exec("ROLLBACK TO SAVEPOINT audit_mutation_probe").Error; err != nil {
		t.Fatalf("rollback rejected %s: %v", operation, err)
	}
	if err := tx.Exec("RELEASE SAVEPOINT audit_mutation_probe").Error; err != nil {
		t.Fatalf("release %s savepoint: %v", operation, err)
	}
	if mutationErr == nil {
		t.Fatalf("%s unexpectedly changed append-only audit record %s", operation, id)
	}
	assertSQLState(t, mutationErr, "42501")
}
