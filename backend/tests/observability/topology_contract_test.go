package observability_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestELKTopologyUsesDirectLogstashTransportWithoutFilebeatOrKafkaLogTopics(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	base := readText(t, filepath.Join(root, "docker-compose.yml"))
	environment := readText(t, filepath.Join(root, "docker-compose-env.yml"))
	production := readText(t, filepath.Join(root, "docker-compose-prod.yml"))
	kafkaInit := readText(t, filepath.Join(root, "backend", "observability", "kafka", "init.sh"))
	kibanaInit := readText(t, filepath.Join(root, "backend", "observability", "kibana", "init.sh"))
	elasticsearchInit := readText(t, filepath.Join(root, "backend", "observability", "elasticsearch", "init.sh"))
	logstash := readText(t, filepath.Join(root, "backend", "observability", "logstash", "pipeline", "lanverse.conf"))
	template := readText(t, filepath.Join(root, "backend", "observability", "logstash", "template", "lanverse-logs-template.json"))
	combined := base + environment + production + kafkaInit + kibanaInit + elasticsearchInit + logstash + template

	for _, required := range []string{
		"docker.elastic.co/logstash/logstash:9.4.4",
		"docker.elastic.co/kibana/kibana:9.4.4",
		"LOGSTASH_ADDRESS:",
		"port => 5000",
		"codec => json_lines",
		"lanverse-logs-application",
		"lanverse-logs-dead-letter",
		"lanverse.log.application",
		"KAFKA_USERNAME: event_worker",
		"KAFKA_AUTHORIZER_CLASS_NAME: org.apache.kafka.metadata.authorizer.StandardAuthorizer",
		"KIBANA_USERNAME",
		"KIBANA_PASSWORD",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("ELK topology is missing %q", required)
		}
	}
	for _, required := range []string{
		"ELASTICSEARCH_INIT_USERNAME",
		"ELASTICSEARCH_INIT_PASSWORD",
		"lanverse-logs-application-write",
		"lanverse-logs-dead-letter-write",
		"index.blocks.write",
		"remove_index",
		"is_write_index",
		"--head",
		`"index.lifecycle.rollover_alias":"lanverse-logs-dead-letter"`,
		`"number_of_replicas":0`,
	} {
		if !strings.Contains(elasticsearchInit, required) {
			t.Errorf("Elasticsearch startup initializer is missing %q", required)
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
			t.Errorf("direct Logstash topology still contains obsolete %q", forbidden)
		}
	}
	for _, businessName := range []string{
		"lanverse.business.script-version.published",
		"lanverse.business.storygraph-version.published",
		"lanverse.search-projector",
		"lanverse-script-search",
		"lanverse-storygraph-search",
	} {
		if strings.Contains(logstash+template, businessName) {
			t.Errorf("log pipeline references business transport or index %q", businessName)
		}
	}
	if strings.Contains(environment+production, "User:ANONYMOUS") ||
		!strings.Contains(environment, "CONTROLLER:SASL_PLAINTEXT") {
		t.Error("Kafka controller traffic must not bypass the authenticated ACL boundary")
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
