package boundaries_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGatewayDependencyClosureDoesNotOwnBusinessData(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "./cmd/gateway")
	cmd.Dir = filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect dependency closure: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	allowed := map[string]bool{
		"cmd/gateway":               true,
		"internal/app/gateway":      true,
		"internal/gateway":          true,
		"internal/platform/httpapi": true,
		"internal/platform/log":     true,
	}
	for _, pkg := range strings.Fields(string(output)) {
		if strings.HasPrefix(pkg, module) && !allowed[strings.TrimPrefix(pkg, module)] {
			t.Errorf("gateway imports a business runtime: %s", pkg)
		}
		for _, forbidden := range []string{"gorm.io/", "github.com/jackc/pgx/", "github.com/redis/", "github.com/twmb/franz-go/"} {
			if strings.HasPrefix(pkg, forbidden) {
				t.Errorf("gateway imports a business storage driver: %s", pkg)
			}
		}
	}
}
