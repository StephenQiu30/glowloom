package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	redisclient "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestPublicRouterSwaggerContract(t *testing.T) {
	// Constructors do not connect these clients. This test inspects production
	// registration and its served schema; dependency readiness is a separate gate.
	redisConn := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisConn.Close() })
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), config.Config{
		Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000",
	}, &gorm.DB{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/swagger/doc.json", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("online Swagger status = %d, want 200", recorder.Code)
	}
	staticJSON, err := os.ReadFile(filepath.Join("..", "..", "docs", "swagger.json"))
	if err != nil {
		t.Fatal(err)
	}
	var static, online map[string]any
	if err := json.Unmarshal(staticJSON, &static); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &online); err != nil {
		t.Fatal(err)
	}
	if !equivalentSwagger(static, online) {
		t.Fatal("served Swagger differs from the committed schema")
	}
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
		if strings.HasPrefix(route.Path, "/api/auth/") {
			t.Fatalf("removed authentication route remains registered: %s", route.Path)
		}
	}
	for _, path := range []string{"/api/auth/login", "/api/auth/me", "/api/auth/logout", "/api/auth/password"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), method, path, nil))
			if response.Code != 404 {
				t.Fatalf("removed %s %s status %d, want 404", method, path, response.Code)
			}
		}
	}
	if err := validatePublicSwagger(static, routes); err != nil {
		t.Fatal(err)
	}
}

func TestPublicRouterSwaggerUI(t *testing.T) {
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), config.Config{
		Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000",
	}, &gorm.DB{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/swagger/index.html", "text/html", `id="swagger-ui"`},
		{"/swagger/swagger-initializer.js", "application/javascript", `url: "doc.json"`},
		{"/swagger/swagger-ui.css", "text/css", ".swagger-ui"},
		{"/swagger/swagger-ui-bundle.js", "application/javascript", "SwaggerUIBundle"},
		{"/swagger/swagger-ui-standalone-preset.js", "application/javascript", "SwaggerUIStandalonePreset"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, tc.contentType) {
				t.Errorf("Content-Type = %q, want %s", contentType, tc.contentType)
			}
			if !strings.Contains(response.Body.String(), tc.contains) {
				t.Errorf("response does not contain %q", tc.contains)
			}
		})
	}
}

func TestWorkspaceAPIDoesNotExposeAnonymousAccessOutsideThisHost(t *testing.T) {
	for _, tc := range []struct{ name, env, address, origin string }{
		{"staging", "staging", "127.0.0.1:8080", "https://localhost:3000"},
		{"production", "prod", "127.0.0.1:8080", "https://localhost:3000"},
		{"wildcard", "local", ":8080", "http://localhost:3000"},
		{"external browser", "local", "127.0.0.1:8080", "https://lanverse.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), config.Config{Env: tc.env, HTTPAddr: tc.address, PublicOrigin: tc.origin}, &gorm.DB{}, nil)
			if err == nil {
				t.Fatal("nonlocal workspace configuration was accepted")
			}
		})
	}
}

func TestPublicSwaggerRejectsContractDrift(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		routes map[string]struct{}
	}{
		{"undocumented route", `{"swagger":"2.0","paths":{"/api/projects":{"get":{"operationId":"listProjects"}}}}`, map[string]struct{}{"GET /api/projects": {}, "POST /api/projects": {}}},
		{"unregistered operation", `{"swagger":"2.0","paths":{"/api/projects":{"get":{"operationId":"listProjects"},"post":{"operationId":"createProject"}}}}`, map[string]struct{}{"GET /api/projects": {}}},
		{"empty operation ID", `{"swagger":"2.0","paths":{"/api/projects":{"get":{"operationId":" "}}}}`, map[string]struct{}{"GET /api/projects": {}}},
		{"duplicate operation ID", `{"swagger":"2.0","paths":{"/api/projects":{"get":{"operationId":"projects"},"post":{"operationId":"projects"}}}}`, map[string]struct{}{"GET /api/projects": {}, "POST /api/projects": {}}},
		{"internal schema path", `{"swagger":"2.0","paths":{"/internal/agent/runs":{"get":{"operationId":"internalRuns"}}}}`, map[string]struct{}{}},
		{"internal public path", `{"swagger":"2.0","paths":{"/api/internal/runs":{"get":{"operationId":"internalRuns"}}}}`, map[string]struct{}{"GET /api/internal/runs": {}}},
		{"provider callback", `{"swagger":"2.0","paths":{"/api/provider-callbacks/{provider_key}":{"post":{"operationId":"providerCallback"}}}}`, map[string]struct{}{"POST /api/provider-callbacks/:provider_key": {}}},
		{"empty schema", `{"swagger":"2.0","paths":{}}`, map[string]struct{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal([]byte(tc.schema), &schema); err != nil {
				t.Fatal(err)
			}
			if err := validatePublicSwagger(schema, tc.routes); err == nil {
				t.Fatal("invalid public contract was accepted")
			}
		})
	}
}

