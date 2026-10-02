package media_test

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestMediaUploadSchemaPersistsReceiptAndReplay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_MEDIA_SCHEMA_DB_DSN")
	if dsn == "" {
		t.Skip("set disposable LV_TEST_MEDIA_SCHEMA_DB_DSN initialized from db/schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("isolated schema database unavailable")
	}
	defer func() { _ = conn.Close() }()
	database := conn.DB.WithContext(ctx)
	if !relationExists(t, database, "media.upload_request") {
		t.Fatal("upload receipt relation missing from initialized schema")
	}
	actor, project := mediaStoreProject(t, database)
	service := mediaapp.NewUploadService(pgmedia.NewStore(database), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, &uploadObjectsFake{}, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	result, err := service.Upload(ctx, in)
	if err != nil {
		t.Fatalf("seed durable receipt: %v", err)
	}
	replayed, err := service.Upload(ctx, in)
	if err != nil || !reflect.DeepEqual(result, replayed) {
		t.Fatalf("permanent upload receipt changed on replay: result=%+v err=%v", replayed, err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.upload_request WHERE asset_id=?`, result.Asset.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("upload receipt was not durably persisted: count=%d err=%v", count, err)
	}
	assertMediaSQLState(t, database.Exec(`
		INSERT INTO media.upload_request
		  (project_id, principal_id, request_key, sha256, file_name, byte_size, asset_id, response)
		SELECT project_id, principal_id, request_key, sha256, file_name, byte_size, asset_id, response
		FROM media.upload_request WHERE asset_id=?
	`, result.Asset.ID).Error, "23505")
}
