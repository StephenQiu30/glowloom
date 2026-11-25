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

func TestPromptOwnsItsActorAndDoesNotImportIdentityImplementation(t *testing.T) {
	root := serviceLayoutRoot(t)
	for _, directory := range []string{"internal/creation/prompt/application", "internal/creation/prompt/adapter/postgres", "internal/creation/prompt/adapter/http"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
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
				if strings.HasPrefix(target, "github.com/StephenQiu30/glowloom-server/backend/internal/identity/") {
					t.Errorf("Prompt owns its actor and private authority boundary; implementation import remains in %s: %s", path, target)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
