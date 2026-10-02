package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// These synthetic intent facts test the media consumer boundary. Installed
// mediatool/operation owning aggregates require their own independent evidence.
type usageIntentFixture struct {
	keys []string
	err  error
}

func (f usageIntentFixture) StorageUsageKeys(context.Context, identityapp.Principal, domain.LibraryScope) ([]string, error) {
	return f.keys, f.err
}

func usageReader(db *gorm.DB, objects *objectstorage.Client, fixture usageIntentFixture) *mediaapp.StorageUsageReader {
	store := pgmedia.NewStorageUsageStore(db, libraryTestAccess, func(*gorm.DB) mediaapp.StorageUsageIntentSource { return fixture })
	return mediaapp.NewStorageUsageReader(store, mediaobjects.NewStorageUsageObjects(objects), time.Now)
}

func usagePut(t *testing.T, objects *objectstorage.Client, key string, data []byte) {
	t.Helper()
	digest := sha256.Sum256(data)
	if err := objects.PutIfAbsent(t.Context(), key, bytes.NewReader(data), int64(len(data)), "application/octet-stream", hex.EncodeToString(digest[:])); err != nil {
		t.Fatal("write unique usage fixture object", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(ctx, key); err != nil {
			t.Error("remove exact usage fixture object", err)
		}
	})
}

func TestMediaStorageUsageActualObjectsIncludesTrashUnknownIntentAndPhysicalRemoval(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, _ := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	asset := libraryPersonalFixture(t, db, objects, actor, "occupied.png", uploadPNG(t))
	var keys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION SELECT object_key FROM media.rendition WHERE media_asset_id=? ORDER BY object_key`, asset.Asset.ID, asset.Asset.ID).Scan(&keys).Error; err != nil || len(keys) != 3 {
		t.Fatal("actual owning registered keys", err)
	}
	var expected int64
	for _, key := range keys {
		info, err := objects.Stat(t.Context(), key)
		if err != nil {
			t.Fatal("actual registered object stat", err)
		}
		expected += info.Size
	}
	unknown := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/unknown-" + uuid.NewString() + ".png"
	intent := "lanverse-library-usage/" + uuid.NewString() + "/unregistered/output.bin"
	foreignKey := "personal/" + foreign.OrgID.String() + "/" + foreign.ID.String() + "/image/foreign-" + uuid.NewString() + ".png"
	usagePut(t, objects, unknown, []byte("unregistered upload"))
	usagePut(t, objects, intent, []byte("unregistered own intent"))
	usagePut(t, objects, foreignKey, []byte("foreign private object"))
	expected += int64(len("unregistered upload") + len("unregistered own intent"))
	reader := usageReader(db, objects, usageIntentFixture{keys: append([]string{intent}, keys...)})
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	initial, err := reader.StorageUsage(t.Context(), actor, scope)
	if err != nil || initial.UsedBytes != expected || initial.ObjectCount != 5 || initial.LimitBytes != nil {
		t.Fatal("actual unique objects omitted unregistered work or counted foreign objects", initial, err)
	}
	catalog := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	page, err := catalog.ListLibrary(t.Context(), actor, scope, libraryQuery())
	if err != nil || len(page.Items) != 1 {
		t.Fatal("load actual catalog revisions", page, err)
	}
	if _, err := catalog.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: page.Revision, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: asset.Asset.ID, Revision: page.Items[0].Revision}}}); err != nil {
		t.Fatal("recycle actual image", err)
	}
	retained, err := reader.StorageUsage(t.Context(), actor, scope)
	if err != nil || retained.UsedBytes != initial.UsedBytes || retained.ObjectCount != initial.ObjectCount {
		t.Fatal("hiding trash incorrectly freed physical bytes", retained, err)
	}
	for _, key := range append(keys, unknown, intent) {
		if err := objects.Remove(t.Context(), key); err != nil {
			t.Fatal("remove only this synthetic private object", err)
		}
	}
	removed, err := reader.StorageUsage(t.Context(), actor, scope)
	if err != nil || removed.UsedBytes != 0 || removed.ObjectCount != 0 {
		t.Fatal("actual missing objects remained billed as row bytes", removed, err)
	}
	if _, err := objects.Stat(t.Context(), foreignKey); err != nil {
		t.Fatal("usage query modified foreign object", err)
	}
}

func usageRouter(reader *mediaapp.StorageUsageReader, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewStorageUsageHandler(reader).Register(group)
	return router
}

func TestMediaStorageUsageActualProjectAuthorityAndHTTPClosedQuery(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	foreign, foreignProject := mediaStoreProject(t, db)
	base := pgmedia.NewStore(db)
	uploads := mediaapp.NewScopedUploadService(base, base, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	input := uploadInput(t)
	input.Actor, input.Request.ProjectID = actor, project
	asset, err := uploads.Upload(t.Context(), input)
	if err != nil {
		t.Fatal("actual project asset", err)
	}
	var keys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION SELECT object_key FROM media.rendition WHERE media_asset_id=?`, asset.Asset.ID, asset.Asset.ID).Scan(&keys).Error; err != nil || len(keys) != 3 {
		t.Fatal("actual project keys", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = objects.Remove(ctx, key)
		}
	})
	reader := usageReader(db, objects, usageIntentFixture{})
	router := usageRouter(reader, actor)
	url := "/api/media/library/storage-usage?scope=project&project_id=" + project.String()
	response := libraryHTTPRequest(t, router, http.MethodGet, url, nil, uuid.Nil)
	var result mediaapp.StorageUsage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.UsedBytes < 1 || result.ObjectCount != 3 || result.CurrentActorID != actor.ID || result.CurrentOrgID != actor.OrgID || result.Scope.ProjectID == nil || *result.Scope.ProjectID != project || !bytes.Contains(response.Body.Bytes(), []byte(`"limit_bytes":null`)) || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("actual safe scoped usage HTTP", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"object_key", "personal_actor_id", "worker_fence", "http://", "occupied.png"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatal("private inventory fact leaked in usage DTO", forbidden)
		}
	}
	for _, query := range []string{"&actor_id=" + foreign.ID.String(), "&page=2", "&scope=project", "&project_id=" + foreignProject.String()} {
		if bad := libraryHTTPRequest(t, router, http.MethodGet, url+query, nil, uuid.Nil); bad.Code != 422 {
			t.Fatal("ambiguous or foreign scope query accepted", query, bad.Code)
		}
	}
	if bad := libraryHTTPRequest(t, usageRouter(reader, foreign), http.MethodGet, url, nil, uuid.Nil); bad.Code != 404 {
		t.Fatal("cross-org project occupancy exposed", bad.Code, bad.Body.String())
	}
	if err := db.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	archived := libraryHTTPRequest(t, router, http.MethodGet, url, nil, uuid.Nil)
	if archived.Code != 200 || archived.Body.String() == "" {
		t.Fatal("archived readable project capacity lost", archived.Code)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if bad := libraryHTTPRequest(t, router, http.MethodGet, url, nil, uuid.Nil); bad.Code != 403 {
		t.Fatal("revoked identity retained current occupancy access", bad.Code, bad.Body.String())
	}
}

