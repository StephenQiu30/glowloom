package media_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func seedIngestOperation(t *testing.T, database *gorm.DB, projectID uuid.UUID, outputType string) uuid.UUID {
	t.Helper()
	suffix := uuid.NewString()
	capability, providerKey, modelKey := outputType+".generate."+suffix, "mock-"+suffix, "mock-"+outputType+"-"+suffix
	mode := "text_to_image"
	if outputType == "video" {
		mode = "text_to_video"
	}
	providerID, profileID, versionID, operationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, ?, ?, ARRAY[?::text], ARRAY['prompt'])
	`, uuid.NewString(), capability, outputType, mode).Error; err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.provider (id, key, name, adapter_key, region)
		VALUES (?::uuid, ?, 'Mock', 'mock', 'overseas')
	`, providerID.String(), providerKey).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile
		  (id, model_key, provider_id, capability, display_name, status)
		VALUES (?::uuid, ?, ?::uuid, ?, 'Mock Image', 'active')
	`, profileID.String(), modelKey, providerID.String(), capability).Error; err != nil {
		t.Fatalf("seed model profile: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits,
		   param_schema, supports_query, supports_cancel, supports_callback,
		   expected_max_ms, queue)
		VALUES (?::uuid, ?::uuid, 1, 'mock-result-v1', ARRAY[?::text],
		        '{}'::jsonb, '[]'::jsonb, true, true, false, 60000, 'agent.mock')
	`, versionID.String(), profileID.String(), mode).Error; err != nil {
		t.Fatalf("seed model version: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, target_type, capability, mode, model_profile_version_id,
		   input_hash, origin, status, region)
		VALUES (?::uuid, ?::uuid, 'free', ?, ?, ?::uuid,
		        ?, 'canvas', 'ingesting', 'overseas')
	`, operationID.String(), projectID.String(), capability, mode, versionID.String(), suffix).Error; err != nil {
		t.Fatalf("seed ingest operation: %v", err)
	}
	return operationID
}

func TestMediaIngestPersistsOneCandidateAndModeratesExactlyOnce(t *testing.T) {
	if os.Getenv("LV_TEST_MEDIA_STORE_DB_DSN") == "" || os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set LV_TEST_MEDIA_STORE_DB_DSN, LV_TEST_OBJECT_STORAGE=1 and LV_ENV_FILE for PostgreSQL and host MinIO")
	}
	database := mediaStoreDB(t)
	_, projectID := mediaStoreProject(t, database)
	opID := seedIngestOperation(t, database, projectID, "image")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load MinIO configuration: %v", err)
	}
	objects, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
		cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatalf("open MinIO client: %v", err)
	}
	var imageBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes.Bytes())
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	activities, err := mediaflow.NewActivities(database, objects, mediaflow.DownloadPolicy{
		AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
	})
	if err != nil {
		t.Fatalf("create media activities: %v", err)
	}
	input := mediaflow.IngestInput{OperationID: opID.String(), SeqNo: 1, URL: server.URL + "/result.png"}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	first, err := activities.Ingest(ctx, input)
	if err != nil {
		t.Fatalf("ingest generated media: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		for _, key := range []string{
			first.ObjectKey,
			strings.TrimSuffix(first.ObjectKey, path.Ext(first.ObjectKey)) + "/thumb_256.png",
			strings.TrimSuffix(first.ObjectKey, path.Ext(first.ObjectKey)) + "/thumb_640.png",
		} {
			if err := objects.Remove(cleanCtx, key); err != nil {
				t.Errorf("remove exact generated test object: %v", err)
			}
		}
	})
	if first.Kind != "image" || first.MIMEType != "image/png" || first.ByteSize != int64(imageBytes.Len()) || first.OutputID == "" || first.MediaAssetID == "" {
		t.Fatalf("ingest output = %+v", first)
	}
	var renditions []struct {
		Kind      string
		ObjectKey string
		Width     int32
		Height    int32
		ByteSize  int64
	}
	if err := database.Raw(`
		SELECT kind, object_key, width, height, byte_size
		FROM media.rendition WHERE media_asset_id = ?::uuid AND NOT is_delete ORDER BY kind
	`, first.MediaAssetID).Scan(&renditions).Error; err != nil {
		t.Fatalf("read image renditions: %v", err)
	}
	if len(renditions) != 2 || renditions[0].Kind != "thumb_256" || renditions[1].Kind != "thumb_640" {
		t.Fatalf("image renditions = %+v", renditions)
	}
	for _, rendition := range renditions {
		info, err := objects.Stat(ctx, rendition.ObjectKey)
		if err != nil || info.Size != rendition.ByteSize || info.ContentType != "image/png" ||
			info.SHA256 == "" || rendition.Width < 1 || rendition.Height < 1 {
			t.Fatalf("image rendition %s: row=%+v object=%+v err=%v", rendition.Kind, rendition, info, err)
		}
	}
	server.Close() // replay must return durable rows without fetching the expired URL.
	replayed, err := activities.Ingest(ctx, input)
	if err != nil || replayed != first {
		t.Fatalf("ingest replay = %+v, %v; want %+v", replayed, err, first)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id = ?::uuid`, opID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("operation output count = %d, %v", count, err)
	}
	if err := database.Raw(`SELECT count(*) FROM media.media_asset WHERE source_operation_id = ?::uuid`, opID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("media asset count = %d, %v", count, err)
	}
	decision := mediaflow.RecordModerationInput{OperationID: opID.String(), OutputID: first.OutputID, Status: "passed"}
	if _, err := activities.RecordModeration(ctx, decision); err != nil {
		t.Fatalf("record moderation: %v", err)
	}
	if _, err := activities.RecordModeration(ctx, decision); err != nil {
		t.Fatalf("replay moderation: %v", err)
	}
	var asset struct {
		Status           string
		ModerationStatus string
		Revision         int32
	}
	if err := database.Raw(`SELECT status, moderation_status, revision FROM media.media_asset WHERE id = ?::uuid`, first.MediaAssetID).Scan(&asset).Error; err != nil ||
		asset.Status != "ready" || asset.ModerationStatus != "passed" || asset.Revision != 2 {
		t.Fatalf("moderated media = %+v, %v", asset, err)
	}
	decision.Status = "rejected"
	if _, err := activities.RecordModeration(ctx, decision); !errors.Is(err, pgmedia.ErrOutputConflict) {
		t.Fatalf("conflicting moderation error = %v", err)
	}
}

