package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestLoadFromDotEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.env.example")
	if err := os.WriteFile(path, []byte("GLOWLOOM_ENV=staging\nGLOWLOOM_HTTP_ADDR=:9011\nGLOWLOOM_WORKER_HEALTH_ADDR=:9012\nGLOWLOOM_RELAY_HEALTH_ADDR=:9013\nGLOWLOOM_LOG_LEVEL=warn\nGLOWLOOM_DB_DSN=postgres://local/db\nGLOWLOOM_PARTITION_MAINTENANCE_DB_DSN=postgres://owner/db\nGLOWLOOM_REDIS_URL=redis://127.0.0.1:6379/2\nGLOWLOOM_KAFKA_BROKERS=127.0.0.1:9092,127.0.0.1:9093\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", path)
	t.Setenv("GLOWLOOM_ENV", "prod")
	t.Setenv("GLOWLOOM_PUBLIC_ORIGIN", "https://glowloom.example")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9011" || cfg.WorkerHealthAddr != ":9012" || cfg.RelayHealthAddr != ":9013" || cfg.LogLevel != "warn" || cfg.DBDSN != "postgres://local/db" || cfg.PartitionMaintenanceDBDSN != "postgres://owner/db" || cfg.RedisURL != "redis://127.0.0.1:6379/2" || cfg.KafkaBrokers != "127.0.0.1:9092,127.0.0.1:9093" {
		t.Error("Load() did not apply the expected environment override and file values")
	}
}

