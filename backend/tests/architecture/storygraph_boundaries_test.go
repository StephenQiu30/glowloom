package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomainAndApplicationDoNotImportInfrastructureClients(t *testing.T) {
	t.Parallel()
	backendRoot := backendDirectory(t)
	for _, layer := range []string{"domain", "application"} {
		layer := layer
		t.Run(layer, func(t *testing.T) {
			t.Parallel()
			err := filepath.WalkDir(filepath.Join(backendRoot, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				relativePath, err := filepath.Rel(backendRoot, path)
				if err != nil {
					return err
				}
				segments := strings.Split(filepath.ToSlash(relativePath), "/")
				if entry.IsDir() || filepath.Ext(path) != ".go" || !containsSegment(segments, layer) {
					return nil
				}
				parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imported := range parsed.Imports {
					importPath, err := strconv.Unquote(imported.Path.Value)
					if err != nil {
						return err
					}
					for _, forbidden := range []string{
						"gorm.io/", "go.temporal.io/", "github.com/twmb/franz-go",
						"github.com/elastic/go-elasticsearch", "github.com/minio/minio-go",
					} {
						if strings.HasPrefix(importPath, forbidden) {
							t.Errorf("%s imports infrastructure client %s", relativePath, importPath)
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("inspect %s imports: %v", layer, err)
			}
		})
	}
}

func TestOptionalInfrastructureDependenciesRequireTheirFirstConsumer(t *testing.T) {
	t.Parallel()
	repositoryRoot := repositoryDirectory(t)
	goModule, err := os.ReadFile(filepath.Join(repositoryRoot, "backend", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	frontendPackage, err := os.ReadFile(filepath.Join(repositoryRoot, "frontend", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		dependency string
		manifest   string
		consumers  []string
	}{
		{"github.com/twmb/franz-go", string(goModule), []string{"backend/internal/eventing/adapter/kafka", "backend/cmd/event-worker"}},
		{"github.com/elastic/go-elasticsearch", string(goModule), []string{"backend/internal/search/adapter/elasticsearch", "backend/cmd/event-worker"}},
		{"@xyflow/react", string(frontendPackage), []string{"frontend/src/features/storygraph"}},
		{"@dagrejs/dagre", string(frontendPackage), []string{"frontend/src/features/storygraph"}},
	}
	for _, check := range checks {
		if !strings.Contains(check.manifest, check.dependency) {
			continue
		}
		for _, consumer := range check.consumers {
			if _, err := os.Stat(filepath.Join(repositoryRoot, consumer)); err != nil {
				t.Errorf("dependency %s exists before real consumer %s", check.dependency, consumer)
			}
		}
	}
}

func containsSegment(segments []string, expected string) bool {
	for _, segment := range segments {
		if segment == expected {
			return true
		}
	}
	return false
}
