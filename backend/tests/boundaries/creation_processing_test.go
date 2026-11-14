package boundaries_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
)

func TestCreationProcessorHasNoStorageMediaOrOtherDomainDependencies(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "./cmd/creation")
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Creation dependency closure: %v\n%s", err, output)
	}
	const module = "github.com/StephenQiu30/glowloom-server/backend/"
	allowed := map[string]bool{"cmd/creation": true, "internal/app/creationruntime": true, "internal/contract/gen/creation": true, "internal/creation/document/application": true, "internal/creation/document/adapter/native": true, "internal/creation/document/adapter/grpcserver": true, "internal/script/domain": true, "internal/platform/serviceauth": true}
	for _, pkg := range strings.Fields(string(output)) {
		if strings.HasPrefix(pkg, module) && !allowed[strings.TrimPrefix(pkg, module)] {
			t.Errorf("Creation processor imports business state: %s", pkg)
		}
		// google/uuid implements database/sql/driver.Value, but does not import
		// database/sql or own a connection. Reject actual clients, not interfaces.
		if pkg == "database/sql" {
			t.Errorf("Creation imports SQL connections: %s", pkg)
		}
		for _, forbidden := range []string{"gorm.io/", "github.com/jackc/pgx/", "github.com/redis/", "github.com/twmb/franz-go/", "github.com/minio/"} {
			if strings.HasPrefix(pkg, forbidden) {
				t.Errorf("Creation imports a storage driver: %s", pkg)
			}
		}
	}
	for _, path := range []string{"internal/app/script/script_wiring.go", "internal/app/agent/media_references.go"} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "internal/script/adapter/extract\"") {
			t.Errorf("production path embeds a local parser: %s", path)
		}
	}
}

func TestCreationContractUsesOneUnversionedDomainNamespace(t *testing.T) {
	file := pb.File_glowloom_creation_extraction_proto
	if string(file.Package()) != "glowloom.creation" || file.Path() != "glowloom/creation/extraction.proto" {
		t.Fatalf("inconsistent contract namespace: %s %s", file.Package(), file.Path())
	}
	service := file.Services().ByName("Extraction")
	if service == nil || string(service.FullName()) != "glowloom.creation.Extraction" || pb.Extraction_Extract_FullMethodName != "/glowloom.creation.Extraction/Extract" {
		t.Fatal("inconsistent generated service identity")
	}
}
