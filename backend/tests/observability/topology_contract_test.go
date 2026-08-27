package observability_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestELKTopologyPinsIndependentKafkaIdentityTopicGroupDLQAndIndex(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	base := readText(t, filepath.Join(root, "docker-compose.yml"))
	environment := readText(t, filepath.Join(root, "docker-compose-env.yml"))
	production := readText(t, filepath.Join(root, "docker-compose-prod.yml"))
	filebeat := readText(t, filepath.Join(root, "backend", "observability", "filebeat", "filebeat.yml"))
	kafkaInit := readText(t, filepath.Join(root, "backend", "observability", "kafka", "init.sh"))
	logstash := readText(t, filepath.Join(root, "backend", "observability", "logstash", "pipeline", "lanverse.conf"))
	template := readText(t, filepath.Join(root, "backend", "observability", "logstash", "template", "lanverse-logs-template.json"))
	combined := base + environment + production + filebeat + kafkaInit + logstash + template

	for _, required := range []string{
		"docker.elastic.co/beats/filebeat:9.4.4",
		"docker.elastic.co/logstash/logstash:9.4.4",
		"docker.elastic.co/kibana/kibana:9.4.4",
		"lanverse.logs.application.v1",
		"lanverse.logs.application.dlq.v1",
		"lanverse.logs-indexer.v1",
		"lanverse-logs-application-v1-*",
		"lanverse.log.v1",
		"KAFKA_USERNAME: event_worker",
		"KAFKA_FILEBEAT_USERNAME: filebeat",
		"KAFKA_LOGSTASH_USERNAME: logstash",
		"KAFKA_AUTHORIZER_CLASS_NAME: org.apache.kafka.metadata.authorizer.StandardAuthorizer",
		"grant_topic filebeat \"${log_topic}\" --operation Write --operation Describe",
		"grant_topic logstash \"${log_topic}\" --operation Read --operation Describe",
		"grant_group logstash lanverse.logs-indexer.v1",
		"create_topic \"${log_topic}\" 259200000",
		"create_topic \"${log_dlq}\" 1209600000",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("ELK topology is missing %q", required)
		}
	}
	if strings.Count(base, "lanverse_log_collect:") != 3 ||
		!strings.Contains(filebeat, "docker.container.labels.lanverse_log_collect: \"true\"") {
		t.Error("Filebeat must collect only the three explicitly labelled application services")
	}
	for _, businessName := range []string{
		"lanverse.business.script-version.v1",
		"lanverse.business.storygraph-version.v1",
		"lanverse.search-projector.v1",
		"lanverse-script-search-v1",
		"lanverse-storygraph-search-v1",
	} {
		if strings.Contains(filebeat+logstash+template, businessName) {
			t.Errorf("log pipeline references business transport or index %q", businessName)
		}
	}
	if strings.Contains(environment+production, "User:ANONYMOUS") ||
		!strings.Contains(environment, "CONTROLLER:SASL_PLAINTEXT") {
		t.Error("Kafka controller traffic must not bypass the authenticated ACL boundary")
	}
}

func TestApplicationEntrypointsUseTheSingleRedactingJSONLogger(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, path := range []string{
		filepath.Join(root, "backend", "cmd", "api", "main.go"),
		filepath.Join(root, "backend", "cmd", "workflow-worker", "main.go"),
		filepath.Join(root, "backend", "cmd", "event-worker", "main.go"),
	} {
		content := readText(t, path)
		if !strings.Contains(content, "telemetry.NewLogger") {
			t.Errorf("%s does not use the shared logger", path)
		}
		if strings.Contains(content, "slog.NewJSONHandler") {
			t.Errorf("%s bypasses the redacting logger", path)
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
