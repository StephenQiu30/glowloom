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
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type packageTestProjectAccess struct{ mediaapp.LibraryProjectAccess }

func packageTestAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return packageTestProjectAccess{libraryTestAccess(tx)}
}

func (a packageTestProjectAccess) Authorize(ctx context.Context, actor identityapp.Principal, id uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	facts, err := a.LibraryProjectAccess.Authorize(ctx, actor, id, write)
	if errors.Is(err, workspacedomain.ErrProjectStateConflict) {
		err = errors.Join(domain.ErrMediaStateConflict, err)
	}
	return facts, err
}

func packageActualService(t *testing.T, db *gorm.DB, objects mediaapp.PackageObjects, access pgmedia.LibraryProjectAccessFactory) (*pgmedia.PackageStore, *mediaapp.LibraryPackageService) {
	t.Helper()
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	uploads := mediaapp.NewScopedUploadService(pgmedia.NewStore(db), pgmedia.NewStore(db), mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, nil, time.Now)
	repo := pgmedia.NewPackageStore(db, access, transferTestGuards, time.Now)
	return repo, mediaapp.NewLibraryPackageService(repo, uploads, objects, time.Now)
}

func packageRouter(actor identityapp.Principal, service *mediaapp.LibraryPackageService) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewPackageHandler(service).Register(group)
	return router
}

func packageHTTPImport(t *testing.T, router *gin.Engine, path string, request []byte, data []byte, key uuid.UUID, extra bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormField("request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(request); err != nil {
		t.Fatal(err)
	}
	part, err = w.CreateFormFile("file", "complete.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if extra {
		if err := w.WriteField("actor_id", uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, &body)
	r.Header.Set("Origin", "http://127.0.0.1:3000")
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.Header.Set("Idempotency-Key", key.String())
	out := httptest.NewRecorder()
	router.ServeHTTP(out, r)
	return out
}

func packageCleanup(t *testing.T, repo *pgmedia.PackageStore, objects mediaapp.PackageObjects, actor identityapp.Principal, scope domain.LibraryScope) []string {
	t.Helper()
	keys, err := repo.StorageUsageKeys(t.Context(), actor, scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = objects.Remove(ctx, key)
		}
	})
	return keys
}

