package eventing_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKafkaTopologyPinsKRaftBusinessDLQIsolationWithoutCommandTopics(t *testing.T) {
	t.Parallel()
	repositoryRoot := eventingRepositoryRoot(t)
	base, err := os.ReadFile(filepath.Join(repositoryRoot, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := os.ReadFile(filepath.Join(repositoryRoot, "docker-compose-env.yml"))
	if err != nil {
		t.Fatal(err)
	}
	goModule, err := os.ReadFile(filepath.Join(repositoryRoot, "backend", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	kafkaInit, err := os.ReadFile(filepath.Join(repositoryRoot, "backend", "observability", "kafka", "init.sh"))
	if err != nil {
		t.Fatal(err)
	}
	baseText, environmentText, kafkaInitText := string(base), string(environment), string(kafkaInit)
	for _, required := range []string{
		"apache/kafka:4.3.1", "lanverse.business.script-version.published",
		"lanverse.business.script-version.dead-letter", "lanverse.business.storygraph-version.published",
		"lanverse.business.storygraph-version.dead-letter", "604800000", "2592000000",
	} {
		if !strings.Contains(baseText+environmentText+kafkaInitText, required) {
			t.Errorf("Kafka service or environment topology is missing %q", required)
		}
	}
	for _, required := range []string{
		"KAFKA_PROCESS_ROLES: broker,controller", `KAFKA_AUTO_CREATE_TOPICS_ENABLE: "false"`,
		"CLUSTER_ID: 4L6g3nShT-eMCtK--X86sw",
	} {
		if !strings.Contains(environmentText, required) {
			t.Errorf("Kafka runtime topology is missing %q", required)
		}
	}
	if !strings.Contains(baseText, "KAFKA_CONSUMER_GROUP: lanverse.search-projector") {
		t.Error("Backend service topology is missing the Event Runtime consumer group")
	}
	if strings.Contains(strings.ToLower(baseText+environmentText), "command-topic") ||
		strings.Contains(strings.ToLower(baseText+environmentText), ".command.") {
		t.Fatal("Kafka command topic must not exist")
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
