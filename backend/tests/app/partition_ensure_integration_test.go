package app_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// The three DSNs must point to the same disposable, migrated database. The
// application account must be restricted, the maintenance account must own
// both partitioned tables, and the third account must own neither table.
func TestEnsurePartitionsUsesSeparateOwnerAndRollsBackBothTables(t *testing.T) {
	appDSN := os.Getenv("LV_TEST_PARTITIONS_DB_DSN")
	ownerDSN := os.Getenv("LV_TEST_PARTITIONS_MAINTENANCE_DB_DSN")
	nonOwnerDSN := os.Getenv("LV_TEST_PARTITIONS_NONOWNER_DB_DSN")
	if appDSN == "" || ownerDSN == "" || nonOwnerDSN == "" {
		t.Skip("set LV_TEST_PARTITIONS_DB_DSN, LV_TEST_PARTITIONS_MAINTENANCE_DB_DSN, and LV_TEST_PARTITIONS_NONOWNER_DB_DSN for one disposable migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	appConn := openPartitionTestDB(ctx, t, appDSN)
	ownerConn := openPartitionTestDB(ctx, t, ownerDSN)
	nonOwnerConn := openPartitionTestDB(ctx, t, nonOwnerDSN)
	assertPartitionTestRoles(ctx, t, appConn.DB, ownerConn.DB, nonOwnerConn.DB)

	current := time.Now().UTC()
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 4, 0)
	for _, table := range []string{"infra.outbox", "audit.audit_log"} {
		var rowCount int64
		if err := ownerConn.DB.WithContext(ctx).Table(table).Count(&rowCount).Error; err != nil {
			t.Fatalf("count rows in %s before isolated test: %v", table, err)
		}
		if rowCount != 0 {
			t.Fatalf("%s contains %d rows; use an empty disposable database", table, rowCount)
		}
		for offset := range 4 {
			month := first.AddDate(0, offset, 0)
			assertPartitionAttached(ctx, t, appConn.DB, table, month, false)
			name := table + "_" + month.Format("200601")
			var exists bool
			if err := ownerConn.DB.WithContext(ctx).Raw("SELECT to_regclass(?) IS NOT NULL", name).Scan(&exists).Error; err != nil {
				t.Fatalf("check %s before isolated test: %v", name, err)
			}
			if exists {
				t.Fatalf("%s already exists; use a new disposable database", name)
			}
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := ownerConn.DB.WithContext(cleanupCtx).Exec("TRUNCATE infra.outbox, audit.audit_log").Error; err != nil {
			t.Errorf("clear isolated partition test rows: %v", err)
		}
		for _, table := range []string{"infra.outbox", "audit.audit_log"} {
			for offset := range 4 {
				name := table + "_" + first.AddDate(0, offset, 0).Format("200601")
				if err := ownerConn.DB.WithContext(cleanupCtx).Exec("DROP TABLE IF EXISTS " + name).Error; err != nil {
					t.Errorf("drop isolated test partition %s: %v", name, err)
				}
			}
		}
	})

	rowTime := first.Add(24 * time.Hour)
	outboxID, auditID, orgID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := appConn.DB.WithContext(ctx).Exec(`
		INSERT INTO infra.outbox(id, topic, partition_key, payload, create_time)
		VALUES (?::uuid, 'lanverse.partition.test.v1', ?, ?::jsonb, ?)
	`, outboxID, orgID, `{"test":"partition-maintenance"}`, rowTime).Error; err != nil {
		t.Fatalf("insert Outbox row through application role: %v", err)
	}
	if err := appConn.DB.WithContext(ctx).Exec(`
		INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, after, create_time)
		VALUES (?::uuid, ?::uuid, 'system', 'partition.checked', 'test', ?, ?::jsonb, ?)
	`, auditID, orgID, auditID, `{"test":"partition-maintenance"}`, rowTime).Error; err != nil {
		t.Fatalf("insert audit row through application role: %v", err)
	}
	assertPartitionRow(ctx, t, appConn.DB, "infra.outbox", outboxID, "infra.outbox_default")
	assertPartitionRow(ctx, t, appConn.DB, "audit.audit_log", auditID, "audit.audit_log_default")

	logger := zap.NewNop()
	for _, check := range []struct {
		name           string
		maintenanceDSN string
	}{
		{"missing maintenance connection", ""},
		{"same application and maintenance account", appDSN},
		{"maintenance account does not own the tables", nonOwnerDSN},
	} {
		t.Run(check.name, func(t *testing.T) {
			err := app.EnsurePartitions(ctx, config.Config{
				DBDSN:                     appDSN,
				PartitionMaintenanceDBDSN: check.maintenanceDSN,
			}, logger, first)
			if err == nil {
				t.Fatal("partition maintenance unexpectedly accepted invalid role configuration")
			}
			assertPartitionRow(ctx, t, appConn.DB, "infra.outbox", outboxID, "infra.outbox_default")
			assertPartitionRow(ctx, t, appConn.DB, "audit.audit_log", auditID, "audit.audit_log_default")
			assertPartitionAttached(ctx, t, appConn.DB, "infra.outbox", first, false)
			assertPartitionAttached(ctx, t, appConn.DB, "audit.audit_log", first, false)
		})
	}

	// A conflict in audit's second month must undo the Outbox move and attach
	// performed earlier in the same command, as well as audit's first month.
	conflictMonth := first.AddDate(0, 1, 0)
	conflictTable := "audit.audit_log_" + conflictMonth.Format("200601")
	if err := ownerConn.DB.WithContext(ctx).Exec("CREATE TABLE " + conflictTable + " (conflict_marker integer)").Error; err != nil {
		t.Fatalf("create conflicting audit month: %v", err)
	}
	conflictExists := true
	t.Cleanup(func() {
		if conflictExists {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			if err := ownerConn.DB.WithContext(cleanupCtx).Exec("DROP TABLE " + conflictTable).Error; err != nil {
				t.Errorf("drop test conflict table: %v", err)
			}
		}
	})
	cfg := config.Config{DBDSN: appDSN, PartitionMaintenanceDBDSN: ownerDSN}
	if err := app.EnsurePartitions(ctx, cfg, logger, first); err == nil {
		t.Fatal("conflicting audit month unexpectedly allowed maintenance")
	}
	for _, table := range []string{"infra.outbox", "audit.audit_log"} {
		assertPartitionAttached(ctx, t, appConn.DB, table, first, false)
	}
	assertPartitionRow(ctx, t, appConn.DB, "infra.outbox", outboxID, "infra.outbox_default")
	assertPartitionRow(ctx, t, appConn.DB, "audit.audit_log", auditID, "audit.audit_log_default")
	if err := ownerConn.DB.WithContext(ctx).Exec("DROP TABLE " + conflictTable).Error; err != nil {
		t.Fatalf("remove test conflict before successful maintenance: %v", err)
	}
	conflictExists = false

	if err := app.EnsurePartitions(ctx, cfg, logger, first); err != nil {
		t.Fatalf("maintain Outbox and audit partitions through owner account: %v", err)
	}
	for _, table := range []string{"infra.outbox", "audit.audit_log"} {
		for offset := range 4 {
			assertPartitionAttached(ctx, t, appConn.DB, table, first.AddDate(0, offset, 0), true)
		}
	}
	assertPartitionRow(ctx, t, appConn.DB, "infra.outbox", outboxID, "infra.outbox_"+first.Format("200601"))
	assertPartitionRow(ctx, t, appConn.DB, "audit.audit_log", auditID, "audit.audit_log_"+first.Format("200601"))
	assertPartitionTestPayloads(ctx, t, appConn.DB, outboxID, auditID, rowTime)

	if err := app.EnsurePartitions(ctx, cfg, logger, first); err != nil {
		t.Fatalf("repeat partition maintenance: %v", err)
	}
	assertPartitionRow(ctx, t, appConn.DB, "infra.outbox", outboxID, "infra.outbox_"+first.Format("200601"))
	assertPartitionRow(ctx, t, appConn.DB, "audit.audit_log", auditID, "audit.audit_log_"+first.Format("200601"))
	assertPartitionTestPayloads(ctx, t, appConn.DB, outboxID, auditID, rowTime)
}

func openPartitionTestDB(ctx context.Context, t *testing.T, dsn string) *db.Connection {
	t.Helper()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open preconfigured partition test database: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close partition test database connection: %v", err)
		}
	})
	return conn
}

