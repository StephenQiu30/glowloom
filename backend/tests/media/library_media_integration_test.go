package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func libraryPersonalFixture(t *testing.T, db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal, name string, data []byte) mediaapp.PersonalUploadResult {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	store := pgmedia.NewStore(db)
	service := mediaapp.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	uploaded, err := service.UploadPersonal(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{Key: uuid.New(), RequestID: uuid.New(), FileName: name}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION ALL SELECT object_key FROM media.rendition WHERE media_asset_id=?`, uploaded.Asset.ID, uploaded.Asset.ID).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("remove exact personal fixture object")
			}
		}
	})
	return uploaded
}
func libraryMediaRouter(db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewLibraryMediaHandler(mediaapp.NewLibraryMediaQuery(pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now), objects, objects)).Register(group)
	return router
}
func TestLibraryMediaActualScopedSignedPreviewsAndAttachmentOnlyDocument(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	imageAsset := libraryPersonalFixture(t, db, objects, actor, "personal.png", uploadPNG(t))
	document := libraryPersonalFixture(t, db, objects, actor, "personal.txt", []byte("当前私人正文\n"))
	router := libraryMediaRouter(db, objects, actor)
	preview := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/items/"+imageAsset.Asset.ID.String()+"/preview?scope=personal", nil, uuid.Nil)
	var result mediaapp.LibraryMediaPreview
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &result) != nil || result.Asset.ID != imageAsset.Asset.ID || len(result.Renditions) != 2 || result.URL == "" || result.ExpiresAt.Before(time.Now()) {
		t.Fatal("actual personal preview", preview.Code, preview.Body.String())
	}
	for _, url := range append([]string{result.URL}, result.Renditions[0].URL, result.Renditions[1].URL) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("signed object actual GET", err)
		}
		decoded, err := png.Decode(response.Body)
		_ = response.Body.Close()
		if err != nil || decoded.Bounds().Dx() < 1 || decoded.Bounds().Dy() < 1 {
			t.Fatal("actual signed PNG is not decodable", err)
		}
	}
	if response := libraryHTTPRequest(t, libraryMediaRouter(db, objects, foreign), http.MethodGet, "/api/media/library/items/"+imageAsset.Asset.ID.String()+"/preview?scope=personal", nil, uuid.Nil); response.Code != 404 {
		t.Fatal("foreign personal signed lease", response.Code)
	}
	if response := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/items/"+imageAsset.Asset.ID.String()+"/preview?scope=project&project_id="+project.String(), nil, uuid.Nil); response.Code != 404 {
		t.Fatal("personal promoted to project read", response.Code)
	}
	if response := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/items/"+document.Asset.ID.String()+"/preview?scope=personal", nil, uuid.Nil); response.Code != 404 {
		t.Fatal("document rendered inline", response.Code)
	}
	download := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/items/"+document.Asset.ID.String()+"/download?scope=personal", nil, uuid.Nil)
	if download.Code != 200 || !bytes.Equal(download.Body.Bytes(), []byte("当前私人正文\n")) || download.Header().Get("Content-Disposition") != "attachment; filename=personal.txt" || download.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("document attachment body/filename", download.Code, download.Header())
	}
}
func TestLibraryMediaDownloadRejectsCorruptBytesAndStorageFactsNeverEnterMetadata(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, _ := mediaStoreProject(t, db)
	uploaded := libraryPersonalFixture(t, db, objects, actor, "integrity.png", uploadPNG(t))
	var key string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=?`, uploaded.Asset.ID).Scan(&key).Error; err != nil {
		t.Fatal(err)
	}
	query := mediaapp.NewLibraryMediaQuery(pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now), objects, objects)
	file, asset, err := query.Download(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, uploaded.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file.File)
	_ = file.Close()
	if err != nil || asset.ID != uploaded.Asset.ID || !bytes.Equal(data, uploadPNG(t)) {
		t.Fatal("actual private download", err)
	}
	if err := objects.Remove(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	var changed bytes.Buffer
	if err := png.Encode(&changed, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	corrupt, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(changed.Bytes()), "integrity.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = corrupt.Close() }()
	if err := objects.PutIfAbsent(t.Context(), key, corrupt.File, corrupt.Size, corrupt.MIMEType, corrupt.SHA256); err != nil {
		t.Fatal(err)
	}
	if response := libraryHTTPRequest(t, libraryMediaRouter(db, objects, actor), http.MethodGet, "/api/media/library/items/"+uploaded.Asset.ID.String()+"/download?scope=personal", nil, uuid.Nil); response.Code != 503 || bytes.Contains(response.Body.Bytes(), changed.Bytes()) {
		t.Fatal("bad original bytes escaped attachment gate", response.Code)
	}
	// Ordinary image downloads are attachment-only as well; no raw URL is kept.
	request := httptest.NewRequestWithContext(t.Context(), "GET", "/api/media/library/items/"+uploaded.Asset.ID.String()+"/preview?scope=personal&actor_id="+actor.ID.String(), nil)
	response := httptest.NewRecorder()
	libraryMediaRouter(db, objects, actor).ServeHTTP(response, request)
	if response.Code != 422 {
		t.Fatal("caller selected preview owner", response.Code)
	}
}
