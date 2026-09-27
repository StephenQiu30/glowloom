package media_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestMediaIngestAndModerationUnderRuntimeRole(t *testing.T) {
	if os.Getenv("LV_TEST_MEDIA_STORE_DB_DSN") == "" || os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set isolated PostgreSQL, native MinIO and root .env")
	}
	database := mediaStoreDB(t)
	_, projectID := mediaStoreProject(t, database)
	operationID := seedIngestOperation(t, database, projectID, "image")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	objects, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
		cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(picture.Bytes())
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	var ingested mediaflow.IngestOutput
	err = database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return fmt.Errorf("assume runtime role: %w", err)
		}
		activities, err := mediaflow.NewActivities(tx, objects, mediaflow.DownloadPolicy{
			AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
		})
		if err != nil {
			return err
		}
		ingested, err = activities.Ingest(ctx, mediaflow.IngestInput{
			OperationID: operationID.String(), SeqNo: 1, URL: server.URL + "/result.png",
		})
		if err != nil {
			return fmt.Errorf("ingest as runtime role: %w", err)
		}
		if _, err := activities.RecordModeration(ctx, mediaflow.RecordModerationInput{
			OperationID: operationID.String(), OutputID: ingested.OutputID, Status: "passed",
		}); err != nil {
			return fmt.Errorf("moderate as runtime role: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(ingested.ObjectKey, path.Ext(ingested.ObjectKey))
	t.Cleanup(func() {
		cleanCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, key := range []string{ingested.ObjectKey, base + "/thumb_256.png", base + "/thumb_640.png"} {
			if err := objects.Remove(cleanCtx, key); err != nil {
				t.Errorf("remove exact runtime test object: %v", err)
			}
		}
	})
	var status struct {
		Asset  string
		Output string
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT a.moderation_status AS asset, o.moderation_status AS output
		FROM media.media_asset AS a
		JOIN operation.operation_output AS o ON o.media_asset_id = a.id
		WHERE o.operation_id = ?::uuid
	`, operationID.String()).Scan(&status).Error; err != nil || status.Asset != "passed" || status.Output != "passed" {
		t.Fatalf("runtime media status = %+v: %v", status, err)
	}
}
