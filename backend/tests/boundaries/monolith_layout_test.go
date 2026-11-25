package boundaries_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func serviceLayoutRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestMonolithLayoutHasSingleEntrypointAndInternalDomains(t *testing.T) {
	root := serviceLayoutRoot(t)

	for _, mustExist := range []string{
		"go.mod",
		"buf.yaml",
		"buf.gen.yaml",
		"cmd/glowloom/main.go",
		"internal/app/compose.go",
		"internal/creation",
		"internal/execution",
		"internal/identity",
		"internal/media",
		"internal/agent",
		"internal/gen/canvas",
		"internal/gen/agent",
		"db/schema.sql",
		"db/data.sql",
	} {
		if _, err := os.Stat(filepath.Join(root, mustExist)); err != nil {
			t.Errorf("missing required monolith path %s: %v", mustExist, err)
		}
	}

	for _, mustNotExist := range []string{
		"gateway",
		"creation",
		"execution",
		"identity",
		"media",
		"internal/platform/writefence",
		"internal/platform/artifactrpc",
		"internal/platform/serviceauth",
		"internal/platform/temporalconn",
		"internal/app/wire.go",
		"internal/app/wire_gen.go",
		"internal/app/codexruntime",
		"internal/app/aitrial",
		"tests/temporalconn",
	} {
		if _, err := os.Stat(filepath.Join(root, mustNotExist)); !os.IsNotExist(err) {
			t.Errorf("retired microservice, legacy compat, or top-level domain path still present: %s", mustNotExist)
		}
	}

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if strings.Contains(string(goMod), "github.com/google/wire") {
		t.Errorf("archived github.com/google/wire must not remain in go.mod")
	}
}
