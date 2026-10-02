package media_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// The fixture database must be initialized from db/schema.sql before this test.
func TestMediaSchemaRelationships(t *testing.T) {
	dsn := os.Getenv("LV_TEST_MEDIA_SCHEMA_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_MEDIA_SCHEMA_DB_DSN to a disposable PostgreSQL database initialized from db/schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open media schema test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	database := conn.DB.WithContext(ctx)
	assertMediaSchema(t, database)
	assertMediaRelationships(t, database)
}

func relationExists(t *testing.T, database *gorm.DB, relation string) bool {
	t.Helper()
	var exists bool
	if err := database.Raw("SELECT to_regclass(?) IS NOT NULL", relation).Scan(&exists).Error; err != nil {
		t.Fatalf("inspect relation %s: %v", relation, err)
	}
	return exists
}

func assertMediaSchema(t *testing.T, database *gorm.DB) {
	t.Helper()
	for _, name := range []string{"media.media_asset", "media.rendition"} {
		if !relationExists(t, database, name) {
			t.Fatalf("%s missing from initialized schema", name)
		}
	}
	for name, want := range map[string][]string{
		"ix_media_project_kind": {"project_id", "kind", "create_time DESC", "WHERE (NOT is_delete)"},
		"ix_media_name_trgm":    {"USING gin", "file_name gin_trgm_ops"},
		"ix_media_sha256":       {"project_id", "sha256", "WHERE (NOT is_delete)"},
	} {
		var definition string
		if err := database.Raw(`
			SELECT indexdef FROM pg_indexes WHERE schemaname = 'media' AND indexname = ?
		`, name).Scan(&definition).Error; err != nil {
			t.Fatalf("inspect index %s: %v", name, err)
		}
		for _, fragment := range want {
			if !strings.Contains(definition, fragment) {
				t.Errorf("index %s definition %q lacks %q", name, definition, fragment)
			}
		}
	}
}

func assertMediaRelationships(t *testing.T, database *gorm.DB) {
	t.Helper()
	projectA := insertMediaProject(t, database)
	projectB := insertMediaProject(t, database)
	operationA := insertMediaOperation(t, database, projectA)
	operationB := insertMediaOperation(t, database, projectB)
	assetA := uuid.NewString()
	assetB := uuid.NewString()
	insertAsset := func(id, projectID, operationID, kind, status, objectKey string, byteSize int64) error {
		return database.Exec(`
			INSERT INTO media.media_asset
			  (id, project_id, kind, origin, status, object_key, file_name, mime_type,
			   byte_size, sha256, source_operation_id)
			VALUES (?::uuid, ?::uuid, ?, 'generated', ?, ?, 'reference.mp4', 'video/mp4',
			        ?, 'same-hash', ?::uuid)
		`, id, projectID, kind, status, objectKey, byteSize, operationID).Error
	}
	if err := insertAsset(assetA, projectA, operationA, "video", "ready", "projects/"+projectA+"/video/a.mp4", 1024); err != nil {
		t.Fatalf("insert project A media asset: %v", err)
	}
	if err := insertAsset(assetB, projectB, operationB, "video", "ready", "projects/"+projectB+"/video/b.mp4", 1024); err != nil {
		t.Fatalf("insert project B media asset: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO media.media_asset
		  (id, project_id, kind, origin, status, object_key, mime_type, byte_size)
		VALUES (?::uuid, ?::uuid, 'image', 'upload', 'uploading', ?, 'image/png', 0)
	`, uuid.NewString(), projectA, "projects/"+projectA+"/image/upload.png").Error; err != nil {
		t.Fatalf("upload without source operation was rejected: %v", err)
	}
	assertMediaSQLState(t, insertAsset(uuid.NewString(), uuid.NewString(), operationA, "video", "ready", "missing-project", 1), "23503")
	assertMediaSQLState(t, insertAsset(uuid.NewString(), projectA, operationB, "video", "ready", "wrong-operation", 1), "23503")
	assertMediaSQLState(t, insertAsset(uuid.NewString(), projectA, operationA, "unknown", "ready", "bad-kind", 1), "23514")
	assertMediaSQLState(t, insertAsset(uuid.NewString(), projectA, operationA, "video", "unknown", "bad-status", 1), "23514")
	assertMediaSQLState(t, insertAsset(uuid.NewString(), projectA, operationA, "video", "ready", "negative-size", -1), "23514")
	assertMediaSQLState(t, insertAsset(uuid.NewString(), projectA, operationA, "video", "ready", "projects/"+projectA+"/video/a.mp4", 1), "23505")
	if err := insertAsset(uuid.NewString(), projectA, operationA, "video", "ready", "same-project-same-hash", 1); err != nil {
		t.Fatalf("same-project duplicate content must remain insertable for reuse choice: %v", err)
	}
	assertMediaSQLState(t, database.Exec(`UPDATE media.media_asset SET origin = 'unknown' WHERE id = ?::uuid`, assetA).Error, "23514")
	assertMediaSQLState(t, database.Exec(`UPDATE media.media_asset SET region = 'unknown' WHERE id = ?::uuid`, assetA).Error, "23514")
	assertMediaSQLState(t, database.Exec(`UPDATE media.media_asset SET moderation_status = 'unknown' WHERE id = ?::uuid`, assetA).Error, "23514")
	assertMediaSQLState(t, database.Exec(`UPDATE media.media_asset SET revision = 0 WHERE id = ?::uuid`, assetA).Error, "23514")

	renderA := uuid.NewString()
	insertRendition := func(id, parentID, kind string) error {
		return database.Exec(`
			INSERT INTO media.rendition (id, media_asset_id, kind, object_key)
			VALUES (?::uuid, ?::uuid, ?, ?)
		`, id, parentID, kind, "rendition/"+id).Error
	}
	if err := insertRendition(renderA, assetA, "poster"); err != nil {
		t.Fatalf("insert parent-linked rendition: %v", err)
	}
	assertMediaSQLState(t, insertRendition(uuid.NewString(), uuid.NewString(), "poster"), "23503")
	assertMediaSQLState(t, insertRendition(uuid.NewString(), assetA, "poster"), "23505")
	assertMediaSQLState(t, insertRendition(uuid.NewString(), assetA, "unknown"), "23514")
	if err := insertRendition(uuid.NewString(), assetB, "poster"); err != nil {
		t.Fatalf("insert project B rendition: %v", err)
	}
	var count int64
	if err := database.Raw(`
		SELECT count(*) FROM media.rendition AS r
		JOIN media.media_asset AS a ON a.id = r.media_asset_id
		WHERE a.project_id = ?::uuid
	`, projectA).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("project A scoped renditions = %d, error = %v, want 1", count, err)
	}
}

func insertMediaProject(t *testing.T, database *gorm.DB) string {
	t.Helper()
	orgID, projectID := uuid.NewString(), uuid.NewString()
	if err := database.Exec(`
		INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)
	`, orgID, "media-"+orgID).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type)
		VALUES (?::uuid, ?::uuid, '媒体测试', '16:9', 'realistic')
	`, projectID, orgID).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return projectID
}

func insertMediaOperation(t *testing.T, database *gorm.DB, projectID string) string {
	t.Helper()
	id := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, target_type, capability, mode, input_hash, origin, status)
		VALUES (?::uuid, ?::uuid, 'free', 'video.upload', 'upload', ?, 'upload', 'draft')
	`, id, projectID, "media-"+id).Error; err != nil {
		t.Fatalf("insert operation: %v", err)
	}
	return id
}

func assertMediaSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("SQLSTATE = %v, want %s", err, want)
	}
}
