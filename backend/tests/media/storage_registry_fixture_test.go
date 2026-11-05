package media_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func mediaFixtureRegistry(t *testing.T) (*objectstorage.Registry, config.ObjectStorageConfig) {
	t.Helper()
	cfg := storagefixture.Configured(t)
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry, cfg
}
func mediaFixtureReadObjects(t *testing.T) *mediaobjects.RoutedReadObjects {
	t.Helper()
	registry, _ := mediaFixtureRegistry(t)
	return mediaobjects.NewRoutedReadObjects(registry)
}
func mediaFixtureRoutedUpload(t *testing.T, db *gorm.DB, repo mediaapp.UploadRepository, personal mediaapp.PersonalUploadRepository, prober mediaapp.Prober, normalizer mediaapp.UploadNormalizer, renderer mediaapp.Renderer) *mediaapp.UploadService {
	t.Helper()
	registry, cfg := mediaFixtureRegistry(t)
	routes := pgmedia.NewStorageRoutes(db)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "fixture-"+uuid.NewString(), cfg.DefaultWriteProfile)); err != nil {
		t.Fatal(err)
	}
	return mediaapp.NewRoutedUploadService(repo, personal, pgmedia.NewUploadAdmission(mediaTestStore(db)), prober, normalizer, renderer, mediaobjects.NewRoutedUploadObjects(registry, routes), time.Now)
}

// mediaFixtureAssetLocation seeds only physical metadata for repository tests;
// it is not proof that any provider holds the referenced bytes.
func mediaFixtureAssetLocation(t *testing.T, db *gorm.DB, asset domain.MediaAsset) domain.MediaAsset {
	t.Helper()
	cfg := storagefixture.Configured(t)
	id := cfg.DefaultWriteProfile
	p := cfg.Profiles[id]
	routes := pgmedia.NewStorageRoutes(db)
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: id, Bucket: p.Bucket, Key: asset.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Table("media.object_location").Where("id = ?", location.ID).Updates(map[string]any{"state": domain.ObjectLocationAccepted, "byte_size": asset.ByteSize, "sha256": asset.SHA256, "mime_type": asset.MimeType}).Error; err != nil {
		t.Fatal(err)
	}
	location.State = domain.ObjectLocationAccepted
	location.ByteSize = &asset.ByteSize
	location.SHA256 = asset.SHA256
	location.MIMEType = &asset.MimeType
	asset.ObjectLocation = &location
	return asset
}

func mediaFixtureRenditionLocation(t *testing.T, db *gorm.DB, rendition domain.Rendition) *domain.ObjectLocation {
	t.Helper()
	cfg := storagefixture.Configured(t)
	id := cfg.DefaultWriteProfile
	p := cfg.Profiles[id]
	location, err := pgmedia.NewStorageRoutes(db).ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: id, Bucket: p.Bucket, Key: rendition.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Table("media.object_location").Where("id = ?", location.ID).Update("state", domain.ObjectLocationAccepted).Error; err != nil {
		t.Fatal(err)
	}
	location.State = domain.ObjectLocationAccepted
	return &location
}

func mediaFixtureMockUpload(t *testing.T, db *gorm.DB, repo mediaapp.UploadRepository, personal mediaapp.PersonalUploadRepository, prober mediaapp.Prober, normalizer mediaapp.UploadNormalizer, renderer mediaapp.Renderer, objects *uploadObjectsFake) *mediaapp.UploadService {
	t.Helper()
	cfg := storagefixture.Configured(t)
	routes := pgmedia.NewStorageRoutes(db)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "fixture-"+uuid.NewString(), cfg.DefaultWriteProfile)); err != nil {
		t.Fatal(err)
	}
	objects.locations = routes
	return mediaapp.NewRoutedUploadService(repo, personal, pgmedia.NewUploadAdmission(mediaTestStore(db)), prober, normalizer, renderer, objects, time.Now)
}