func TestMediaPackageHTTPAllSixRoutesClosedMultipartActualAttachmentAndCurrentAuthority(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	router := packageRouter(actor, service)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true}
	body, _ := json.Marshal(in)
	data, key := ownPackageZIP(t), uuid.New()
	path := "/api/media/library/packages/imports"
	for _, injected := range [][]byte{
		bytes.Replace(body, []byte(`"local_review_confirmed":true`), []byte(`"local_review_confirmed":false`), 1),
		bytes.Replace(body, []byte(`"scope":`), []byte(`"actor_id":"wrong","scope":`), 1),
		bytes.Replace(body, []byte(`"scope":`), []byte(`"expected_revision":5,"scope":`), 1),
	} {
		if response := packageHTTPImport(t, router, path, injected, data, uuid.New(), false); response.Code != 422 {
			t.Fatal("open multipart request", response.Code, response.Body.String())
		}
	}
	if response := packageHTTPImport(t, router, path, body, data, uuid.New(), true); response.Code != 422 {
		t.Fatal("extra part accepted", response.Code)
	}
	if response := packageHTTPImport(t, router, path+"?actor_id=wrong", body, data, uuid.New(), false); response.Code != 422 {
		t.Fatal("query injection", response.Code)
	}
	created := packageHTTPImport(t, router, path, body, data, key, false)
	var job mediaapp.PackageJob
	if created.Code != 202 || json.Unmarshal(created.Body.Bytes(), &job) != nil || job.Status != "succeeded" || job.ItemCount != 2 {
		t.Fatal(created.Code, created.Body.String())
	}
	packageCleanup(t, repo, objects, actor, scope)
	for _, private := range []string{"object_key", "source.zip", "source-text", "plain_text", "principal_id", "request_id"} {
		if strings.Contains(created.Body.String(), private) {
			t.Fatal("private frozen facts leaked", private)
		}
	}
	if replay := packageHTTPImport(t, packageRouter(actor, service), path, body, data, key, false); replay.Code != 202 || !bytes.Equal(created.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("permanent multipart replay", replay.Code, replay.Body.String())
	}
	changed := bytes.Replace(body, []byte(`"expected_revision":0`), []byte(`"expected_revision":1`), 1)
	if response := packageHTTPImport(t, router, path, changed, data, key, false); response.Code != 409 {
		t.Fatal("changed permanent body accepted", response.Code)
	}
	get := libraryHTTPRequest(t, router, http.MethodGet, path+"/"+job.ID.String(), nil, uuid.Nil)
	if get.Code != 200 || !bytes.Equal(created.Body.Bytes(), get.Body.Bytes()) {
		t.Fatal("current job", get.Code, get.Body.String())
	}
	query := "?scope=project&project_id=" + project.String()
	list := libraryHTTPRequest(t, router, http.MethodGet, path+query+"&page_size=1", nil, uuid.Nil)
	var page mediaapp.PackagePage
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || page.CurrentActorID != actor.ID || page.Total != 1 || len(page.Jobs) != 1 {
		t.Fatal("scoped history", list.Code, list.Body.String())
	}
	for _, q := range []string{"?scope=personal&scope=project", "?page=1&page=2", "?page=0", "?scope=personal&project_id=" + project.String(), "?org_id=" + actor.OrgID.String()} {
		if result := libraryHTTPRequest(t, router, http.MethodGet, path+q, nil, uuid.Nil); result.Code != 422 {
			t.Fatal("ambiguous history", q, result.Code)
		}
	}
	download := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/packages/export"+query, nil, uuid.Nil)
	digest := sha256.Sum256(download.Body.Bytes())
	if download.Code != 200 || download.Header().Get("X-Content-SHA256") != hex.EncodeToString(digest[:]) || !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment;") || download.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("actual ZIP attachment", download.Code, download.Body.String())
	}
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(download.Body.Bytes()), int64(download.Body.Len()))
	if err != nil || len(archive.Manifest.Items) != 2 {
		t.Fatal("unreadable attachment", err)
	}
	control, _ := json.Marshal(mediaapp.PackageControl{ExpectedRevision: job.Revision})
	reconcileKey := uuid.New()
	if response := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+job.ID.String()+"/reconcile", control, reconcileKey); response.Code != 202 {
		t.Fatal("terminal reconciliation", response.Code, response.Body.String())
	}
	if response := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+job.ID.String()+"/reconcile", []byte(`{"expected_revision":1,"expected_revision":2}`), uuid.New()); response.Code != 422 {
		t.Fatal("duplicate control fields", response.Code)
	}
	if response := packageHTTPImport(t, router, path, changed, data, reconcileKey, false); response.Code != 409 {
		t.Fatal("control key became a new batch", response.Code)
	}
	var admitted int64
	if err := db.Raw(`SELECT count(*) FROM media.package_job WHERE actor_id=?`, actor.ID).Scan(&admitted).Error; err != nil || admitted != 1 {
		t.Fatal("conflicting key left a new admission", admitted, err)
	}
	if response := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+job.ID.String()+"/cancel", control, uuid.New()); response.Code != 409 {
		t.Fatal("published originals cancelled", response.Code)
	}
	other, _ := mediaStoreProject(t, db)
	if response := libraryHTTPRequest(t, packageRouter(other, service), http.MethodGet, path+"/"+job.ID.String(), nil, uuid.Nil); response.Code != 404 {
		t.Fatal("foreign job discovered", response.Code)
	}
	if err := db.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if response := libraryHTTPRequest(t, router, http.MethodGet, "/api/media/library/packages/export"+query, nil, uuid.Nil); response.Code != 200 {
		t.Fatal("archived read lost", response.Code)
	}
	if response := packageHTTPImport(t, router, path, changed, data, uuid.New(), false); response.Code != 409 {
		t.Fatal("archived write accepted", response.Code, response.Body.String())
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if response := packageHTTPImport(t, router, path, body, data, key, false); response.Code != 403 {
		t.Fatal("disabled actor received permanent source receipt", response.Code)
	}
}

