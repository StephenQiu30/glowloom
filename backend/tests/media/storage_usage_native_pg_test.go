package media_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type usageSDKInventory struct {
	mu                       sync.Mutex
	omit, failList, failStat string
	calls                    int
}

func usageSDKFixture(t *testing.T) (*cosStageFixture, *httptest.Server, *usageSDKInventory) {
	t.Helper()
	data, server := newCOSStageFixture(t)
	state := &usageSDKInventory{}
	original := server.Config.Handler
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		state.calls++
		omit, failList, failStat := state.omit, state.failList, state.failStat
		state.mu.Unlock()
		if r.Method == http.MethodHead && strings.TrimPrefix(r.URL.Path, "/") == failStat && failStat != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/" && r.URL.Query().Get("prefix") != "" {
			prefix := r.URL.Query().Get("prefix")
			if failList != "" && strings.HasPrefix(prefix, failList) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			type content struct {
				Key  string `xml:"Key"`
				Size int64  `xml:"Size"`
			}
			result := struct {
				XMLName   xml.Name  `xml:"ListBucketResult"`
				Name      string    `xml:"Name"`
				Prefix    string    `xml:"Prefix"`
				Truncated bool      `xml:"IsTruncated"`
				Contents  []content `xml:"Contents"`
			}{Name: "fixture-123", Prefix: prefix}
			data.mu.Lock()
			for key, item := range data.objects {
				key = strings.TrimPrefix(key, "/")
				if strings.HasPrefix(key, prefix) && key != omit {
					result.Contents = append(result.Contents, content{key, int64(len(item.body))})
				}
			}
			data.mu.Unlock()
			sort.Slice(result.Contents, func(i, j int) bool { return result.Contents[i].Key < result.Contents[j].Key })
			w.Header().Set("Content-Type", "application/xml")
			if err := xml.NewEncoder(w).Encode(result); err != nil {
				t.Error(err)
			}
			return
		}
		original.ServeHTTP(w, r)
	})
	return data, server, state
}
func TestNativeStorageUsagePGSDKIncludesHistoricalProfilesWholeScopeAndPackageArchive(t *testing.T) {
	owner, runtime := packageLocationDB(t)
	_, firstServer, firstInventory := usageSDKFixture(t)
	_, secondServer, _ := usageSDKFixture(t)
	cfg := stageConfiguration(t, firstServer.URL, secondServer.URL)
	suffix := uuid.NewString()[:8]
	oldID, newID := "usage-old-"+suffix, "usage-new-"+suffix
	cfg.Profiles[oldID], cfg.Profiles[newID] = cfg.Profiles["stage-old"], cfg.Profiles["stage-new"]
	delete(cfg.Profiles, "stage-old")
	delete(cfg.Profiles, "stage-new")
	cfg.DefaultWriteProfile = newID
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	routes := pgmedia.NewStorageRoutes(owner)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle}}); err != nil {
			t.Fatal(err)
		}
	}
	prior, _ := routes.ReadActive(t.Context())
	if _, err := routes.Activate(t.Context(), prior.Revision, "usage-"+uuid.NewString(), newID); err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, owner)
	foreign, _ := mediaStoreProject(t, owner)
	base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
	expected := int64(0)
	expectedCount := 0
	put := func(profile, key string, data []byte, ledger bool, state string) domain.ObjectLocation {
		t.Helper()
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		p := cfg.Profiles[profile]
		if err := registry.PutIfAbsent(t.Context(), objectstorage.Ref{ProfileID: profile, Bucket: p.Bucket, Key: key}, bytes.NewReader(data), int64(len(data)), "application/octet-stream", sum); err != nil {
			t.Fatal(err)
		}
		var l domain.ObjectLocation
		if ledger {
			var err error
			l, err = routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: profile, Bucket: p.Bucket, Key: key})
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Exec(`UPDATE media.object_location SET state=?,byte_size=?,sha256=?,mime_type='application/octet-stream' WHERE id=?`, state, len(data), sum, l.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		return l
	}
	own := func(profile, key string, data []byte, ledger bool, state string) domain.ObjectLocation {
		expected += int64(len(data))
		expectedCount++
		return put(profile, key, data, ledger, state)
	}
	own(oldID, base+"audio/same.bin", []byte("old copy"), true, "accepted")
	own(newID, base+"audio/same.bin", []byte("new copy"), true, "accepted")
	unknown := base + "transcriptions/unknown.json"
	own(oldID, unknown, []byte("unknown write really exists"), true, "unknown")
	firstInventory.mu.Lock()
	firstInventory.omit = unknown
	firstInventory.mu.Unlock()
	own(newID, base+"script/history/orphan.json", []byte("unregistered script bytes"), false, "")
	own(oldID, base+"image/unregistered.bin", []byte("unregistered image bytes"), true, "accepted")
	put(oldID, "personal/"+foreign.OrgID.String()+"/"+foreign.ID.String()+"/script/private.bin", []byte("foreign secret"), true, "present")
	retired, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: oldID, Bucket: cfg.Profiles[oldID].Bucket, Key: base + "script/deleted.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.object_location SET state='deleted' WHERE id=?`, retired.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := mediaTestStore(runtime)
	uploads := media.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, nil, time.Now)
	packageObjects := mediaobjects.NewRoutedPackageObjects(registry, pgmedia.NewStorageRoutes(runtime))
	packageStore := mediaPackageStore(runtime, libraryTestAccess, transferTestGuards, time.Now).WithObjectLocations()
	packages := media.NewLibraryPackageService(packageStore, uploads, packageObjects, time.Now)
	job, err := packages.Import(t.Context(), actor, media.PackageImportRequest{Scope: domain.LibraryScope{Kind: domain.LibraryPersonal}, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, packageDownloaded(t, packageFilesZIP(t, []packageFixtureFile{{"image", "usage.png", "image/png", uploadPNG(t)}})))
	if err != nil || job.Status != "succeeded" {
		t.Fatal(job, err)
	}
	var packageBytes int64
	if err := owner.Raw(`SELECT sum(byte_size) FROM media.package_object WHERE job_id=?`, job.ID).Scan(&packageBytes).Error; err != nil {
		t.Fatal(err)
	}
	expected += packageBytes
	expectedCount += 4
	// Retained catalog trash still occupies bytes until real physical removal.
	var catalogAsset struct{ ID uuid.UUID }
	if err := owner.Raw(`SELECT id FROM media.media_asset WHERE personal_actor_id=?`, actor.ID).Scan(&catalogAsset).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mediaLibraryStore(runtime, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, media.LibraryCommand{Scope: domain.LibraryScope{Kind: domain.LibraryPersonal}, Key: uuid.New(), Action: "recycle_items", ExpectedRevision: 1, Items: []media.LibraryItemRevision{{ID: catalogAsset.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	old := cfg.Profiles[oldID]
	old.Mode = "read_only"
	cfg.Profiles[oldID] = old
	readRegistry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readRegistry.Close)
	reader := mediawire.ProvideRoutedMediaStorageUsage(runtime, readRegistry, &cfg)
	got, err := reader.StorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal})
	if err != nil || got.UsedBytes != expected || got.ObjectCount != expectedCount || got.LimitBytes != nil {
		t.Fatal("whole physical inventory", got, expected, expectedCount, err)
	}
	// Unknown producer output omitted from LIST still requires its exact saved STAT.
	firstInventory.mu.Lock()
	firstInventory.failStat = unknown
	firstInventory.mu.Unlock()
	failed, err := reader.StorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal})
	if !errors.Is(err, media.ErrUnavailable) || failed.UsedBytes != 0 || failed.ObjectCount != 0 {
		t.Fatal("unknown stat failure became absence/subtotal", failed, err)
	}
	firstInventory.mu.Lock()
	firstInventory.failStat = ""
	firstInventory.failList = base
	firstInventory.mu.Unlock()
	if result, err := reader.StorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}); !errors.Is(err, media.ErrUnavailable) || result.UsedBytes != 0 {
		t.Fatal("old profile list failure ignored", result, err)
	}
	firstInventory.mu.Lock()
	firstInventory.failList = ""
	firstInventory.mu.Unlock()
	// Project namespaces include transcriptions and script, not only five media kinds.
	projectBase := "projects/" + project.String() + "/"
	put(oldID, projectBase+"transcriptions/final.json", []byte("project transcript"), true, "unknown")
	put(newID, projectBase+"script/orphan.json", []byte("project script"), false, "")
	got, err = reader.StorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project})
	if err != nil || got.UsedBytes != int64(len("project transcript")+len("project script")) || got.ObjectCount != 2 {
		t.Fatal("whole project namespace", got, err)
	}
	firstInventory.mu.Lock()
	before := firstInventory.calls
	firstInventory.mu.Unlock()
	if _, err := reader.StorageUsage(t.Context(), foreign, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}); err == nil {
		t.Fatal("foreign project usage disclosed")
	}
	firstInventory.mu.Lock()
	after := firstInventory.calls
	firstInventory.mu.Unlock()
	if after != before {
		t.Fatal("unauthorized request reached private SDK")
	}
	// A configured profile cannot rebind the saved endpoint identity even if its
	// name and bucket are unchanged. Reject before querying the new resource.
	_, driftServer, driftInventory := usageSDKFixture(t)
	driftCfg := cfg
	driftCfg.Profiles = make(map[string]config.StorageProfile, len(cfg.Profiles))
	for id, p := range cfg.Profiles {
		driftCfg.Profiles[id] = p
	}
	driftProfile := driftCfg.Profiles[oldID]
	driftProfile.Endpoint, driftProfile.SigningEndpoint = driftServer.URL, driftServer.URL
	driftCfg.Profiles[oldID] = driftProfile
	driftRegistry, err := objectstorage.OpenProfiles(driftCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(driftRegistry.Close)
	if got, err := mediawire.ProvideRoutedMediaStorageUsage(runtime, driftRegistry, &driftCfg).StorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}); !errors.Is(err, media.ErrStorageNotReady) || got.UsedBytes != 0 {
		t.Fatal("rebound profile was queried as original resource", got, err)
	}
	driftInventory.mu.Lock()
	calls := driftInventory.calls
	driftInventory.mu.Unlock()
	if calls != 0 {
		t.Fatal("rebound profile received private inventory I/O")
	}
}

