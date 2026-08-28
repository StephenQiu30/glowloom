package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendHasOneRuntimeEntrypoint(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryDirectory(t)
	commandRoot := filepath.Join(repositoryRoot, "backend", "cmd")
	entries, err := os.ReadDir(commandRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].IsDir() || entries[0].Name() != "main.go" {
		t.Fatalf("backend/cmd must contain only main.go, got %v", entryNames(entries))
	}

	mainSource := readArchitectureFile(t, filepath.Join(commandRoot, "main.go"))
	for _, runtime := range []string{"bootstrap.RunAPI(logger)", "bootstrap.RunWorkflowWorker(logger)", "bootstrap.RunEventWorker(logger)"} {
		if !strings.Contains(mainSource, runtime) {
			t.Errorf("single Backend entrypoint does not start %s", runtime)
		}
	}

	dockerfile := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "Dockerfile"))
	for _, forbidden := range []string{"lanverse-api", "lanverse-workflow-worker", "lanverse-event-worker"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("Backend image still contains obsolete binary %q", forbidden)
		}
	}
	if !strings.Contains(dockerfile, "ENTRYPOINT [\"/usr/local/bin/lanverse\"]") {
		t.Error("Backend image must start the single lanverse binary")
	}

	apiSource := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "internal", "bootstrap", "api_process.go"))
	for _, required := range []string{
		"generationhttp.NewProviderBindingHandler(",
		"providerBindingHandler.Register(mux)",
	} {
		if !strings.Contains(apiSource, required) {
			t.Errorf("single Backend API composition is missing %q", required)
		}
	}
	if strings.Contains(apiSource, "configuration.RunwareAPIKey") {
		t.Error("Provider Binding API must not receive the Runware secret")
	}

	playwrightConfig := readArchitectureFile(t, filepath.Join(repositoryRoot, "frontend", "playwright.config.ts"))
	if !strings.Contains(playwrightConfig, "go run ./cmd") || strings.Contains(playwrightConfig, "go run ./cmd/api") {
		t.Error("Playwright must start the single Backend entrypoint")
	}

	compose := readArchitectureFile(t, filepath.Join(repositoryRoot, "docker-compose.yml"))
	for _, forbidden := range []string{"\n  workflow-worker:", "\n  event-worker:"} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("project Compose still declares obsolete service %q", strings.TrimSpace(forbidden))
		}
	}
	for _, environmentService := range []string{"postgres", "minio", "temporal", "kafka", "elasticsearch", "filebeat", "logstash", "kibana"} {
		if strings.Contains(compose, "\n  "+environmentService+":") {
			t.Errorf("service Compose owns environment service %q", environmentService)
		}
	}
	environmentCompose := readArchitectureFile(t, filepath.Join(repositoryRoot, "docker-compose-env.yml"))
	for _, applicationService := range []string{"backend", "frontend"} {
		if strings.Contains(environmentCompose, "\n  "+applicationService+":") {
			t.Errorf("environment Compose owns application service %q", applicationService)
		}
	}
	for _, required := range []string{"\n  temporal:", "\n  logstash:"} {
		if !strings.Contains(environmentCompose, required) {
			t.Errorf("environment Compose is missing %q", strings.TrimSpace(required))
		}
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func readArchitectureFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
