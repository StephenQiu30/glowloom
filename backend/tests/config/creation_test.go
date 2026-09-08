package config_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/config"
)

func TestCreationRequiresExplicitTrustedEndpointAndSeparateSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://test:test@localhost:5432/test")
	t.Setenv("CREATION_AGENT_URL", "")
	t.Setenv("CREATION_AGENT_SECRET", "")
	if value, err := config.Load(); err != nil || value.CreationAgentURL != "" {
		t.Fatalf("disabled config: %v", err)
	}
	for _, endpoint := range []string{"http://remote.example", "https://user:secret@example.invalid", "https://example.invalid/path", "https://example.invalid?query=1"} {
		t.Setenv("CREATION_AGENT_URL", endpoint)
		t.Setenv("CREATION_AGENT_SECRET", strings.Repeat("x", 32))
		if _, err := config.Load(); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	t.Setenv("CREATION_AGENT_URL", "http://127.0.0.1:8788")
	t.Setenv("CREATION_AGENT_SECRET", "short")
	if _, err := config.Load(); err == nil {
		t.Fatal("weak secret accepted")
	}
	t.Setenv("CREATION_AGENT_SECRET", strings.Repeat("x", 32))
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestDockerCreationPeerRequiresExplicitLocalNetworkPolicy(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://test:test@localhost/test")
	t.Setenv("CREATION_AGENT_SECRET", strings.Repeat("x", 32))
	t.Setenv("CREATION_DOCKER_NETWORK", "true")
	t.Setenv("CREATION_AGENT_URL", "http://agent:8787")
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://remote.example:8788", "http://agent:1234", "http://agent.evil:8787"} {
		t.Setenv("CREATION_AGENT_URL", endpoint)
		if _, err := config.Load(); err == nil {
			t.Fatal("arbitrary plaintext endpoint accepted")
		}
	}
	t.Setenv("CREATION_AGENT_URL", "http://agent:8787")
	t.Setenv("CREATION_DOCKER_NETWORK", "false")
	if _, err := config.Load(); err == nil {
		t.Fatal("implicit Docker transport accepted")
	}
}

func TestCreationRelocationRequiresValidTargetAndOrigin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://test:test@localhost:5432/test")
	t.Setenv("CREATION_AGENT_URL", "https://creation.example.test")
	t.Setenv("CREATION_AGENT_SECRET", strings.Repeat("creation-secret", 3))
	t.Setenv("CREATION_AGENT_RELOCATED_FROM", "http://127.0.0.1:8788")
	value, err := config.Load()
	if err != nil || value.CreationAgentRelocatedFrom != "http://127.0.0.1:8788" {
		t.Fatalf("relocation: %v", err)
	}
	for _, source := range []string{"invalid", "http://user:pass@localhost:8788", "http://localhost:8788/path"} {
		t.Setenv("CREATION_AGENT_RELOCATED_FROM", source)
		if _, err := config.Load(); err == nil {
			t.Fatalf("invalid relocation accepted: %s", source)
		}
	}
	t.Setenv("CREATION_AGENT_RELOCATED_FROM", "http://127.0.0.1:8788")
	t.Setenv("CREATION_AGENT_URL", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("relocation without target accepted")
	}
}
