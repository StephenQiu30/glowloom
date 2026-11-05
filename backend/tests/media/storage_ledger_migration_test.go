package media_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func storageLedgerMigrationFixture(t *testing.T, shape ...string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_MIGRATION_DSN")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires isolated storage migration PostgreSQL")
		}
		t.Skip("set a newly created task-owned glowloom_subject_storage_ database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "glowloom_subject_storage_") {
		t.Fatal("storage migration fixture requires a task-owned loopback database")
	}
	conn, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	// The canonical schema is loaded ONLY into the newly created, empty fixture.
	// Removing storage objects here models a pre-ledger shape, not a business
	// database copy and not evidence that a particular deployment matches it.
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(schema)).Error; err != nil {
		t.Fatal("initialize the empty disposable fixture", err)
	}
	if len(shape) > 0 && shape[0] == "canonical" {
		return tx
	}
	if err := tx.Exec(`DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='media' AND c.relkind='r' AND (c.relname LIKE 'storage_%' OR c.relname IN ('object_location','upload_admission')) LOOP EXECUTE format('DROP TABLE media.%I CASCADE',r.relname); END LOOP;
 FOR r IN SELECT * FROM (VALUES ('media.media_asset','object_location_id'),('media.rendition','object_location_id'),('media.package_object','object_location_id'),('media.project_copy_object','source_location_id'),('media.project_copy_object','target_location_id'),('media.transfer_object','source_location_id'),('media.transfer_object','target_location_id'),('media.purge_object','object_location_id'),('mediatool.depth_object','object_location_id'),('script.object_intent','object_location_id'),('script.object_state','object_location_id')) v(tbl,col) LOOP EXECUTE format('ALTER TABLE %s DROP COLUMN %I CASCADE',r.tbl,r.col); END LOOP;
 END $$;`).Error; err != nil {
		t.Fatal(err)
	}
	if len(shape) > 0 && shape[0] == "partial" {
		if err := tx.Exec(`ALTER TABLE media.media_asset ADD COLUMN object_location_id uuid; ALTER TABLE media.rendition ADD COLUMN object_location_id uuid;`).Error; err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "media_storage_prerequisites_incremental.sql"))
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\nBEGIN;\n", "\n"), "\nCOMMIT;\n", "\n")
		if err := tx.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

func TestStorageLedgerExpansionRequiresPhysicalLocationsAndDoesNotInventRoutes(t *testing.T) {
	for _, shape := range []string{"legacy", "partial", "canonical"} {
		t.Run(shape, func(t *testing.T) { testStorageLedgerExpansion(t, shape) })
	}
}
func testStorageLedgerExpansion(t *testing.T, shape string) {
	tx := storageLedgerMigrationFixture(t, shape)
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "storage_ledger_expand_incremental.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := tx.Exec(string(migration)).Error; err != nil {
			t.Fatal("expand or exact replay failed", err)
		}
	}
	var facts struct {
		TableCount, BoundColumns, BoundForeignKeys, Profiles, Routes, LegacyPrimaryKeys int
	}
	if err := tx.Raw(`SELECT
 (SELECT count(*) FROM pg_tables WHERE schemaname='media' AND tablename IN ('object_location','storage_bucket_evidence','storage_config_revision','storage_probe_evidence','storage_probe_intent','storage_profile','storage_route','storage_work_intent','upload_admission')) AS table_count,
 (SELECT count(*) FROM pg_attribute WHERE attrelid IN ('media.media_asset'::regclass,'media.rendition'::regclass,'media.package_object'::regclass,'media.project_copy_object'::regclass,'media.transfer_object'::regclass,'media.purge_object'::regclass,'mediatool.depth_object'::regclass,'script.object_intent'::regclass,'script.object_state'::regclass) AND attname IN ('object_location_id','source_location_id','target_location_id') AND atttypid='uuid'::regtype AND attnotnull AND NOT attisdropped) AS bound_columns,
 (SELECT count(*) FROM pg_constraint WHERE confrelid='media.object_location'::regclass AND conrelid IN ('media.media_asset'::regclass,'media.rendition'::regclass,'media.package_object'::regclass,'media.project_copy_object'::regclass,'media.transfer_object'::regclass,'media.purge_object'::regclass,'mediatool.depth_object'::regclass,'script.object_intent'::regclass,'script.object_state'::regclass) AND contype='f' AND convalidated) AS bound_foreign_keys,
 (SELECT count(*) FROM media.storage_profile) AS profiles,
 (SELECT count(*) FROM media.storage_route) AS routes,
 (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('script.object_intent'::regclass,'script.object_state'::regclass) AND contype='p') AS legacy_primary_keys`).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.TableCount != 9 || facts.BoundColumns != 11 || facts.BoundForeignKeys != 11 || facts.Profiles != 0 || facts.Routes != 0 || facts.LegacyPrimaryKeys != 2 {
		t.Fatalf("expanded schema changed legacy ownership or invented resource facts: %+v", facts)
	}
	profiles := []mediaapp.StorageProfile{{ID: "fixture-tos", Provider: "tos", Endpoint: "https://tos.example.invalid", SigningEndpoint: "https://tos.example.invalid", Region: "test-region", Bucket: "migration-fixture", AddressingStyle: ""}}
	routes := pgmedia.NewStorageRoutes(tx)
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	// A real schema with a foreign location ID is rejected; nullable old rows
	// remain permitted until byte-proved backfill, never silently guessed.
	if err := tx.SavePoint("bad_location").Error; err != nil {
		t.Fatal(err)
	}
	err = tx.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,'fixture-tos','wrong-bucket','fixture.mp3','reserved')`, uuid.New()).Error
	if err == nil {
		t.Fatal("unproved location was accepted")
	}
	if err := tx.RollbackTo("bad_location").Error; err != nil {
		t.Fatal(err)
	}
	job, scope := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'subject-voice',?,?,'fixture-tos','migration-fixture','1')`, uuid.New(), scope, job).Error; err != nil {
		t.Fatal("voice work dependency missing", err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ReadStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: "subject-voice", OwnerID: scope, WorkID: job}); err != nil {
		t.Fatal("runtime grants missing", err)
	}
}
