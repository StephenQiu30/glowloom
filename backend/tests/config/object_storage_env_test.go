package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestObjectStorageUniformEnvironmentBuildsNativeTOSProfile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "configuration.env")
	body := "GLOWLOOM_ENV=local\nGLOWLOOM_STORAGE_MAINTENANCE_DSN=postgres://maintenance-fixture@127.0.0.1/fixture\nGLOWLOOM_OBJECT_STORAGE_PROVIDER=tos\nGLOWLOOM_OBJECT_STORAGE_ENDPOINT=https://tos-cn-beijing.volces.com\nGLOWLOOM_OBJECT_STORAGE_BUCKET=fixture-assets\nGLOWLOOM_OBJECT_STORAGE_REGION=cn-beijing\nGLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=fixture-access\nGLOWLOOM_OBJECT_STORAGE_SECRET_KEY=fixture-secret\nGLOWLOOM_OBJECT_STORAGE_PROFILE_ID=tos-main\nGLOWLOOM_OBJECT_STORAGE_REVISION=storage-v1\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorageMaintenanceDSN != "postgres://maintenance-fixture@127.0.0.1/fixture" {
		t.Fatal("operator maintenance DSN omitted from unified dotenv")
	}
	if cfg.ObjectStorage == nil || cfg.ObjectStorage.DefaultWriteProfile != "tos-main" || cfg.ObjectStorage.Revision != "storage-v1" || cfg.ObjectStorage.Profiles["tos-main"].Provider != "tos" {
		t.Fatal("unified env did not create native immutable profile")
	}
	r, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal("dotenv credentials were not available without changing process environment", err)
	}
	r.Close()
	if legacy, err := objectstorage.OpenLegacy(cfg); err != nil || legacy != nil {
		t.Fatal("new TOS bucket became an implicit legacy bare-key reader", err)
	}
}

func TestObjectStorageUniformEnvironmentBuildsNativeCOSProfileWithLegacyMinIO(t *testing.T) {
	file := filepath.Join(t.TempDir(), "configuration.env")
	body := "GLOWLOOM_ENV=local\n" +
		"GLOWLOOM_OBJECT_STORAGE_PROVIDER=cos\n" +
		"GLOWLOOM_OBJECT_STORAGE_ENDPOINT=https://assets-fixture-1250000000.cos.ap-guangzhou.myqcloud.com\n" +
		"GLOWLOOM_OBJECT_STORAGE_BUCKET=assets-fixture-1250000000\n" +
		"GLOWLOOM_OBJECT_STORAGE_REGION=ap-guangzhou\n" +
		"GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=fixture-cos-ak\n" +
		"GLOWLOOM_OBJECT_STORAGE_SECRET_KEY=fixture-cos-sk\n" +
		"GLOWLOOM_OBJECT_STORAGE_PROFILE_ID=cos-main\n" +
		"GLOWLOOM_OBJECT_STORAGE_REVISION=cos-env-v1\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_PROVIDER=minio\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_PROFILE_ID=legacy-minio\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_ENDPOINT=http://127.0.0.1:9000\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_BUCKET=glowloom-media\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_REGION=us-east-1\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_ACCESS_KEY=minioadmin\n" +
		"GLOWLOOM_OBJECT_STORAGE_LEGACY_SECRET_KEY=minioadmin\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ObjectStorage == nil {
		t.Fatal("expected ObjectStorage config to be populated")
	}
	if cfg.ObjectStorage.DefaultWriteProfile != "cos-main" {
		t.Fatalf("expected DefaultWriteProfile to be cos-main, got %s", cfg.ObjectStorage.DefaultWriteProfile)
	}
	if cfg.ObjectStorage.Profiles["cos-main"].Provider != "cos" {
		t.Fatalf("expected cos-main provider to be cos, got %s", cfg.ObjectStorage.Profiles["cos-main"].Provider)
	}
	if cfg.ObjectStorage.Profiles["cos-main"].Mode != "read_write" {
		t.Fatalf("expected cos-main mode to be read_write, got %s", cfg.ObjectStorage.Profiles["cos-main"].Mode)
	}
	minioProfile, ok := cfg.ObjectStorage.Profiles["legacy-minio"]
	if !ok {
		t.Fatal("expected legacy-minio profile in profiles")
	}
	if minioProfile.Provider != "minio" {
		t.Fatalf("expected legacy-minio provider to be minio, got %s", minioProfile.Provider)
	}
	if minioProfile.Mode != "read_only" {
		t.Fatalf("expected legacy-minio mode to be read_only, got %s", minioProfile.Mode)
	}

	r, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal("failed to open profiles with COS and legacy MinIO", err)
	}
	r.Close()

	legacy, err := objectstorage.OpenLegacy(cfg)
	if err != nil || legacy == nil {
		t.Fatalf("expected OpenLegacy to return legacy MinIO client, got %v, err: %v", legacy, err)
	}
	legacy.Close()
}
func TestObjectStorageEmptyNativeConfigurationDoesNotBlockOtherCapabilities(t *testing.T) {
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_PROVIDER", "tos")
	for _, field := range []string{"ENDPOINT", "BUCKET", "REGION", "ACCESS_KEY", "SECRET_KEY"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_"+field, "")
	}
	cfg, err := config.Load()
	if err != nil || cfg.ObjectStorage != nil {
		t.Fatal("empty optional TOS configuration blocked ordinary service", err)
	}
	if legacy, err := objectstorage.OpenLegacy(cfg); err != nil || legacy != nil {
		t.Fatal("unconfigured TOS invented legacy storage", err)
	}
}
func TestObjectStoragePartialNativeAndRuntimeYAMLRejected(t *testing.T) {
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_PROVIDER", "tos")
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_ENDPOINT", "https://tos-cn-beijing.volces.com")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("partial TOS configuration accepted", err)
	}
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "/must-not-read-storage.yaml")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("runtime YAML bypassed uniform env configuration", err)
	}
}
