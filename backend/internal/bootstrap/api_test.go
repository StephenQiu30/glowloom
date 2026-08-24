package bootstrap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHealthIsOwnedByGoRuntime(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls++
	}))
	t.Cleanup(upstream.Close)

	handler := newTestHandler(t, upstream.URL)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	if upstreamCalls != 0 {
		t.Fatalf("health request reached legacy runtime %d times", upstreamCalls)
	}
	if !strings.Contains(response.Body.String(), `"service":"lanverse-api"`) {
		t.Fatalf("health body = %q", response.Body.String())
	}
}

func TestUnmigratedRoutesAreProxiedWithoutChangingContract(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.RequestURI() != "/api/v1/projects?archived=false" {
			t.Errorf("request URI = %q", request.URL.RequestURI())
		}
		if string(body) != `{"name":"project"}` {
			t.Errorf("body = %q", body)
		}
		if request.Header.Get("Idempotency-Key") != "request-1" {
			t.Errorf("idempotency key was not forwarded")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"id":"project-1"}`))
	}))
	t.Cleanup(upstream.Close)

	handler := newTestHandler(t, upstream.URL)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects?archived=false",
		strings.NewReader(`{"name":"project"}`),
	)
	request.Header.Set("Idempotency-Key", "request-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("proxy status = %d, want %d", response.Code, http.StatusCreated)
	}
	if response.Body.String() != `{"id":"project-1"}` {
		t.Fatalf("proxy body = %q", response.Body.String())
	}
}

func TestReadinessReflectsLegacyRuntime(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/readyz" {
			t.Fatalf("readiness path = %q", request.URL.Path)
		}
		http.Error(writer, "database unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)

	handler := newTestHandler(t, upstream.URL)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("readiness body = %q", response.Body.String())
	}
}

func newTestHandler(t *testing.T, rawURL string) http.Handler {
	t.Helper()
	upstream, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return NewAPIHandler(upstream, http.DefaultClient)
}
