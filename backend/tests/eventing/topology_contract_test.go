package eventing_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKafkaBusinessTopologyKeepsDLQsIsolatedWithoutOwningInfrastructure(t *testing.T) {
	t.Parallel()
	repositoryRoot := eventingRepositoryRoot(t)
	base, err := os.ReadFile(filepath.Join(repositoryRoot, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	goModule, err := os.ReadFile(filepath.Join(repositoryRoot, "backend", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := os.ReadFile(filepath.Join(repositoryRoot, "backend/internal/config/config.go"))
	if err != nil {
		t.Fatal(err)
	}
	baseText, configurationText := string(base), string(configuration)
	for _, required := range []string{
		"lanverse.business.script-version.published",
		"lanverse.business.script-version.dead-letter", "lanverse.business.storygraph-version.published",
		"lanverse.business.storygraph-version.dead-letter",
	} {
		if !strings.Contains(configurationText, required) {
			t.Errorf("Kafka business topology is missing %q", required)
		}
	}
	if !strings.Contains(configurationText, `defaultKafkaConsumerGroup = "lanverse.search-projector"`) {
		t.Error("Backend service topology is missing the Event Runtime consumer group")
	}
	if strings.Contains(strings.ToLower(baseText+configurationText), "command-topic") ||
		strings.Contains(strings.ToLower(baseText+configurationText), ".command.") {
		t.Fatal("Kafka command topic must not exist")
	}
	if strings.Contains(baseText, "\n  kafka:") {
		t.Fatal("service Compose must reuse the already-running Kafka environment")
	}
	if !strings.Contains(string(goModule), "github.com/twmb/franz-go v1.21.6") {
		t.Fatal("franz-go must remain pinned to the accepted version")
	}
}

func eventingRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve eventing test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
