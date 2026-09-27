package app_test

import (
	"bufio"
	"context"
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
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/sse"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestAllRoleRunsAndStopsOnLocalServices(t *testing.T) {
	dsn := os.Getenv("LV_TEST_ALL_DB_DSN")
	brokers := os.Getenv("LV_TEST_ALL_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_ALL_REDIS_URL")
	temporalAddr := os.Getenv("LV_TEST_ALL_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_ALL_TEMPORAL_NAMESPACE")
	if dsn == "" || brokers == "" || redisURL == "" || temporalAddr == "" || namespace == "" || os.Getenv("LV_ENV_FILE") == "" {
		t.Skip("set absolute LV_ENV_FILE with local MinIO credentials and disposable LV_TEST_ALL_* service configuration")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load local configuration: %v", err)
	}
	apiAddr, workerAddr, relayAddr := localHealthAddress(t), localHealthAddress(t), localHealthAddress(t)
	if apiAddr == workerAddr || apiAddr == relayAddr || workerAddr == relayAddr {
		t.Fatal("role test addresses collided")
	}
	cfg.HTTPAddr = apiAddr
	cfg.WorkerHealthAddr = workerAddr
	cfg.RelayHealthAddr = relayAddr
	cfg.DBDSN = dsn
	cfg.RedisURL = redisURL
	cfg.KafkaBrokers = brokers
	cfg.TemporalAddr = temporalAddr
	cfg.TemporalNamespace = namespace
	cfg.OTelEndpoint = ""

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open test Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })

	runCtx, stopAll := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var eventID, projectID string
	go func() { done <- app.RunAll(runCtx, cfg, zap.NewNop()) }()
	t.Cleanup(func() {
		stopAll()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stop combined roles: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("combined roles did not stop after cancellation")
		}
		for _, address := range []string{apiAddr, workerAddr, relayAddr} {
			assertRoleHealthStopped(t, address)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if eventID != "" {
			if err := dbConn.DB.WithContext(cleanupCtx).Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", eventID).Error; err != nil {
				t.Errorf("delete combined-role Outbox row: %v", err)
			}
		}
		if projectID != "" {
			if err := redisConn.Client.Del(cleanupCtx, "project:"+projectID+":events").Err(); err != nil {
				t.Errorf("delete combined-role Redis stream: %v", err)
			}
		}
	})
	assertRoleHealth(ctx, t, workerAddr)
	assertRoleHealth(ctx, t, relayAddr)
	assertAPIReady(ctx, t, apiAddr)

	eventID, projectID, operationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	streamHandler, err := sse.NewHandler(redisConn.Client, func(_ *http.Request, id string) bool {
		return id == projectID
	}, zap.NewNop(), sse.Options{})
	if err != nil {
		t.Fatalf("create test-authorized SSE handler: %v", err)
	}
	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/events")
		streamHandler.ServeProject(w, r, id)
	}))
	t.Cleanup(streamServer.Close)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, streamServer.URL+"/api/projects/"+projectID+"/events", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	response, err := streamServer.Client().Do(request)
	if err != nil {
		t.Fatalf("connect project SSE: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("project SSE response = %d, %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("project SSE connection frame = %q, error = %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("project SSE connection frame ending = %q, error = %v", line, err)
	}

	stream := "project:" + projectID + ":events"
	payload := `{"event_id":"` + eventID + `","event_type":"` + realtime.OperationStatusTopic +
		`","project_id":"` + projectID + `","aggregate":{"type":"operation","id":"` + operationID +
		`"},"data":{"target_type":"shot_frame","status":"submitted"}}`
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload) VALUES (?::uuid, ?, ?, ?::jsonb)",
		eventID, realtime.OperationStatusTopic, projectID, payload,
	).Error; err != nil {
		t.Fatalf("insert combined-role event: %v", err)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, processed bool
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid", eventID,
		).Scan(&published).Error; err != nil {
			t.Fatalf("read Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT EXISTS (SELECT 1 FROM infra.processed_event WHERE consumer = 'realtime' AND event_id = ?::uuid)", eventID,
		).Scan(&processed).Error; err != nil {
			t.Fatalf("read consumer marker: %v", err)
		}
		if published && processed {
			entries, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
			if err != nil || len(entries) != 1 || entries[0].Values["id"] != eventID {
				t.Fatalf("Redis projected entries = %+v, error = %v", entries, err)
			}
			break
		}
		select {
		case err := <-done:
			done <- err
			t.Fatalf("combined roles exited before projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for combined-role event: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	frame := nextNamedSSEFrame(t, reader)
	if !strings.Contains(frame, "id: "+eventID+"\n") ||
		!strings.Contains(frame, "event: operation.updated\n") ||
		!strings.Contains(frame, `"operation_id":"`+operationID+`"`) ||
		!strings.Contains(frame, `"status":"submitted"`) {
		t.Fatalf("project SSE event = %q", frame)
	}
}

func nextNamedSSEFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read project SSE event: %v", err)
		}
		frame.WriteString(line)
		if line == "\n" {
			if strings.Contains(frame.String(), "event: ") {
				return frame.String()
			}
			frame.Reset()
		}
	}
}

func assertAPIReady(ctx context.Context, t *testing.T, address string) {
	t.Helper()
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for {
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+address+"/readyz", nil)
		if err != nil {
			t.Fatalf("create API readiness request: %v", err)
		}
		response, err := client.Do(req)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
			if response.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("API readiness status = %d", response.StatusCode)
			}
		}
		if probeCtx.Err() != nil {
			t.Fatalf("API did not become ready at %s: %v", address, probeCtx.Err())
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			t.Fatalf("API did not become ready at %s: %v", address, probeCtx.Err())
		case <-timer.C:
		}
	}
}
