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
	raw := "object_storage:\n  schema_version: 1\n  revision: 1\n  default_write_profile: cos-main\n  profiles:\n    cos-main:\n      provider: cos\n      mode: read_write\n      signing_endpoint: https://assets-test-1250000000.cos.ap-shanghai.myqcloud.com\n      endpoint: https://assets-test-1250000000.cos.ap-shanghai.myqcloud.com\n      bucket: assets-test-1250000000\n      region: ap-shanghai\n      credentials:\n        source: env\n        access_key_env: ABSENT_STORAGE_TEST_AK\n        secret_key_env: ABSENT_STORAGE_TEST_SK\n"
	if err := os.WriteFile(file, []byte(raw+storagefixture.YAMLPolicy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorage(file, "production")
	if err != nil {
		t.Fatal(err)
	}
	first, err := storageoperator.Configuration(cfg)
	if err != nil || len(first.ConfigSHA256) != 64 || first.DefaultWriteProfile != "cos-main" || first.Revision != "1" {
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