type packageUnknownRemove struct {
	mediaapp.PackageObjects
	unknown bool
}

func (o *packageUnknownRemove) Remove(ctx context.Context, key string) error {
	err := o.PackageObjects.Remove(ctx, key)
	if err == nil && o.unknown {
		o.unknown = false
		return io.ErrUnexpectedEOF
	}
	return err
}

func TestMediaPackageHTTPCancelUnknownRetainsReservationOriginalReceiptAndFullAbsentProof(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	unknown := &packageUnknownWrite{PackageObjects: objects}
	repo, service := packageActualService(t, db, unknown, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	job, err := service.Import(t.Context(), actor, in, packageDownloaded(t, ownPackageZIP(t)))
	if err != nil || job.Status != "needs_reconciliation" {
		t.Fatal(job, err)
	}
	keys := packageCleanup(t, repo, objects, actor, scope)
	_, cancelService := packageActualService(t, db, &packageUnknownRemove{PackageObjects: objects, unknown: true}, packageTestAccess)
	router := packageRouter(actor, cancelService)
	path := "/api/media/library/packages/imports/" + job.ID.String() + "/cancel"
	body, _ := json.Marshal(mediaapp.PackageControl{ExpectedRevision: job.Revision})
	key := uuid.New()
	first := libraryHTTPRequest(t, router, http.MethodPost, path, body, key)
	var current mediaapp.PackageJob
	if first.Code != 202 || json.Unmarshal(first.Body.Bytes(), &current) != nil || current.Status != "needs_reconciliation" {
		t.Fatal("uncertain removal reported cancelled", first.Code, first.Body.String())
	}
	if busy, err := repo.HasInflightPackageWork(t.Context(), actor, project); err != nil || !busy {
		t.Fatal("unknown cleanup released reservation", busy, err)
	}
	if replay := libraryHTTPRequest(t, router, http.MethodPost, path, body, key); replay.Code != 202 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("unknown receipt changed", replay.Code)
	}
	second := libraryHTTPRequest(t, router, http.MethodPost, path, body, uuid.New())
	if second.Code != 202 || json.Unmarshal(second.Body.Bytes(), &current) != nil || current.Status != "cancelled" || current.Revision != 2 {
		t.Fatal("full verified cleanup", second.Code, second.Body.String())
	}
	for _, key := range keys {
		if exists, err := objects.Exists(t.Context(), key); err != nil || exists {
			t.Fatal("cancelled with retained object", exists, err)
		}
	}
	if busy, err := repo.HasInflightPackageWork(t.Context(), actor, project); err != nil || busy {
		t.Fatal("cancelled batch busy", busy, err)
	}
	if _, err := service.Get(t.Context(), actor, job.ID); err != nil {
		t.Fatal(err)
	}
	var assets int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=?`, project).Scan(&assets).Error; err != nil || assets != 0 {
		t.Fatal("cancel published partial assets", assets, err)
	}
	if replay := libraryHTTPRequest(t, router, http.MethodPost, path, body, key); replay.Code != 202 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("later head rewrote unknown command", replay.Code)
	}
	if response := libraryHTTPRequest(t, router, http.MethodPost, strings.TrimSuffix(path, "cancel")+"reconcile", body, key); response.Code != 409 {
		t.Fatal("same key different action accepted", response.Code)
	}
	if _, err := service.Reconcile(t.Context(), actor, job.ID, mediaapp.PackageControl{ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()}); !errors.Is(err, mediaapp.ErrPackageConflict) {
		t.Fatal("stale terminal head accepted", err)
	}
}
