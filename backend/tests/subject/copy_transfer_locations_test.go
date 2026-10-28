package subject_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Every test owns a new database. Canonical schema is never replayed into the
// selected shared or business database, and no configured cloud is contacted.
func copyStorageDatabase(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	_, admin := subjectPostgres(t)
	u, err := url.Parse(os.Getenv("GLOWLOOM_TEST_SUBJECT_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "glowloom_subject_alignment_cp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE DATABASE ` + name).Error; err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	open := func(dsn string) *gorm.DB {
		c, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB
	}
	owner := open(u.String())
	_, file, _, _ := runtime.Caller(0)
	schema, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(string(schema)).Error; err != nil {
		t.Fatal("only own empty copy fixture", err)
	}
	q := u.Query()
	q.Set("options", "-c role=glowloom_app")
	u.RawQuery = q.Encode()
	runtimeDB := open(u.String())
	t.Log("owned copy fixture database", name)
	return runtimeDB, owner
}

type locatedCopyFixture struct {
	runtime, owner               *gorm.DB
	actor                        identity.Principal
	source, target, asset        uuid.UUID
	sourceProfile, targetProfile string
	registry                     *objectstorage.Registry
	data                         []byte
	mu                           sync.Mutex
	objects                      map[string][]byte
	puts                         map[string]int
}

func newLocatedCopy(t *testing.T) *locatedCopyFixture {
	t.Helper()
	db, owner := copyStorageDatabase(t)
	f := &locatedCopyFixture{runtime: db, owner: owner, objects: map[string][]byte{}, puts: map[string]int{}}
	f.actor = subjectSession(t, owner)
	f.source = subjectProject(t, owner, f.actor)
	f.target = subjectProject(t, owner, f.actor)
	if err := owner.Exec(`UPDATE workspace.project SET status='copying' WHERE id=?`, f.target).Error; err != nil {
		t.Fatal(err)
	}
	legacy := subjectStorage(t)
	_, _, f.data = subjectStoredPNG(t, owner, f.actor, legacy)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		k := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Type", "image/png")
		switch r.Method {
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			if !bytes.Contains(raw, f.data) {
				http.Error(w, "wrong bytes", 400)
				return
			}
			if _, ok := f.objects[k]; ok {
				http.Error(w, `<Error><Code>PreconditionFailed</Code></Error>`, http.StatusPreconditionFailed)
				return
			}
			f.objects[k] = bytes.Clone(f.data)
			f.puts[k]++
		case http.MethodGet, http.MethodHead:
			body, ok := f.objects[k]
			if !ok {
				http.Error(w, `<Error><Code>NoSuchKey</Code></Error>`, 404)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.Header().Set("X-Amz-Meta-Sha256", hexHash(body))
			if r.Method == http.MethodGet {
				_, _ = w.Write(body)
			}
		case http.MethodDelete:
			delete(f.objects, k)
			w.WriteHeader(204)
		default:
			http.Error(w, "unsupported", 400)
		}
	}))
	t.Cleanup(server.Close)
	f.sourceProfile = "old-" + uuid.NewString()
	f.targetProfile = "new-" + uuid.NewString()
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("GLOWLOOM_ENV", "local")
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "")
	for k, v := range map[string]string{"PROVIDER": "minio", "ENDPOINT": server.URL, "BUCKET": "target-bucket", "REGION": "us-east-1", "ACCESS_KEY": "fixture-access", "SECRET_KEY": "fixture-secret", "PROFILE_ID": f.targetProfile, "REVISION": "copy-fixture-v1"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_"+k, v)
	}
	for _, k := range []string{"PROVIDER", "ENDPOINT", "BUCKET", "REGION", "ACCESS_KEY", "SECRET_KEY", "PROFILE_ID"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_LEGACY_"+k, "")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.ObjectStorage.Profiles[f.targetProfile]
	p.Mode = "read_only"
	p.Bucket = "source-bucket"
	cfg.ObjectStorage.Profiles[f.sourceProfile] = p
	f.registry, err = objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.registry.Close)
	profiles := []media.StorageProfile{}
	for id, p := range cfg.ObjectStorage.Profiles {
		profiles = append(profiles, media.StorageProfile{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.Endpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: p.AddressingStyle})
	}
	if err := pgmedia.NewStorageRoutes(owner).RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_route(singleton,profile_id,revision) VALUES(1,?,'copy-fixture-v1')`, f.targetProfile).Error; err != nil {
		t.Fatal(err)
	}
	f.asset = uuid.New()
	key := "projects/" + f.source.String() + "/image/2026/10/" + f.asset.String() + ".png"
	f.objects["source-bucket/"+key] = bytes.Clone(f.data)
	location := uuid.New()
	if err := owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,?,?,?,'accepted',?,?,'image/png')`, location, f.sourceProfile, "source-bucket", key, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,?,'fixture.png','image/png',?,?,'passed')`, f.asset, f.source, key, location, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func TestLocatedProjectCopyFreezesTargetAcceptsExactReceiptAndReplays(t *testing.T) {
	f := newLocatedCopy(t)
	rend := uuid.New()
	rendLocation := uuid.New()
	rendKey := "projects/" + f.source.String() + "/image/2026/10/" + f.asset.String() + "/thumb_256.png"
	if err := f.owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,?,?,?,'accepted',?,?,'image/png')`, rendLocation, f.sourceProfile, "source-bucket", rendKey, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.owner.Exec(`INSERT INTO media.rendition(id,media_asset_id,kind,object_key,object_location_id,byte_size) VALUES(?,?,'thumb_256',?,?,?)`, rend, f.asset, rendKey, rendLocation, len(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	f.objects["source-bucket/"+rendKey] = bytes.Clone(f.data)
	binding := media.ProjectCopyBinding{JobID: uuid.New(), OrgID: f.actor.OrgID, SourceProjectID: f.source, TargetProjectID: f.target}
	var snapshot media.ProjectCopySnapshot
	if err := f.runtime.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).WithRoutedStorage().Freeze(t.Context(), f.actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'located fixture','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, f.actor.OrgID, f.actor.ID, f.actor.ScopeID, f.source, f.target).Error
	}); err != nil {
		t.Fatal("freeze routed copy", err)
	}
	repo := &locatedCopyTransactions{ProjectCopyStore: pgmedia.NewScopedProjectCopyStore(f.runtime, app.NewMediaWorkerScopeAccess).WithRoutedStorage(), db: f.runtime}
	objects, err := repo.Objects(t.Context(), f.actor, binding, snapshot)
	if err != nil || len(objects) != 2 || objects[0].SourceLocation == nil || objects[0].TargetLocation == nil || objects[0].TargetLocation.Ref.ProfileID != f.targetProfile || objects[0].SourceLocation.Ref.ProfileID != f.sourceProfile {
		t.Fatal("frozen physical copy identities missing", err)
	}
	if err := f.owner.Exec(`UPDATE media.storage_route SET profile_id=?,revision='switched-v2' WHERE singleton=1`, f.sourceProfile).Error; err != nil {
		t.Fatal(err)
	}
	transfer := media.NewRoutedProjectCopyTransfer(repo, mediaobjects.NewRoutedCopyObjects(f.registry, pgmedia.NewStorageRoutes(f.runtime)), t.TempDir())
	for range 2 {
		if err := transfer.Transfer(t.Context(), f.actor, binding, snapshot); err != nil {
			t.Fatal("actual independent SDK copy/recovery", err)
		}
	}
	for range 2 {
		if _, err := repo.Register(t.Context(), f.actor, binding, snapshot); err != nil {
			t.Fatal("atomic media/location acceptance or replay", err)
		}
	}
	digest := hexHash(f.data)
	fact := media.ReferenceFact{AssetID: f.asset, Revision: 1, Kind: mediadomain.KindImage, SHA256: digest, ByteSize: int64(len(f.data)), RenditionID: &rend, RenditionSHA256: &digest}
	q := media.NewRoutedProjectCopyReferenceQuery(repo, mediaobjects.NewRoutedReadObjects(f.registry), nil)
	mappings, e := q.FreezeReferences(t.Context(), f.actor, binding, snapshot, []media.ReferenceFact{fact})
	if e != nil {
		t.Fatal("current exact source reference", e)
	}
	if e = q.VerifyTransferred(t.Context(), f.actor, binding, snapshot, mappings); e != nil {
		t.Fatal("copied Bible media original and independent thumb bytes", e)
	}
	var actual struct {
		LocationID       uuid.UUID
		State, ProfileID string
	}
	if err := f.owner.Raw(`SELECT a.object_location_id AS location_id,l.state,l.profile_id FROM media.media_asset a JOIN media.object_location l ON l.id=a.object_location_id WHERE a.id=?`, snapshot.AssetMapping[f.asset]).Scan(&actual).Error; err != nil || actual.LocationID != objects[0].TargetLocation.ID || actual.State != "accepted" || actual.ProfileID != f.targetProfile {
		t.Fatal("formal copy lost the originally admitted target", actual, err)
	}
	f.mu.Lock()
	puts := f.puts["target-bucket/"+objects[0].TargetObjectKey]
	source := bytes.Clone(f.objects["source-bucket/"+objects[0].SourceObjectKey])
	f.mu.Unlock()
	if puts != 1 || !bytes.Equal(source, f.data) {
		t.Fatal("replay repeated write or source mutated", puts)
	}
	var intentState string
	if err := f.owner.Raw(`SELECT state FROM media.storage_work_intent WHERE kind='project-copy' AND owner_id=? AND work_id=?`, f.target, snapshot.ID).Scan(&intentState).Error; err != nil || intentState != "accepted" {
		t.Fatal("whole-copy intent not accepted", err)
	}
}

// New native transfers independently accept successful items and clean a target
// that was never written, without changing the original library or its bytes.
func TestLocatedTransferPartialFailureRetainsSourceAndPermanentReplay(t *testing.T) {
	f := newLocatedCopy(t)
	missing := uuid.New()
	key := "projects/" + f.source.String() + "/image/2026/10/" + missing.String() + ".png"
	location := uuid.New()
	if err := f.owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,?,?,?,'accepted',?,?,'image/png')`, location, f.sourceProfile, "source-bucket", key, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,?,'missing fixture.png','image/png',?,?,'passed')`, missing, f.source, key, location, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	input := media.TransferInput{Source: mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &f.source}, Target: mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}, Items: []media.LibraryItemRevision{{ID: f.asset}, {ID: missing}}, ExpectedSourceRevision: 0, ExpectedTargetRevision: 0, ExpectedProjectRevision: 1, Key: uuid.New()}
	sourceLibrary, _ := input.Source.Identity(f.actor.OrgID, f.actor.ID)
	var sourceBefore int
	if err := f.owner.Raw(`SELECT count(*) FROM media.library WHERE id=?`, sourceLibrary).Scan(&sourceBefore).Error; err != nil || sourceBefore != 0 {
		t.Fatal("fixture must retain unmaterialized legacy library", err)
	}
	requests := mediawire.ProvideRoutedMediaTransferStore(f.runtime)
	job, err := requests.CreateTransfer(t.Context(), f.actor, input)
	if err != nil {
		t.Fatal("atomic target reservation", err)
	}
	foreign := subjectSession(t, f.owner)
	rejected := input
	rejected.Key = uuid.New()
	if _, e := requests.CreateTransfer(t.Context(), foreign, rejected); e == nil {
		t.Fatal("foreign project scope accepted")
	}
	if err = f.owner.Exec(`UPDATE media.storage_route SET profile_id=?,revision='changed-after-admission' WHERE singleton=1`, f.sourceProfile).Error; err != nil {
		t.Fatal(err)
	}
	targetLibrary, _ := input.Target.Identity(f.actor.OrgID, f.actor.ID)
	worker, _ := mediawire.ProvideMediaTransferWorkerWithObjects(f.runtime, f.registry, nil)
	work := media.TransferWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()}
	result, err := worker.Execute(t.Context(), work)
	if err != nil {
		t.Fatal("actual partial transfer", err)
	}
	if result.Status != "partial_failed" || len(result.Items) != 2 || result.Items[0].Status != "succeeded" || result.Items[1].Status != "failed" {
		t.Fatalf("wrong partial result %+v", result)
	}
	if _, err = worker.Execute(t.Context(), work); err != nil {
		t.Fatal("terminal recovery", err)
	}
	replay, err := requests.CreateTransfer(t.Context(), f.actor, input)
	if err != nil || replay.ID != job.ID {
		t.Fatal("same command lost original job", err)
	}
	var receipts []struct{ State, ProfileID string }
	if err = f.owner.Raw(`SELECT state,profile_id FROM media.storage_work_intent WHERE kind='transfer' AND owner_id=? ORDER BY state`, targetLibrary).Scan(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 2 || receipts[0].State != "accepted" || receipts[1].State != "cancelled" || receipts[0].ProfileID != f.targetProfile || receipts[1].ProfileID != f.targetProfile {
		t.Fatalf("per-item immutable target receipts %+v", receipts)
	}
	var sourceCount int
	if err = f.owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id IN (?,?) AND project_id=? AND NOT is_delete AND status='ready' AND revision=1 AND sha256=?`, f.asset, missing, f.source, hexHash(f.data)).Scan(&sourceCount).Error; err != nil || sourceCount != 2 {
		t.Fatal("source changed", err, sourceCount)
	}
	var targetProfile string
	if err = f.owner.Raw(`SELECT l.profile_id FROM media.media_asset a JOIN media.object_location l ON l.id=a.object_location_id WHERE a.id=? AND l.state='accepted'`, *result.Items[0].TargetAssetID).Scan(&targetProfile).Error; err != nil || targetProfile != f.targetProfile {
		t.Fatal("published target lost frozen location", err)
	}
	var deleted int
	if err = f.owner.Raw(`SELECT count(*) FROM media.object_location l JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE w.kind='transfer' AND w.owner_id=? AND w.state='cancelled' AND l.state='deleted'`, targetLibrary).Scan(&deleted).Error; err != nil || deleted != 1 {
		t.Fatal("never-written target lacks actual absence receipt", err, deleted)
	}
}
