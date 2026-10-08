package canvas_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestPublicCanvasAPIWithLocalPostgres(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_HTTP_DB_DSN")
	_, project := seedCanvasActor(t, database)
	var storage *objectstorage.Client
	var mediaDigests map[uuid.UUID][32]byte
	if os.Getenv("LV_TEST_CANVAS_USE_ENV_OBJECT_STORAGE") == "1" {
		// Only the caller can opt in to its already configured local object storage.
		localConfig, err := config.Load()
		if err != nil {
			t.Fatal("object storage test configuration unavailable")
		}
		storage, err = objectstorage.Open(localConfig.ObjectStorageEndpoint, localConfig.ObjectStorageBucket, localConfig.ObjectStorageAccessKey, localConfig.ObjectStorageSecretKey, localConfig.ObjectStorageRegion)
		if err != nil {
			t.Fatal("object storage test client unavailable")
		}
		mediaDigests = seedOwnedCanvasBrowserMedia(t, database, storage, project)
	}
	const origin = "http://127.0.0.1:3140"
	router, err := app.NewBusinessRouter(zap.NewNop(), func(_ context.Context) error { return nil }, noop.NewTracerProvider(), config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: origin}, database, storage)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 30 * time.Second}
	request := func(method, path string, body any, key string, originHeader string) (int, []byte) {
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
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, err = io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode >= 400 && (!strings.HasPrefix(res.Header.Get("Content-Type"), "application/problem+json") || res.Header.Get("X-Request-Id") == "") {
			t.Fatalf("invalid public error headers: %v", res.Header)
		}
		return res.StatusCode, data
	}
	if code, body := request("GET", "/api/projects", nil, "", ""); code != 200 || !bytes.Contains(body, []byte(project.String())) {
		t.Fatalf("project %d %s", code, body)
	}
	createPath := "/api/projects/" + project.String() + "/canvases"
	if code, _ := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, uuid.NewString(), "http://evil.invalid"); code != 403 {
		t.Fatalf("origin %d", code)
	}
	key := uuid.NewString()
	code, body := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, key, origin)
	if code != 201 {
		t.Fatalf("create %d %s", code, body)
	}
	var doc domain.Document
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	code, replayed := request("POST", createPath, map[string]any{"name": "HTTP画布", "scope": map[string]any{}}, key, origin)
	if code != 201 || !bytes.Equal(body, replayed) {
		t.Fatalf("create replay %d %s", code, replayed)
	}
	path := "/api/canvases/" + doc.ID.String() + "/commands"
	input := map[string]any{"expected_revision": 1, "commands": []map[string]any{{"type": "AddNodes", "nodes": []map[string]any{{"id": uuid.NewString(), "title": "文字", "node_type": "text", "node_action": "resource", "config": map[string]string{"text": "持久备注"}, "x": 0, "y": 0}}}}}
	key = uuid.NewString()
	code, body = request("POST", path, input, key, origin)
	if code != 200 {
		t.Fatalf("commands %d %s", code, body)
	}
	code, replayed = request("POST", path, input, key, origin)
	if code != 200 || !bytes.Equal(body, replayed) {
		t.Fatalf("command replay %d %s", code, replayed)
	}
	if code, body := request("POST", path, input, uuid.NewString(), origin); code != 409 || !bytes.Contains(body, []byte(`"current_revision":2`)) {
		t.Fatalf("conflict %d %s", code, body)
	}
	if code, body := request("POST", path, map[string]any{"expected_revision": 2, "commands": []map[string]any{{"type": "RunNodes"}}}, uuid.NewString(), origin); code != 422 || !bytes.Contains(body, []byte(`"command_index":0`)) {
		t.Fatalf("unsupported %d %s", code, body)
	}
	if code, body := request("GET", "/swagger/doc.json", nil, "", ""); code != http.StatusNotFound {
		t.Fatalf("removed swagger route %d %s", code, body)
	}
	if code, _ := request("GET", "/api/not-open", nil, "", ""); code != 404 {
		t.Fatalf("missing route %d", code)
	}
	renameKey := uuid.NewString()
	renameInput := map[string]any{"expected_revision": 2, "name": "HTTP正式画布"}
	if code, body := request("PATCH", "/api/canvases/"+doc.ID.String(), renameInput, renameKey, origin); code != 200 || !bytes.Contains(body, []byte(`"revision":3`)) {
		t.Fatalf("rename %d %s", code, body)
	}
	if code, body := request("PATCH", "/api/canvases/"+doc.ID.String(), renameInput, renameKey, origin); code != 200 || !bytes.Contains(body, []byte(`"revision":3`)) {
		t.Fatalf("rename replay %d %s", code, body)
	}
	deleteKey := uuid.NewString()
	for range 2 {
		if code, body := request("DELETE", "/api/canvases/"+doc.ID.String(), map[string]any{"expected_revision": 3}, deleteKey, origin); code != 200 || !bytes.Contains(body, []byte(`"deleted":true`)) {
			t.Fatalf("delete %d %s", code, body)
		}
	}
	if code, _ := request("GET", "/api/canvases/"+doc.ID.String(), nil, "", ""); code != 404 {
		t.Fatalf("deleted readable %d", code)
	}
	code, body = request("GET", "/api/projects/"+project.String()+"/media", nil, "", "")
	var mediaPage mediaapp.AssetPage
	if code != 200 || json.Unmarshal(body, &mediaPage) != nil || len(mediaPage.Items) != len(mediaDigests) {
		t.Fatalf("owned media list status=%d", code)
	}
	for _, asset := range mediaPage.Items {
		code, body = request("GET", "/api/projects/"+project.String()+"/media/"+asset.ID.String()+"/preview", nil, "", "")
		var preview mediaapp.Preview
		if code != 200 || json.Unmarshal(body, &preview) != nil || preview.Asset.ID != asset.ID || !preview.ExpiresAt.After(time.Now()) || preview.ExpiresAt.After(time.Now().Add(11*time.Minute)) {
			t.Fatalf("owned media preview status=%d", code)
		}
		// The signed URL is never printed or persisted; verify actual object
		// bytes using a client with no application session cookies.
		objectRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, preview.URL, nil)
		if err != nil {
			t.Fatal("owned media preview URL invalid")
		}
		objectClient := &http.Client{Timeout: 30 * time.Second}
		response, err := objectClient.Do(objectRequest)
		if err != nil {
			t.Fatal("owned media preview download failed")
		}
		content, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK || int64(len(content)) != asset.ByteSize || sha256.Sum256(content) != mediaDigests[asset.ID] {
			t.Fatal("owned media preview bytes do not match uploaded fixture")
		}
	}
	for _, path := range []string{"/api/auth/me", "/api/auth/login", "/api/auth/logout", "/api/auth/password"} {
		if code, _ := request("GET", path, nil, "", ""); code != 404 {
			t.Fatalf("removed auth route %s status %d", path, code)
		}
	}
	if address := os.Getenv("LV_TEST_CANVAS_API_ADDR"); address != "" {
		// Browser verification uses the same router and isolated stores, never a production fixture.
		var listenerConfig net.ListenConfig
		listener, err := listenerConfig.Listen(t.Context(), "tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: router, ReadHeaderTimeout: 30 * time.Second}
		done := make(chan struct{})
		go func() { defer close(done); _ = srv.Serve(listener) }()
		t.Cleanup(func() { _ = srv.Close(); <-done })
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
