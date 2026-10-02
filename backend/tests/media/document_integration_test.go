package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func documentTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	runtime, owner := os.Getenv("LV_TEST_DOCUMENT_DB_DSN"), os.Getenv("LV_TEST_DOCUMENT_OWNER_DSN")
	if runtime == "" || owner == "" {
		t.Skip("set isolated document runtime/owner PostgreSQL DSNs")
	}
	open := func(dsn string) *gorm.DB {
		c, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB.WithContext(t.Context())
	}
	r, o := open(runtime), open(owner)
	var current string
	if err := r.Raw(`SELECT current_user`).Scan(&current).Error; err != nil || current != "lanverse_app" {
		t.Fatal("document test needs nonowner runtime role", current, err)
	}
	return r, o
}

func documentUploadService(db *gorm.DB, objects *objectstorage.Client) *mediaapp.UploadService {
	return mediaapp.NewUploadService(pgmedia.NewStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
}

func documentUpload(t *testing.T, db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal, project uuid.UUID, name string, data []byte) mediaapp.UploadResult {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	result, err := documentUploadService(db, objects).Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: name}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("document actual reviewed upload", err)
	}
	asset, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(ctx, asset.ObjectKey); err != nil {
			t.Error("remove exact synthetic document object")
		}
	})
	return result
}

func documentRouter(db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewUploadHandler(documentUploadService(db, objects)).Register(group)
	mediahttp.NewHandler(mediaapp.NewAssetQuery(pgmedia.NewStore(db), objects)).Register(group)
	mediahttp.NewDocumentHandler(mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(db), objects)).Register(group)
	return router
}

func documentUploadRequest(t *testing.T, project, key uuid.UUID, name string, data []byte, reviewed bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
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
	r := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+project.String()+"/media/uploads", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.Header.Set("Origin", "http://127.0.0.1:3000")
	r.Header.Set("Idempotency-Key", key.String())
	return r
}

func TestDocumentHTTPNonownerActualPrivateOriginalReviewReplayAndAttachment(t *testing.T) {
	db, owner := documentTestDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, owner)
	_, foreign := mediaStoreProject(t, owner)
	router := documentRouter(db, objects, actor)
	for _, sample := range []struct {
		name string
		data []byte
	}{{"带 引号\".txt", []byte("原文\r\n不转换。")}, {"剧本.docx", documentDOCX(t, nil)}} {
		t.Run(sample.name, func(t *testing.T) {
			key := uuid.New()
			missing := httptest.NewRecorder()
			router.ServeHTTP(missing, documentUploadRequest(t, project, key, sample.name, sample.data, false))
			if missing.Code != 422 {
				t.Fatal("document rights declaration missing accepted", missing.Code)
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, documentUploadRequest(t, project, key, sample.name, sample.data, true))
			if out.Code != 201 {
				t.Fatal("actual HTTP document upload", out.Code, out.Body.String())
			}
			var result mediaapp.UploadResult
			if json.Unmarshal(out.Body.Bytes(), &result) != nil || result.Asset.Kind != "document" || result.Asset.Width != nil || result.Asset.DurationMS != nil {
				t.Fatal("safe formal document projection")
			}
			asset, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
			if err != nil || asset.Codec == nil || asset.SHA256 == nil {
				t.Fatal("durable document facts", err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = objects.Remove(ctx, asset.ObjectKey)
			})
			rends, err := pgmedia.NewStore(db).FindRenditions(t.Context(), actor, project, asset.ID)
			if err != nil || len(rends) != 0 {
				t.Fatal("document fake rendition", err)
			}
			replay := httptest.NewRecorder()
			router.ServeHTTP(replay, documentUploadRequest(t, project, key, sample.name, sample.data, true))
			if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), out.Body.Bytes()) {
				t.Fatal("document permanent receipt replay changed", replay.Code)
			}
			changed := httptest.NewRecorder()
			router.ServeHTTP(changed, documentUploadRequest(t, project, key, sample.name, append(bytes.Clone(sample.data), ' '), true))
			if changed.Code != 409 {
				t.Fatal("changed input under permanent key", changed.Code)
			}
			url := "/api/projects/" + project.String() + "/media/" + asset.ID.String()
			download := httptest.NewRecorder()
			router.ServeHTTP(download, httptest.NewRequestWithContext(t.Context(), "GET", url+"/download", nil))
			disposition, params, err := mime.ParseMediaType(download.Header().Get("Content-Disposition"))
			if err != nil || disposition != "attachment" || params["filename"] != sample.name || download.Code != 200 || !bytes.Equal(download.Body.Bytes(), sample.data) || download.Header().Get("X-Content-Type-Options") != "nosniff" || download.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("document attachment facts", download.Code, disposition, err)
			}
			preview := httptest.NewRecorder()
			router.ServeHTTP(preview, httptest.NewRequestWithContext(t.Context(), "GET", url+"/preview", nil))
			if preview.Code != 404 {
				t.Fatal("document inline preview", preview.Code)
			}
			wrong := httptest.NewRecorder()
			router.ServeHTTP(wrong, httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+foreign.String()+"/media/"+asset.ID.String()+"/download", nil))
			if wrong.Code != 404 {
				t.Fatal("cross-project attachment", wrong.Code)
			}
			query := mediaapp.NewAssetQuery(pgmedia.NewStore(db), objects)
			if _, err := query.Reference(t.Context(), actor, project, asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("document canvas reference", err)
			}
		})
	}
	query := mediaapp.NewAssetQuery(pgmedia.NewStore(db), objects)
	page, err := query.List(t.Context(), actor, project, "document", "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("document page1", err)
	}
	next, err := query.List(t.Context(), actor, project, "document", *page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal("document page2", err)
	}
	page, err = query.List(t.Context(), actor, project, "", "", 50)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("default canvas library leaked document", err)
	}
}

