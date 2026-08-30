package observability_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

func TestHTTPLoggingProducesBoundedTraceableRecordWithoutRequestContent(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := telemetry.NewLogger(&output, "lanverse-api", "test")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/{project_id}/commands", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := telemetry.HTTPLoggingMiddleware(logger)(mux)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/projects/private-project-id/commands?token=secret-query-value",
		strings.NewReader(`{"prompt":"secret-body-value"}`),
	)
	request.Header.Set("X-Request-ID", "5cffb37f-79b1-42f9-9e34-c55b1d8e9702")
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") != "5cffb37f-79b1-42f9-9e34-c55b1d8e9702" {
		t.Fatalf("response lost request correlation: %q", response.Header().Get("X-Request-ID"))
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]any{
		"event":       "http_request",
		"request_id":  "5cffb37f-79b1-42f9-9e34-c55b1d8e9702",
		"trace_id":    "4bf92f3577b34da6a3ce929d0e0e4736",
		"method":      http.MethodPost,
		"route":       "/api/projects/{project_id}/commands",
		"error_code":  "http_status_503",
		"status_code": float64(http.StatusServiceUnavailable),
	} {
		if actual := record[key]; actual != expected {
			t.Errorf("field %s = %#v, want %#v", key, actual, expected)
		}
	}
	if _, ok := record["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms is not numeric: %#v", record["duration_ms"])
	}
	encoded := output.String()
	for _, forbidden := range []string{"private-project-id", "secret-query-value", "secret-body-value"} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("HTTP log leaked request content %q: %s", forbidden, encoded)
		}
	}
}