func (s *uploadObjectsFake) CreateUploadObject(ctx context.Context, target mediaapp.UploadTarget, key string, file *mediaapp.Downloaded) (domain.ObjectLocation, error) {
	ref := domain.ObjectRef{ProfileID: target.ProfileID, Bucket: target.Bucket, Key: key}
	var location domain.ObjectLocation
	if s.locations == nil {
		location = domain.ObjectLocation{ID: uuid.New(), Ref: ref, State: domain.ObjectLocationReserved}
	} else {
		var err error
		location, err = s.locations.ReserveUploadLocation(ctx, target.SessionID, ref)
		if err != nil {
			return location, err
		}
	}
	if err := s.PutIfAbsent(ctx, key, io.NewSectionReader(file.File, 0, file.Size), file.Size, file.MIMEType, file.SHA256); err != nil && !errors.Is(err, mediaapp.ErrObjectAlreadyExists) {
		return location, err
	}
	info, err := s.Stat(ctx, key)
	if err != nil {
		return location, err
	}
	if info.Size != file.Size || info.SHA256 != file.SHA256 || info.ContentType != file.MIMEType {
		return location, mediaapp.ErrObjectMismatch
	}
	if s.locations != nil {
		if err := s.locations.ConfirmUploadLocation(ctx, location, info); err != nil {
			return location, err
		}
	}
	location.State = domain.ObjectLocationPresent
	location.ByteSize = &file.Size
	location.SHA256 = &file.SHA256
	location.MIMEType = &file.MIMEType
	return location, nil
}
func (s *uploadObjectsFake) RemoveUploadObject(ctx context.Context, location domain.ObjectLocation) error {
	if s.locations == nil {
		return s.Remove(ctx, location.Ref.Key)
	}
	remove, err := s.locations.BeginUploadRemoval(ctx, location)
	if err != nil || !remove {
		return err
	}
	if err := s.Remove(ctx, location.Ref.Key); err != nil {
		return err
	}
	return s.locations.ConfirmUploadRemoval(ctx, location)
}

func mediaFixtureCopyObjects(t *testing.T, db *gorm.DB) *mediaobjects.RoutedCopyObjects {
	t.Helper()
	registry, _ := mediaFixtureRegistry(t)
	return mediaobjects.NewRoutedCopyObjects(registry, pgmedia.NewStorageRoutes(db))
}

// packageFixtureObjects is the current located package port used by fault mocks.
type packageFixtureObjects interface {
	mediaapp.PackageObjects
	mediaapp.LocatedPackageObjects
}

func mediaFixturePackageObjects(t *testing.T, db *gorm.DB) *mediaobjects.RoutedPackageObjects {
	t.Helper()
	registry, cfg := mediaFixtureRegistry(t)
	routes := pgmedia.NewStorageRoutes(db)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "fixture-"+uuid.NewString(), cfg.DefaultWriteProfile)); err != nil {
		t.Fatal(err)
	}
	return mediaobjects.NewRoutedPackageObjects(registry, routes)
}

func fixtureResourceDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type purgeFixtureObjects interface {
	mediaapp.PurgeObjects
	mediaapp.LocatedPurgeObjects
}

func mediaFixturePurgeObjects(t *testing.T) *mediaobjects.RoutedPurgeObjects {
	t.Helper()
	registry, _ := mediaFixtureRegistry(t)
	return mediaobjects.NewRoutedPurgeObjects(registry)
}

func mediaFixtureIngestAdmission(t *testing.T, db *gorm.DB, project, operation uuid.UUID) {
	t.Helper()
	cfg := storagefixture.Configured(t)
	routes := pgmedia.NewStorageRoutes(db)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "fixture-"+uuid.NewString(), cfg.DefaultWriteProfile)); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.PinStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindOperation, OwnerID: project, WorkID: operation}); err != nil {
		t.Fatal(err)
	}
}

type unavailableStagedFixture struct{}

func (unavailableStagedFixture) ValidateReceipt(context.Context, operationapp.ProviderReceipt) error {
	return mediaapp.ErrInvalidIngest
}
func (unavailableStagedFixture) Download(context.Context, operationapp.ProviderReceipt) (*mediaapp.Downloaded, error) {
	return nil, mediaapp.ErrInvalidIngest
}

