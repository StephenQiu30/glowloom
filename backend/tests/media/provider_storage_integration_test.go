package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type cosStageEntry struct {
	body      []byte
	mime, sha string
}
type cosStageFixture struct {
	mu             sync.Mutex
	objects        map[string]cosStageEntry
	failRemovalPut bool
}

type memoryProbeTracker struct{ intents []mediaapp.StorageProbeIntent }

func (*memoryProbeTracker) RegisterProfiles(context.Context, []mediaapp.StorageProfile) error {
	return nil
}

func (s *memoryProbeTracker) BeginStorageProbe(_ context.Context, in mediaapp.StorageProbeIntent) error {
	s.intents = append(s.intents, in)
	return nil
}
func (*memoryProbeTracker) CompleteStorageProbe(context.Context, uuid.UUID, string) error { return nil }

func newCOSStageFixture(t *testing.T) (*cosStageFixture, *httptest.Server) {
	t.Helper()
	f := &cosStageFixture{objects: make(map[string]cosStageEntry)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		entry, exists := f.objects[r.URL.Path]
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("Authorization") == "" || r.Header.Get("X-Cos-Forbid-Overwrite") != "true" {
				t.Error("SDK signature or overwrite fence missing")
			}
			if exists {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, "<Error><Code>ObjectAlreadyExists</Code></Error>")
				return
			}
			if f.failRemovalPut && strings.HasSuffix(r.URL.Path, ".remove") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.objects[r.URL.Path] = cosStageEntry{body: data, mime: r.Header.Get("Content-Type"), sha: r.Header.Get("X-Cos-Meta-Sha256")}
			w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum(data, crc64.MakeTable(crc64.ECMA))))
		case http.MethodHead, http.MethodGet:
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				}
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(entry.body)))
			w.Header().Set("Content-Type", entry.mime)
			w.Header().Set("X-Cos-Meta-Sha256", entry.sha)
			w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum(entry.body, crc64.MakeTable(crc64.ECMA))))
			if r.Method == http.MethodGet {
				_, _ = w.Write(entry.body)
			}
		case http.MethodDelete:
			delete(f.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return f, server
}

