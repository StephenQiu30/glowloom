package media_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func storageRouteDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_ROUTE_DSN")
	if dsn == "" {
		t.Skip("set disposable GLOWLOOM_TEST_STORAGE_ROUTE_DSN; no deployment configuration is read")
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
		{ID: "cos-first", Provider: "cos", Endpoint: "https://storage-test-1250000000.cos.ap-shanghai.myqcloud.com", SigningEndpoint: "https://storage-test-1250000000.cos.ap-shanghai.myqcloud.com", Region: "ap-shanghai", Bucket: "storage-test-1250000000"},
		{ID: "cos-main", Provider: "cos", Endpoint: "https://storage-test-123.cos.ap-shanghai.myqcloud.com", SigningEndpoint: "https://storage-test-123.cos.ap-shanghai.myqcloud.com", Region: "ap-shanghai", Bucket: "storage-test-123"},
	}
	if err := store.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	return profiles
}

func TestStorageMigrationIsRepeatable(t *testing.T) {
	database := storageRouteDB(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "storage_ledger_expand_incremental.sql"))
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

func TestStorageEnvironmentRevisionAndLostReply(t *testing.T) {
	database := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = database.Rollback().Error })
	store := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, store)
	input := storageEnvironmentFixture(t, database, "environment-"+uuid.NewString(), "cos-first")
	first, err := store.ApplyEnvironment(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.ApplyEnvironment(t.Context(), input)
	if err != nil || first != replay {
		t.Fatal("lost configuration response changed identity", err)
	}
	conflicting := input
	conflicting.DefaultWriteProfile = "cos-main"
	if _, err := store.ApplyEnvironment(t.Context(), conflicting); !errors.Is(err, application.ErrStorageRouteConflict) {
		t.Fatal("same revision rebound", err)
	}
	next := storageEnvironmentFixture(t, database, "environment-"+uuid.NewString(), "cos-main")
	switched, err := store.ApplyEnvironment(t.Context(), next)
	if err != nil || switched.ProfileID != "cos-main" {
		t.Fatal("explicit environment route not applied", err)
	}
	current, err := store.ReadConfigured(t.Context())
	if err != nil || current != switched {
		t.Fatal("configured projection changed", err)
	}
}

func TestStorageLocationIdentityAndAlias(t *testing.T) {
	store := pgmedia.NewStorageRoutes(storageRouteDB(t))
	profiles := storageProfiles(t, store)
	alias := profiles[0]
	alias.ID = "cos-alias"
	if err := store.RegisterProfiles(t.Context(), []application.StorageProfile{alias}); !errors.Is(err, application.ErrStorageIdentityConflict) {
		t.Fatal("duplicate physical resource alias", err)
	}
	r := domain.ObjectRef{ProfileID: "cos-first", Bucket: "storage-test-1250000000", Key: "projects/same/image/key.png"}
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
