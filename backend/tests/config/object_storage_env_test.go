package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestObjectStorageRegistryEnvironmentKeepsNativeProfilesAndCredentialReferences(t *testing.T) {
	file := filepath.Join(t.TempDir(), "configuration.env")
	body := "GLOWLOOM_ENV=local\nGLOWLOOM_STORAGE_MAINTENANCE_DSN=postgres://maintenance-fixture@127.0.0.1/fixture\nTEST_REGISTRY_AK=fixture-access\nTEST_REGISTRY_SK=fixture-secret\nGLOWLOOM_OBJECT_STORAGE_CONFIG={\"schema_version\":1,\"revision\":\"1\",\"upload\":{\"mode\":\"relay\",\"signed_url_ttl\":600000000000,\"session_ttl\":86400000000000,\"multipart_threshold_bytes\":33554432,\"part_size_bytes\":8388608,\"part_concurrency\":3,\"staging_retention\":172800000000000},\"access\":{\"preview_ttl\":300000000000,\"max_signed_url_ttl\":900000000000},\"limits\":{\"max_image_bytes\":20971520,\"max_video_bytes\":524288000,\"max_audio_bytes\":104857600,\"max_model_bytes\":67108864,\"max_document_bytes\":20971520},\"default_write_profile\":\"main\",\"profiles\":{\"main\":{\"provider\":\"cos\",\"mode\":\"read_write\",\"signing_endpoint\":\"http://127.0.0.1:9000\",\"endpoint\":\"http://127.0.0.1:9000\",\"bucket\":\"fixture-assets\",\"region\":\"us-east-1\",\"credentials\":{\"source\":\"env\",\"access_key_env\":\"TEST_REGISTRY_AK\",\"secret_key_env\":\"TEST_REGISTRY_SK\"}}}}\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorageMaintenanceDSN == "" || cfg.ObjectStorage == nil || cfg.ObjectStorage.DefaultWriteProfile != "main" || cfg.ObjectStorage.Profiles["main"].Provider != config.StorageProviderCOS {
		t.Fatal("registry identity missing")
	}
	r, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal("dotenv credentials unavailable", err)
	}
	r.Close()
}

func TestObjectStorageEmptyRegistryDoesNotBlockOtherCapabilities(t *testing.T) {
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "")
	cfg, err := config.Load()
	if err != nil || cfg.ObjectStorage != nil {
		t.Fatal("optional registry blocked ordinary service", err)
	}
}

func TestObjectStorageRejectsObsoleteSingleBucketAndLegacyEnvironment(t *testing.T) {
	for _, field := range []string{"PROVIDER", "ENDPOINT", "SIGNING_ENDPOINT", "BUCKET", "REGION", "PROFILE_ID", "ACCESS_KEY", "SECRET_KEY", "REVISION", "LEGACY_PROVIDER", "LEGACY_ENDPOINT", "LEGACY_SIGNING_ENDPOINT", "LEGACY_BUCKET", "LEGACY_REGION", "LEGACY_PROFILE_ID", "LEGACY_ACCESS_KEY", "LEGACY_SECRET_KEY"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv("GLOWLOOM_OBJECT_STORAGE_"+field, "obsolete-fixture")
			if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
				t.Fatal("obsolete storage source accepted", err)
			}
		})
	}
}

func TestObjectStoragePartialRegistryAndRuntimeYAMLRejected(t *testing.T) {
	for _, raw := range []string{`{"schema_version":1}`, "/must-not-read-storage.yaml", `{"schema_version":1,"legacy_profile":"old"}`} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", raw)
		if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
			t.Fatal("invalid registry accepted", err)
		}
	}
}
