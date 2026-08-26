package bootstrap_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

func TestHealthIsOwnedByGoRuntimeAndDoesNotProbeDependencies(t *testing.T) {
	readyCalls := 0
	handler := newTestHandler(func(context.Context) error {
		readyCalls++
		return errors.New("database down")
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK || readyCalls != 0 {
		t.Fatalf("health response=%d ready_calls=%d", response.Code, readyCalls)
	}
}

func TestUnknownBusinessRouteIsNotProxied(t *testing.T) {
	handler := newTestHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/not-implemented", strings.NewReader(`{}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestRegisteredBusinessRouteIsOwnedByGo(t *testing.T) {
	handler := bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{
		Metrics: telemetry.NewHTTPMetrics(),
		RegisterRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("POST /api/v1/projects", func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(`{"owner":"go"}`))
			})
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{}`)))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"owner":"go"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReadinessReflectsAnyRequiredDependency(t *testing.T) {
	handler := newTestHandler(func(context.Context) error { return errors.New("required dependency down") })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"reason":"dependency_unavailable"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestVersionAndOpenAPIAreOwnedByGoRuntime(t *testing.T) {
	handler := newTestHandler(nil)
	versionResponse := httptest.NewRecorder()
	handler.ServeHTTP(versionResponse, httptest.NewRequest(http.MethodGet, "/version", nil))
	var version map[string]string
	if err := json.Unmarshal(versionResponse.Body.Bytes(), &version); err != nil {
		t.Fatal(err)
	}
	if version["version"] != "test-version" || version["commit"] != "test-commit" {
		t.Fatalf("version = %#v", version)
	}
	openAPIResponse := httptest.NewRecorder()
	handler.ServeHTTP(openAPIResponse, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document struct {
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(openAPIResponse.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if openAPIResponse.Code != http.StatusOK || document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi response=%d version=%q", openAPIResponse.Code, document.OpenAPI)
	}
}

func TestMetricsUseBoundedRouteLabels(t *testing.T) {
	handler := newTestHandler(nil)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if !strings.Contains(body, "lanverse_http_requests_total") || !strings.Contains(body, `method="GET",route="/healthz",status_class="2xx"`) {
		t.Fatalf("metrics = %q", body)
	}
}

func TestCORSAllowsOnlyConfiguredBrowserOrigin(t *testing.T) {
	handler := bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{Metrics: telemetry.NewHTTPMetrics(), AllowedOrigins: []string{"http://127.0.0.1:8123"}})
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/projects", nil)
	request.Header.Set("Origin", "http://127.0.0.1:8123")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	handler.ServeHTTP(allowed, request)
	if allowed.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8123" || allowed.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed preflight headers = %#v", allowed.Header())
	}

	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodOptions, "/api/v1/projects", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	handler.ServeHTTP(denied, request)
	if denied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted origin was allowed: %#v", denied.Header())
	}
}

func newTestHandler(ready func(context.Context) error) http.Handler {
	return bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{
		Build:   bootstrap.BuildInfo{Service: "lanverse-api", Version: "test-version", Commit: "test-commit", BuiltAt: "2026-08-24T00:00:00Z"},
		Metrics: telemetry.NewHTTPMetrics(),
		Ready:   ready,
	})
}
