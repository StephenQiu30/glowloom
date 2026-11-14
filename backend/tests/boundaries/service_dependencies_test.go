package boundaries_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is a ceiling on existing coupling, not proof of completed service isolation.
// Removing an old import is allowed; adding one requires an explicit owner review.
func TestServiceImportsDoNotExpandTransitionalCoupling(t *testing.T) {
	root := serviceLayoutRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "tests/boundaries/testdata/service_dependencies.json"))
	if err != nil {
		t.Fatal(err)
	}
	var edges []string
	if err := json.Unmarshal(data, &edges); err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]bool, len(edges))
	for _, edge := range edges {
		if allowed[edge] || !strings.Contains(edge, " -> ") {
			t.Fatalf("invalid or repeated dependency in the migration baseline: %q", edge)
		}
		allowed[edge] = true
	}

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./gateway/...", "./identity/...", "./creation/...", "./media/...", "./execution/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect service imports: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		source := strings.TrimPrefix(pkg.ImportPath, module)
		owner := serviceImportOwner(source)
		if owner == "" {
			t.Fatalf("package outside the requested services: %s", pkg.ImportPath)
		}
		for _, imported := range pkg.Imports {
			if !strings.HasPrefix(imported, module) {
				continue
			}
			target := strings.TrimPrefix(imported, module)
			if target == "internal/app" || strings.HasPrefix(target, "internal/app/") {
				t.Errorf("service implementation imports the legacy composition root: %s -> %s", source, target)
				continue
			}
			otherOwner := serviceImportOwner(target)
			crossService := otherOwner != "" && otherOwner != owner
			legacyBusiness := strings.HasPrefix(target, "internal/") && !strings.HasPrefix(target, "internal/platform/") && !strings.HasPrefix(target, "internal/contract/")
			if (crossService || legacyBusiness) && !allowed[source+" -> "+target] {
				t.Errorf("unreviewed cross-owner dependency: %s -> %s", source, target)
			}
		}
	}
}

func serviceImportOwner(path string) string {
	owner, _, _ := strings.Cut(path, "/")
	switch owner {
	case "gateway", "identity", "creation", "media", "execution":
		return owner
	default:
		return ""
	}
}
