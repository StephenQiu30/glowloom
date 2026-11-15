package boundaries_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The actual executable's transitive closure is the boundary; a source adapter's
// direct-import check cannot detect a legacy package pulled through its neighbor.
func TestCreationWorkspaceExecutableHasNoForeignBusinessImplementation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "./creation/cmd/workspace")
	command.Dir = filepath.Join("..", "..")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("actual workspace executable dependency closure: %v\n%s", err, output)
	}
	base := "github.com/StephenQiu30/glowloom-server/backend/"
	for _, path := range strings.Fields(string(output)) {
		for _, foreign := range []string{"identity/", "media/", "execution/", "internal/app"} {
			if strings.HasPrefix(path, base+foreign) {
				t.Errorf("native Creation executable imports foreign implementation: %s", path)
			}
		}
	}
}