func stageConfiguration(t *testing.T, first, second string) config.ObjectStorageConfig {
	t.Helper()
	t.Setenv("STAGE_FIXTURE_AK", "fixture-access")
	t.Setenv("STAGE_FIXTURE_SK", "fixture-secret")
	file := filepath.Join(t.TempDir(), "storage.yaml")
	raw := fmt.Sprintf("object_storage:\n  schema_version: 1\n  revision: fixture-v1\n  default_write_profile: stage-old\n  profiles:\n    stage-old:\n      provider: cos\n      endpoint: %s\n      bucket: fixture-123\n      region: ap-shanghai\n      credentials:\n        source: env\n        access_key_env: STAGE_FIXTURE_AK\n        secret_key_env: STAGE_FIXTURE_SK\n    stage-new:\n      provider: cos\n      endpoint: %s\n      bucket: fixture-123\n      region: ap-shanghai\n      credentials:\n        source: env\n        access_key_env: STAGE_FIXTURE_AK\n        secret_key_env: STAGE_FIXTURE_SK\n", first, second)
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorage(file, "local")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestStorageOperatorProbeChecksActualSDKBytesAndRetainedReadOnlySentinel(t *testing.T) {
	_, first := newCOSStageFixture(t)
	_, second := newCOSStageFixture(t)
	cfg := stageConfiguration(t, first.URL, second.URL)
	receipt, err := storageoperator.Probe(t.Context(), cfg, "stage-old", "", &memoryProbeTracker{})
	if err != nil || len(receipt.EvidenceSHA256) != 64 {
		t.Fatal("native SDK probe failed", err)
	}
	profile := cfg.Profiles["stage-old"]
	profile.Mode = "read_only"
	cfg.Profiles["stage-old"] = profile
	cfg.DefaultWriteProfile = "stage-new"
	if _, err := storageoperator.Probe(t.Context(), cfg, "stage-old", "", &memoryProbeTracker{}); err == nil {
		t.Fatal("read-only probe without resource proof accepted")
	}
	ro, err := storageoperator.Probe(t.Context(), cfg, "stage-old", receipt.SentinelKey, &memoryProbeTracker{})
	if err != nil || ro.ResourceSHA256 != receipt.ResourceSHA256 {
		t.Fatal("retained historical resource unavailable", err)
	}
}

type routedProviderRecords struct {
	receipt operationapp.ProviderReceipt
	found   bool
}

type boundedStageWork struct {
	*pgmedia.StorageRoutes
	t *testing.T
}

type agedProbeTracker struct {
	*pgmedia.StorageRoutes
	db *gorm.DB
}

func (s agedProbeTracker) BeginStorageProbe(ctx context.Context, in mediaapp.StorageProbeIntent) error {
	// Simulate a failed diagnostic whose original execution deadline has drained.
	return s.db.WithContext(ctx).Exec(`INSERT INTO media.storage_probe_intent(id,profile_id,bucket,config_sha256,sentinel_key,removal_key,mode,created_at) VALUES(?,?,?,?,?,?,?,clock_timestamp()-interval '11 minutes')`, in.ID, in.ProfileID, in.Bucket, in.ConfigSHA256, in.SentinelKey, in.RemovalKey, in.Mode).Error
}

func (s boundedStageWork) ReserveWorkLocation(ctx context.Context, id uuid.UUID, key string) (domain.ObjectLocation, error) {
	deadline, bounded := ctx.Deadline()
	if !bounded || time.Until(deadline) > 5*time.Minute {
		s.t.Error("write timeout started after reservation")
	}
	return s.StorageRoutes.ReserveWorkLocation(ctx, id, key)
}

func (r *routedProviderRecords) CompleteProviderCall(_ context.Context, in operationapp.CompleteProviderCallInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	r.receipt = *in.Receipt
	r.found = true
	return nil
}
func (r *routedProviderRecords) ProviderReceipt(context.Context, operationapp.ProviderDispatchIdentity) (operationapp.ProviderReceipt, bool, error) {
	return r.receipt, r.found, nil
}

func TestProviderStagingOfficialSDKAndPersistentIntentKeepAdmittedTarget(t *testing.T) {
	database := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = database.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(database)
	oldFixture, first := newCOSStageFixture(t)
	newFixture, second := newCOSStageFixture(t)
	cfg := stageConfiguration(t, first.URL, second.URL)
	public, err := storageoperator.Configuration(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	// Unique IDs avoid rebinding fixtures that ran against this disposable DB.
	suffix := uuid.NewString()[:8]
	for i := range public.Profiles {
		public.Profiles[i].ID += "-" + suffix
	}
	public.DefaultWriteProfile += "-" + suffix
	profiles := cfg.Profiles
	cfg.Profiles = make(map[string]config.StorageProfile)
	for id, p := range profiles {
		cfg.Profiles[id+"-"+suffix] = p
	}
	cfg.DefaultWriteProfile = public.DefaultWriteProfile
	for _, p := range public.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{p.StorageProfile}); err != nil {
			t.Fatal(err)
		}
	}
	current, _ := routes.ReadActive(t.Context())
	oldRevision := "provider-old-" + suffix
	if _, err := routes.Activate(t.Context(), current.Revision, oldRevision, cfg.DefaultWriteProfile); err != nil {
		t.Fatal(err)
	}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	factory := staging.NewRoutedFactory(registry, boundedStageWork{StorageRoutes: routes, t: t})
	id := operationapp.ProviderDispatchIdentity{ProjectID: uuid.New(), OperationID: uuid.New(), Action: "submit", Attempt: 1, RequestKey: "fixture/request", ModelProfileVersionID: uuid.New(), PriceRuleVersionID: uuid.New()}
	if err := factory.PinOperationStorage(t.Context(), id.ProjectID, id.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Activate(t.Context(), oldRevision, "provider-next-"+suffix, "stage-new-"+suffix); err != nil {
		t.Fatal(err)
	}
	data := []byte("native SDK staged original")
	sum := sha256.Sum256(data)
	output := operationapp.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: "image/png", SHA256: hex.EncodeToString(sum[:])}
	records := &routedProviderRecords{}
	service := operationapp.NewRoutedProviderStageService(factory, records)
	receipt, err := service.StageImage(t.Context(), id, operationapp.ProviderImageEvidence{ThreadID: "thread", TurnID: "turn", ItemID: "item", Model: "fixture"}, output, data)
	if err != nil {
		t.Fatal(err)
	}
	restarted := operationapp.NewRoutedProviderStageService(staging.NewRoutedFactory(registry, pgmedia.NewStorageRoutes(database)), records)
	recovered, err := restarted.RecoverImage(t.Context(), id)
	if err != nil || recovered == nil || recovered.ManifestSHA256 != receipt.ManifestSHA256 {
		t.Fatal("restart lost original target", err)
	}
	_, actual, err := restarted.ReadImage(t.Context(), receipt)
	if err != nil || !bytes.Equal(data, actual) {
		t.Fatal("readback changed target", err)
	}
	oldFixture.mu.Lock()
	oldCount := len(oldFixture.objects)
	oldFixture.mu.Unlock()
	newFixture.mu.Lock()
	newCount := len(newFixture.objects)
	newFixture.mu.Unlock()
	if oldCount != 2 || newCount != 0 {
		t.Fatalf("old/new writes=%d/%d", oldCount, newCount)
	}
	// A bounded writer publishes its deadline before I/O. Cancellation cannot
	// clear debt while that deadline is live, and a paused writer cannot restart.
	work, err := routes.PinStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: "export", OwnerID: uuid.New(), WorkID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveWorkLocation(t.Context(), work.ID, "private/fixture-exports/"+work.ID.String()+"/result.bin")
	if err != nil {
		t.Fatal(err)
	}
	writeCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := routes.BeginWorkWrite(writeCtx, work.ID, location); err != nil {
		t.Fatal(err)
	}
	ref := objectstorage.Ref{ProfileID: location.Ref.ProfileID, Bucket: location.Ref.Bucket, Key: location.Ref.Key}
	if err := registry.PutIfAbsent(writeCtx, ref, bytes.NewReader(data), int64(len(data)), "application/octet-stream", output.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := routes.ConfirmWorkLocation(writeCtx, work.ID, location, mediaapp.ObjectInfo{Size: int64(len(data)), ContentType: "application/octet-stream", SHA256: output.SHA256}); err != nil {
		t.Fatal(err)
	}
	// Only this disposable fixture ages the first cancellation transition.
	if err := database.Exec(`UPDATE media.storage_work_intent SET state='cancelled',closed_at=clock_timestamp()-interval '11 minutes' WHERE id=?`, work.ID).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := routes.BeginWorkRemoval(t.Context(), work.ID, location); err != nil || allowed {
		t.Fatal("live writer deadline ignored", err)
	}
	<-writeCtx.Done()
	removed, err := mediaapp.NewStorageCleanup(routes, mediaobjects.NewRoutedReadObjects(registry)).ReconcileBatch(t.Context(), 100)
	if err != nil || removed != 1 {
		t.Fatal("cancelled work debt not reconciled", err)
	}
	saved, err := routes.ReadLocation(t.Context(), location.ID)
	if err != nil || saved.State != "deleted" {
		t.Fatal("cleanup did not record confirmed absence", err)
	}
	if err := routes.BeginWorkWrite(writeCtx, work.ID, location); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("paused writer restarted after cleanup", err)
	}
	oldFixture.mu.Lock()
	oldFixture.failRemovalPut = true
	oldFixture.mu.Unlock()
	failed, err := storageoperator.Probe(t.Context(), cfg, "stage-old-"+suffix, "", agedProbeTracker{StorageRoutes: routes, db: database})
	if err == nil || failed.ProbeID == uuid.Nil || failed.SentinelKey == "" || failed.EvidenceSHA256 != "" {
		t.Fatal("failed probe discarded exact ownership", err)
	}
	var recorded bool
	if err := database.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_probe_intent WHERE id=? AND sentinel_key=? AND removal_key=? AND state='running')`, failed.ProbeID, failed.SentinelKey, failed.SentinelKey+".remove").Scan(&recorded).Error; err != nil || !recorded {
		t.Fatal("probe intent not durable before failure", err)
	}
	wrongFixture, wrongServer := newCOSStageFixture(t)
	wrongFixture.objects["/"+failed.SentinelKey] = cosStageEntry{body: data, mime: "application/octet-stream", sha: output.SHA256}
	wrong := cfg
	wrong.Profiles = make(map[string]config.StorageProfile, len(cfg.Profiles))
	for id, p := range cfg.Profiles {
		wrong.Profiles[id] = p
	}
	p := wrong.Profiles["stage-old-"+suffix]
	p.Endpoint = wrongServer.URL
	p.SigningEndpoint = wrongServer.URL
	wrong.Profiles["stage-old-"+suffix] = p
	if err := storageoperator.CleanupProbe(t.Context(), wrong, failed.ProbeID, routes); !errors.Is(err, mediaapp.ErrStorageIdentityConflict) {
		t.Fatal("same profile ID was rebound during cleanup", err)
	}
	wrongFixture.mu.Lock()
	retainedWrongTarget := len(wrongFixture.objects) == 1
	wrongFixture.mu.Unlock()
	if !retainedWrongTarget {
		t.Fatal("cleanup deleted another resource's same-named object")
	}
	if err := storageoperator.CleanupProbe(t.Context(), cfg, failed.ProbeID, routes); err != nil {
		t.Fatal("failed probe debt could not be recovered", err)
	}
	var cleaned bool
	if err := database.Raw(`SELECT state='cleaned' FROM media.storage_probe_intent WHERE id=?`, failed.ProbeID).Scan(&cleaned).Error; err != nil || !cleaned {
		t.Fatal("probe cleanup not recorded", err)
	}
	if err := storageoperator.CleanupProbe(t.Context(), cfg, failed.ProbeID, routes); err != nil {
		t.Fatal("lost cleanup response was not recoverable", err)
	}
}
