package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestMediaPackageProductionRouterInstallsAllSix(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000"}, &gorm.DB{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /api/media/library/packages/imports", "GET /api/media/library/packages/imports", "GET /api/media/library/packages/imports/:job_id", "POST /api/media/library/packages/imports/:job_id/reconcile", "POST /api/media/library/packages/imports/:job_id/cancel", "GET /api/media/library/packages/export",
	} {
		if !routes[route] {
			t.Errorf("production router missing %s", route)
		}
	}
}

func packageCompositionZIP(t *testing.T, kind domain.LibraryKind) []byte {
	t.Helper()
	var imageBytes bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.NRGBA{R: 50, G: uint8(x * 2), B: uint8(y * 2), A: 255})
		}
	}
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(imageBytes.Bytes())
	filePath := "files/actual.png"
	manifest := mediaapp.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: time.Now().UTC(), LibraryKind: kind, Folders: []mediaapp.LibraryPackageFolder{}, Items: []mediaapp.LibraryPackageItem{{ID: "actual-image", Kind: "image", FilePath: &filePath, State: "active", Metadata: mediaapp.LibraryMetadata{Title: "根接线原图", Category: "material", Tags: []string{}}}}, Files: []mediaapp.LibraryPackageFile{{Path: filePath, FileName: "原图.png", MIMEType: "image/png", ByteSize: int64(imageBytes.Len()), SHA256: hex.EncodeToString(digest[:])}}}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	for _, file := range []struct {
		name string
		data []byte
	}{{"manifest.json", body}, {filePath, imageBytes.Bytes()}} {
		entry, err := w.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func packageCompositionImport(t *testing.T, router http.Handler, input mediaapp.PackageImportRequest, archive []byte, key uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	jsonBody, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	part, err := w.CreateFormField("request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(jsonBody); err != nil {
		t.Fatal(err)
	}
	part, err = w.CreateFormFile("file", "actual.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/media/library/packages/imports", &body)
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.Header.Set("Idempotency-Key", key.String())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, r)
	return response
}

func TestMediaPackageProductionRouterActualBothScopesCompleteUsageAndAllSixRoutes(t *testing.T) {
	for _, kind := range []domain.LibraryKind{domain.LibraryPersonal, domain.LibraryProject} {
		t.Run(string(kind), func(t *testing.T) {
			f := newMediaCompositionFixture(t)
			scope := domain.LibraryScope{Kind: kind}
			query := "?scope=" + string(kind)
			if kind == domain.LibraryProject {
				scope.ProjectID = &f.project
				query += "&project_id=" + f.project.String()
			}
			read := mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library"+query, nil)
			var library mediaapp.LibraryPage
			if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &library) != nil {
				t.Fatal("formal current scoped library", read.Code)
			}
			var before mediaapp.StorageUsage
			usage := mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library/storage-usage"+query, nil)
			if usage.Code != 200 || json.Unmarshal(usage.Body.Bytes(), &before) != nil {
				t.Fatal("formal scoped usage before package", usage.Code)
			}
			in := mediaapp.PackageImportRequest{Scope: scope, ExpectedRevision: library.Revision, LocalReviewConfirmed: true}
			if kind == domain.LibraryProject {
				if err := f.owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, f.project).Scan(&in.ExpectedProjectRevision).Error; err != nil {
					t.Fatal(err)
				}
			}
			archive, key := packageCompositionZIP(t, kind), uuid.New()
			imported := packageCompositionImport(t, f.router, in, archive, key)
			var job mediaapp.PackageJob
			if imported.Code != 202 || json.Unmarshal(imported.Body.Bytes(), &job) != nil || job.Status != "succeeded" || job.ItemCount != 1 {
				t.Fatal("formal package import", imported.Code, imported.Body.String())
			}
			var keys []string
			if err := f.owner.Raw(`SELECT object_key FROM media.package_object WHERE job_id=? ORDER BY object_key`, job.ID).Scan(&keys).Error; err != nil || len(keys) != 4 {
				t.Fatal("complete source ZIP, original and two actual renditions", len(keys), err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				for _, key := range keys {
					if err := f.objects.Remove(ctx, key); err != nil {
						t.Error("remove exact package fixture object", err)
					}
				}
			})
			expected := before.UsedBytes
			for _, key := range keys {
				object, err := f.objects.Stat(t.Context(), key)
				if err != nil {
					t.Fatal("actual retained object", err)
				}
				expected += object.Size
			}
			usage = mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library/storage-usage"+query, nil)
			var after mediaapp.StorageUsage
			if usage.Code != 200 || json.Unmarshal(usage.Body.Bytes(), &after) != nil || after.UsedBytes != expected || after.ObjectCount != before.ObjectCount+4 || after.LimitBytes != nil {
				t.Fatal("package source/output keys omitted or counted twice", usage.Code, after)
			}
			if replay := packageCompositionImport(t, f.router, in, archive, key); replay.Code != 202 || !bytes.Equal(replay.Body.Bytes(), imported.Body.Bytes()) {
				t.Fatal("production permanent replay", replay.Code)
			}
			path := "/api/media/library/packages/imports/" + job.ID.String()
			if response := mediaCompositionRequest(t, f.router, http.MethodGet, path, nil); response.Code != 200 {
				t.Fatal("production package detail", response.Code)
			}
			list := mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library/packages/imports"+query, nil)
			var page mediaapp.PackagePage
			if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || page.CurrentActorID != f.actor.ID || page.CurrentOrgID != f.actor.OrgID {
				t.Fatal("production scoped history", list.Code)
			}
			if response := mediaCompositionRequest(t, f.router, http.MethodPost, path+"/reconcile", mediaapp.PackageControl{ExpectedRevision: job.Revision}); response.Code != 202 {
				t.Fatal("production terminal reconciliation", response.Code, response.Body.String())
			}
			if response := mediaCompositionRequest(t, f.router, http.MethodPost, path+"/cancel", mediaapp.PackageControl{ExpectedRevision: job.Revision}); response.Code != 409 {
				t.Fatal("production rejected published cancellation", response.Code)
			}
			exported := mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library/packages/export"+query, nil)
			if exported.Code != 200 || !strings.HasPrefix(exported.Header().Get("Content-Disposition"), "attachment;") {
				t.Fatal("production complete actual attachment", exported.Code, exported.Body.String())
			}
			actual, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
			if err != nil || len(actual.Manifest.Items) != len(library.Items)+1 {
				t.Fatal("production export dropped scoped items", err)
			}
		})
	}
}

