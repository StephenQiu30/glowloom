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
