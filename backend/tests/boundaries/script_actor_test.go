package boundaries_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This slice checks Script's actor contract; its Media ownership ports have a
// separate migration and must not be reported as a complete executable closure.
func TestScriptOwnsActorAndProjectPorts(t *testing.T) {
	root := filepath.Join(serviceLayoutRoot(t), "creation", "script")
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
			for _, owner := range []string{"identity", "internal/app"} {
				prefix := "github.com/StephenQiu30/glowloom-server/backend/" + owner
				if target == prefix || strings.HasPrefix(target, prefix+"/") {
					t.Errorf("Script must consume its own actor port: %s imports %s", path, target)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
