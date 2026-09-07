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
