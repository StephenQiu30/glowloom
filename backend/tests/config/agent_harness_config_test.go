package config_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestAgentHarnessConfigurationIsOptInAndRequiresSignedRuntime(t *testing.T) {
	t.Setenv("GLOWLOOM_AGENT_HARNESS_ENABLED", "false")
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_URL", "")
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_SIGNING_KEY", "")
	if cfg, err := config.Load(); err != nil || cfg.AgentHarnessEnabled {
		t.Fatalf("disabled Harness config = %+v, %v", cfg, err)
	}

	t.Setenv("GLOWLOOM_AGENT_HARNESS_ENABLED", "true")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("enabled Harness accepted missing runtime config: %v", err)
	}
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_URL", "http://runtime.internal:8020")
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_SIGNING_KEY", "0123456789abcdef0123456789abcdef")
	cfg, err := config.Load()
	if err != nil || !cfg.AgentHarnessEnabled || cfg.AgentRuntimeURL != "http://runtime.internal:8020" || len(cfg.AgentRuntimeSigningKey) != 32 {
		t.Fatalf("valid Harness config = %+v, %v", cfg, err)
	}

	for _, runtimeURL := range []string{"http://user@runtime.internal", "http://runtime.internal/base", "http://runtime.internal?secret=1", "file:///runtime"} {
		t.Setenv("GLOWLOOM_AGENT_RUNTIME_URL", runtimeURL)
		if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
			t.Errorf("accepted invalid runtime URL %q: %v", runtimeURL, err)
		}
	}
}

func TestAgentHarnessRuntimeSettingsCannotBePartiallyConfigured(t *testing.T) {
	t.Setenv("GLOWLOOM_AGENT_HARNESS_ENABLED", "false")
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_URL", "http://runtime.internal:8020")
	t.Setenv("GLOWLOOM_AGENT_RUNTIME_SIGNING_KEY", "")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("partially configured runtime accepted: %v", err)
	}
}
