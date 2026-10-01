package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func glbTestObjects(t *testing.T) *objectstorage.Client {
	t.Helper()
	file := os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if file == "" {
		t.Skip("set isolated LV_TEST_RECEIPT_STORAGE_CONFIG; no production configuration is read")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("read synthetic object storage configuration")
	}
	var cfg struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil {
		t.Fatal("invalid synthetic object storage configuration")
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || cfg.Bucket != "lanverse-receipt-test" {
		t.Fatal("storage must be an isolated loopback test bucket")
	}
	objects, err := objectstorage.Open(cfg.Endpoint, cfg.Bucket, cfg.AccessKey, cfg.SecretKey, "")
	if err != nil {
		t.Fatal("initialize synthetic private object storage")
	}
	return objects
}

func glbUploadRequest(t *testing.T, project, key uuid.UUID, data []byte, reviewed bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "场景.glb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if reviewed {
		if err := w.WriteField("local_review_confirmed", "true"); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+project.String()+"/media/uploads", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Idempotency-Key", key.String())
	return req
}

func TestGLBUploadRealHTTPPrivateStorageReplayAndAuthorization(t *testing.T) {
	database := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, database)
	_, otherProject := mediaStoreProject(t, database)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	store := pgmedia.NewStore(database)
	service := mediaapp.NewUploadService(store, mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	query := mediaapp.NewAssetQuery(store, objects)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewUploadHandler(service).Register(group)
	mediahttp.NewHandler(query).Register(group)
	data := glbBytes(t, glbDocument(), glbGeometry())
	key := uuid.New()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, glbUploadRequest(t, project, key, data, true))
	if response.Code != 201 {
		t.Fatalf("real GLB upload status=%d body=%s", response.Code, response.Body.String())
	}
	var result mediaapp.UploadResult
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Asset.Kind != "model" || result.Asset.MIMEType != "model/gltf-binary" || result.Asset.Width != nil || result.Asset.Height != nil || result.Asset.DurationMS != nil {
		t.Fatal("model metadata contract")
	}
	asset, err := store.FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil || !asset.CanReference() || asset.Kind != domain.KindModel || asset.Codec == nil || *asset.Codec != "glb2" {
		t.Fatalf("formal referenceable model: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(ctx, asset.ObjectKey); err != nil {
			t.Error("clean exact test model object")
		}
	})
	rends, err := store.FindRenditions(t.Context(), actor, project, asset.ID)
	if err != nil || len(rends) != 0 {
		t.Fatalf("model has fabricated renditions: %v count=%d", err, len(rends))
	}
	preview, err := query.Preview(t.Context(), actor, project, asset.ID)
	if err != nil || preview.Asset.Kind != "model" {
		t.Fatalf("model preview: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), "GET", preview.URL, nil)
	if err != nil {
		t.Fatal("invalid private preview")
	}
	served, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("fetch real private GLB")
	}
	defer func() { _ = served.Body.Close() }()
	got, err := io.ReadAll(io.LimitReader(served.Body, int64(len(data))+1))
	wantHash, gotHash := sha256.Sum256(data), sha256.Sum256(got)
	if err != nil || served.StatusCode != 200 || !bytes.Equal(data, got) || hex.EncodeToString(gotHash[:]) != hex.EncodeToString(wantHash[:]) {
		t.Fatal("private model retrieval differs from uploaded original")
	}
	if _, err := query.Reference(t.Context(), actor, otherProject, asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("foreign model reference: %v", err)
	}
	for i := 0; i < 2; i++ {
		replay := httptest.NewRecorder()
		router.ServeHTTP(replay, glbUploadRequest(t, project, key, data, true))
		var value mediaapp.UploadResult
		if replay.Code != 201 || json.Unmarshal(replay.Body.Bytes(), &value) != nil || !reflect.DeepEqual(value, result) {
			t.Fatal("durable identical model upload replay")
		}
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+project.String()+"/media?kind=model", nil))
	var page mediaapp.AssetPage
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != asset.ID {
		t.Fatalf("model list status=%d", list.Code)
	}
	invalid := glbDocument()
	invalid["buffers"].([]any)[0].(map[string]any)["uri"] = "https://fixture.invalid/private.bin"
	for _, tc := range []struct {
		body     []byte
		key      uuid.UUID
		reviewed bool
		status   int
	}{
		{data, uuid.New(), false, 422},
		{glbBytes(t, invalid, glbGeometry()), uuid.New(), true, 415},
		{glbBytes(t, invalid, glbGeometry()), key, true, 409},
	} {
		failed := httptest.NewRecorder()
		router.ServeHTTP(failed, glbUploadRequest(t, project, tc.key, tc.body, tc.reviewed))
		if failed.Code != tc.status {
			t.Fatalf("bad model status=%d expected=%d body=%s", failed.Code, tc.status, failed.Body.String())
		}
	}
	var counts struct{ Assets, Receipts, Audits int }
	if err := database.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE project_id=?) AS receipts,(SELECT count(*) FROM audit.audit_log WHERE project_id=? AND action='media.uploaded') AS audits`, project, project, project).Scan(&counts).Error; err != nil || counts.Assets != 1 || counts.Receipts != 1 || counts.Audits != 1 {
		t.Fatalf("model durable facts=%+v err=%v", counts, err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET width=1 WHERE id=?`, asset.ID).Error; err == nil {
		t.Fatal("SQL accepted fabricated model image facts")
	}
	if err := database.Exec(`UPDATE media.media_asset SET origin='generated' WHERE id=?`, asset.ID).Error; err == nil {
		t.Fatal("SQL accepted fake generated model provenance")
	}
	down, err := os.ReadFile("../../db/migrations/202610010050_media_model.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(down)).Error; err == nil {
		t.Fatal("migration discarded the model contract while model assets remain")
	}
	var constraints int
	if err := database.Raw(`SELECT count(*) FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname IN ('media_asset_kind_check','media_asset_model_facts_check')`).Scan(&constraints).Error; err != nil || constraints != 2 {
		t.Fatalf("failed downgrade changed the formal model contract: count=%d err=%v", constraints, err)
	}
	if err := database.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Preview(t.Context(), actor, project, asset.ID); err != nil {
		t.Fatalf("archived project retains read-only preview=%v", err)
	}
	archived := httptest.NewRecorder()
	router.ServeHTTP(archived, glbUploadRequest(t, project, uuid.New(), data, true))
	if archived.Code != 409 {
		t.Fatalf("archived upload status=%d", archived.Code)
	}
}
