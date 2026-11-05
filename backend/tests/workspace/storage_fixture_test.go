package workspace_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// businessStorageConfig uses only the explicitly selected isolated storage fixture.
func businessStorageConfig(t *testing.T) config.ObjectStorageConfig {
	t.Helper()
	return storagefixture.Configured(t)
}

func businessStorageRegistry(t *testing.T) *objectstorage.Registry {
	t.Helper()
	return storagefixture.OpenConfigured(t)
}

func businessStorageEnvironment(t *testing.T, database *gorm.DB) {
	t.Helper()
	environment, err := storageoperator.Configuration(businessStorageConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgmedia.NewStorageRoutes(database).ApplyEnvironment(t.Context(), environment); err != nil {
		t.Fatal(err)
	}
}

// businessFixtureUploads uses production admission before bytes, preserving its saved route.
func businessFixtureUploads(t *testing.T, database *gorm.DB, store *pgmedia.Store, prober mediaapp.Prober) *mediaapp.UploadService {
	t.Helper()
	businessStorageEnvironment(t, database)
	return mediaapp.NewRoutedUploadService(store, store, pgmedia.NewUploadAdmission(store), prober, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaobjects.NewRoutedUploadObjects(businessStorageRegistry(t), pgmedia.NewStorageRoutes(database)), time.Now)
}
func businessFixtureUpload(ctx context.Context, t *testing.T, service *mediaapp.UploadService, input mediaapp.UploadInput) (mediaapp.UploadResult, error) {
	t.Helper()
	scope := mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &input.Request.ProjectID}
	target, err := service.BeginUpload(ctx, input.Actor, scope, input.Request.Key)
	if err != nil {
		return mediaapp.UploadResult{}, err
	}
	input.Target = &target
	return service.Upload(ctx, input)
}

// businessFixtureLocation creates metadata-only historical storage evidence. It
// does not create bytes and must never be used as proof of real object I/O.
func businessFixtureLocation(t *testing.T, database *gorm.DB, key string) uuid.UUID {
	t.Helper()
	cfg := businessStorageConfig(t)
	p := cfg.Profiles[cfg.DefaultWriteProfile]
	profile := mediaapp.StorageProfile{ID: cfg.DefaultWriteProfile, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: string(p.AddressingStyle)}
	if err := pgmedia.NewStorageRoutes(database).RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := database.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted') ON CONFLICT(profile_id,bucket,object_key) DO NOTHING`, id, profile.ID, profile.Bucket, key).Error; err != nil {
		t.Fatal(err)
	}
	var saved struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT id FROM media.object_location WHERE profile_id=? AND bucket=? AND object_key=?`, profile.ID, profile.Bucket, key).Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	return saved.ID
}

// businessExpireStorageWindow advances only the isolated fixture's elapsed clock
// after a synchronous cleanup call durably closed its work. Ownership, state,
// receipts and references remain untouched; production guards are restored in
// the same owner transaction before the next actual SDK cleanup attempt.
func businessExpireStorageWindow(t *testing.T, owner *gorm.DB, target uuid.UUID) {
	t.Helper()
	var database string
	if err := owner.Raw(`SELECT current_database()`).Scan(&database).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "glowloom_") {
		t.Fatal("storage clock fixture requires isolated test database", database)
	}
	var closed int64
	if err := owner.Table("media.storage_work_intent").Where("(owner_id=? OR work_id=?) AND closed_at IS NOT NULL", target, target).Count(&closed).Error; err != nil || closed == 0 {
		t.Fatal("storage clock requires durable work closure", closed, err)
	}
	if err := owner.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`ALTER TABLE media.object_location DISABLE TRIGGER keep_storage_write_deadline`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE media.storage_work_intent DISABLE TRIGGER keep_storage_work`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.object_location SET write_until=clock_timestamp()-interval '11 minutes' WHERE owner_intent_id IN (SELECT id FROM media.storage_work_intent WHERE owner_id=? OR work_id=?) AND write_until IS NOT NULL`, target, target).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.storage_work_intent SET closed_at=clock_timestamp()-interval '11 minutes' WHERE (owner_id=? OR work_id=?) AND closed_at IS NOT NULL`, target, target).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE media.object_location ENABLE TRIGGER keep_storage_write_deadline`).Error; err != nil {
			return err
		}
		return tx.Exec(`ALTER TABLE media.storage_work_intent ENABLE TRIGGER keep_storage_work`).Error
	}); err != nil {
		t.Fatal(err)
	}
}