func TestLoadRejectsMissingDotEnvFile(t *testing.T) {
	t.Setenv("GLOWLOOM_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	_, err := config.Load()
	if !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "local" {
		t.Errorf("Env = %q, want %q", cfg.Env, "local")
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8080")
	}
	if cfg.WorkerHealthAddr != ":8081" || cfg.RelayHealthAddr != ":8082" {
		t.Errorf("role health addresses = %q/%q, want :8081/:8082", cfg.WorkerHealthAddr, cfg.RelayHealthAddr)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.DBDSN != "" {
		t.Errorf("DBDSN = %q, want empty", cfg.DBDSN)
	}
	if cfg.RedisURL != "" {
		t.Errorf("RedisURL = %q, want empty", cfg.RedisURL)
	}
	if cfg.KafkaBrokers != "" {
		t.Errorf("KafkaBrokers = %q, want empty", cfg.KafkaBrokers)
	}
}

func TestCredentialSealingConfigurationRequiresPublicKeyAndIdentity(t *testing.T) {
	t.Setenv("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_FILE", "/configured/public.pem")
	t.Setenv("GLOWLOOM_CREDENTIAL_KEY_ID", "")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("incomplete public key configuration accepted: %v", err)
	}
	t.Setenv("GLOWLOOM_CREDENTIAL_KEY_ID", "agent-key-1")
	cfg, err := config.Load()
	if err != nil || cfg.CredentialPublicKeyFile != "/configured/public.pem" || cfg.CredentialKeyID != "agent-key-1" {
		t.Fatalf("public key configuration not loaded: %v", err)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GLOWLOOM_ENV", "staging")
	t.Setenv("GLOWLOOM_PUBLIC_ORIGIN", "https://glowloom.example")
	t.Setenv("GLOWLOOM_HTTP_ADDR", ":9090")
	t.Setenv("GLOWLOOM_WORKER_HEALTH_ADDR", ":9091")
	t.Setenv("GLOWLOOM_RELAY_HEALTH_ADDR", ":9092")
	t.Setenv("GLOWLOOM_LOG_LEVEL", "debug")
	t.Setenv("GLOWLOOM_DB_DSN", "postgres://localhost/glowloom")
	t.Setenv("GLOWLOOM_REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("GLOWLOOM_KAFKA_BROKERS", "localhost:9092")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := config.Config{Env: "staging", PublicOrigin: "https://glowloom.example", HTTPAddr: ":9090", WorkerHealthAddr: ":9091", RelayHealthAddr: ":9092", LogLevel: "debug", DBDSN: "postgres://localhost/glowloom", RedisURL: "redis://localhost:6379/1", KafkaBrokers: "localhost:9092"}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadObjectStorageFromEnv(t *testing.T) {
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", `{"schema_version":1,"revision":"1","upload":{"mode":"relay","signed_url_ttl":600000000000,"session_ttl":86400000000000,"multipart_threshold_bytes":33554432,"part_size_bytes":8388608,"part_concurrency":3,"staging_retention":172800000000000},"access":{"preview_ttl":300000000000,"max_signed_url_ttl":900000000000},"limits":{"max_image_bytes":20971520,"max_video_bytes":524288000,"max_audio_bytes":104857600,"max_model_bytes":67108864,"max_document_bytes":20971520},"default_write_profile":"main","profiles":{"main":{"provider":"minio","mode":"read_write","addressing_style":"path","signing_endpoint":"http://127.0.0.1:9000","endpoint":"http://127.0.0.1:9000","bucket":"glowloom-local","region":"us-east-1","credentials":{"source":"env","access_key_env":"TEST_STORAGE_AK","secret_key_env":"TEST_STORAGE_SK"}}}}`)
	t.Setenv("TEST_STORAGE_AK", "test-access")
	t.Setenv("TEST_STORAGE_SK", "test-secret")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ObjectStorage == nil {
		t.Fatal("registry configuration missing")
	}
	profile := cfg.ObjectStorage.Profiles["main"]
	credentials, err := cfg.ObjectStorage.ResolveCredentials(profile.Credentials)
	if err != nil || profile.Bucket != "glowloom-local" || profile.Endpoint != "http://127.0.0.1:9000" || credentials.AccessKey != "test-access" || credentials.SecretKey != "test-secret" {
		t.Fatal("registry identity or credential references changed", err)
	}
}

func TestLoadMediaResultDownloadPolicy(t *testing.T) {
	t.Setenv("GLOWLOOM_MEDIA_RESULT_ALLOWED_ORIGINS", "https://results.example.test")
	t.Setenv("GLOWLOOM_MEDIA_ALLOW_TEST_LOOPBACK_TLS", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MediaResultAllowedOrigins != "https://results.example.test" || !cfg.MediaAllowTestLoopbackTLS {
		t.Fatal("Load() did not preserve media result policy")
	}
	t.Setenv("GLOWLOOM_ENV", "prod")
	t.Setenv("GLOWLOOM_PUBLIC_ORIGIN", "https://glowloom.example")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("production loopback policy error = %v, want ErrInvalid", err)
	}
}

func TestLoadOptionalWhisperEndpoint(t *testing.T) {
	t.Setenv("GLOWLOOM_WHISPER_ENDPOINT", "")
	cfg, err := config.Load()
	if err != nil || cfg.WhisperEndpoint != "" {
		t.Fatalf("unconfigured speech endpoint must stay disabled: %v", err)
	}
	t.Setenv("GLOWLOOM_WHISPER_ENDPOINT", " http://127.0.0.1:19282 ")
	cfg, err = config.Load()
	if err != nil || cfg.WhisperEndpoint != "http://127.0.0.1:19282" {
		t.Fatalf("speech endpoint was not trimmed and loaded: %v", err)
	}
}

func TestLoadOTelEndpointFromEnv(t *testing.T) {
	t.Setenv("GLOWLOOM_OTEL_ENDPOINT", "http://127.0.0.1:4318")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OTelEndpoint != "http://127.0.0.1:4318" {
		t.Errorf("OTelEndpoint = %q", cfg.OTelEndpoint)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{name: "unknown env", key: "GLOWLOOM_ENV", val: "dev"},
		{name: "unknown log level", key: "GLOWLOOM_LOG_LEVEL", val: "verbose"},
		{name: "empty http addr", key: "GLOWLOOM_HTTP_ADDR", val: " "},
		{name: "empty worker health addr", key: "GLOWLOOM_WORKER_HEALTH_ADDR", val: " "},
		{name: "empty relay health addr", key: "GLOWLOOM_RELAY_HEALTH_ADDR", val: " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.val)

			_, err := config.Load()
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("Load() error = %v, want ErrInvalid", err)
			}
		})
	}
}
