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

func TestMonolithLayoutHasSingleEntrypointAndNoRetiredMicroservices(t *testing.T) {
	root := serviceLayoutRoot(t)

	for _, mustExist := range []string{
		"go.mod",
		"cmd/glowloom/main.go",
		"internal/app/compose.go",
		"db/schema.sql",
		"db/data.sql",
	} {
		if _, err := os.Stat(filepath.Join(root, mustExist)); err != nil {
			t.Errorf("missing required monolith file %s: %v", mustExist, err)
		}
	}

	for _, mustNotExist := range []string{
		"gateway",
		"creation/main.go",
		"creation/app",
		"creation/Dockerfile",
		"creation/cmd",
		"media/main.go",
		"media/app",
		"media/Dockerfile",
		"identity/main.go",
		"identity/app",
		"internal/platform/writefence",
		"internal/platform/artifactrpc",
		"internal/app/wire.go",
		"internal/app/wire_gen.go",
	} {
		if _, err := os.Stat(filepath.Join(root, mustNotExist)); !os.IsNotExist(err) {
			t.Errorf("retired microservice or archived Wire path still present: %s", mustNotExist)
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
