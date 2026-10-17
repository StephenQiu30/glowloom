package media_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func storageRouteDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_STORAGE_ROUTE_DSN")
	if dsn == "" {
		t.Skip("set disposable LV_TEST_STORAGE_ROUTE_DSN; no deployment configuration is read")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB
}

func storageProfiles(t *testing.T, store *pgmedia.StorageRoutes) []application.StorageProfile {
	t.Helper()
	profiles := []application.StorageProfile{
		{ID: "minio-main", Provider: "minio", Endpoint: "https://minio.example.test", SigningEndpoint: "https://minio.example.test", Region: "us-east-1", Bucket: "storage-test", AddressingStyle: "path"},
		{ID: "cos-main", Provider: "cos", Endpoint: "https://storage-test-123.cos.ap-shanghai.myqcloud.com", SigningEndpoint: "https://storage-test-123.cos.ap-shanghai.myqcloud.com", Region: "ap-shanghai", Bucket: "storage-test-123"},
	}
	if err := store.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	return profiles
}

func TestStorageMigrationIsRepeatable(t *testing.T) {
	database := storageRouteDB(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "008_object_storage_profiles.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := database.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := pgmedia.NewStorageRoutes(database)
	profiles := storageProfiles(t, store)
	if err := store.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	profiles[0].Endpoint = "https://replaced.example.test"
	if err := store.RegisterProfiles(t.Context(), profiles); !errors.Is(err, application.ErrStorageIdentityConflict) {
		t.Fatalf("profile identity changed: %v", err)
	}
}

func TestStorageRouteActivationCASAndLostReply(t *testing.T) {
	database := storageRouteDB(t)
	store := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, store)
	if err := database.Exec(`TRUNCATE media.storage_route,media.storage_activation,media.storage_activation_guard`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadActive(t.Context()); !errors.Is(err, application.ErrStorageNotReady) {
		t.Fatal("uninitialized route acquired implicit default", err)
	}
	if _, err := store.Activate(t.Context(), "", "initial-v1", "minio-main"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, rev := range []string{"switch-a", "switch-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Activate(t.Context(), "initial-v1", rev, "cos-main")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, application.ErrStorageRouteConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS = %d success, %d conflict", success, conflict)
	}
	route, err := store.ReadActive(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(t.Context(), "initial-v1", route.Revision, route.ProfileID); err != nil {
		t.Fatal("lost activation reply was not idempotent", err)
	}
	if _, err := store.Activate(t.Context(), route.Revision, "initial-v1", "minio-main"); !errors.Is(err, application.ErrStorageRouteConflict) {
		t.Fatal("reused historical revision", err)
	}
}

func TestStorageLocationIdentityAndAlias(t *testing.T) {
	store := pgmedia.NewStorageRoutes(storageRouteDB(t))
	profiles := storageProfiles(t, store)
	alias := profiles[0]
	alias.ID = "minio-alias"
	if err := store.RegisterProfiles(t.Context(), []application.StorageProfile{alias}); !errors.Is(err, application.ErrStorageIdentityConflict) {
		t.Fatal("duplicate physical resource alias", err)
	}
	r := domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: "projects/same/image/key.png"}
	a, err := store.ReserveLocation(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.ReserveLocation(t.Context(), r)
	if err != nil || a.ID != again.ID {
		t.Fatal("non-idempotent reserve", err)
	}
	r.ProfileID = "cos-main"
	r.Bucket = "storage-test-123"
	b, err := store.ReserveLocation(t.Context(), r)
	if err != nil || a.ID == b.ID {
		t.Fatal("cross-profile key collision", err)
	}
	read, err := store.ReadLocation(t.Context(), a.ID)
	if err != nil || read.Ref != a.Ref || read.State != "reserved" {
		t.Fatal("location changed", err)
	}
	r.Bucket = "wrong-bucket"
	if _, err := store.ReserveLocation(t.Context(), r); err == nil {
		t.Fatal("profile bucket mismatch accepted")
	}
}

func TestStorageMigrationRejectsCOSPhysicalAlias(t *testing.T) {
	store := pgmedia.NewStorageRoutes(storageRouteDB(t))
	profiles := storageProfiles(t, store)
	wrong := profiles[1]
	wrong.ID = "cos-wrong-bucket"
	wrong.Bucket = "another-bucket-123"
	if err := store.RegisterProfiles(t.Context(), []application.StorageProfile{wrong}); !errors.Is(err, application.ErrStorageIdentityConflict) {
		t.Fatal("COS BucketURL was rebound by a declaration", err)
	}
	alias := profiles[1]
	alias.ID = "cos-domain-alias"
	alias.Endpoint = strings.Replace(alias.Endpoint, "myqcloud.com", "tencentcos.cn", 1)
	alias.SigningEndpoint = alias.Endpoint
	if err := store.RegisterProfiles(t.Context(), []application.StorageProfile{alias}); !errors.Is(err, application.ErrStorageIdentityConflict) {
		t.Fatal("native domain aliases acquired separate cleanup identities", err)
	}
}
