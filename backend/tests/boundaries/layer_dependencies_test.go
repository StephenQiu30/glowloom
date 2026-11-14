package boundaries_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// This enforces the project's dependency direction, not a Go-mandated directory template.
func TestInnerPackagesDoNotImportOuterLayers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = serviceLayoutRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect package layers: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	checked := 0
	for {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		source := strings.TrimPrefix(pkg.ImportPath, module)
		parts := strings.Split(source, "/")
		domain := slices.Contains(parts, "domain")
		if !domain && !slices.Contains(parts, "application") {
			continue
		}
		checked++
		for _, dependency := range pkg.Imports {
			if !strings.HasPrefix(dependency, module) {
				continue
			}
			target := strings.TrimPrefix(dependency, module)
			layers := strings.Split(target, "/")
			outer := slices.Contains(layers, "adapter")
			if domain {
				outer = outer || slices.Contains(layers, "application") || strings.HasPrefix(target, "internal/contract/gen/")
			}
			if outer {
				t.Errorf("inner package imports an outer layer: %s -> %s", source, target)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no domain or application packages inspected")
	}
}