func TestDocumentSourcesPGReauthorizationFrozenCASAndActualObjectHash(t *testing.T) {
	db, owner := documentTestDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, owner)
	data := []byte("可靠原文")
	result := documentUpload(t, db, objects, actor, project, "剧本.txt", data)
	reader := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(db), objects)
	frozen, err := reader.Freeze(t.Context(), actor, project, []uuid.UUID{result.Asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	file, err := reader.Open(t.Context(), actor, project, frozen[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := owner.Exec(`UPDATE media.media_asset SET revision=revision+1 WHERE id=?`, result.Asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Open(t.Context(), actor, project, frozen[0]); !errors.Is(err, mediaapp.ErrDocumentSourceConflict) {
		t.Fatal("stale original source", err)
	}
	frozen, err = reader.Freeze(t.Context(), actor, project, []uuid.UUID{result.Asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.Put(t.Context(), asset.ObjectKey, bytes.NewReader([]byte("恶意变更")), 12, asset.MimeType); err != nil {
		t.Fatal("mutate only scoped synthetic bytes", err)
	}
	if _, err := reader.Open(t.Context(), actor, project, frozen[0]); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("object SHA mismatch", err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Freeze(t.Context(), actor, project, []uuid.UUID{result.Asset.ID}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("stale actor eligibility", err)
	}
}

func TestDocumentHTTPLifecycleAndContainerRejectsBeforePublishing(t *testing.T) {
	db, owner := documentTestDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, owner)
	result := documentUpload(t, db, objects, actor, project, "当前.txt", []byte("当前项目原文"))
	router := documentRouter(db, objects, actor)
	url := "/api/projects/" + project.String() + "/media/" + result.Asset.ID.String() + "/download"
	for _, state := range []struct {
		status string
		want   int
	}{{"archived", 200}, {"copying", 404}, {"active", 200}} {
		if err := owner.Exec(`UPDATE workspace.project SET status=? WHERE id=?`, state.status, project).Error; err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", url, nil))
		if response.Code != state.want {
			t.Fatal("document read project lifecycle", state.status, response.Code)
		}
		if state.status == "archived" {
			upload := httptest.NewRecorder()
			router.ServeHTTP(upload, documentUploadRequest(t, project, uuid.New(), "new.txt", []byte("新增正文"), true))
			if upload.Code != 409 {
				t.Fatal("archived document upload", upload.Code)
			}
		}
	}
	malformed := httptest.NewRecorder()
	router.ServeHTTP(malformed, documentUploadRequest(t, project, uuid.New(), "malformed.docx", documentDOCX(t, map[string]string{"word/document.xml": "<invalid>"}), true))
	if malformed.Code != 415 {
		t.Fatal("malformed container HTTP boundary", malformed.Code)
	}
	page, err := mediaapp.NewAssetQuery(pgmedia.NewStore(db), objects).List(t.Context(), actor, project, "document", "", 100)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("invalid document published", err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequestWithContext(t.Context(), "GET", url, nil))
	if denied.Code != 403 {
		t.Fatal("stale principal served original", denied.Code)
	}
}

func TestDocumentSourcesPGSharedLocksRetainFrozenFactsUntilOwningCommit(t *testing.T) {
	db, owner := documentTestDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, owner)
	result := documentUpload(t, db, objects, actor, project, "剧本.txt", []byte("锁住原文"))
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	frozen, err := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(tx), objects).Freeze(t.Context(), actor, project, []uuid.UUID{result.Asset.ID})
	if err != nil || len(frozen) != 1 {
		t.Fatal("owning source freeze", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	mutator := owner.WithContext(ctx).Begin()
	if mutator.Error != nil {
		t.Fatal(mutator.Error)
	}
	defer func() { _ = mutator.Rollback().Error }()
	var pid int
	if err := mutator.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- mutator.Exec(`UPDATE media.media_asset SET revision=revision+1 WHERE id=?`, result.Asset.ID).Error
	}()
	blocked := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		var count int
		if err := owner.Raw(`SELECT cardinality(pg_blocking_pids(?))`, pid).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			blocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("source mutation did not converge", ctx.Err())
	}
	if !blocked {
		t.Fatal("source SHARE lock released before owning admission commit")
	}
	if err := mutator.Commit().Error; err != nil {
		t.Fatal(err)
	}
}

type documentUnknownUpload struct{ mediaapp.UploadRepository }

func (r documentUnknownUpload) CommitUpload(ctx context.Context, actor identityapp.Principal, in mediaapp.UploadRequest, asset domain.MediaAsset, rends []domain.Rendition) (mediaapp.UploadResult, error) {
	if _, err := r.UploadRepository.CommitUpload(ctx, actor, in, asset, rends); err != nil {
		return mediaapp.UploadResult{}, err
	}
	return mediaapp.UploadResult{}, io.ErrUnexpectedEOF
}

func TestDocumentActualUploadUnknownCommitPreservesOriginalAndPermanentReplay(t *testing.T) {
	db, owner := documentTestDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, owner)
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader([]byte("未知回执原文")), "source.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	in := mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "source.txt"}, File: file, LocalReviewConfirmed: true}
	store := pgmedia.NewStore(db)
	service := mediaapp.NewUploadService(documentUnknownUpload{store}, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("unknown commit not propagated", err)
	}
	first, err := documentUploadService(db, objects).Upload(t.Context(), in)
	if err != nil {
		t.Fatal("durable upload original key replay", err)
	}
	second, err := documentUploadService(db, objects).Upload(t.Context(), in)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("permanent source receipt changed", err)
	}
	asset, err := store.FindAsset(t.Context(), actor, project, first.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = objects.Remove(ctx, asset.ObjectKey)
	})
	reader := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(db), objects)
	frozen, err := reader.Freeze(t.Context(), actor, project, []uuid.UUID{asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := reader.Open(t.Context(), actor, project, frozen[0])
	if err != nil {
		t.Fatal("unknown upload cleaned durable original", err)
	}
	_ = actual.Close()
}
