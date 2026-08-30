package observability_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

func TestStructuredLoggerKeepsCorrelationFieldsAndDropsSensitiveValues(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := telemetry.NewLogger(&output, "lanverse-api", "test")
	logger.Error(
		"workflow node failed",
		"event", "workflow_node_failed",
		"trace_id", "4bf92f3577b34da6a3ce929d0e0e4736",
		"request_id", "5cffb37f-79b1-42f9-9e34-c55b1d8e9702",
		"workflow_run_id", "6699a715-06ec-4434-aa32-55e30feb78dc",
		"node_run_id", "4fc3c4f4-f0d5-48b7-b365-b403532e4fd8",
		"node_id", "reconcile_story",
		"task_id", "0bcf2621-fc5c-49a8-93d6-f3ccb29ef909",
		"decision_id", "e365da65-fba6-4551-b7ef-7b81c9d677a3",
		"provider_job_id", "8a40a7d2-e34a-4fc6-ac9f-d4851a09e070",
		"receipt_id", "0833d033-794c-47da-9069-c3157fdefaf3",
		"error_code", "provider_outcome_unknown",
		"access_token", "secret-access-token-value",
		"claim_token", "secret-claim-token-value",
		"prompt", "secret-complete-prompt-value",
		"candidate", map[string]any{"script": "secret-candidate-value"},
		"private_url", "https://private.example.test/artifact?signature=secret-url-value",
		"error", errors.New("provider failed password=secret-password-value token=secret-error-token https://private.example.test/object?signature=secret"),
	)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]string{
		"schema_version":  "lanverse.log.application",
		"service":         "lanverse-api",
		"environment":     "test",
		"event":           "workflow_node_failed",
		"trace_id":        "4bf92f3577b34da6a3ce929d0e0e4736",
		"request_id":      "5cffb37f-79b1-42f9-9e34-c55b1d8e9702",
		"workflow_run_id": "6699a715-06ec-4434-aa32-55e30feb78dc",
		"node_run_id":     "4fc3c4f4-f0d5-48b7-b365-b403532e4fd8",
		"node_id":         "reconcile_story",
		"task_id":         "0bcf2621-fc5c-49a8-93d6-f3ccb29ef909",
		"decision_id":     "e365da65-fba6-4551-b7ef-7b81c9d677a3",
		"provider_job_id": "8a40a7d2-e34a-4fc6-ac9f-d4851a09e070",
		"receipt_id":      "0833d033-794c-47da-9069-c3157fdefaf3",
		"error_code":      "provider_outcome_unknown",
	} {
		if actual, _ := record[key].(string); actual != expected {
			t.Errorf("field %s = %q, want %q", key, actual, expected)
		}
	}
	encoded := output.String()
	for _, forbidden := range []string{
		"secret-access-token-value", "secret-claim-token-value", "secret-complete-prompt-value",
		"secret-candidate-value", "secret-url-value", "secret-password-value", "secret-error-token",
		"private.example.test",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("structured log leaked %q: %s", forbidden, encoded)
		}
	}
	for _, forbiddenKey := range []string{"access_token", "claim_token", "prompt", "candidate", "private_url"} {
		if _, exists := record[forbiddenKey]; exists {
			t.Errorf("structured log retained sensitive key %q", forbiddenKey)
		}
	}
}

func TestStructuredLoggerSanitizesNestedStructValuesBeforeCollection(t *testing.T) {
	t.Parallel()
	type providerResponse struct {
		Status      string `json:"status"`
		AccessToken string `json:"access_token"`
		Script      string `json:"script"`
		PrivateURL  string `json:"private_url"`
	}
	var output bytes.Buffer
	logger := telemetry.NewLogger(&output, "lanverse-backend", "test")
	logger.Info("provider response", "response", &providerResponse{
		Status:      "accepted",
		AccessToken: "nested-secret-token",
		Script:      "nested-complete-script",
		PrivateURL:  "https://private.example.test/nested?signature=secret",
	})

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	response, ok := record["response"].(map[string]any)
	if !ok || response["status"] != "accepted" {
		t.Fatalf("safe nested fields were not preserved: %#v", record["response"])
	}
	for _, forbidden := range []string{"access_token", "script", "private_url", "nested-secret", "private.example.test"} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("nested structured log leaked %q: %s", forbidden, output.String())
		}
	}
}

func TestStructuredLoggerSuppressesSensitiveWithGroupValues(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := telemetry.NewLogger(&output, "lanverse-api", "test")
	logger.WithGroup("access_token").Info("provider accepted", "value", "grouped-secret-value")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if _, exists := record["access_token"]; exists || strings.Contains(output.String(), "grouped-secret-value") {
		t.Fatalf("sensitive WithGroup values reached the structured log: %s", output.String())
	}
}
