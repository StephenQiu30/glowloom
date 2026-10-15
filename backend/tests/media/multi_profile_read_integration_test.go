package media_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestMultiProfileStoredReadsRetainOriginalAndRenditionLocations(t *testing.T) {
	db := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	current := activeTestStorageRoute(t, routes)
	initial := "read-minio-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), current.Revision, initial, "minio-main"); err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, db)
	store := routingMediaStore(db)
	service := mediaapp.NewRoutedUploadService(store, store, pgmedia.NewUploadAdmission(store, uploadRoutingPolicy(), 24*time.Hour), routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, &persistedRoutedUpload{locations: routes}, time.Now)
	input := uploadInput(t)
	input.Actor = actor
	input.Request.ProjectID = project
	target, err := service.BeginUpload(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, input.Request.Key)
	if err != nil {
		t.Fatal(err)
	}
	input.Target = &target
	uploaded, err := service.Upload(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Activate(t.Context(), initial, "read-cos-"+uuid.NewString(), "cos-main"); err != nil {
		t.Fatal(err)
	}
	asset, err := store.FindAsset(t.Context(), actor, project, uploaded.Asset.ID)
	if err != nil || asset.ObjectLocation == nil || asset.ObjectLocation.Ref.ProfileID != "minio-main" || asset.ObjectLocation.Ref.Bucket != "storage-test" {
		t.Fatal("asset reader lost historical identity", err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, uploaded.Asset.ID)
	if err != nil || len(rends) != 2 {
		t.Fatal("rendition reader", err)
	}
	for _, rend := range rends {
		if rend.ObjectLocation == nil || rend.ObjectLocation.Ref.ProfileID != "minio-main" || rend.ObjectLocation.Ref.Key != rend.ObjectKey {
			t.Fatal("rendition reader lost historical identity")
		}
	}
	library := pgmedia.NewScopedLibraryStore(db, func(*gorm.DB) mediaapp.LibraryProjectAccess { return routingProjectAccess{} }, time.Now, func(*gorm.DB) mediaapp.ScopeAccess { return routingScopeStub{} })
	for _, read := range []func() (mediaapp.LibraryMediaFile, error){
		func() (mediaapp.LibraryMediaFile, error) {
			return library.FindLibraryMedia(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, asset.ID)
		},
		func() (mediaapp.LibraryMediaFile, error) {
			return library.ReadReferenceFactSource(t.Context(), actor, project, asset.ID, domain.KindImage)
		},
		func() (mediaapp.LibraryMediaFile, error) {
			return library.ReadFrozenReferenceFactSource(t.Context(), actor, project, asset.ID, domain.KindImage)
		},
	} {
		file, err := read()
		if err != nil || file.Asset.ObjectLocation == nil || file.Asset.ObjectLocation.Ref != asset.ObjectLocation.Ref || len(file.Renditions) != 2 {
			t.Fatal("library or frozen reader lost historical identity", err)
		}
		for _, rend := range file.Renditions {
			if rend.ObjectLocation == nil || rend.ObjectLocation.Ref.ProfileID != "minio-main" {
				t.Fatal("frozen rendition lost location")
			}
		}
	}
}

// Authorization is stubbed only at the external workspace boundary; route and
// source rows, acceptance, location joins and object facts use real PostgreSQL.
type routingProjectAccess struct{}

func (routingProjectAccess) Authorize(_ context.Context, actor identityapp.Principal, project uuid.UUID, _ bool) (mediaapp.LibraryProjectFacts, error) {
	return mediaapp.LibraryProjectFacts{ProjectID: project, OrgID: actor.OrgID, Revision: 1}, nil
}
func (routingProjectAccess) TouchContent(context.Context, identityapp.Principal, uuid.UUID, int64) (int64, error) {
	return 0, mediaapp.ErrUnavailable
}

type emptyLocationIntents struct{}

func (emptyLocationIntents) StorageUsageLocations(context.Context, identityapp.Principal, domain.LibraryScope) ([]domain.ObjectRef, error) {
	return nil, nil
}

func TestRoutedUsageSourceReadsSavedLocationsAfterSwitch(t *testing.T) {
	db := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	activeTestStorageRoute(t, routes)
	actor, project := mediaStoreProject(t, db)
	store := routingMediaStore(db)
	service := mediaapp.NewRoutedUploadService(store, store, pgmedia.NewUploadAdmission(store, uploadRoutingPolicy(), 24*time.Hour), routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, &persistedRoutedUpload{locations: routes}, time.Now)
	input := uploadInput(t)
	input.Actor = actor
	input.Request.ProjectID = project
	target, err := service.BeginUpload(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, input.Request.Key)
	if err != nil {
		t.Fatal(err)
	}
	input.Target = &target
	if _, err := service.Upload(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	library := pgmedia.NewScopedLibraryStore(db, func(*gorm.DB) mediaapp.LibraryProjectAccess { return routingProjectAccess{} }, time.Now, func(*gorm.DB) mediaapp.ScopeAccess { return routingScopeStub{} })
	buckets := []mediaapp.StorageUsageBucket{{ProfileID: "minio-main", Bucket: "storage-test"}, {ProfileID: "cos-main", Bucket: "storage-test-123"}}
	source := pgmedia.NewRoutedStorageUsageStore(library, buckets, func(*gorm.DB) mediaapp.StorageUsageLocationSource { return emptyLocationIntents{} })
	called := false
	err = source.InspectStorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(in mediaapp.StorageUsageInventory) error {
		called = true
		if len(in.Locations) != 3 || len(in.PhysicalPrefixes) != 10 || len(in.ObjectKeys) != 0 || len(in.Prefixes) != 0 {
			t.Fatal("inventory omitted saved identities")
		}
		for _, ref := range in.Locations {
			if ref.ProfileID != target.ProfileID || ref.Bucket != target.Bucket {
				t.Fatal("usage consulted active route")
			}
		}
		return nil
	})
	if err != nil || !called {
		t.Fatal("read physical inventory", err)
	}
	// The legacy source must not erase the FK and silently count the original bucket.
	legacy := pgmedia.NewScopedStorageUsageStore(db, func(*gorm.DB) mediaapp.LibraryProjectAccess { return routingProjectAccess{} }, func(*gorm.DB) mediaapp.StorageUsageIntentSource { return usageIntentFixture{} }, func(*gorm.DB) mediaapp.ScopeAccess { return routingScopeStub{} })
	legacyCalled := false
	legacyErr := legacy.InspectStorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(mediaapp.StorageUsageInventory) error { legacyCalled = true; return nil })
	if !errors.Is(legacyErr, mediaapp.ErrUnavailable) || legacyCalled {
		t.Fatal("legacy usage source erased a saved location", legacyErr)
	}
	// A key-only row is unresolved migration debt, never a guessed current profile.
	legacyAsset := sourceDocumentAsset([]byte("unproved legacy"))
	legacyAsset.ObjectKey = "projects/" + project.String() + "/document/2026/10/" + legacyAsset.ID.String() + ".txt"
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,codec,moderation_status,revision) VALUES(?,?,'document','upload','ready',?,?,?,?,?,'txt','passed',1)`, legacyAsset.ID, project, legacyAsset.ObjectKey, legacyAsset.FileName, legacyAsset.MimeType, legacyAsset.ByteSize, *legacyAsset.SHA256).Error; err != nil {
		t.Fatal(err)
	}
	called = false
	err = source.InspectStorageUsage(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(mediaapp.StorageUsageInventory) error { called = true; return nil })
	if !errors.Is(err, mediaapp.ErrStorageNotReady) || called {
		t.Fatal("unproven legacy row reached inventory consumer", err)
	}
}
