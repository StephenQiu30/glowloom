package boundaries_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIndependentServicesOwnTheirEntrypoints(t *testing.T) {
	root := serviceLayoutRoot(t)
	for _, service := range []string{"gateway", "creation", "media"} {
		t.Run(service, func(t *testing.T) {
			for _, name := range []string{"main.go", "app", "Dockerfile"} {
				if _, err := os.Stat(filepath.Join(root, service, name)); err != nil {
					t.Errorf("service implementation must own %s/%s: %v", service, name, err)
				}
			}
			for _, old := range []string{filepath.Join("cmd", service), service + ".Dockerfile"} {
				if _, err := os.Stat(filepath.Join(root, old)); !os.IsNotExist(err) {
					t.Errorf("retired service location still exists: %s (error: %v)", old, err)
				}
			}
		})
	}
}

func TestBusinessPackagesBelongToTheirService(t *testing.T) {
	root := serviceLayoutRoot(t)
	groups := map[string][]string{
		"identity":  {"domain", "application", "adapter"},
		"creation":  {"document", "workspace", "canvas", "script", "bible", "subject", "prompt"},
		"media":     {"domain", "application", "adapter", "processing", "mediatool"},
		"execution": {"operation", "catalog", "billing"},
	}
	for service, packages := range groups {
		for _, pkg := range packages {
			if _, err := os.Stat(filepath.Join(root, service, pkg)); err != nil {
				t.Errorf("missing service-owned implementation %s/%s: %v", service, pkg, err)
			}
		}
	}
	for _, name := range []string{"identity", "creation", "gateway", "workspace", "canvas", "script", "bible", "subject", "prompt", "media", "mediatool", "operation", "catalog", "billing"} {
		if _, err := os.Stat(filepath.Join(root, "internal", name)); !os.IsNotExist(err) {
			t.Errorf("business implementation remains in retired location internal/%s: %v", name, err)
		}
	}
	for _, name := range []string{"gateway", "creationruntime", "mediaruntime"} {
		if _, err := os.Stat(filepath.Join(root, "internal", "app", name)); !os.IsNotExist(err) {
			t.Errorf("independent lifecycle remains in legacy app/%s: %v", name, err)
		}
	}
}

func TestServiceLayoutKeepsOneModuleAndCanonicalSharedSources(t *testing.T) {
	root := serviceLayoutRoot(t)
	for _, name := range []string{"go.mod", "go.sum", "cmd/glowloom/main.go", "api/proto/glowloom", "internal/contract/gen", "internal/platform", "db/schema.sql", "db/data.sql", "tests"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing canonical source %s: %v", name, err)
		}
	}
	for _, service := range []string{"gateway", "identity", "creation", "media", "execution"} {
		err := filepath.WalkDir(filepath.Join(root, service), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && (entry.Name() == "go.mod" || entry.Name() == "go.sum") {
				t.Errorf("directory normalization must not duplicate module definitions: %s", path)
			}
			if entry.IsDir() && entry.Name() == "cmd" {
				if service == "creation" && path == filepath.Join(root, "creation", "cmd") {
					checkCreationPrograms(t, root, path)
				} else {
					t.Errorf("service must not wrap its only entrypoint in another cmd directory: %s", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".dockerignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"gateway", "identity", "creation", "media", "execution"} {
		if !strings.Contains(string(ignore), "!"+service+"/\n") {
			t.Errorf("module build context excludes service implementation: %s", service)
		}
	}
}

// Creation has two real programs with separate dependency closures. The pure
// document processor keeps its original entrypoint and dependency gate.
func checkCreationPrograms(t *testing.T, root, commandDir string) {
	t.Helper()
	for _, name := range []string{"creation/main.go", "creation/app", "creation/cmd/workspace/main.go", "creation/workspace/app", "creation/workspace/Dockerfile"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("Creation must retain both actual programs: %s: %v", name, err)
		}
	}
	programs, err := os.ReadDir(commandDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(programs) != 1 || !programs[0].IsDir() || programs[0].Name() != "workspace" {
		t.Error("Creation cmd contains an unreviewed program; only the independent workspace process is accepted")
	}
}

func serviceLayoutRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
}
