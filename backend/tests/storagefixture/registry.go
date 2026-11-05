// Package storagefixture constructs explicit immutable storage profiles for tests.
package storagefixture

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Configuration resolves fixture credentials without mutating process environment.
func Configuration(t testing.TB, endpoint, bucket string) config.ObjectStorageConfig {
	t.Helper()
	input := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "1", DefaultWriteProfile: "test-main", Profiles: map[string]config.StorageProfile{"test-main": {Provider: config.StorageProviderMinIO, Mode: config.StorageProfileReadWrite, Endpoint: endpoint, SigningEndpoint: endpoint, AddressingStyle: config.StorageAddressingPath, Bucket: bucket, Region: "us-east-1", Credentials: config.StorageCredentialsRef{Source: config.StorageCredentialsEnvironment, AccessKeyEnv: "TEST_STORAGE_AK", SecretKeyEnv: "TEST_STORAGE_SK"}}}}
	input.Upload, input.Access, input.Limits = Policy()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorageEnvironment(string(raw), "local", func(key string) string {
		if key == "TEST_STORAGE_AK" {
			return "fixture-access"
		}
		if key == "TEST_STORAGE_SK" {
			return "fixture-secret"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Open constructs a registry, with cleanup owned by the calling test.
func Open(t testing.TB, endpoint, bucket string) (*objectstorage.Registry, error) {
	t.Helper()
	registry, err := objectstorage.OpenProfiles(Configuration(t, endpoint, bucket))
	if err == nil {
		t.Cleanup(registry.Close)
	}
	return registry, err
}

// Policy supplies the explicit accepted transfer, access, and input budgets.
func Policy() (config.StorageUploadPolicy, config.StorageAccessPolicy, config.StorageLimits) {
	return config.StorageUploadPolicy{Mode: config.StorageUploadRelay, SignedURLTTL: 10 * time.Minute, SessionTTL: 24 * time.Hour, MultipartThresholdBytes: 32 << 20, PartSizeBytes: 8 << 20, PartConcurrency: 3, StagingRetention: 48 * time.Hour}, config.StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute}, config.StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}
}

// YAMLPolicy is a complete explicit fixture policy at object_storage indentation.
const YAMLPolicy = `  upload:
    mode: relay
    signed_url_ttl: 10m
    session_ttl: 24h
    multipart_threshold_bytes: 33554432
    part_size_bytes: 8388608
    part_concurrency: 3
    staging_retention: 48h
  access:
    preview_ttl: 5m
    max_signed_url_ttl: 15m
  limits:
    max_image_bytes: 20971520
    max_video_bytes: 524288000
    max_audio_bytes: 104857600
    max_model_bytes: 67108864
    max_document_bytes: 20971520
`

// Configured reads the explicit integration registry with referenced credentials.
// Callers must apply their integration environment guard before invoking it.
func Configured(t testing.TB) config.ObjectStorageConfig {
	t.Helper()
	raw := os.Getenv("GLOWLOOM_OBJECT_STORAGE_CONFIG")
	if raw == "" {
		t.Fatal("GLOWLOOM_OBJECT_STORAGE_CONFIG is required for the integration fixture")
	}
	cfg, err := config.LoadObjectStorageEnvironment(raw, "local", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// OpenConfigured opens the integration registry and owns its transport cleanup.
func OpenConfigured(t testing.TB) *objectstorage.Registry {
	t.Helper()
	registry, err := objectstorage.OpenProfiles(Configured(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry
}