func TestNativeStorageUsageRetiredProfileDoesNotBlockAuthorizedEmptyScope(t *testing.T) {
	owner, runtime := packageLocationDB(t)
	_, oldServer, _ := usageSDKFixture(t)
	_, newServer, _ := usageSDKFixture(t)
	cfg := stageConfiguration(t, oldServer.URL, newServer.URL)
	suffix := uuid.NewString()[:8]
	oldID, newID := "usage-retired-"+suffix, "usage-active-"+suffix
	cfg.Profiles[oldID], cfg.Profiles[newID] = cfg.Profiles["stage-old"], cfg.Profiles["stage-new"]
	delete(cfg.Profiles, "stage-old")
	delete(cfg.Profiles, "stage-new")
	cfg.DefaultWriteProfile = newID
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	routes := pgmedia.NewStorageRoutes(owner)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle}}); err != nil {
			t.Fatal(err)
		}
	}
	actor, _ := mediaStoreProject(t, owner)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	asset := uuid.New()
	key := fmt.Sprintf("personal/%s/%s/document/2026/10/%s.txt", actor.OrgID, actor.ID, asset)
	body := []byte("synthetic private text")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	ref := objectstorage.Ref{ProfileID: oldID, Bucket: cfg.Profiles[oldID].Bucket, Key: key}
	if err := registry.PutIfAbsent(t.Context(), ref, bytes.NewReader(body), int64(len(body)), "text/plain", digest); err != nil {
		t.Fatal(err)
	}
	l, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: ref.ProfileID, Bucket: ref.Bucket, Key: ref.Key})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.object_location SET state='accepted',byte_size=?,sha256=?,mime_type='text/plain' WHERE id=?`, len(body), digest, l.ID).Error; err != nil {
		t.Fatal(err)
	}
	// A synthetic system-owned document fixture does not claim an upload or AIGC receipt.
	if err := owner.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,codec,moderation_status) VALUES(?,?,?,'document','system','ready',?,?,'fixture.txt','text/plain',?,?,'txt','passed')`, asset, actor.OrgID, actor.ID, key, l.ID, len(body), digest).Error; err != nil {
		t.Fatal(err)
	}
	library := mediaLibraryStore(runtime, libraryTestAccess, time.Now)
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, media.LibraryCommand{Scope: scope, Action: "update_item", Key: uuid.New(), ItemID: &asset, Metadata: &media.LibraryMetadata{Title: "Synthetic text", Category: "material", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, media.LibraryCommand{Scope: scope, Action: "recycle_items", Key: uuid.New(), ExpectedRevision: 1, Items: []media.LibraryItemRevision{{ID: asset, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	purge := mediaPurgeStore(runtime, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := purge.CreatePurge(t.Context(), actor, media.PurgeInput{Scope: scope, ExpectedRevision: 2, Items: []media.LibraryItemRevision{{ID: asset, Revision: 2}}, PermanentDeleteConfirmed: true, Key: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := media.NewPurgeWorker(purge.Worker, mediaobjects.NewRoutedPurgeObjects(registry, pgmedia.NewStorageRoutes(runtime))).Execute(t.Context(), media.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || result.Status != "succeeded" {
		t.Fatal("real owning purge did not finish", result, err)
	}
	if _, err := registry.Stat(t.Context(), ref); !errors.Is(err, objectstorage.ErrObjectMissing) {
		t.Fatal("retired bytes still exist", err)
	}
	old := cfg.Profiles[oldID]
	old.Mode = "disabled"
	cfg.Profiles[oldID] = old
	readRegistry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readRegistry.Close)
	usage, err := mediawire.ProvideRoutedMediaStorageUsage(runtime, readRegistry, &cfg).StorageUsage(t.Context(), actor, scope)
	if err != nil || usage.UsedBytes != 0 || usage.ObjectCount != 0 {
		t.Fatal("retired physical proof demanded disabled historical access", usage, err)
	}
}
