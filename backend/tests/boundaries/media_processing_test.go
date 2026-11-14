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

func TestMediaProcessorHasNoBusinessStorageOrOtherDomainDependencies(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "./cmd/media")
	cmd.Dir = filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Media dependency closure: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	allowed := map[string]bool{
		"cmd/media":                                    true,
		"internal/app/mediaruntime":                    true,
		"internal/contract/gen/media":                  true,
		"internal/media/processing/application":        true,
		"internal/media/processing/adapter/ffmpeg":     true,
		"internal/media/processing/adapter/grpcserver": true,
		"internal/platform/serviceauth":                true,
	}
	for _, pkg := range strings.Fields(string(output)) {
		if strings.HasPrefix(pkg, module) && !allowed[strings.TrimPrefix(pkg, module)] {
			t.Errorf("Media processor imports business state: %s", pkg)
		}
		for _, forbidden := range []string{"database/sql", "gorm.io/", "github.com/jackc/pgx/", "github.com/redis/", "github.com/twmb/franz-go/", "github.com/minio/"} {
			if strings.HasPrefix(pkg, forbidden) {
				t.Errorf("Media processor imports a storage driver: %s", pkg)
			}
		}
	}
}
