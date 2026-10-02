package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	identitypg "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

type mediaCompositionFixture struct {
	router  *gin.Engine
	runtime *gorm.DB
	owner   *gorm.DB
	objects *objectstorage.Client
	actor   identityapp.Principal
	project uuid.UUID
	asset   uuid.UUID
}

func mediaCompositionRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func newMediaCompositionFixture(t *testing.T) mediaCompositionFixture {
	t.Helper()
	dsn, ownerDSN, storageFile := os.Getenv("LV_TEST_MEDIA_COMPOSITION_DB_DSN"), os.Getenv("LV_TEST_MEDIA_COMPOSITION_OWNER_DB_DSN"), os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if dsn == "" || ownerDSN == "" || storageFile == "" {
		t.Skip("set isolated media composition database and synthetic private storage configuration")
	}
	open := func(value string, runtime bool) *gorm.DB {
		connection, err := db.Open(t.Context(), value, noop.NewTracerProvider())
		if err != nil {
			t.Fatal("open isolated composition database")
		}
		t.Cleanup(func() { _ = connection.Close() })
		var scope struct{ Database, Role string }
		if err := connection.DB.Raw(`SELECT current_database() AS database,current_user AS role`).Scan(&scope).Error; err != nil || !strings.HasPrefix(scope.Database, "lanverse_composition_") || runtime && scope.Role != "lanverse_app" {
			t.Fatal("composition requires isolated database and the actual non-owner runtime role")
		}
		return connection.DB
	}
	fixture := mediaCompositionFixture{runtime: open(dsn, true), owner: open(ownerDSN, false)}
	raw, err := os.ReadFile(storageFile)
	if err != nil {
		t.Fatal("read synthetic private storage configuration")
	}
	var storage struct {
		Endpoint, Bucket string
		AccessKey        string `json:"access_key"`
		SecretKey        string `json:"secret_key"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&storage) != nil {
		t.Fatal("invalid synthetic private storage configuration")
	}
	endpoint, err := url.Parse(storage.Endpoint)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || storage.Bucket != "lanverse-receipt-test" {
		t.Fatal("composition requires the isolated loopback test bucket")
	}
	fixture.objects, err = objectstorage.Open(storage.Endpoint, storage.Bucket, storage.AccessKey, storage.SecretKey, "")
	if err != nil {
		t.Fatal("open synthetic private storage")
	}
	fixture.router, err = app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000"}, fixture.runtime, fixture.objects)
	if err != nil {
		t.Fatal(err)
	}
	user, err := identitypg.NewStore(fixture.runtime).EnsureWorkspace(t.Context())
	if err != nil {
		t.Fatal("resolve formal workspace identity", err)
	}
	fixture.actor = identityapp.Principal{ID: user.ID, OrgID: user.OrgID, Role: user.Role}
	created := mediaCompositionRequest(t, fixture.router, http.MethodPost, "/api/projects", map[string]string{"name": "素材组合验收 " + uuid.NewString()[:8], "aspect_ratio": "16:9", "style_type": "realistic"})
	var project workspaceapp.CreatedProject
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &project) != nil || project.ID == uuid.Nil {
		t.Fatal("formal project creation", created.Code)
	}
	fixture.project = project.ID
	var pngBody bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 3), B: 100, A: 255})
		}
	}
	if err := png.Encode(&pngBody, img); err != nil {
		t.Fatal(err)
	}
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(pngBody.Bytes()), "组合验证.png")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	store := mediapg.NewStore(fixture.runtime)
	uploads := mediaapp.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(fixture.objects), time.Now)
	uploaded, err := uploads.Upload(t.Context(), mediaapp.UploadInput{Actor: fixture.actor, Request: mediaapp.UploadRequest{ProjectID: fixture.project, Key: uuid.New(), FileName: "组合验证.png", RequestID: uuid.New()}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual reviewed private upload", err)
	}
	fixture.asset = uploaded.Asset.ID
	var keys []string
	if err := fixture.owner.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION SELECT object_key FROM media.rendition WHERE media_asset_id=?`, fixture.asset, fixture.asset).Scan(&keys).Error; err != nil || len(keys) != 3 {
		t.Fatal("complete original and actual renditions", err)
	}
	for _, key := range keys {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := fixture.objects.Remove(ctx, key); err != nil {
				t.Error("remove only this exact synthetic fixture object")
			}
		})
	}
	return fixture
}

func (f mediaCompositionFixture) library(t *testing.T, state string) mediaapp.LibraryPage {
	t.Helper()
	response := mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library?scope=project&project_id="+f.project.String()+"&catalog_state="+state, nil)
	var page mediaapp.LibraryPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatal("actual scoped library", response.Code)
	}
	return page
}