type packageCompositionAccess struct {
	store *pgworkspace.ProjectContentAccessStore
}

func (a packageCompositionAccess) Authorize(ctx context.Context, actor identityapp.Principal, id uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	facts, err := a.store.Authorize(ctx, actor, id, write)
	return mediaapp.LibraryProjectFacts{ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, err
}
func (a packageCompositionAccess) TouchContent(ctx context.Context, actor identityapp.Principal, id uuid.UUID, revision int64) (int64, error) {
	return a.store.TouchContent(ctx, actor, id, revision)
}

type packageCompositionIdleGuard struct{}

func (packageCompositionIdleGuard) HasInflightProjectWork(context.Context, identityapp.Principal, uuid.UUID) (bool, error) {
	return false, nil
}

type packageCompositionUnknownWrite struct {
	mediaapp.PackageObjects
	once bool
}

func (o *packageCompositionUnknownWrite) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	err := o.PackageObjects.PutIfAbsent(ctx, key, r, size, mime, digest)
	if err == nil && !o.once {
		o.once = true
		return io.ErrUnexpectedEOF
	}
	return err
}
func TestMediaPackageProductionLifecycleUnknownAndOtherUnreadableOwnerRemainClosed(t *testing.T) {
	f := newMediaCompositionFixture(t)
	repo := pgmedia.NewPackageStore(f.runtime, func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return packageCompositionAccess{pgworkspace.NewProjectContentAccessStore(tx)}
	}, func(*gorm.DB) mediaapp.LibraryWorkGuards { return packageCompositionIdleGuard{} }, time.Now)
	uploads := mediaapp.NewScopedUploadService(pgmedia.NewStore(f.runtime), pgmedia.NewStore(f.runtime), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, nil, time.Now)
	objects := mediaobjects.NewProjectCopyObjects(f.objects)
	service := mediaapp.NewLibraryPackageService(repo, uploads, &packageCompositionUnknownWrite{PackageObjects: objects}, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &f.project}
	in := mediaapp.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	if err := f.owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, f.project).Scan(&in.ExpectedProjectRevision).Error; err != nil {
		t.Fatal(err)
	}
	file, err := mediaapp.ReadPackageUpload(t.Context(), bytes.NewReader(packageCompositionZIP(t, domain.LibraryProject)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	job, err := service.Import(t.Context(), f.actor, in, file)
	if err != nil || job.Status != "needs_reconciliation" {
		t.Fatal("actual unknown private package", job, err)
	}
	keys, err := repo.StorageUsageKeys(t.Context(), f.actor, scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = f.objects.Remove(ctx, key)
		}
	})
	transition := map[string]int64{"expected_revision": in.ExpectedProjectRevision}
	path := "/api/projects/" + f.project.String() + "/archive"
	response := mediaCompositionRequest(t, f.router, http.MethodPost, path, transition)
	if response.Code != 409 {
		t.Fatal("production lifecycle ignored actual package reservation", response.Code, response.Body.String())
	}
	if err := f.owner.Exec(`REVOKE SELECT ON mediatool.export_job FROM lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.owner.Exec(`GRANT SELECT ON mediatool.export_job TO lanverse_app`).Error; err != nil {
			t.Error("restore isolated installed owner permission", err)
		}
	})
	response = mediaCompositionRequest(t, f.router, http.MethodPost, path, transition)
	if response.Code != 503 {
		t.Fatal("unknown package hid another unreadable lifecycle owner", response.Code, response.Body.String())
	}
	response = mediaCompositionRequest(t, f.router, http.MethodGet, "/api/media/library/storage-usage?scope=project&project_id="+f.project.String(), nil)
	if response.Code != 503 || strings.Contains(response.Body.String(), "used_bytes") {
		t.Fatal("retained source ZIP hid another unreadable usage owner", response.Code)
	}
	if err := f.owner.Exec(`GRANT SELECT ON mediatool.export_job TO lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	actual := mediaapp.NewLibraryPackageService(repo, uploads, objects, time.Now)
	cancelled, err := actual.Cancel(t.Context(), f.actor, job.ID, mediaapp.PackageControl{ExpectedRevision: job.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatal("actual unknown package cleanup", cancelled, err)
	}
	response = mediaCompositionRequest(t, f.router, http.MethodPost, path, transition)
	if response.Code != 200 {
		t.Fatal("terminal package still blocked production lifecycle", response.Code, response.Body.String())
	}
}
