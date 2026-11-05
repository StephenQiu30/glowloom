package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestGlowloomEnvironmentOverridesLegacyKeys(t *testing.T) {
	t.Setenv("LV_HTTP_ADDR", ":9001")
	t.Setenv("GLOWLOOM_HTTP_ADDR", ":9002")
	t.Setenv("LV_MEDIA_ALLOW_TEST_LOOPBACK_TLS", "true")
	t.Setenv("GLOWLOOM_MEDIA_ALLOW_TEST_LOOPBACK_TLS", "false")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9002" || cfg.MediaAllowTestLoopbackTLS {
		t.Fatalf("canonical configuration was not authoritative")
	}
}

func TestGlowloomEmptyKeyDoesNotReuseLegacySecretOrRequiredValue(t *testing.T) {
	t.Setenv("LV_HTTP_ADDR", ":9001")
	t.Setenv("GLOWLOOM_HTTP_ADDR", "")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("explicit empty canonical value must not fall back: %v", err)
	}
}

func TestGlowloomDotEnvUsesOnlyCanonicalFileValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.env")
	body := "GLOWLOOM_ENV=staging\nGLOWLOOM_PUBLIC_ORIGIN=https://glowloom.example\nGLOWLOOM_HTTP_ADDR=:9002\nLV_HTTP_ADDR=:9001\nLV_WORKER_HEALTH_ADDR=:9003\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", path)
	t.Setenv("LV_ENV_FILE", filepath.Join(t.TempDir(), "must-not-be-read.env"))
	t.Setenv("LV_HTTP_ADDR", ":9004")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "staging" || cfg.HTTPAddr != ":9002" || cfg.WorkerHealthAddr != ":8081" {
		t.Fatalf("canonical file values or default health address changed")
	}
	t.Setenv("GLOWLOOM_HTTP_ADDR", ":9005")
	cfg, err = config.Load()
	if err != nil || cfg.HTTPAddr != ":9005" {
		t.Fatalf("canonical environment must override canonical file: %v", err)
	}
}

func TestGlowloomIgnoresLegacyProcessKeys(t *testing.T) {
	for _, key := range []string{"GLOWLOOM_HTTP_ADDR", "GLOWLOOM_WORKER_HEALTH_ADDR"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("LV_HTTP_ADDR", ":9001")
	t.Setenv("LV_WORKER_HEALTH_ADDR", ":9003")
	cfg, err := config.Load()
	if err != nil || cfg.HTTPAddr != "127.0.0.1:8080" || cfg.WorkerHealthAddr != ":8081" {
		t.Fatalf("legacy process keys took effect: http=%q health=%q err=%v", cfg.HTTPAddr, cfg.WorkerHealthAddr, err)
	}
}
func TestGlowloomIgnoresLegacyEnvironmentFileKey(t *testing.T) {
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	if err := os.Unsetenv("GLOWLOOM_ENV_FILE"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LV_ENV_FILE", filepath.Join(t.TempDir(), "must-not-be-read.env"))
	if _, err := config.Load(); err != nil {
		t.Fatalf("legacy environment file key was read: %v", err)
	}
}