func assertPartitionTestRoles(ctx context.Context, t *testing.T, appDB, ownerDB, nonOwnerDB *gorm.DB) {
	t.Helper()
	identities := make([]struct {
		Database string
		Role     string
		Super    bool
	}, 3)
	for index, handle := range []*gorm.DB{appDB, ownerDB, nonOwnerDB} {
		if err := handle.WithContext(ctx).Raw(`
			SELECT current_database() AS database, current_user AS role, r.rolsuper AS super
			FROM pg_roles AS r WHERE r.rolname = current_user
		`).Scan(&identities[index]).Error; err != nil {
			t.Fatalf("read partition test role %d: %v", index, err)
		}
	}
	if identities[0].Database == "" || identities[0].Database != identities[1].Database || identities[0].Database != identities[2].Database {
		t.Fatal("partition test DSNs must name one PostgreSQL database")
	}
	if identities[0].Role == identities[1].Role || identities[0].Role == identities[2].Role || identities[1].Role == identities[2].Role {
		t.Fatal("partition test requires three distinct current roles")
	}
	if identities[0].Super || identities[2].Super {
		t.Fatal("application and non-owner partition test roles must not be superusers")
	}
	for _, table := range []string{"infra.outbox", "audit.audit_log"} {
		var owner string
		if err := ownerDB.WithContext(ctx).Raw(
			"SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = ?::regclass", table,
		).Scan(&owner).Error; err != nil {
			t.Fatalf("read %s owner: %v", table, err)
		}
		if owner != identities[1].Role {
			t.Fatalf("%s owner = %q, want maintenance current role %q", table, owner, identities[1].Role)
		}
		for _, restricted := range []struct {
			name   string
			handle *gorm.DB
		}{
			{"application", appDB},
			{"non-owner", nonOwnerDB},
		} {
			var member bool
			if err := restricted.handle.WithContext(ctx).Raw("SELECT pg_has_role(current_user, ?, 'member')", owner).Scan(&member).Error; err != nil {
				t.Fatalf("check %s ownership of %s: %v", restricted.name, table, err)
			}
			if member {
				t.Fatalf("%s role must not inherit ownership of %s", restricted.name, table)
			}
		}
	}
}