func TestVideoIngestCreatesProjectAspectProxyAndPoster(t *testing.T) {
	if os.Getenv("LV_TEST_MEDIA_STORE_DB_DSN") == "" || os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set LV_TEST_MEDIA_STORE_DB_DSN, LV_TEST_OBJECT_STORAGE=1 and LV_ENV_FILE for PostgreSQL and host MinIO")
	}
	database := mediaStoreDB(t)
	_, projectID := mediaStoreProject(t, database) // Project fixture uses 16:9.
	opID := seedIngestOperation(t, database, projectID, "video")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	objects, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
		cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatal(err)
	}
	fixture := path.Join(t.TempDir(), "source.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-y", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25", "-t", "1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create MP4 fixture: %v: %s", err, output)
	}
	content, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	activities, err := mediaflow.NewActivities(database, objects, mediaflow.DownloadPolicy{
		AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	input := mediaflow.IngestInput{OperationID: opID.String(), SeqNo: 1, URL: server.URL + "/result.mp4"}
	first, err := activities.Ingest(ctx, input)
	if err != nil {
		t.Fatalf("ingest video: %v", err)
	}
	prefix := strings.TrimSuffix(first.ObjectKey, path.Ext(first.ObjectKey))
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		for _, key := range []string{first.ObjectKey, prefix + "/poster.png", prefix + "/proxy_720p.mp4"} {
			if err := objects.Remove(cleanCtx, key); err != nil {
				t.Errorf("remove exact video test object: %v", err)
			}
		}
	})
	if first.Kind != "video" || first.MIMEType != "video/mp4" || first.ByteSize != int64(len(content)) {
		t.Fatalf("video ingest output = %+v", first)
	}
	var renditions []struct {
		Kind      string
		ObjectKey string
		Width     int32
		Height    int32
		ByteSize  int64
	}
	if err := database.Raw(`
		SELECT kind, object_key, width, height, byte_size
		FROM media.rendition WHERE media_asset_id = ?::uuid AND NOT is_delete ORDER BY kind
	`, first.MediaAssetID).Scan(&renditions).Error; err != nil {
		t.Fatal(err)
	}
	if len(renditions) != 2 || renditions[0].Kind != "poster" || renditions[1].Kind != "proxy_720p" ||
		renditions[1].Width != 1280 || renditions[1].Height != 720 {
		t.Fatalf("video renditions = %+v", renditions)
	}
	for _, rendition := range renditions {
		info, err := objects.Stat(ctx, rendition.ObjectKey)
		wantType := "image/png"
		if rendition.Kind == "proxy_720p" {
			wantType = "video/mp4"
		}
		if err != nil || info.Size != rendition.ByteSize || info.ContentType != wantType || info.SHA256 == "" {
			t.Fatalf("video rendition %s: row=%+v object=%+v err=%v", rendition.Kind, rendition, info, err)
		}
	}
	server.Close()
	replayed, err := activities.Ingest(ctx, input)
	if err != nil || replayed != first {
		t.Fatalf("video ingest replay = %+v, %v; want %+v", replayed, err, first)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.rendition WHERE media_asset_id = ?::uuid`, first.MediaAssetID).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("video rendition count = %d, %v", count, err)
	}
}
