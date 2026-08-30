package observability_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type elkTestEnvironment struct {
	logstashAddress       string
	elasticsearchURL      string
	elasticsearchUsername string
	elasticsearchPassword string
	kibanaURL             string
}

func TestRealLogstashPipelineIndexesCorrelationsRedactsSecretsAndBoundsInvalidLogs(t *testing.T) {
	environment := requireELKEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	requestID := uuid.NewString()
	correlations := map[string]string{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "request_id": requestID,
		"workflow_run_id": uuid.NewString(), "node_run_id": uuid.NewString(), "node_id": "reconcile_story",
		"task_id": uuid.NewString(), "decision_id": uuid.NewString(), "provider_job_id": uuid.NewString(),
		"receipt_id": uuid.NewString(), "error_code": "ci_pipeline_verification",
	}
	record := map[string]any{
		"@timestamp": time.Now().UTC().Format(time.RFC3339Nano), "schema_version": "lanverse.log.application",
		"service": "lanverse-ci", "environment": "test", "level": "INFO", "event": "elk_pipeline_verification",
		"msg": "ELK pipeline verification", "status_code": 503, "duration_ms": 12.5,
		"access_token": "secret-access-token-value", "prompt": "secret-complete-prompt-value",
		"private_url": "https://private.example.test/object?signature=secret-url-value",
	}
	for key, value := range correlations {
		record[key] = value
	}
	writeLogRecord(t, environment.logstashAddress, record)

	indexed := waitForIndexedLog(t, ctx, environment, "lanverse-logs-application", map[string]any{
		"term": map[string]any{"request_id": requestID},
	})
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
	writeRawLogRecord(t, environment.logstashAddress, []byte(`{"schema_version":"unsupported","token":"`+invalidSecret+`"}`))
	deadLetter := waitForIndexedLog(t, ctx, environment, "lanverse-logs-dead-letter", map[string]any{
		"term": map[string]any{"error_code": "invalid_log_record"},
	})
	if deadLetter["schema_version"] != "lanverse.log.dead-letter" {
		t.Fatalf("invalid log did not use the bounded dead-letter schema: %#v", deadLetter)
	}
	rawHash, _ := deadLetter["raw_sha256"].(string)
	if len(rawHash) != 64 {
		t.Fatalf("invalid log dead-letter hash = %q", rawHash)
	}
	if _, err = hex.DecodeString(rawHash); err != nil {
		t.Fatalf("invalid log dead-letter hash is not hexadecimal: %q", rawHash)
	}
	deadLetterJSON, err := json.Marshal(deadLetter)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(deadLetterJSON, []byte(invalidSecret)) {
		t.Fatalf("invalid log dead-letter retained raw sensitive content: %s", deadLetterJSON)
	}

	requestJSON(t, environment, http.MethodGet, environment.kibanaURL+"/api/data_views/data_view/lanverse-logs-application", nil)
}

func requireELKEnvironment(t *testing.T) elkTestEnvironment {
	t.Helper()
	logstashAddress := strings.TrimSpace(os.Getenv("LANVERSE_TEST_LOGSTASH_ADDRESS"))
	elasticsearchURL := strings.TrimRight(os.Getenv("LANVERSE_TEST_ELASTICSEARCH_URL"), "/")
	kibanaURL := strings.TrimRight(os.Getenv("LANVERSE_TEST_KIBANA_URL"), "/")
	username := strings.TrimSpace(os.Getenv("LANVERSE_TEST_ELASTICSEARCH_USERNAME"))
	password := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_PASSWORD")
	if logstashAddress == "" || elasticsearchURL == "" || kibanaURL == "" {
		t.Skip("set the real Logstash/Elasticsearch/Kibana environment to run the ELK journey")
	}
	if (username == "") != (password == "") {
		t.Fatal("LANVERSE_TEST_ELASTICSEARCH_USERNAME and LANVERSE_TEST_ELASTICSEARCH_PASSWORD must be configured together")
	}
	return elkTestEnvironment{
		logstashAddress: logstashAddress, elasticsearchURL: elasticsearchURL,
		elasticsearchUsername: username, elasticsearchPassword: password, kibanaURL: kibanaURL,
	}
}

func writeLogRecord(t *testing.T, address string, record map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeRawLogRecord(t, address, encoded)
}

func writeRawLogRecord(t *testing.T, address string, record []byte) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err = connection.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Write(append(record, '\n')); err != nil {
		t.Fatal(err)
	}
}

func waitForIndexedLog(
	t *testing.T,
	ctx context.Context,
	environment elkTestEnvironment,
	index string,
	query map[string]any,
) map[string]any {
	t.Helper()
	payload := map[string]any{
		"query": query,
		"sort":  []map[string]any{{"@timestamp": map[string]string{"order": "desc"}}},
		"size":  1,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		response := requestJSON(t, environment, http.MethodPost, environment.elasticsearchURL+"/"+index+"/_search", encoded)
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
	t.Fatalf("timed out waiting for a document in %s", index)
	return nil
}

func requestJSON(t *testing.T, environment elkTestEnvironment, method, url string, body []byte) []byte {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if environment.elasticsearchUsername != "" {
		request.SetBasicAuth(environment.elasticsearchUsername, environment.elasticsearchPassword)
	}
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
