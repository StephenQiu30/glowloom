package deployment_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestComposeUsesModularMonolithTopologyWithGRPC(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(filepath.Join(repoRoot(t), "docker-compose.yml"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	services := v.GetStringMap("services")
	for _, expected := range []string{"frontend", "backend", "canvas", "agent", "workspace"} {
		if _, ok := services[expected]; !ok {
			t.Errorf("expected service %q in docker-compose.yml", expected)
		}
	}
	for _, retired := range []string{"gateway", "creation", "media", "worker", "relay"} {
		if _, ok := services[retired]; ok {
			t.Errorf("retired microservice container %q must not remain in docker-compose.yml", retired)
		}
	}
	if got := v.GetString("services.backend.environment.GLOWLOOM_CANVAS_RUNTIME_URL"); got != "grpc://canvas:4402" {
		t.Errorf("backend canvas URL = %q, want grpc://canvas:4402", got)
	}
	if got := v.GetString("services.backend.environment.GLOWLOOM_AGENT_ORIGIN"); got != "grpc://agent:4402" {
		t.Errorf("backend agent origin = %q, want grpc://agent:4402", got)
	}
}
