package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestLoadFromDotEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.env.example")
	if err := os.WriteFile(path, []byte("LV_ENV=staging\nLV_HTTP_ADDR=:9011\nLV_WORKER_HEALTH_ADDR=:9012\nLV_RELAY_HEALTH_ADDR=:9013\nLV_LOG_LEVEL=warn\nLV_DB_DSN=postgres://local/db\nLV_PARTITION_MAINTENANCE_DB_DSN=postgres://owner/db\nLV_REDIS_URL=redis://127.0.0.1:6379/2\nLV_SESSION_IDLE_TTL=2h\nLV_SESSION_ABSOLUTE_TTL=48h\nLV_KAFKA_BROKERS=127.0.0.1:9092,127.0.0.1:9093\nLV_TEMPORAL_ADDR=127.0.0.1:7233\nLV_TEMPORAL_NAMESPACE=lanverse-staging\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LV_ENV_FILE", path)
	t.Setenv("LV_ENV", "prod")
	t.Setenv("LV_PUBLIC_ORIGIN", "https://lanverse.example")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9011" || cfg.WorkerHealthAddr != ":9012" || cfg.RelayHealthAddr != ":9013" || cfg.LogLevel != "warn" || cfg.DBDSN != "postgres://local/db" || cfg.PartitionMaintenanceDBDSN != "postgres://owner/db" || cfg.RedisURL != "redis://127.0.0.1:6379/2" || cfg.SessionIdleTTL != 2*time.Hour || cfg.SessionAbsoluteTTL != 48*time.Hour || cfg.KafkaBrokers != "127.0.0.1:9092,127.0.0.1:9093" || cfg.TemporalAddr != "127.0.0.1:7233" || cfg.TemporalNamespace != "lanverse-staging" {
		t.Error("Load() did not apply the expected environment override and file values")
	}
}

func TestLoadRejectsMissingDotEnvFile(t *testing.T) {
	t.Setenv("LV_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
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
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
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
	if cfg.SessionIdleTTL != 12*time.Hour || cfg.SessionAbsoluteTTL != 7*24*time.Hour {
		t.Errorf("session durations = %s/%s, want 12h/168h", cfg.SessionIdleTTL, cfg.SessionAbsoluteTTL)
	}
	if cfg.KafkaBrokers != "" {
		t.Errorf("KafkaBrokers = %q, want empty", cfg.KafkaBrokers)
	}
	if cfg.TemporalAddr != "" || cfg.TemporalNamespace != "" {
		t.Errorf("Temporal config = %q/%q, want empty", cfg.TemporalAddr, cfg.TemporalNamespace)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LV_ENV", "staging")
	t.Setenv("LV_PUBLIC_ORIGIN", "https://lanverse.example")
	t.Setenv("LV_HTTP_ADDR", ":9090")
	t.Setenv("LV_WORKER_HEALTH_ADDR", ":9091")
	t.Setenv("LV_RELAY_HEALTH_ADDR", ":9092")
	t.Setenv("LV_LOG_LEVEL", "debug")
	t.Setenv("LV_DB_DSN", "postgres://localhost/lanverse")
	t.Setenv("LV_REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("LV_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("LV_TEMPORAL_ADDR", "localhost:7233")
	t.Setenv("LV_TEMPORAL_NAMESPACE", "lanverse-staging")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := config.Config{Env: "staging", PublicOrigin: "https://lanverse.example", HTTPAddr: ":9090", WorkerHealthAddr: ":9091", RelayHealthAddr: ":9092", LogLevel: "debug", DBDSN: "postgres://localhost/lanverse", RedisURL: "redis://localhost:6379/1", SessionIdleTTL: 12 * time.Hour, SessionAbsoluteTTL: 7 * 24 * time.Hour, KafkaBrokers: "localhost:9092", TemporalAddr: "localhost:7233", TemporalNamespace: "lanverse-staging"}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadObjectStorageFromEnv(t *testing.T) {
	t.Setenv("LV_OBJECT_STORAGE_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("LV_OBJECT_STORAGE_BUCKET", "lanverse-local")
	t.Setenv("LV_OBJECT_STORAGE_ACCESS_KEY", "test-access")
	t.Setenv("LV_OBJECT_STORAGE_SECRET_KEY", "test-secret")
	t.Setenv("LV_OBJECT_STORAGE_REGION", "us-east-1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ObjectStorageEndpoint != "http://127.0.0.1:9000" || cfg.ObjectStorageBucket != "lanverse-local" || cfg.ObjectStorageAccessKey != "test-access" || cfg.ObjectStorageSecretKey != "test-secret" || cfg.ObjectStorageRegion != "us-east-1" {
		t.Fatal("Load() did not preserve object storage configuration")
	}
}

func TestLoadMediaResultDownloadPolicy(t *testing.T) {
	t.Setenv("LV_MEDIA_RESULT_ALLOWED_ORIGINS", "https://results.example.test")
	t.Setenv("LV_MEDIA_ALLOW_TEST_LOOPBACK_TLS", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MediaResultAllowedOrigins != "https://results.example.test" || !cfg.MediaAllowTestLoopbackTLS {
		t.Fatal("Load() did not preserve media result policy")
	}
	t.Setenv("LV_ENV", "prod")
	t.Setenv("LV_PUBLIC_ORIGIN", "https://lanverse.example")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("production loopback policy error = %v, want ErrInvalid", err)
	}
}

func TestLoadOTelEndpointFromEnv(t *testing.T) {
	t.Setenv("LV_OTEL_ENDPOINT", "http://127.0.0.1:4318")
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
		{name: "unknown env", key: "LV_ENV", val: "dev"},
		{name: "unknown log level", key: "LV_LOG_LEVEL", val: "verbose"},
		{name: "empty http addr", key: "LV_HTTP_ADDR", val: " "},
		{name: "empty worker health addr", key: "LV_WORKER_HEALTH_ADDR", val: " "},
		{name: "empty relay health addr", key: "LV_RELAY_HEALTH_ADDR", val: " "},
		{name: "invalid session idle", key: "LV_SESSION_IDLE_TTL", val: "never"},
		{name: "zero session idle", key: "LV_SESSION_IDLE_TTL", val: "0s"},
		{name: "submillisecond session idle", key: "LV_SESSION_IDLE_TTL", val: "1ns"},
		{name: "negative absolute", key: "LV_SESSION_ABSOLUTE_TTL", val: "-1h"},
		{name: "absolute shorter than idle", key: "LV_SESSION_ABSOLUTE_TTL", val: "1h"},
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
