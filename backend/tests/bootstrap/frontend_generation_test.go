package bootstrap_test

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
)

// This opt-in contract check serves the same handler as the application. It
// requires the locked frontend development dependencies, but no database.
func TestFrontendGenerationFromOnlineContract(t *testing.T) {
	if os.Getenv("LANVERSE_TEST_FRONTEND_GENERATION") != "true" {
		t.Skip("set LANVERSE_TEST_FRONTEND_GENERATION=true to regenerate the frontend client")
	}
	server := httptest.NewServer(bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "pnpm", "openapi")
	command.Dir = filepath.Join("..", "..", "..", "frontend")
	command.Env = append(os.Environ(), "OPENAPI_SCHEMA_URL="+server.URL+"/openapi.json")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generate frontend from online backend contract: %v\n%s", err, output)
	}
}
