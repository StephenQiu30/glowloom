package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func TestStorageOperatorDigestBindsWholeConfigurationWithoutResolvingSecrets(t *testing.T) {
	file := filepath.Join(t.TempDir(), "storage.yaml")
	raw := "object_storage:\n  schema_version: 1\n  revision: 1\n  default_write_profile: minio-main\n  profiles:\n    minio-main:\n      provider: minio\n      mode: read_write\n      signing_endpoint: https://storage.example.test\n      addressing_style: path\n      endpoint: https://storage.example.test\n      bucket: original-files\n      region: us-east-1\n      credentials:\n        source: env\n        access_key_env: ABSENT_STORAGE_TEST_AK\n        secret_key_env: ABSENT_STORAGE_TEST_SK\n"
	if err := os.WriteFile(file, []byte(raw+storagefixture.YAMLPolicy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorage(file, "production")
	if err != nil {
		t.Fatal(err)
	}
	first, err := storageoperator.Configuration(cfg)
	if err != nil || len(first.ConfigSHA256) != 64 || first.DefaultWriteProfile != "minio-main" || first.Revision != "1" {
		t.Fatal("invalid public activation", err)
	}
	second, err := storageoperator.Configuration(cfg)
	if err != nil || first.ConfigSHA256 != second.ConfigSHA256 {
		t.Fatal("identical configuration changed identity", err)
	}
	cfg.Limits.MaxImageBytes /= 2
	changed, err := storageoperator.Configuration(cfg)
	if err != nil || changed.ConfigSHA256 == first.ConfigSHA256 || changed.UploadPolicy.MaxImageBytes != cfg.Limits.MaxImageBytes {
		t.Fatal("policy not bound to revision", err)
	}
	body, err := json.Marshal(first)
	if err != nil || strings.Contains(string(body), "ABSENT_STORAGE_TEST") {
		t.Fatal("credential reference escaped into activation", err)
	}
}
