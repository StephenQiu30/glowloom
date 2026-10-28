package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func packageLocationDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_PACKAGE_LOCATION_DSN")
	if dsn == "" {
		t.Skip("set an isolated task-owned storage package database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "glowloom_subject_storage_") {
		t.Fatal("requires isolated loopback storage database")
	}
	owner, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	var initialized bool
	if err := owner.DB.Raw(`SELECT to_regclass('media.package_job') IS NOT NULL`).Scan(&initialized).Error; err != nil {
		t.Fatal(err)
	}
	if !initialized {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.DB.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
		routes := pgmedia.NewStorageRoutes(owner.DB)
		sha := strings.Repeat("c", 64)
		if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
			t.Fatal(err)
		}
		if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
			t.Fatal(err)
		}
	}
	q := u.Query()
	q.Set("options", "-c role=glowloom_app")
	u.RawQuery = q.Encode()
	runtime, err := platformdb.Open(t.Context(), u.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return owner.DB, runtime.DB
}
func TestPackageLocatedPGSDKWholeSetReplayDefaultSwitchAndPurge(t *testing.T) {
	owner, runtime := packageLocationDB(t)
	old, oldServer := newCOSStageFixture(t)
	newer, newServer := newCOSStageFixture(t)
	cfg := stageConfiguration(t, oldServer.URL, newServer.URL)
	suffix := uuid.NewString()[:8]
	oldID, newID := "stage-old-"+suffix, "stage-new-"+suffix
	cfg.Profiles[oldID], cfg.Profiles[newID] = cfg.Profiles["stage-old"], cfg.Profiles["stage-new"]
	delete(cfg.Profiles, "stage-old")
	delete(cfg.Profiles, "stage-new")
	cfg.DefaultWriteProfile = oldID
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	routes := pgmedia.NewStorageRoutes(owner)
	profiles := []media.StorageProfile{}
	for id, p := range cfg.Profiles {
		profiles = append(profiles, media.StorageProfile{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle})
	}
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	if err := pgmedia.NewStorageRoutes(runtime).RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal("runtime immutable profile read demanded UPDATE permission", err)
	}
	err = runtime.Exec(`UPDATE media.storage_profile SET bucket=bucket WHERE id=?`, oldID).Error
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("runtime gained immutable storage profile UPDATE", err)
	}

	prior, _ := routes.ReadActive(t.Context())
	rev := "fixture-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), prior.Revision, rev, oldID); err != nil {
		t.Fatal(err)
	}
	actor, _ := mediaStoreProject(t, owner)
	repo := mediaPackageStore(runtime, libraryTestAccess, transferTestGuards, time.Now).WithObjectLocations()
	store := mediaTestStore(runtime)
	uploads := media.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, nil, time.Now)
	objects := mediaobjects.NewRoutedPackageObjects(registry, pgmedia.NewStorageRoutes(runtime))
	service := media.NewLibraryPackageService(repo, uploads, objects, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	data := packageFilesZIP(t, []packageFixtureFile{{"image", "image.png", "image/png", uploadPNG(t)}})
	in := media.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	job, err := service.Import(ctx, actor, in, packageDownloaded(t, data))
	if err != nil || job.Status != "succeeded" {
		t.Fatal("native package import", job, err)
	}
	var proof struct{ Count, Accepted int }
	if err := owner.Raw(`SELECT count(*) AS count,count(*) FILTER(WHERE l.state='accepted' AND w.state='accepted') AS accepted FROM media.package_object p JOIN media.object_location l ON l.id=p.object_location_id AND l.object_key=p.object_key JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE p.job_id=?`, job.ID).Scan(&proof).Error; err != nil || proof.Count != 4 || proof.Accepted != 4 {
		t.Fatal("incomplete publication proof", proof, err)
	}
	if len(old.objects) != 4 || len(newer.objects) != 0 {
		t.Fatal("route changed physical destination", len(old.objects), len(newer.objects))
	}
	if _, err := routes.Activate(t.Context(), rev, "fixture-"+uuid.NewString(), newID); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Import(ctx, actor, in, packageDownloaded(t, data))
	if err != nil || replay.ID != job.ID || len(old.objects) != 4 || len(newer.objects) != 0 {
		t.Fatal("permanent replay repinned or wrote objects", err)
	}
	exported, err := service.Export(ctx, actor, scope)
	if err != nil {
		t.Fatal("located export after switch", err)
	}
	_ = exported.Close()
	var assetRow struct{ ID uuid.UUID }
	if err := owner.Raw(`SELECT id FROM media.media_asset WHERE personal_actor_id=?`, actor.ID).Scan(&assetRow).Error; err != nil {
		t.Fatal(err)
	}
	library := mediaLibraryStore(runtime, libraryTestAccess, time.Now)
	if _, err := library.ApplyLibraryCommand(ctx, actor, media.LibraryCommand{Scope: scope, Action: "recycle_items", Key: uuid.New(), ExpectedRevision: 1, Items: []media.LibraryItemRevision{{ID: assetRow.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	purge := mediaPurgeStore(runtime, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	purgeJob, err := purge.CreatePurge(ctx, actor, media.PurgeInput{Scope: scope, Items: []media.LibraryItemRevision{{ID: assetRow.ID, Revision: 2}}, ExpectedRevision: 2, PermanentDeleteConfirmed: true, Key: uuid.New()})
	if err != nil {
		t.Fatal("located purge admission", err)
	}
	purged, err := media.NewPurgeWorker(purge.Worker, mediaobjects.NewRoutedPurgeObjects(registry, pgmedia.NewStorageRoutes(runtime))).Execute(ctx, media.PurgeWorkID{JobID: purgeJob.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || purged.Status != "succeeded" {
		t.Fatal("located purge", purged, err)
	}
	if len(old.objects) != 1 || len(newer.objects) != 0 {
		t.Fatal("purge did not retain only source ZIP on original profile", len(old.objects), len(newer.objects))
	}
	var frozen []byte
	if err := owner.Raw(`SELECT frozen FROM media.purge_item WHERE job_id=?`, purgeJob.ID).Row().Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(frozen, []byte("stage-old")) {
		t.Fatal("frozen purge lost original location")
	}
}

type failPackageAfterArchive struct {
	media.PackageObjects
	media.LocatedPackageObjects
	calls int
}

func (p *failPackageAfterArchive) PutPackage(ctx context.Context, o media.PackageObject, r io.Reader) error {
	p.calls++
	if p.calls > 1 {
		return io.ErrUnexpectedEOF
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return p.LocatedPackageObjects.PutPackage(ctx, o, r)
}
func TestPackageLocatedCancellationChecksBytesAndRetiresOnlyOwnUnpublishedSet(t *testing.T) {
	owner, runtime := packageLocationDB(t)
	old, oldServer := newCOSStageFixture(t)
	newer, newServer := newCOSStageFixture(t)
	cfg := stageConfiguration(t, oldServer.URL, newServer.URL)
	suffix := uuid.NewString()[:8]
	oldID, newID := "stage-old-"+suffix, "stage-new-"+suffix
	cfg.Profiles[oldID], cfg.Profiles[newID] = cfg.Profiles["stage-old"], cfg.Profiles["stage-new"]
	delete(cfg.Profiles, "stage-old")
	delete(cfg.Profiles, "stage-new")
	cfg.DefaultWriteProfile = oldID
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	routes := pgmedia.NewStorageRoutes(owner)
	profiles := []media.StorageProfile{}
	for id, p := range cfg.Profiles {
		profiles = append(profiles, media.StorageProfile{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle})
	}
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	prior, _ := routes.ReadActive(t.Context())
	rev := "fixture-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), prior.Revision, rev, oldID); err != nil {
		t.Fatal(err)
	}
	actor, _ := mediaStoreProject(t, owner)
	repo := mediaPackageStore(runtime, libraryTestAccess, transferTestGuards, time.Now).WithObjectLocations()
	store := mediaTestStore(runtime)
	uploads := media.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, nil, time.Now)
	objects := mediaobjects.NewRoutedPackageObjects(registry, pgmedia.NewStorageRoutes(runtime))
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}

	data := packageFilesZIP(t, []packageFixtureFile{{"image", "cancel.png", "image/png", uploadPNG(t)}})
	failure := &failPackageAfterArchive{PackageObjects: objects, LocatedPackageObjects: objects}
	service := media.NewLibraryPackageService(repo, uploads, failure, time.Now)
	in := media.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	job, err := service.Import(t.Context(), actor, in, packageDownloaded(t, data))
	if err != nil || job.Status != "needs_reconciliation" || len(old.objects) != 1 {
		t.Fatal("failed write was published or forgotten", job, err)
	}
	var work struct{ ID uuid.UUID }
	if err := owner.Raw(`SELECT id FROM media.storage_work_intent WHERE kind='package' AND work_id=?`, job.ID).Scan(&work).Error; err != nil {
		t.Fatal(err)
	}
	old.mu.Lock()
	var key string
	var original cosStageEntry
	for k, e := range old.objects {
		key, original = k, e
	}
	bad := original
	bad.body = []byte("different archive bytes")
	old.objects[key] = bad
	old.mu.Unlock()
	cancelled, err := service.Cancel(t.Context(), actor, job.ID, media.PackageControl{ExpectedRevision: job.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || cancelled.Status != "needs_reconciliation" || len(old.objects) != 1 {
		t.Fatal("mismatched bytes were deleted", cancelled, err)
	}
	old.mu.Lock()
	old.objects[key] = original
	old.mu.Unlock()
	// Isolated fixture starts the cancelled state eleven minutes in the past.
	// No production clock, immutable receipt or real deployment is altered.
	if err := owner.Exec(`UPDATE media.storage_work_intent SET state='cancelled',closed_at=clock_timestamp()-interval '11 minutes' WHERE id=? AND state='open'`, work.ID).Error; err != nil {
		t.Fatal(err)
	}
	// The real bounded SDK fence expires naturally; never regress its durable deadline.
	timer := time.NewTimer(600 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	cancelled, err = service.Cancel(t.Context(), actor, job.ID, media.PackageControl{ExpectedRevision: job.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || cancelled.Status != "cancelled" || len(old.objects) != 0 || len(newer.objects) != 0 {
		t.Fatal("own unpublished cleanup remained pinned", cancelled, err)
	}
	var proof struct{ Count, Deleted int }
	if err := owner.Raw(`SELECT count(*) AS count,count(*) FILTER(WHERE l.state='deleted' AND p.status='removed') AS deleted FROM media.package_object p JOIN media.object_location l ON l.id=p.object_location_id WHERE p.job_id=?`, job.ID).Scan(&proof).Error; err != nil || proof.Count != 4 || proof.Deleted != 4 {
		t.Fatal("unknown removals claimed complete", proof, err)
	}
	var assets int
	if err := owner.Raw(`SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?`, actor.ID).Scan(&assets).Error; err != nil || assets != 0 {
		t.Fatal("cancelled package published assets", assets, err)
	}
}