func TestMediaLibraryCompositionActualPhysicalUsageAndCorruptOwnerRefusesSubtotal(t *testing.T) {
	f := newMediaCompositionFixture(t)
	var expected int64
	var keys []string
	if err := f.owner.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION SELECT object_key FROM media.rendition WHERE media_asset_id=?`, f.asset, f.asset).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		info, err := f.objects.Stat(t.Context(), key)
		if err != nil {
			t.Fatal("read actual persisted size", err)
		}
		expected += info.Size
	}
	path := "/api/media/library/storage-usage?scope=project&project_id=" + f.project.String()
	response := mediaCompositionRequest(t, f.router, http.MethodGet, path, nil)
	var usage struct {
		UsedBytes   int64  `json:"used_bytes"`
		ObjectCount int    `json:"object_count"`
		LimitBytes  *int64 `json:"limit_bytes"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &usage) != nil || usage.UsedBytes != expected || usage.ObjectCount != 3 || usage.LimitBytes != nil {
		t.Fatal("formal composed usage omitted real objects or invented a limit", response.Code, usage)
	}
	// An incomplete owning execution row must invalidate even a complete media
	// inventory. This proves the production router installs the retained reader.
	if err := f.owner.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at) VALUES(?,?,?,?,'producer',?,?,1,'{"unknown_media":"not-an-empty-proof"}','cancelled','cancelled',0,1,1,now(),now())`, uuid.New(), f.project, f.actor.OrgID, f.actor.ID, uuid.New(), uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	response = mediaCompositionRequest(t, f.router, http.MethodGet, path, nil)
	if response.Code != 503 || strings.Contains(response.Body.String(), "used_bytes") {
		t.Fatal("incomplete installed owner returned a subtotal", response.Code)
	}
}

func TestMediaLibraryCompositionCurrentCoverAndUnreadableOwnerPreventPurge(t *testing.T) {
	f := newMediaCompositionFixture(t)
	if err := f.owner.Exec(`UPDATE workspace.project SET cover_asset_id=? WHERE id=?`, f.asset, f.project).Error; err != nil {
		t.Fatal(err)
	}
	page := f.library(t, "active")
	response := mediaCompositionRequest(t, f.router, http.MethodPost, "/api/media/library/commands", mediaapp.LibraryCommand{Scope: page.Scope, ExpectedRevision: page.Revision, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: f.asset, Revision: page.Items[0].Revision}}})
	if response.Code != 200 {
		t.Fatal("formal recycle command", response.Code)
	}
	page = f.library(t, "trashed")
	var revision int64
	if err := f.owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, f.project).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"scope": page.Scope, "expected_revision": page.Revision, "expected_project_revision": revision, "permanent_delete_confirmed": true, "items": []mediaapp.LibraryItemRevision{{ID: f.asset, Revision: page.Items[0].Revision}}}
	response = mediaCompositionRequest(t, f.router, http.MethodPost, "/api/media/library/purges", input)
	var job struct {
		Items []struct {
			Status      string  `json:"status"`
			FailureCode *string `json:"failure_code"`
		} `json:"items"`
	}
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &job) != nil || len(job.Items) != 1 || job.Items[0].Status != "blocked" || job.Items[0].FailureCode == nil || *job.Items[0].FailureCode != "in_use" {
		t.Fatal("installed workspace cover guard failed to block", response.Code, job)
	}
	var before int64
	if err := f.owner.Raw(`SELECT count(*) FROM media.purge_job WHERE project_id=?`, f.project).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.owner.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at) VALUES(?,?,?,?,'producer',?,?,1,'{"unknown_media":"not-an-empty-proof"}','cancelled','cancelled',0,1,1,now(),now())`, uuid.New(), f.project, f.actor.OrgID, f.actor.ID, uuid.New(), uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	response = mediaCompositionRequest(t, f.router, http.MethodPost, "/api/media/library/purges", input)
	var after int64
	if err := f.owner.Raw(`SELECT count(*) FROM media.purge_job WHERE project_id=?`, f.project).Scan(&after).Error; err != nil || response.Code != 503 || after != before {
		t.Fatal("used cover short-circuited another unreadable owner or admission partly committed", response.Code, before, after, err)
	}
	var rows []struct{ ObjectKey, SHA256 string }
	if err := f.owner.Raw(`SELECT object_key,sha256 FROM media.media_asset WHERE id=?`, f.asset).Scan(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	original, err := f.objects.Get(t.Context(), rows[0].ObjectKey)
	if err != nil {
		t.Fatal("blocked purge removed actual original", err)
	}
	defer func() { _ = original.Close() }()
	var stored bytes.Buffer
	if _, err := stored.ReadFrom(original); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(stored.Bytes())
	if hex.EncodeToString(digest[:]) != rows[0].SHA256 {
		t.Fatal("blocked purge changed actual original bytes")
	}
}