func TestPublicSwaggerNormalizesPathParameters(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(`{"swagger":"2.0","paths":{"/api/projects/{pid}/canvases/{id}":{"get":{"operationId":"readCanvas"}}}}`), &schema); err != nil {
		t.Fatal(err)
	}
	routes := map[string]struct{}{"GET /api/projects/:pid/canvases/:id": {}, "GET /healthz": {}, "GET /swagger/doc.json": {}}
	if err := validatePublicSwagger(schema, routes); err != nil {
		t.Fatal(err)
	}
}

func TestSwaggerRuntimeDefaultsDoNotHideNonEmptyDrift(t *testing.T) {
	static := map[string]any{"swagger": "2.0"}
	for _, tc := range []struct {
		name   string
		online map[string]any
		want   bool
	}{
		{"omitted metadata", map[string]any{"swagger": "2.0"}, true},
		{"empty runtime defaults", map[string]any{"swagger": "2.0", "host": "", "schemes": []any{}}, true},
		{"nonempty host", map[string]any{"swagger": "2.0", "host": "api.example.com"}, false},
		{"nonempty schemes", map[string]any{"swagger": "2.0", "schemes": []any{"https"}}, false},
		{"null schemes", map[string]any{"swagger": "2.0", "schemes": nil}, false},
		{"wrong schemes type", map[string]any{"swagger": "2.0", "schemes": ""}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := equivalentSwagger(static, tc.online); got != tc.want {
				t.Fatalf("equivalent = %v, want %v", got, tc.want)
			}
		})
	}
	if equivalentSwagger(map[string]any{"host": "configured.example"}, map[string]any{"host": ""}) {
		t.Fatal("explicit static host was discarded")
	}
}

func equivalentSwagger(static, online map[string]any) bool {
	if _, present := static["host"]; !present && online["host"] == "" {
		delete(online, "host")
	}
	if _, present := static["schemes"]; !present {
		if schemes, ok := online["schemes"].([]any); ok && len(schemes) == 0 {
			delete(online, "schemes")
		}
	}
	return reflect.DeepEqual(static, online)
}

func validatePublicSwagger(schema map[string]any, routes map[string]struct{}) error {
	if schema["swagger"] != "2.0" {
		return fmt.Errorf("public contract must be Swagger 2.0")
	}
	if base, present := schema["basePath"]; present && base != "/" {
		return fmt.Errorf("public paths already include /api; basePath must be /")
	}
	paths, ok := schema["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		return fmt.Errorf("public contract has no paths")
	}
	operations := make(map[string]struct{})
	ids := make(map[string]string)
	for path, value := range paths {
		if !publicContractPath(path) {
			return fmt.Errorf("non-public contract path: %s", path)
		}
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid path item: %s", path)
		}
		for method, value := range item {
			switch method {
			case "get", "post", "put", "patch", "delete", "head", "options":
			default:
				if method == "parameters" || strings.HasPrefix(method, "x-") {
					continue
				}
				return fmt.Errorf("unsupported path item field %s: %s", method, path)
			}
			operation, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid operation: %s %s", method, path)
			}
			id, ok := operation["operationId"].(string)
			if !ok || strings.TrimSpace(id) == "" {
				return fmt.Errorf("missing operationId: %s %s", method, path)
			}
			key := strings.ToUpper(method) + " " + path
			if previous, duplicate := ids[id]; duplicate {
				return fmt.Errorf("duplicate operationId %s: %s and %s", id, previous, key)
			}
			ids[id] = key
			operations[key] = struct{}{}
		}
	}
	if len(operations) == 0 {
		return fmt.Errorf("public contract has no operations")
	}
	publicRoutes := make(map[string]struct{})
	for route := range routes {
		parts := strings.SplitN(route, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid registered route: %s", route)
		}
		if !strings.HasPrefix(parts[1], "/api/") && parts[1] != "/api" {
			continue
		}
		if !publicContractPath(parts[1]) {
			return fmt.Errorf("non-public API route: %s", route)
		}
		segments := strings.Split(parts[1], "/")
		for i, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				segments[i] = "{" + strings.TrimPrefix(segment, ":") + "}"
			}
		}
		key := parts[0] + " " + strings.Join(segments, "/")
		publicRoutes[key] = struct{}{}
		if _, documented := operations[key]; !documented {
			return fmt.Errorf("registered route has no Swagger operation: %s", key)
		}
	}
	for operation := range operations {
		if _, registered := publicRoutes[operation]; !registered {
			return fmt.Errorf("Swagger operation has no registered route: %s", operation)
		}
	}
	return nil
}

func publicContractPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasPrefix(path, "/api/") && !strings.HasPrefix(lower, "/api/internal/") &&
		lower != "/api/internal" && !strings.Contains(lower, "callback") && !strings.Contains(lower, "webhook")
}
