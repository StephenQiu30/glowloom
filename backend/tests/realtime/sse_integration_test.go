package realtime_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/sse"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestProjectSSERequiresAuthorizer(t *testing.T) {
	client := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := sse.NewHandler(client, nil, zap.NewNop(), sse.Options{}); !errors.Is(err, sse.ErrInvalidHandler) {
		t.Fatalf("missing authorizer = %v, want ErrInvalidHandler", err)
	}
}

func TestProjectSSECanonicalizesProjectIDBeforeAuthorize(t *testing.T) {
	client := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	want := uuid.NewString()
	var got string
	handler, err := sse.NewHandler(client, func(_ *http.Request, id string) bool {
		got = id
		return false
	}, zap.NewNop(), sse.Options{})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	w := httptest.NewRecorder()
	handler.ServeProject(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/events", nil), strings.ToUpper(want))
	if got != want || w.Code != http.StatusForbidden {
		t.Fatalf("authorization project = %q, status = %d, want %q and 403", got, w.Code, want)
	}
}

func TestProjectSSEReplaysThenStreamsAndDeduplicates(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	t.Cleanup(func() {
		if err := conn.Client.Del(context.Background(), "project:"+projectID+":events").Err(); err != nil {
			t.Errorf("delete replay stream: %v", err)
		}
	})
	sink := redisrealtime.NewSink(conn.Client)
	publish := func(id string) {
		t.Helper()
		if err := sink.Publish(ctx, realtime.Event{ID: id, ProjectID: projectID, Type: "operation.updated", OperationID: uuid.NewString(), TargetType: "shot_frame", Status: "submitted"}); err != nil {
			t.Fatalf("publish event: %v", err)
		}
	}
	first, second, third, fourth := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	publish(first)
	publish(second)
	handler, err := sse.NewHandler(conn.Client, func(_ *http.Request, id string) bool { return id == projectID }, zap.NewNop(), sse.Options{HeartbeatInterval: 100 * time.Millisecond, MaxConnectionAge: 5 * time.Second})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeProject(w, r, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/events"))
	}))
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/projects/"+projectID+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Last-Event-ID", first)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE response = %d, %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(resp.Body)
	if frame := nextEventFrame(t, reader); !strings.Contains(frame, "id: "+second+"\n") || !strings.Contains(frame, "event: operation.updated\n") {
		t.Fatalf("replay frame = %q", frame)
	}
	publish(third)
	if frame := nextEventFrame(t, reader); !strings.Contains(frame, "id: "+third+"\n") {
		t.Fatalf("live frame = %q", frame)
	}
	publish(third)
	publish(fourth)
	if frame := nextEventFrame(t, reader); !strings.Contains(frame, "id: "+fourth+"\n") {
		t.Fatalf("deduplicated frame = %q", frame)
	}
}

func TestProjectSSERequiresAuthorizationAndResyncsMissingCursor(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	handler, err := sse.NewHandler(conn.Client, func(_ *http.Request, id string) bool { return id == projectID }, zap.NewNop(), sse.Options{HeartbeatInterval: 20 * time.Millisecond, MaxConnectionAge: time.Second})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	denied := httptest.NewRecorder()
	handler.ServeProject(denied, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/events", nil), uuid.NewString())
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d, want 403", denied.Code)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeProject(w, r, projectID)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Last-Event-ID", uuid.NewString())
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	if frame := nextEventFrame(t, reader); !strings.Contains(frame, "event: resync\n") || !strings.Contains(frame, "data: {}\n") {
		t.Fatalf("resync frame = %q", frame)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read heartbeat: %v", err)
		}
		if line == ": heartbeat\n" {
			break
		}
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("read until maximum connection age: %v", err)
	}
}

func TestProjectSSERechecksAuthorizationBeforeReplay(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	t.Cleanup(func() {
		if err := conn.Client.Del(context.Background(), "project:"+projectID+":events").Err(); err != nil {
			t.Errorf("delete replay stream: %v", err)
		}
	})
	sink := redisrealtime.NewSink(conn.Client)
	cursorID, eventID := uuid.NewString(), uuid.NewString()
	for _, id := range []string{cursorID, eventID} {
		if err := sink.Publish(t.Context(), realtime.Event{
			ID: id, ProjectID: projectID, Type: "resync",
		}); err != nil {
			t.Fatalf("seed replay event: %v", err)
		}
	}
	var calls atomic.Int32
	handler, err := sse.NewHandler(conn.Client, func(_ *http.Request, id string) bool {
		return id == projectID && calls.Add(1) == 1
	}, zap.NewNop(), sse.Options{HeartbeatInterval: 20 * time.Millisecond, MaxConnectionAge: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/events", nil)
	req.Header.Set("Last-Event-ID", cursorID)
	handler.ServeProject(w, req, projectID)
	if calls.Load() < 2 || w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), eventID) {
		t.Fatalf("replay after revocation: auth calls %d, status %d, body %q", calls.Load(), w.Code, w.Body.String())
	}
}

func TestProjectSSEStopsAfterAuthorizationIsRevoked(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	t.Cleanup(func() {
		if err := conn.Client.Del(context.Background(), "project:"+projectID+":events").Err(); err != nil {
			t.Errorf("delete replay stream: %v", err)
		}
	})
	var authorized atomic.Bool
	authorized.Store(true)
	handler, err := sse.NewHandler(conn.Client, func(_ *http.Request, id string) bool {
		return id == projectID && authorized.Load()
	}, zap.NewNop(), sse.Options{HeartbeatInterval: 5 * time.Second, MaxConnectionAge: 2 * time.Second})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeProject(w, r, projectID)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new SSE request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d, want 200", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("first SSE line = %q, error %v", line, err)
	}
	authorized.Store(false)
	eventID := uuid.NewString()
	if err := redisrealtime.NewSink(conn.Client).Publish(ctx, realtime.Event{
		ID: eventID, ProjectID: projectID, Type: "resync",
	}); err != nil {
		t.Fatalf("publish event after revocation: %v", err)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil || strings.Contains(string(remaining), eventID) || strings.Contains(string(remaining), "event: resync") {
		t.Fatalf("SSE after revocation = %q, error %v", remaining, err)
	}
}

func TestProjectSSEClosesOnHeartbeatAfterAuthorizationIsRevoked(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	var authorized atomic.Bool
	authorized.Store(true)
	handler, err := sse.NewHandler(conn.Client, func(_ *http.Request, id string) bool {
		return id == projectID && authorized.Load()
	}, zap.NewNop(), sse.Options{HeartbeatInterval: 20 * time.Millisecond, MaxConnectionAge: 2 * time.Second})
	if err != nil {
		t.Fatalf("new SSE handler: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeProject(w, r, projectID)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new SSE request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("first SSE line = %q, error %v", line, err)
	}
	authorized.Store(false)
	remaining, err := io.ReadAll(reader)
	if err != nil || strings.Contains(string(remaining), ": heartbeat") {
		t.Fatalf("SSE after idle revocation = %q, error %v", remaining, err)
	}
}

func nextEventFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	for {
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					t.Fatal("SSE closed before next event")
				}
				t.Fatalf("read SSE frame: %v", err)
			}
			frame.WriteString(line)
			if line == "\n" {
				break
			}
		}
		if strings.Contains(frame.String(), "event: ") {
			return frame.String()
		}
	}
}
