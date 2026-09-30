package canvas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestPublicCanvasAPIWithLocalPostgresRedis(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_HTTP_DB_DSN")
	redisURL := os.Getenv("LV_TEST_CANVAS_REDIS_URL")
	if redisURL == "" {
		t.Skip("set dedicated LV_TEST_CANVAS_REDIS_URL")
	}
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	redisConn := redisclient.NewClient(options)
	t.Cleanup(func() { _ = redisConn.Close() })
	actor, project := seedCanvasActor(t, database)
	hash, err := identitydomain.HashPassword("CanvasTest123!", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE identity."user" SET login_name='canvas-browser',password_hash=?,must_change_password=true WHERE id=?`, hash, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	const origin = "http://127.0.0.1:3140"
	router, err := app.NewBusinessRouter(zap.NewNop(), func(_ context.Context) error { return nil }, noop.NewTracerProvider(), config.Config{Env: "local", PublicOrigin: origin}, database, redisConn)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	request := func(method, path string, body any, key string, csrf bool, originHeader string) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if originHeader != "" {
			req.Header.Set("Origin", originHeader)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if csrf {
			for _, cookie := range jar.Cookies(req.URL) {
				if cookie.Name == "lv_csrf" {
					req.Header.Set("X-CSRF-Token", cookie.Value)
				}
			}
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err = io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode >= 400 && (!strings.HasPrefix(res.Header.Get("Content-Type"), "application/problem+json") || res.Header.Get("X-Request-Id") == "") {
			t.Fatalf("invalid public error headers: %v", res.Header)
		}
		return res.StatusCode, data
	}
	if code, _ := request("GET", "/api/projects", nil, "", false, ""); code != 401 {
		t.Fatalf("unauthenticated %d", code)
	}
	login := map[string]string{"login_name": "canvas-browser", "password": "CanvasTest123!"}
	if code, _ := request("POST", "/api/auth/login", login, "", false, "http://evil.invalid"); code != 403 {
		t.Fatalf("origin %d", code)
	}
	if code, body := request("POST", "/api/auth/login", login, "", false, origin); code != 200 {
		t.Fatalf("login %d %s", code, body)
	}
	if code, _ := request("GET", "/api/projects", nil, "", false, ""); code != 403 {
		t.Fatalf("forced password gate %d", code)
	}
	if code, body := request("POST", "/api/auth/password", map[string]string{"current_password": "CanvasTest123!", "new_password": "CanvasReady123!"}, uuid.NewString(), true, origin); code != 200 {
		t.Fatalf("change %d %s", code, body)
	}
	if code, body := request("GET", "/api/projects", nil, "", false, ""); code != 200 || !bytes.Contains(body, []byte(project.String())) {
		t.Fatalf("project %d %s", code, body)
	}
	createPath := "/api/projects/" + project.String() + "/canvases"
	if code, _ := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, uuid.NewString(), false, origin); code != 403 {
		t.Fatalf("csrf %d", code)
	}
	key := uuid.NewString()
	code, body := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, key, true, origin)
	if code != 201 {
		t.Fatalf("create %d %s", code, body)
	}
	var doc domain.Document
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	code, replayed := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, key, true, origin)
	if code != 201 || !bytes.Equal(body, replayed) {
		t.Fatalf("create replay %d %s", code, replayed)
	}
	path := "/api/canvases/" + doc.ID.String() + "/commands"
	input := map[string]any{"expected_revision": 1, "commands": []map[string]any{{"type": "AddNodes", "nodes": []map[string]any{{"id": uuid.NewString(), "node_type": "text", "node_action": "resource", "config": map[string]string{"text": "持久备注"}, "x": 0, "y": 0}}}}}
	key = uuid.NewString()
	code, body = request("POST", path, input, key, true, origin)
	if code != 200 {
		t.Fatalf("commands %d %s", code, body)
	}
	code, replayed = request("POST", path, input, key, true, origin)
	if code != 200 || !bytes.Equal(body, replayed) {
		t.Fatalf("command replay %d %s", code, replayed)
	}
	if code, body := request("POST", path, input, uuid.NewString(), true, origin); code != 409 || !bytes.Contains(body, []byte(`"current_revision":2`)) {
		t.Fatalf("conflict %d %s", code, body)
	}
	if code, body := request("POST", path, map[string]any{"expected_revision": 2, "commands": []map[string]any{{"type": "RunNodes"}}}, uuid.NewString(), true, origin); code != 422 || !bytes.Contains(body, []byte(`"command_index":0`)) {
		t.Fatalf("unsupported %d %s", code, body)
	}
	if code, body := request("GET", "/swagger/doc.json", nil, "", false, ""); code != 200 || !bytes.Contains(body, []byte("applyCanvasCommands")) {
		t.Fatalf("swagger %d %s", code, body)
	}
	if code, _ := request("GET", "/api/not-open", nil, "", false, ""); code != 404 {
		t.Fatalf("missing route %d", code)
	}
	if code, _ := request("POST", "/api/auth/logout", nil, uuid.NewString(), true, origin); code != 204 {
		t.Fatalf("logout %d", code)
	}
	if code, _ := request("GET", "/api/auth/me", nil, "", false, ""); code != 401 {
		t.Fatalf("logout session %d", code)
	}
	if address := os.Getenv("LV_TEST_CANVAS_API_ADDR"); address != "" {
		// Browser verification uses the same router and isolated stores, never a production fixture.
		if err := database.Exec(`UPDATE identity."user" SET password_hash=?,must_change_password=true,session_epoch=session_epoch+1 WHERE id=?`, hash, actor.ID).Error; err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: router, ReadHeaderTimeout: 30 * time.Second}
		go func() { _ = srv.Serve(listener) }()
		t.Cleanup(func() { _ = srv.Close() })
		t.Logf("isolated browser API ready at %s, project %s", address, project)
		deadline := time.Now().Add(30 * time.Minute)
		for time.Now().Before(deadline) {
			if _, err := os.Stat("/tmp/lanverse-canvas-e3607-stop"); err == nil {
				return
			}
			time.Sleep(time.Second)
		}
	}
}
