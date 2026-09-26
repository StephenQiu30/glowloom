package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDotEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.env.example")
	if err := os.WriteFile(path, []byte("LV_ENV=staging\nLV_HTTP_ADDR=:9011\nLV_LOG_LEVEL=warn\nLV_DB_DSN=postgres://local/db\nLV_REDIS_URL=redis://127.0.0.1:6379/2\nLV_KAFKA_BROKERS=127.0.0.1:9092,127.0.0.1:9093\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LV_ENV_FILE", path)
	t.Setenv("LV_ENV", "prod")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9011" || cfg.LogLevel != "warn" || cfg.DBDSN != "postgres://local/db" || cfg.RedisURL != "redis://127.0.0.1:6379/2" || cfg.KafkaBrokers != "127.0.0.1:9092,127.0.0.1:9093" {
		t.Errorf("Load() = %+v, want environment override and file values", cfg)
	}
}

func TestLoadRejectsMissingDotEnvFile(t *testing.T) {
	t.Setenv("LV_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	_, err := Load()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "local" {
		t.Errorf("Env = %q, want %q", cfg.Env, "local")
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
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

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LV_ENV", "staging")
	t.Setenv("LV_HTTP_ADDR", ":9090")
	t.Setenv("LV_LOG_LEVEL", "debug")
	t.Setenv("LV_DB_DSN", "postgres://localhost/lanverse")
	t.Setenv("LV_REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("LV_KAFKA_BROKERS", "localhost:9092")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{Env: "staging", HTTPAddr: ":9090", LogLevel: "debug", DBDSN: "postgres://localhost/lanverse", RedisURL: "redis://localhost:6379/1", KafkaBrokers: "localhost:9092"}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.val)

			_, err := Load()
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Load() error = %v, want ErrInvalid", err)
			}
		})
	}
}
