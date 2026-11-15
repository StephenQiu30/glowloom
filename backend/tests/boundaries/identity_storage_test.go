package boundaries_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityStorageDoesNotCompileOtherBusinessOwners(t *testing.T) {
	command := exec.CommandContext(t.Context(), "go", "list", "-deps", "./identity/adapter/postgres")
	command.Dir = filepath.Join("..", "..")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect identity storage dependencies: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	for _, dependency := range strings.Fields(string(output)) {
		for _, owner := range []string{"creation", "media", "execution"} {
			if strings.HasPrefix(dependency, module+owner+"/") {
				t.Errorf("identity storage imports business owner %q: %s", owner, dependency)
			}
		}
		if strings.HasPrefix(dependency, module+"internal/app") {
			t.Errorf("identity storage imports legacy composition: %s", dependency)
		}
	}
}
