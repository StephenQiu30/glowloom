package architecture_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestApplicationTestsLiveOutsideProductionSource(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryDirectory(t)
	applications := []string{"backend", "agent", "frontend"}
	for _, application := range applications {
		application := application
		t.Run(application, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(repositoryRoot, application)
			err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				relativePath, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				segments := strings.Split(filepath.ToSlash(relativePath), "/")
				if entry.IsDir() && slices.Contains([]string{".next", ".venv", "node_modules", "__pycache__"}, entry.Name()) {
					return filepath.SkipDir
				}
				if entry.IsDir() || !isTestFile(entry.Name()) {
					return nil
				}
				if len(segments) < 2 || segments[0] != "tests" {
					t.Errorf("%s test is mixed with production source; move it under %s/tests", filepath.ToSlash(relativePath), application)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("inspect %s test layout: %v", application, err)
			}
		})
	}
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go") ||
		strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") ||
		strings.HasSuffix(name, "_test.py") ||
		strings.Contains(name, ".test.") ||
		strings.Contains(name, ".spec.")
}

func repositoryDirectory(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test layout path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
