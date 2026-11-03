package config_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestAgentHarnessConfigurationIsOptIn(t *testing.T) {
	t.Setenv("GLOWLOOM_AGENT_HARNESS_ENABLED", "false")
	if cfg, err := config.Load(); err != nil || cfg.AgentHarnessEnabled {
		t.Fatalf("disabled Harness config = %+v, %v", cfg, err)
	}

	t.Setenv("GLOWLOOM_AGENT_HARNESS_ENABLED", "true")
	cfg, err := config.Load()
	if err != nil || !cfg.AgentHarnessEnabled {
		t.Fatalf("enabled Harness config = %+v, %v", cfg, err)
	}
}
