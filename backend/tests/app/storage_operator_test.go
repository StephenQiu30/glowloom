package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestStorageOperatorDigestBindsWholeConfigurationWithoutResolvingSecrets(t *testing.T) {
	file := filepath.Join(t.TempDir(), "storage.yaml")
	raw := "object_storage:\n  schema_version: 1\n  revision: release-one\n  profiles:\n    minio-main:\n      provider: minio\n      endpoint: https://storage.example.test\n      bucket: original-files\n      region: us-east-1\n      credentials:\n        source: env\n        access_key_env: ABSENT_STORAGE_TEST_AK\n        secret_key_env: ABSENT_STORAGE_TEST_SK\n"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorage(file, "production")
	if err != nil {
		t.Fatal(err)
	}
	first, err := storageoperator.Configuration(cfg, "old-revision")
	if err != nil || len(first.ConfigSHA256) != 64 || first.DefaultWriteProfile != "minio-main" || first.ExpectedRevision != "old-revision" {
		t.Fatal("invalid public activation", err)
	}
	second, err := storageoperator.Configuration(cfg, "different-expected")
	if err != nil || first.ConfigSHA256 != second.ConfigSHA256 {
		t.Fatal("CAS precondition changed config identity", err)
	}
	cfg.Limits.MaxImageBytes /= 2
	changed, err := storageoperator.Configuration(cfg, "old-revision")
	if err != nil || changed.ConfigSHA256 == first.ConfigSHA256 || changed.UploadPolicy.MaxImageBytes != cfg.Limits.MaxImageBytes {
		t.Fatal("policy not bound to revision", err)
	}
	body, err := json.Marshal(first)
	if err != nil || strings.Contains(string(body), "ABSENT_STORAGE_TEST") {
		t.Fatal("credential reference escaped into activation", err)
	}
}
