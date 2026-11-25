package canvas_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func canvasRouterStorage(t *testing.T, cfg *config.Config) *objectstorage.Registry {
	t.Helper()
	storage := storagefixture.Configured(t)
	cfg.ObjectStorage = &storage
	registry, err := objectstorage.OpenProfiles(storage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry
}

func seedCanvasLocation(t *testing.T, db *gorm.DB, key, profile, bucket string) uuid.UUID {
	t.Helper()
	cfg := storagefixture.Configured(t)
	configured, ok := cfg.Profiles[profile]
	if !ok || configured.Bucket != bucket {
		t.Fatal("fixture location must use an explicitly configured physical resource")
	}
	fact := media.StorageProfile{ID: profile, Provider: string(configured.Provider), Endpoint: configured.Endpoint, SigningEndpoint: configured.SigningEndpoint, Region: configured.Region, Bucket: configured.Bucket, AddressingStyle: string(configured.AddressingStyle)}
	if err := pgmedia.NewStorageRoutes(db).RegisterProfiles(t.Context(), []media.StorageProfile{fact}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted')`, id, profile, bucket, key).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func seedCanvasMetadataLocation(t *testing.T, db *gorm.DB, key string) uuid.UUID {
	t.Helper()
	cfg := storagefixture.Configured(t)
	return seedCanvasLocation(t, db, key, cfg.DefaultWriteProfile, cfg.Profiles[cfg.DefaultWriteProfile].Bucket)
}
