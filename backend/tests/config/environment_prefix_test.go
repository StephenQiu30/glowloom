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

func TestGlowloomDotEnvIsPreferredAndSupportsLegacyFileValues(t *testing.T) {
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
	if cfg.Env != "staging" || cfg.HTTPAddr != ":9002" || cfg.WorkerHealthAddr != ":9003" {
		t.Fatalf("canonical file or compatible legacy file values were lost")
	}
	t.Setenv("GLOWLOOM_HTTP_ADDR", ":9005")
	cfg, err = config.Load()
	if err != nil || cfg.HTTPAddr != ":9005" {
		t.Fatalf("canonical environment must override canonical file: %v", err)
	}
}
