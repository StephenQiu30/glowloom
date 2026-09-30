package media_test

import (
	"context"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestUploadMigrationUpDownUpAndReceiptRollbackGuard(t *testing.T) {
	dsn := os.Getenv("LV_TEST_MEDIA_UPLOAD_MIGRATION_DB_DSN")
	if dsn == "" {
		t.Skip("set disposable empty LV_TEST_MEDIA_UPLOAD_MIGRATION_DB_DSN migrated through 202610010010")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("isolated migration database unavailable")
	}
	defer func() { _ = conn.Close() }()
	database := conn.DB.WithContext(ctx)
	var businessRows int64
	if err := database.Raw(`SELECT (SELECT count(*) FROM workspace.project)+(SELECT count(*) FROM identity."user")+(SELECT count(*) FROM operation.operation)+(SELECT count(*) FROM catalog.provider)+(SELECT count(*) FROM media.media_asset)`).Scan(&businessRows).Error; err != nil || businessRows != 0 {
		t.Fatalf("migration test requires empty business tables; rows=%d err=%v", businessRows, err)
	}
	up, err := os.ReadFile("../../db/migrations/202610010020_create_media_upload_request.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../db/migrations/202610010020_create_media_upload_request.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	var present bool
	if err := database.Raw(`SELECT to_regclass('media.upload_request') IS NOT NULL`).Scan(&present).Error; err != nil || present {
		t.Fatal("migration fixture must precede upload migration")
	}
	if err := database.Exec(string(up)).Error; err != nil {
		t.Fatalf("upload up: %v", err)
	}
	if err := database.Exec(string(down)).Error; err != nil {
		t.Fatalf("empty upload down: %v", err)
	}
	if err := database.Exec(string(up)).Error; err != nil {
		t.Fatalf("upload second up: %v", err)
	}
	actor, project := mediaStoreProject(t, database)
	service := mediaapp.NewUploadService(pgmedia.NewStore(database), mediaflow.FFUploadProber{}, mediaflow.FFUploadRenderer{}, &uploadObjectsFake{}, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	result, err := service.Upload(ctx, in)
	if err != nil {
		t.Fatalf("seed durable receipt: %v", err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	session, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.ExecContext(ctx, string(down)); err == nil {
		t.Fatal("down erased a durable receipt")
	}
	if _, err := session.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.upload_request WHERE asset_id=?`, result.Asset.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("guard damaged receipt: count=%d err=%v", count, err)
	}
}