func assertPartitionAttached(ctx context.Context, t *testing.T, handle *gorm.DB, parent string, month time.Time, want bool) {
	t.Helper()
	prefix := "outbox_"
	schema := "infra"
	if parent == "audit.audit_log" {
		prefix, schema = "audit_log_", "audit"
	}
	var attached bool
	if err := handle.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_inherits AS i
			JOIN pg_class AS c ON c.oid = i.inhrelid
			JOIN pg_namespace AS n ON n.oid = c.relnamespace
			WHERE i.inhparent = ?::regclass AND n.nspname = ? AND c.relname = ?
		)
	`, parent, schema, prefix+month.Format("200601")).Scan(&attached).Error; err != nil {
		t.Fatalf("check %s month %s: %v", parent, month.Format("200601"), err)
	}
	if attached != want {
		t.Fatalf("%s month %s attached = %t, want %t", parent, month.Format("200601"), attached, want)
	}
}

func assertPartitionRow(ctx context.Context, t *testing.T, handle *gorm.DB, parent, id, want string) {
	t.Helper()
	if parent != "infra.outbox" && parent != "audit.audit_log" {
		t.Fatalf("unsupported partition parent %q", parent)
	}
	var actual string
	query := fmt.Sprintf("SELECT tableoid::regclass::text FROM %s WHERE id = ?::uuid", parent)
	if err := handle.WithContext(ctx).Raw(query, id).Scan(&actual).Error; err != nil || actual != want {
		t.Fatalf("row %s in %s partition = %q, error = %v, want %q", id, parent, actual, err, want)
	}
}

func assertPartitionTestPayloads(ctx context.Context, t *testing.T, handle *gorm.DB, outboxID, auditID string, rowTime time.Time) {
	t.Helper()
	for _, item := range []struct {
		table string
		id    string
		field string
	}{
		{"infra.outbox", outboxID, "payload"},
		{"audit.audit_log", auditID, "after"},
	} {
		var result struct {
			Count int64
			Value string
		}
		query := fmt.Sprintf(`
			SELECT count(*) AS count, max(%s->>'test') AS value
			FROM %s WHERE id = ?::uuid AND create_time = ?
		`, item.field, item.table)
		if err := handle.WithContext(ctx).Raw(query, item.id, rowTime).Scan(&result).Error; err != nil {
			t.Fatalf("read preserved %s record: %v", item.table, err)
		}
		if result.Count != 1 || result.Value != "partition-maintenance" {
			t.Fatalf("preserved %s record = %+v, want one unchanged row", item.table, result)
		}
	}
}
