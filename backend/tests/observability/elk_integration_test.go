package observability_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type elkTestEnvironment struct {
	brokers             []string
	logTopic            string
	logDLQTopic         string
	businessTopic       string
	adminPassword       string
	eventWorkerPassword string
	filebeatPassword    string
	logstashPassword    string
	elasticsearchURL    string
	kibanaURL           string
}

func TestRealELKPipelineIndexesCorrelationsRedactsSecretsAndRoutesInvalidLogsToDLQ(t *testing.T) {
	environment := requireELKEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	dlqConsumer := newKafkaClient(t, environment, "admin", environment.adminPassword,
		"lanverse.logs.acceptance."+uuid.NewString(), []string{environment.logDLQTopic}, kgo.NewOffset().AtEnd())
	primeKafkaConsumer(dlqConsumer)

	filebeatProducer := newKafkaClient(t, environment, "filebeat", environment.filebeatPassword, "", nil, kgo.Offset{})
	requestID := uuid.NewString()
	correlations := map[string]string{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "request_id": requestID,
		"workflow_run_id": uuid.NewString(), "node_run_id": uuid.NewString(), "node_id": "reconcile_story",
		"task_id": uuid.NewString(), "decision_id": uuid.NewString(), "provider_job_id": uuid.NewString(),
		"receipt_id": uuid.NewString(), "error_code": "ci_pipeline_verification",
	}
	record := map[string]any{
		"@timestamp": time.Now().UTC().Format(time.RFC3339Nano), "schema_version": "lanverse.log.v1",
		"service": "lanverse-ci", "environment": "test", "level": "INFO", "event": "elk_pipeline_verification",
		"msg": "ELK pipeline verification", "status_code": 503, "duration_ms": 12.5,
		"access_token": "secret-access-token-value", "prompt": "secret-complete-prompt-value",
		"private_url": "https://private.example.test/object?signature=secret-url-value",
	}
	for key, value := range correlations {
		record[key] = value
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err = filebeatProducer.ProduceSync(ctx, &kgo.Record{Topic: environment.logTopic, Key: []byte(requestID), Value: encoded}).FirstErr(); err != nil {
		t.Fatal(err)
	}

	indexed := waitForIndexedLog(t, ctx, environment.elasticsearchURL, requestID)
	for key, expected := range correlations {
		if actual, _ := indexed[key].(string); actual != expected {
			t.Errorf("indexed correlation %s = %q, want %q", key, actual, expected)
		}
	}
	indexedJSON, err := json.Marshal(indexed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"access_token", "prompt", "private_url", "secret-access-token-value",
		"secret-complete-prompt-value", "secret-url-value", "private.example.test",
	} {
		if bytes.Contains(indexedJSON, []byte(forbidden)) {
			t.Errorf("Elasticsearch log document leaked %q: %s", forbidden, indexedJSON)
		}
	}

	invalidSecret := "invalid-log-secret-" + uuid.NewString()
	invalid := []byte(`{"schema_version":"unsupported","token":"` + invalidSecret + `"}`)
	if err = filebeatProducer.ProduceSync(ctx, &kgo.Record{Topic: environment.logTopic, Value: invalid}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	dlqRecord := waitForLogDLQ(t, ctx, dlqConsumer)
	var deadLetter map[string]any
	if err = json.Unmarshal(dlqRecord.Value, &deadLetter); err != nil {
		t.Fatal(err)
	}
	if deadLetter["schema_version"] != "lanverse.log.dlq.v1" || deadLetter["error_code"] != "invalid_log_record" {
		t.Fatalf("invalid log did not use the bounded DLQ schema: %s", dlqRecord.Value)
	}
	rawHash, _ := deadLetter["raw_sha256"].(string)
	if len(rawHash) != sha256.Size*2 {
		t.Fatalf("invalid log DLQ hash = %q", rawHash)
	}
	if _, err = hex.DecodeString(rawHash); err != nil || bytes.Contains(dlqRecord.Value, []byte(invalidSecret)) {
		t.Fatalf("invalid log DLQ retained raw sensitive content: %s", dlqRecord.Value)
	}

	requestJSON(t, http.MethodGet, environment.kibanaURL+"/api/data_views/data_view/lanverse-logs-application-v1", nil)
}

func TestRealKafkaACLsRejectCrossChainWrites(t *testing.T) {
	environment := requireELKEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filebeat := newKafkaClient(t, environment, "filebeat", environment.filebeatPassword, "", nil, kgo.Offset{})
	errorValue := filebeat.ProduceSync(ctx, &kgo.Record{
		Topic: environment.businessTopic, Value: []byte(`{"schema_version":"lanverse.log.v1"}`),
	}).FirstErr()
	if !errors.Is(errorValue, kerr.TopicAuthorizationFailed) {
		t.Fatalf("Filebeat identity crossed into the business topic: %v", errorValue)
	}

	eventWorker := newKafkaClient(t, environment, "event_worker", environment.eventWorkerPassword, "", nil, kgo.Offset{})
	errorValue = eventWorker.ProduceSync(ctx, &kgo.Record{
		Topic: environment.logTopic, Value: []byte(`{"schema_version":"lanverse.event.v1"}`),
	}).FirstErr()
	if !errors.Is(errorValue, kerr.TopicAuthorizationFailed) {
		t.Fatalf("event-worker identity crossed into the log topic: %v", errorValue)
	}

	logstash := newKafkaPartitionConsumer(t, environment, "logstash", environment.logstashPassword, environment.businessTopic)
	fetches := logstash.PollFetches(ctx)
	denied := false
	for _, fetchError := range fetches.Errors() {
		if errors.Is(fetchError.Err, kerr.TopicAuthorizationFailed) {
			denied = true
		}
	}
	if !denied {
		t.Fatalf("Logstash identity crossed into the business topic: %v", fetches.Errors())
	}
}

func requireELKEnvironment(t *testing.T) elkTestEnvironment {
	t.Helper()
	rawBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	elasticsearchURL := strings.TrimRight(os.Getenv("LANVERSE_TEST_ELASTICSEARCH_URL"), "/")
	kibanaURL := strings.TrimRight(os.Getenv("LANVERSE_TEST_KIBANA_URL"), "/")
	adminPassword := os.Getenv("LANVERSE_TEST_KAFKA_ADMIN_PASSWORD")
	eventPassword := os.Getenv("LANVERSE_TEST_KAFKA_EVENT_WORKER_PASSWORD")
	filebeatPassword := os.Getenv("LANVERSE_TEST_KAFKA_FILEBEAT_PASSWORD")
	logstashPassword := os.Getenv("LANVERSE_TEST_KAFKA_LOGSTASH_PASSWORD")
	if rawBrokers == "" || elasticsearchURL == "" || kibanaURL == "" || adminPassword == "" || eventPassword == "" || filebeatPassword == "" || logstashPassword == "" {
		t.Skip("set the real Kafka/Elasticsearch/Kibana observability environment to run the ELK journey")
	}
	return elkTestEnvironment{
		brokers:       strings.Split(rawBrokers, ","),
		logTopic:      environmentOr("LANVERSE_TEST_KAFKA_LOG_TOPIC", "lanverse.logs.application.v1"),
		logDLQTopic:   environmentOr("LANVERSE_TEST_KAFKA_LOG_DLQ_TOPIC", "lanverse.logs.application.dlq.v1"),
		businessTopic: environmentOr("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC", "lanverse.business.storygraph-version.v1"),
		adminPassword: adminPassword, eventWorkerPassword: eventPassword, filebeatPassword: filebeatPassword,
		logstashPassword: logstashPassword,
		elasticsearchURL: elasticsearchURL, kibanaURL: kibanaURL,
	}
}

func newKafkaClient(t *testing.T, environment elkTestEnvironment, username, password, group string, topics []string, reset kgo.Offset) *kgo.Client {
	t.Helper()
	options := []kgo.Opt{
		kgo.SeedBrokers(environment.brokers...), kgo.ClientID("lanverse-observability-test-" + uuid.NewString()),
		kgo.SASL(plain.Auth{User: username, Pass: password}.AsMechanism()),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	}
	if group != "" {
		options = append(options, kgo.ConsumerGroup(group), kgo.ConsumeTopics(topics...), kgo.DisableAutoCommit(), kgo.ConsumeResetOffset(reset))
	}
	client, err := kgo.NewClient(options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func newKafkaPartitionConsumer(t *testing.T, environment elkTestEnvironment, username, password, topic string) *kgo.Client {
	t.Helper()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(environment.brokers...),
		kgo.ClientID("lanverse-observability-acl-test-"+uuid.NewString()),
		kgo.SASL(plain.Auth{User: username, Pass: password}.AsMechanism()),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: {0: kgo.NewOffset().AtEnd()}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func primeKafkaConsumer(client *kgo.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client.PollRecords(ctx, 1)
}

func waitForLogDLQ(t *testing.T, ctx context.Context, client *kgo.Client) *kgo.Record {
	t.Helper()
	for ctx.Err() == nil {
		fetches := client.PollRecords(ctx, 1)
		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatalf("poll log DLQ: %v", errs)
		}
		if records := fetches.Records(); len(records) == 1 {
			return records[0]
		}
	}
	t.Fatal("timed out waiting for the log DLQ")
	return nil
}

func waitForIndexedLog(t *testing.T, ctx context.Context, elasticsearchURL, requestID string) map[string]any {
	t.Helper()
	query := map[string]any{"query": map[string]any{"term": map[string]any{"request_id": requestID}}, "size": 1}
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		response := requestJSON(t, http.MethodPost, elasticsearchURL+"/lanverse-logs-application-v1/_search", encoded)
		var result struct {
			Hits struct {
				Hits []struct {
					Source map[string]any `json:"_source"`
				} `json:"hits"`
			} `json:"hits"`
		}
		if err = json.Unmarshal(response, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Hits.Hits) == 1 {
			return result.Hits.Hits[0].Source
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the Elasticsearch log document")
	return nil
}

func requestJSON(t *testing.T, method, url string, body []byte) []byte {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s returned %d: %s", method, url, response.StatusCode, content)
	}
	return content
}

func environmentOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
