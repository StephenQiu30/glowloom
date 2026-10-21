package config_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

const storageFixture = `object_storage:
  schema_version: 1
  revision: storage-test-1
  profiles:
    minio-main:
      provider: minio
      endpoint: https://storage.example.test
      bucket: assets-test
      region: us-east-1
      credentials:
        source: env
        access_key_env: GLOWLOOM_TEST_STORAGE_ACCESS
        secret_key_env: GLOWLOOM_TEST_STORAGE_SECRET
`

func storageConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "storage.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestObjectStorageDefaultsToMinIO(t *testing.T) {
	cfg, err := config.LoadObjectStorage(storageConfigFile(t, storageFixture), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultWriteProfile != "minio-main" || cfg.Profiles["minio-main"].Provider != "minio" || cfg.Upload.Mode != "relay" {
		t.Fatal("a MinIO-only deployment did not choose its declared default")
	}
	if cfg.Access.PreviewTTL != 5*time.Minute || cfg.Access.MaxSignedURLTTL != 15*time.Minute {
		t.Fatal("bounded private access defaults were lost")
	}
	if _, err := config.LoadObjectStorage(storageConfigFile(t, strings.Replace(storageFixture, "  revision:", "  default_write_profile: cos-missing\n  revision:", 1)), "prod"); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("missing explicit COS target silently fell back: %v", err)
	}
}

func TestObjectStorageRejectsUnknownFieldsAndMixedSources(t *testing.T) {
	for _, body := range []string{
		storageFixture + "  typo_option: true\n",
		strings.Replace(storageFixture, "provider: minio", "provider: arbitrary", 1),
		strings.Replace(storageFixture, "schema_version: 1", "schema_version: 7", 1),
		storageFixture + "---\nobject_storage: {}\n",
		strings.Replace(storageFixture, "  revision:", "  upload:\n    signed_url_ttl: 0s\n  revision:", 1),
		strings.Replace(storageFixture, "  revision:", "  upload:\n    mode: direct\n  revision:", 1),
		strings.Replace(storageFixture, "https://storage.example.test", "http://storage.example.test", 1),
		strings.Replace(storageFixture, "https://storage.example.test", "https://storage.example.test/private", 1),
		strings.Replace(storageFixture, "source: env", "source: env\n        file: /secrets/storage.json", 1),
	} {
		if _, err := config.LoadObjectStorage(storageConfigFile(t, body), "prod"); !errors.Is(err, config.ErrInvalid) {
			t.Fatalf("unsafe or unsupported configuration accepted: %v", err)
		}
	}
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", storageConfigFile(t, storageFixture))
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_ENDPOINT", "https://old-storage.example.test")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("YAML and legacy environment fields were mixed: %v", err)
	}
}

func TestObjectStorageRejectsDuplicateResourceAlias(t *testing.T) {
	body := storageFixture + strings.Replace(strings.SplitN(storageFixture, "    minio-main:", 2)[1], "      provider:", "    alias:\n      provider:", 1)
	if _, err := config.LoadObjectStorage(storageConfigFile(t, body), "prod"); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("same physical resource was registered twice: %v", err)
	}
}

func TestObjectStorageBindsCOSOriginToBucketAndRegion(t *testing.T) {
	valid := strings.NewReplacer("provider: minio", "provider: cos", "https://storage.example.test", "https://assets-a-1250000000.cos.ap-shanghai.myqcloud.com", "bucket: assets-test", "bucket: assets-a-1250000000", "region: us-east-1", "region: ap-shanghai").Replace(storageFixture)
	if _, err := config.LoadObjectStorage(storageConfigFile(t, valid), "prod"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(valid, "bucket: assets-a-1250000000", "bucket: assets-b-1250000000", 1),
		strings.Replace(valid, "region: ap-shanghai", "region: ap-beijing", 1),
		strings.Replace(valid, "https://assets-a-1250000000.cos.ap-shanghai.myqcloud.com", "https://custom.example.test", 1),
	} {
		if _, err := config.LoadObjectStorage(storageConfigFile(t, bad), "prod"); !errors.Is(err, config.ErrInvalid) {
			t.Fatal("COS declared identity differed from actual BucketURL", err)
		}
	}
	profile := strings.SplitN(valid, "    minio-main:", 2)[1]
	for _, host := range []string{"assets-a-1250000000.cos.ap-shanghai.tencentcos.cn", "assets-a-1250000000.cos-internal.ap-shanghai.tencentcos.cn"} {
		alias := strings.Replace(profile, "assets-a-1250000000.cos.ap-shanghai.myqcloud.com", host, 1)
		if _, err := config.LoadObjectStorage(storageConfigFile(t, valid+"    cos-alias:"+alias), "prod"); !errors.Is(err, config.ErrInvalid) {
			t.Fatal("known COS domain alias acquired a separate identity", err)
		}
	}
}

func TestObjectStorageRegionCannotAliasSamePhysicalBucket(t *testing.T) {
	profile := strings.SplitN(storageFixture, "    minio-main:", 2)[1]
	alias := strings.Replace(profile, "region: us-east-1", "region: us-east-2", 1)
	if _, err := config.LoadObjectStorage(storageConfigFile(t, storageFixture+"    minio-alias:"+alias), "prod"); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("a signing region split the same physical bucket identity", err)
	}
}

func TestObjectStorageRedactsSecretErrors(t *testing.T) {
	secret := "private-value-never-log"
	body := strings.Replace(storageFixture, "source: env", "source: env\n        secret_key: "+secret, 1)
	_, err := config.LoadObjectStorage(storageConfigFile(t, body), "prod")
	if !errors.Is(err, config.ErrInvalid) || strings.Contains(fmt.Sprint(err), secret) {
		t.Fatalf("unknown secret field was accepted or disclosed: %v", err)
	}
	t.Setenv("GLOWLOOM_TEST_STORAGE_ACCESS", "test-access")
	t.Setenv("GLOWLOOM_TEST_STORAGE_SECRET", secret)
	cfg, err := config.LoadObjectStorage(storageConfigFile(t, storageFixture), "prod")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := config.ResolveStorageCredentials(cfg.Profiles["minio-main"].Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessKey != "test-access" || credentials.SecretKey != secret {
		t.Fatal("credential reference did not resolve")
	}
	for _, out := range []string{fmt.Sprintf("%v", credentials), fmt.Sprintf("%+v", credentials), fmt.Sprintf("%#v", credentials)} {
		if strings.Contains(out, secret) {
			t.Fatal("credential formatting disclosed a secret")
		}
	}
}

func TestObjectStorageSecretFileAndExpiredCredentials(t *testing.T) {
	file := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(file, []byte(`{"access_key":"test-access","secret_key":"test-secret","session_token":"test-token","expires_at":"2099-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := config.StorageCredentialsRef{Source: "file", File: file}
	credentials, err := config.ResolveStorageCredentials(ref)
	if err != nil || credentials.SessionToken != "test-token" || credentials.ExpiresAt.IsZero() {
		t.Fatalf("secret file did not preserve bounded temporary credentials: %v", err)
	}
	if err := os.WriteFile(file, []byte(`{"access_key":"test-access","secret_key":"test-secret","expires_at":"2000-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ResolveStorageCredentials(ref); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("expired credentials accepted: %v", err)
	}
}

func TestObjectStorageCredentialSerializationRedactsSecrets(t *testing.T) {
	credentials := config.StorageCredentials{AccessKey: "private-access", SecretKey: "private-secret", SessionToken: "private-token"}
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		data, err := marshal(credentials)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "private-") {
			t.Fatal("credential serialization disclosed an access-bearing value")
		}
	}
}
