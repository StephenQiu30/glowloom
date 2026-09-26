package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDotEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.env.example")
	if err := os.WriteFile(path, []byte("LV_ENV=staging\nLV_HTTP_ADDR=:9011\nLV_LOG_LEVEL=warn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LV_ENV_FILE", path)
	t.Setenv("LV_ENV", "prod")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9011" || cfg.LogLevel != "warn" {
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
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LV_ENV", "staging")
	t.Setenv("LV_HTTP_ADDR", ":9090")
	t.Setenv("LV_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{Env: "staging", HTTPAddr: ":9090", LogLevel: "debug"}
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