// fixtureProjectUpload follows the formal admission-before-body command flow.
// A supplied frozen target is preserved so stale-target negative cases remain valid.
func fixtureProjectUpload(ctx context.Context, service *mediaapp.UploadService, input mediaapp.UploadInput) (mediaapp.UploadResult, error) {
	if input.Target == nil {
		project := input.Request.ProjectID
		target, err := service.BeginUpload(ctx, input.Actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, input.Request.Key)
		if err != nil {
			return mediaapp.UploadResult{}, err
		}
		if target.SessionID != uuid.Nil {
			input.Target = &target
		}
	}
	return service.Upload(ctx, input)
}
func fixturePersonalUpload(ctx context.Context, service *mediaapp.UploadService, input mediaapp.UploadInput) (mediaapp.PersonalUploadResult, error) {
	if input.Target == nil {
		target, err := service.BeginUpload(ctx, input.Actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, input.Request.Key)
		if err != nil {
			return mediaapp.PersonalUploadResult{}, err
		}
		if target.SessionID != uuid.Nil {
			input.Target = &target
		}
	}
	return service.UploadPersonal(ctx, input)
}

func mediaFixtureUnownedLocation(t *testing.T, db *gorm.DB, key string) domain.ObjectLocation {
	t.Helper()
	cfg := storagefixture.Configured(t)
	id := cfg.DefaultWriteProfile
	p := cfg.Profiles[id]
	routes := pgmedia.NewStorageRoutes(db)
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: id, Bucket: p.Bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	return location
}

// drainFixtureStorageWork advances only this stopped synthetic execution beyond
// its bounded write and cleanup grace periods; no production deadline is altered.
func drainFixtureStorageWork(t *testing.T, owner *gorm.DB, kind mediaapp.StorageWorkKind, work uuid.UUID) {
	t.Helper()
	var ids []uuid.UUID
	if err := owner.Table("media.storage_work_intent").Where("kind = ? AND work_id = ?", kind, work).Pluck("id", &ids).Error; err != nil || len(ids) != 1 {
		t.Fatal("exact fixture storage work missing", err)
	}
	if err := owner.Table("media.storage_work_intent").Where("id IN ?", ids).Updates(map[string]any{"state": mediaapp.StorageWorkCancelled, "closed_at": time.Now().Add(-11 * time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	waitFixtureWriteFences(t, owner, ids)
}

func waitFixtureWriteFences(t *testing.T, owner *gorm.DB, ids []uuid.UUID) {
	t.Helper()
	var deadline *time.Time
	if err := owner.Table("media.object_location").Select("max(write_until)").Where("owner_intent_id IN ?", ids).Row().Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	if deadline == nil {
		return
	}
	wait := time.Until(*deadline) + 20*time.Millisecond
	if wait <= 0 {
		return
	}
	if wait > 5*time.Second {
		t.Fatal("fixture must use an explicitly short execution deadline before cleanup")
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}
func fixtureCopyContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func drainFixtureTransfer(t *testing.T, owner *gorm.DB, job uuid.UUID) {
	t.Helper()
	var ids []uuid.UUID
	if err := owner.Table("media.transfer_object AS o").Select("DISTINCT l.owner_intent_id").Joins("JOIN media.object_location AS l ON l.id=o.target_location_id").Joins("JOIN media.storage_work_intent AS w ON w.id=l.owner_intent_id").Where("o.job_id = ? AND w.state = ?", job, mediaapp.StorageWorkOpen).Pluck("l.owner_intent_id", &ids).Error; err != nil || len(ids) == 0 {
		t.Fatal("unfinished fixture transfer intents missing", err)
	}
	if err := owner.Table("media.storage_work_intent").Where("id IN ? AND state = ? AND closed_at IS NULL", ids, mediaapp.StorageWorkOpen).Updates(map[string]any{"state": mediaapp.StorageWorkCancelled, "closed_at": time.Now().Add(-11 * time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	waitFixtureWriteFences(t, owner, ids)
}

func prepareFixtureCopyCleanup(t *testing.T, owner *gorm.DB, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot) {
	t.Helper()
	if err := owner.Table("workspace.project_copy_job").Where("id = ? AND target_project_id = ?", binding.JobID, binding.TargetProjectID).Update("stage", "cleanup").Error; err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	if err := owner.Table("media.storage_work_intent").Where("kind = ? AND work_id = ?", mediaapp.StorageWorkKindProjectCopy, snapshot.ID).Pluck("id", &ids).Error; err != nil || len(ids) != 1 {
		t.Fatal("exact fixture copy work missing", err)
	}
	waitFixtureWriteFences(t, owner, ids)
}