func TestMediaStorageUsageActualMissingIntentOwnerAndCancelledReadsFailClosed(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, _ := mediaStoreProject(t, db)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	missing := mediaapp.NewStorageUsageReader(pgmedia.NewStorageUsageStore(db, libraryTestAccess, nil), mediaobjects.NewStorageUsageObjects(objects), time.Now)
	if _, err := missing.StorageUsage(t.Context(), actor, scope); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing installed owning query returned zero usage", err)
	}
	broken := usageReader(db, objects, usageIntentFixture{err: mediaapp.ErrUnavailable})
	if response := libraryHTTPRequest(t, usageRouter(broken, actor), http.MethodGet, "/api/media/library/storage-usage?scope=personal", nil, uuid.Nil); response.Code != 503 {
		t.Fatal("owner failure became zero DTO", response.Code, response.Body.String())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := usageReader(db, objects, usageIntentFixture{}).StorageUsage(ctx, actor, scope); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled private inventory query continued", err)
	}
}

func TestMediaStorageUsageActualUnknownTransferCountsIndependentTargetBeforeRegistration(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	reader := usageReader(f.db, f.objects, usageIntentFixture{})
	source, err := reader.StorageUsage(t.Context(), f.actor, f.input.Source)
	if err != nil || source.ObjectCount != 3 || source.UsedBytes < 1 {
		t.Fatal("source physical image and thumbnails", source, err)
	}
	before, err := reader.StorageUsage(t.Context(), f.actor, f.input.Target)
	if err != nil || before.ObjectCount != 0 || before.UsedBytes != 0 {
		t.Fatal("pending object intent without a real object was counted", before, err)
	}
	objects := &transferUnknownWrite{ProjectCopyObjects: mediaobjects.NewProjectCopyObjects(f.objects), failAt: 1}
	unknown, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
	if err != nil || !unknown.NeedsReconciliation || unknown.Status != "needs_reconciliation" {
		t.Fatal("actual unknown transfer write fixture", unknown, err)
	}
	during, err := reader.StorageUsage(t.Context(), f.actor, f.input.Target)
	if err != nil || during.ObjectCount != 1 || during.UsedBytes < 1 {
		t.Fatal("unregistered physical target was omitted", during, err)
	}
	var registered int64
	if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, unknown.Items[0].TargetAssetID).Scan(&registered).Error; err != nil || registered != 0 {
		t.Fatal("unknown fixture already registered target", registered, err)
	}
	reconcile, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, uuid.New(), unknown.Revision, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	ready, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(reconcile))
	if err != nil || ready.Status != "succeeded" {
		t.Fatal("actual transfer recovery", ready, err)
	}
	complete, err := reader.StorageUsage(t.Context(), f.actor, f.input.Target)
	if err != nil || complete.ObjectCount != 3 || complete.UsedBytes != source.UsedBytes {
		t.Fatal("independent target counted twice through metadata and intent", complete, source, err)
	}
	retained, err := reader.StorageUsage(t.Context(), f.actor, f.input.Source)
	if err != nil || retained.ObjectCount != 3 || retained.UsedBytes != source.UsedBytes {
		t.Fatal("target ownership altered source accounting", retained, source, err)
	}
}
