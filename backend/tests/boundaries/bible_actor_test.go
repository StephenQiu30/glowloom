package boundaries_test

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Bible must consume its own actor/project ports. Transitional policy bridges
// remain in the composition root, where their transaction ownership is explicit.
func TestBibleOwnsActorAndProjectPorts(t *testing.T) {
	root := filepath.Join(serviceLayoutRoot(t), "internal", "creation", "bible")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			target, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			for _, forbidden := range []string{"internal/identity", "internal/creation/workspace", "internal/app"} {
				prefix := "github.com/StephenQiu30/glowloom-server/backend/" + forbidden
				if target == prefix || strings.HasPrefix(target, prefix+"/") {
					t.Errorf("Bible must consume local actor/project ports: %s imports %s", path, target)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A local port must not regain another owner's implementation indirectly.
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "./internal/creation/bible/...")
	command.Dir = serviceLayoutRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Bible executable dependencies: %v\n%s", err, output)
	}
	for dependency := range strings.FieldsSeq(string(output)) {
		for _, owner := range []string{"internal/identity", "internal/execution", "internal/media", "internal/app"} {
			prefix := "github.com/StephenQiu30/glowloom-server/backend/" + owner
			if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
				t.Errorf("Bible executable dependency crosses owner boundary: %s", dependency)
			}
		}
	}
}
