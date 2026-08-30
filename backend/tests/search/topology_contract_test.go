package search_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSearchTopologyPinsElasticsearchAndKeepsAPIAvailableDuringIndexOutage(t *testing.T) {
	t.Parallel()
	repositoryRoot := searchRepositoryRoot(t)
	base := mustReadSearchFile(t, filepath.Join(repositoryRoot, "docker-compose.yml"))
	environment := mustReadSearchFile(t, filepath.Join(repositoryRoot, "docker-compose-env.yml"))
	module := mustReadSearchFile(t, filepath.Join(repositoryRoot, "backend", "go.mod"))
	workflow := mustReadSearchFile(t, filepath.Join(repositoryRoot, ".github", "workflows", "ci.yml"))
	for _, required := range []string{
		"docker.elastic.co/elasticsearch/elasticsearch:9.4.4", "discovery.type: single-node",
		`xpack.security.enabled: "false"`, "ELASTICSEARCH_SCRIPT_ALIAS", "ELASTICSEARCH_STORYGRAPH_ALIAS",
	} {
		if !strings.Contains(base+environment, required) {
			t.Errorf("Search runtime topology is missing %q", required)
		}
	}
	if !strings.Contains(module, "github.com/elastic/go-elasticsearch/v9 v9.4.3") {
		t.Fatal("official Elasticsearch Go client must remain pinned to the accepted version")
	}
	for _, required := range []string{`["version"]["number"]`, "_alias/lanverse-script-search", "backend event runtime stayed ready while Elasticsearch was unavailable"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("real CI Search proof is missing %q", required)
		}
	}
	if strings.Contains(environment, "\n  backend:") {
		t.Fatal("environment Compose must not own the Backend service")
	}
	backendStart := strings.Index(base, "\n  backend:")
	if backendStart < 0 {
		t.Fatal("service Compose is missing Backend")
	}
	backendBlock := base[backendStart:]
	if backendEnd := strings.Index(backendBlock, "\nnetworks:"); backendEnd > 0 {
		backendBlock = backendBlock[:backendEnd]
	}
	if strings.Contains(backendBlock, "depends_on:") {
		t.Fatal("Backend service startup must reuse an already-running environment")
	}
}

func TestElasticsearchReindexUsesFixedFormalBlueGreenBackings(t *testing.T) {
	t.Parallel()
	repositoryRoot := searchRepositoryRoot(t)
	source := mustReadSearchFile(t, filepath.Join(repositoryRoot, "backend", "internal", "search", "adapter", "elasticsearch", "index.go"))
	for _, required := range []string{`alias + "-blue"`, `alias + "-green"`, "deleteInactiveBacking("} {
		if !strings.Contains(source, required) {
			t.Errorf("formal Elasticsearch reindex is missing %q", required)
		}
	}
	if strings.Contains(source, "uuid.NewString()") {
		t.Error("formal Elasticsearch reindex still creates one random backing index per execution")
	}
}

func searchRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve Search test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func mustReadSearchFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
