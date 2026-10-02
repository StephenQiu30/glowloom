package script_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestScriptMigrationPGFullDDLRuntimeColumnACLAndAtomicRollback(t *testing.T) {
	_, owner := scriptTestDB(t)
	var connected string
	if err := owner.Raw(`SELECT current_database()`).Scan(&connected).Error; err != nil || connected != "lanverse_script" {
		t.Fatal("DDL gate requires task-owned isolated database", connected, err)
	}
	dsn, err := url.Parse(os.Getenv("LV_TEST_SCRIPT_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "lanverse_script_ddl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := owner.Exec(`CREATE DATABASE ` + name).Error; err != nil {
		t.Fatal(err)
	}
	// This exact fresh test database has no business tables, credentials or data.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := owner.WithContext(ctx).Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`).Error; err != nil {
			t.Error(err)
		}
	})
	dsn.Path = "/" + name
	client, err := platformdb.Open(t.Context(), dsn.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	up, err := os.ReadFile("../../db/migrations/202610020050_script.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DB.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(up)).Error }); err != nil {
		t.Fatal("fresh complete DDL", err)
	}
	var tables int64
	if err := client.DB.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='script' AND table_type='BASE TABLE'`).Scan(&tables).Error; err != nil || tables != 31 {
		t.Fatal("complete schema missing tables", tables, err)
	}
	for _, table := range []string{"script_source", "script_version", "version_source", "split_set", "episode_structure", "scene", "dialogue_line", "action_line", "split_confirmation", "command", "command_result", "object_intent", "review_command", "request", "source_control", "copy_snapshot", "copy_object_intent", "copy_receipt", "import_job", "import_file", "import_attempt", "import_file_result", "import_publication", "import_command"} {
		var insert, update, deleteAllowed bool
		if err := client.DB.Raw(`SELECT has_table_privilege('lanverse_app',?,'INSERT'),has_table_privilege('lanverse_app',?,'UPDATE'),has_table_privilege('lanverse_app',?,'DELETE')`, "script."+table, "script."+table, "script."+table).Row().Scan(&insert, &update, &deleteAllowed); err != nil || !insert || update || deleteAllowed {
			t.Fatal("immutable runtime ACL", table, insert, update, deleteAllowed, err)
		}
	}
	for _, column := range []string{"revision", "status", "cancellation_requested", "reconciliation_requested", "needs_reconciliation", "io_owner_id", "io_state"} {
		var allowed bool
		if err := client.DB.Raw(`SELECT has_column_privilege('lanverse_app','script.import_state',?,'UPDATE')`, column).Scan(&allowed).Error; err != nil || !allowed {
			t.Fatal("own state column denied", column, allowed, err)
		}
	}
	var immutable bool
	if err := client.DB.Raw(`SELECT has_column_privilege('lanverse_app','script.import_state','job_id','UPDATE')`).Scan(&immutable).Error; err != nil || immutable {
		t.Fatal("worker may rebind job identity", immutable, err)
	}
	// Re-applying a non-idempotent full migration must fail atomically, retaining
	// the original schema and permissions rather than applying half a contract.
	if err := client.DB.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(up)).Error }); err == nil {
		t.Fatal("unexpected repeated schema success")
	}
	var after int64
	if err := client.DB.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='script' AND table_type='BASE TABLE'`).Scan(&after).Error; err != nil || after != tables {
		t.Fatal("DDL failure changed original schema", after, err)
	}
}
