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

func storageSchemaFixture(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_SCHEMA_DSN")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires isolated storage schema PostgreSQL")
		}
		t.Skip("set a newly created task-owned glowloom_subject_storage_ database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "glowloom_subject_storage_") {
		t.Fatal("storage schema fixture requires a task-owned loopback database")
	}
	conn, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(schema)).Error; err != nil {
		t.Fatal("initialize the empty disposable fixture", err)
	}
	return tx
}

func TestStorageLedgerSchemaRequiresPhysicalLocationsAndStartsWithoutRoutes(t *testing.T) {
	tx := storageSchemaFixture(t)
	var facts struct {
		TableCount, BoundColumns, BoundForeignKeys, Profiles, Routes, LegacyPrimaryKeys int
	}
	if err := tx.Raw(`SELECT
 (SELECT count(*) FROM pg_tables WHERE schemaname='media' AND tablename IN ('object_location','storage_bucket_evidence','storage_config_revision','storage_probe_evidence','storage_probe_intent','storage_profile','storage_route','storage_work_intent','upload_admission')) AS table_count,
 (SELECT count(*) FROM pg_attribute WHERE attrelid IN ('media.media_asset'::regclass,'media.rendition'::regclass,'media.package_object'::regclass,'media.project_copy_object'::regclass,'media.transfer_object'::regclass,'media.purge_object'::regclass,'mediatool.depth_object'::regclass,'script.object_intent'::regclass,'script.object_state'::regclass) AND attname IN ('object_location_id','source_location_id','target_location_id') AND atttypid='uuid'::regtype AND attnotnull AND NOT attisdropped) AS bound_columns,
 (SELECT count(*) FROM pg_constraint WHERE confrelid='media.object_location'::regclass AND conrelid IN ('media.media_asset'::regclass,'media.rendition'::regclass,'media.package_object'::regclass,'media.project_copy_object'::regclass,'media.transfer_object'::regclass,'media.purge_object'::regclass,'mediatool.depth_object'::regclass,'script.object_intent'::regclass,'script.object_state'::regclass) AND contype='f' AND convalidated
  AND cardinality(conkey)=2
  AND confkey=ARRAY[
   (SELECT attnum FROM pg_attribute WHERE attrelid=confrelid AND attname='id'),
   (SELECT attnum FROM pg_attribute WHERE attrelid=confrelid AND attname='object_key')
  ]::smallint[]) AS bound_foreign_keys,
 (SELECT count(*) FROM media.storage_profile) AS profiles,
 (SELECT count(*) FROM media.storage_route) AS routes,
 (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('script.object_intent'::regclass,'script.object_state'::regclass) AND contype='p') AS legacy_primary_keys`).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.TableCount != 9 || facts.BoundColumns != 11 || facts.BoundForeignKeys != 11 || facts.Profiles != 0 || facts.Routes != 0 || facts.LegacyPrimaryKeys != 2 {
		t.Fatalf("unexpected storage schema or initial storage facts: %+v", facts)
	}
	profiles := []mediaapp.StorageProfile{{ID: "fixture-cos", Provider: "cos", Endpoint: "http://127.0.0.1:9171", SigningEndpoint: "http://127.0.0.1:9171", Region: "test-region", Bucket: "storage-schema-fixture", AddressingStyle: ""}}
	routes := pgmedia.NewStorageRoutes(tx)
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	// A physical location must use the bucket registered for its profile.
	if err := tx.SavePoint("bad_location").Error; err != nil {
		t.Fatal(err)
	}
	err := tx.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,'fixture-cos','wrong-bucket','fixture.mp3','reserved')`, uuid.New()).Error
	if err == nil {
		t.Fatal("unproved location was accepted")
	}
	if err := tx.RollbackTo("bad_location").Error; err != nil {
		t.Fatal(err)
	}
	job, scope := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'subject-voice',?,?,'fixture-cos','storage-schema-fixture','1')`, uuid.New(), scope, job).Error; err != nil {
		t.Fatal("voice work dependency missing", err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ReadStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: "subject-voice", OwnerID: scope, WorkID: job}); err != nil {
		t.Fatal("runtime grants missing", err)
	}
}
