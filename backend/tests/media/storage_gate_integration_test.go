package media_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageEnvironmentRequiresCompleteExplicitWritableDefault(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	input := storageEnvironmentFixture(t, db, "gate-"+uuid.NewString(), "cos-first")
	for _, mutate := range []func(*mediaapp.StorageEnvironment){func(i *mediaapp.StorageEnvironment) { i.ConfigSHA256 = "invalid" }, func(i *mediaapp.StorageEnvironment) { i.DefaultWriteProfile = "unconfigured" }, func(i *mediaapp.StorageEnvironment) { i.UploadSessionTTL = 0 }, func(i *mediaapp.StorageEnvironment) {
		for n := range i.Profiles {
			if i.Profiles[n].ID == i.DefaultWriteProfile {
				i.Profiles[n].Mode = mediaapp.StorageProfileModeReadOnly
			}
		}
	}} {
		bad := input
		bad.Profiles = append([]mediaapp.StorageDeploymentProfile(nil), input.Profiles...)
		mutate(&bad)
		if _, err := routes.ApplyEnvironment(t.Context(), bad); !errors.Is(err, mediaapp.ErrStorageNotReady) {
			t.Fatal("incomplete environment applied", err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), input); err != nil {
		t.Fatal("complete environment rejected", err)
	}
}

func TestStorageEnvironmentSerializesConcurrentOldProfileAdmission(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_RACE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty GLOWLOOM_TEST_STORAGE_RACE_DSN")
	}
	t.Setenv("GLOWLOOM_TEST_STORAGE_ROUTE_DSN", dsn)
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, database, "race-"+uuid.NewString(), "cos-first")); err != nil {
		t.Fatal(err)
	}
	input := storageEnvironmentFixture(t, database, "cutover-"+uuid.NewString(), "cos-main")
	for n := range input.Profiles {
		if input.Profiles[n].ID == "cos-first" {
			input.Profiles[n].Mode = mediaapp.StorageProfileModeReadOnly
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	blocker := database.WithContext(ctx).Begin()
	defer func() { _ = blocker.Rollback().Error }()
	if err := blocker.Exec(`SELECT 1 FROM media.storage_route_lock WHERE singleton=1 FOR UPDATE`).Error; err != nil {
		t.Fatal(err)
	}
	activator := database.WithContext(ctx).Begin()
	defer func() { _ = activator.Rollback().Error }()
	var pid int
	if err := activator.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := pgmedia.NewStorageRoutes(activator).ApplyEnvironment(ctx, input); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := database.Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=? AND wait_event_type='Lock')`, pid).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("configuration skipped serialization", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("configuration never waited")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := pgmedia.NewStorageRoutes(blocker).PinStorageWork(ctx, mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindOperation, OwnerID: uuid.New(), WorkID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("concurrent old writer lost permission", err)
	}
}

func TestStorageEnvironmentReplayAndSwitchPreserveInflightTargets(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	first := storageEnvironmentFixture(t, db, "first-"+uuid.NewString(), "cos-first")
	route, err := routes.ApplyEnvironment(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, db)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	admission := pgmedia.NewUploadAdmission(routingMediaStore(db))
	key := uuid.New()
	upload, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	owner := mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindOperation, OwnerID: uuid.New(), WorkID: uuid.New()}
	work, err := routes.PinStorageWork(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := routes.ApplyEnvironment(t.Context(), first)
	if err != nil || replay != route {
		t.Fatal("lost configuration reply changed", err)
	}
	next := storageEnvironmentFixture(t, db, "next-"+uuid.NewString(), "cos-main")
	for n := range next.Profiles {
		if next.Profiles[n].ID == "cos-first" {
			next.Profiles[n].Mode = mediaapp.StorageProfileModeReadOnly
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), next); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("old in-flight writer lost permission", err)
	}
	for n := range next.Profiles {
		next.Profiles[n].Mode = mediaapp.StorageProfileModeReadWrite
	}
	if _, err := routes.ApplyEnvironment(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	saved, err := routes.ReadStorageWork(t.Context(), owner)
	if err != nil || saved != work {
		t.Fatal("switch rebound existing work", err)
	}
	retry, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil || retry != upload {
		t.Fatal("switch rebound existing upload", err)
	}
	fresh, err := admission.PinUpload(t.Context(), actor, scope, uuid.New())
	if err != nil || fresh.ProfileID != "cos-main" {
		t.Fatal("new upload missed new route", err)
	}
}

func TestStorageFinalSchemaRequiresCurrentPipelineVersion(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_GATE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty GLOWLOOM_TEST_STORAGE_GATE_DSN")
	}
	t.Setenv("GLOWLOOM_TEST_STORAGE_ROUTE_DSN", dsn)
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	if err := routes.CheckStorageFinalSchema(t.Context()); err != nil {
		t.Fatal("current schema pipeline version missing", err)
	}
	if err := db.Exec(`DELETE FROM media.storage_schema_version WHERE version=2`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("missing current schema version passed readiness", err)
	}
	if err := db.Exec(`INSERT INTO media.storage_schema_version(version) VALUES(1) ON CONFLICT DO NOTHING`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("obsolete schema version passed readiness", err)
	}
}

func TestStorageFinalSchemaCheckDoesNotMutatePipelineVersion(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_STORAGE_GATE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty GLOWLOOM_TEST_STORAGE_GATE_DSN")
	}
	t.Setenv("GLOWLOOM_TEST_STORAGE_ROUTE_DSN", dsn)
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	var before, after int64
	if err := db.Table("media.storage_schema_version").Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := routes.FinalizeStorageSchema(t.Context(), strings.Repeat("a", 64)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Table("media.storage_schema_version").Count(&after).Error; err != nil || before != after {
		t.Fatal("runtime changed migration ownership", before, after, err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), "invalid"); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("invalid digest accepted", err)
	}
}
