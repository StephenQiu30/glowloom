package observability_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPerCommitCIDoesNotOwnELKResourcesOrFaultJourneys(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	ci := readText(t, filepath.Join(root, ".github", "workflows", "ci.yml"))
	for _, forbidden := range []string{
		"compose.dependencies.yml", "logstash", "kibana", "elasticsearch",
		"_index_template", "_ilm/policy", "environment_compose", "create_project_during_fault",
	} {
		if strings.Contains(strings.ToLower(ci), strings.ToLower(forbidden)) {
			t.Errorf("per-commit CI still owns ELK environment acceptance via %q", forbidden)
		}
	}
}

func TestELKRuntimeUsesDirectLogstashTransportWithoutOwningInfrastructure(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	base := readText(t, filepath.Join(root, "docker-compose.yml"))
	main := readText(t, filepath.Join(root, "backend", "cmd", "main.go"))
	combined := base + main

	for _, required := range []string{"LOGSTASH_ADDRESS:", "telemetry.NewLogstashLogger("} {
		if !strings.Contains(combined, required) {
			t.Errorf("ELK runtime contract is missing %q", required)
		}
	}
	for _, relativePath := range []string{
		"backend/tests/search/elasticsearch_integration_test.go",
		"backend/tests/search/adapter/gormdb/persistence_integration_test.go",
		"backend/tests/search/adapter/gormdb/kafka_elasticsearch_integration_test.go",
	} {
		source := readText(t, filepath.Join(root, relativePath))
		for _, required := range []string{"ScriptAlias: formalScriptSearchAlias", "StoryGraphAlias: formalStoryGraphSearchAlias"} {
			if !strings.Contains(source, required) {
				t.Errorf("%s does not verify the formal Elasticsearch alias %q", relativePath, required)
			}
		}
		for _, forbidden := range []string{"lanverse-test-", "lanverse-pg-search-", "lanverse-kafka-search-", "deleteSearchIndices("} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s still creates or removes per-test Elasticsearch indices via %q", relativePath, forbidden)
			}
		}
	}
	for _, forbidden := range []string{
		"filebeat", "lanverse.logs.application", "lanverse.logs.application.dead-letter",
		"lanverse.logs-indexer", "KAFKA_LOGSTASH", "user_logstash",
	} {
		if strings.Contains(strings.ToLower(combined), strings.ToLower(forbidden)) {
			t.Errorf("direct Logstash runtime still contains obsolete %q", forbidden)
		}
	}
	for _, environmentService := range []string{"elasticsearch", "logstash", "kibana"} {
		if strings.Contains(base, "\n  "+environmentService+":") {
			t.Errorf("service Compose must reuse the already-running %s environment", environmentService)
		}
	}
}

func TestSingleBackendEntrypointOwnsTheRedactingLogstashLogger(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	main := readText(t, filepath.Join(root, "backend", "cmd", "main.go"))
	if !strings.Contains(main, "telemetry.NewLogstashLogger(") ||
		!strings.Contains(main, `os.Stdout, "lanverse-backend"`) {
		t.Error("the single Backend entrypoint does not own the shared Logstash logger")
	}
	if strings.Contains(main, "slog.NewJSONHandler") {
		t.Error("the Backend entrypoint bypasses the redacting logger")
	}
	for _, path := range []string{
		filepath.Join(root, "backend", "internal", "bootstrap", "api_process.go"),
		filepath.Join(root, "backend", "internal", "bootstrap", "workflow_process.go"),
		filepath.Join(root, "backend", "internal", "bootstrap", "event_process.go"),
	} {
		if strings.Contains(readText(t, path), "telemetry.NewLogger") {
			t.Errorf("%s creates a second runtime logger", path)
		}
	}
}

func TestELKEnvironmentResourcesRemainOutsideBackendStartup(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, path := range []string{"backend/Dockerfile", "docker-compose.yml"} {
		source := readText(t, filepath.Join(root, path))
		for _, forbidden := range []string{"elasticsearch-init", "kibana-init", "ELASTICSEARCH_INIT_", "KIBANA_USERNAME", "KIBANA_PASSWORD"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s still owns ELK management through %q", path, forbidden)
			}
		}
	}
	for _, path := range []string{"elasticsearch/init.sh", "kibana/init.sh"} {
		if _, err := os.Stat(filepath.Join(root, "backend/observability", path)); !os.IsNotExist(err) {
			t.Errorf("ELK initialization script must be removed: %s (stat: %v)", path, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve observability test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
