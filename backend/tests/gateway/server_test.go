package gateway_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	appgateway "github.com/StephenQiu30/glowloom-server/backend/internal/app/gateway"
)

func TestConfigDoesNotLoadBusinessDependencies(t *testing.T) {
	for name, value := range map[string]string{
		"GLOWLOOM_ENV": "local", "GLOWLOOM_LOG_LEVEL": "info", "GLOWLOOM_GATEWAY_HTTP_ADDR": "127.0.0.1:0", "GLOWLOOM_GATEWAY_BACKEND_ORIGIN": "http://127.0.0.1:8080",
		"GLOWLOOM_DB_DSN": "", "GLOWLOOM_REDIS_URL": "", "GLOWLOOM_KAFKA_BROKERS": "", "GLOWLOOM_IDENTITY_KEY": "",
	} {
		t.Setenv(name, value)
	}
	cfg, err := appgateway.LoadConfig()
	if err != nil || cfg.HTTPAddr != "127.0.0.1:0" || cfg.BackendOrigin != "http://127.0.0.1:8080" {
		t.Fatalf("configuration requires business dependencies: cfg=%+v error=%v", cfg, err)
	}
	t.Setenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN", "")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("missing backend origin accepted")
	}
	t.Setenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN", "http://127.0.0.1:8080")
	t.Setenv("GLOWLOOM_ENV", "prod")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("unreviewed production topology accepted")
	}
}

func TestServeStopsHTTPAndUpgradedConnections(t *testing.T) {
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		defer close(upstreamClosed)
		_, _, _ = conn.Read(r.Context())
	}))
	defer upstream.Close()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- appgateway.Serve(ctx, listener, upstream.URL, zap.NewNop())
	}()
	dialCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(dialCtx, "ws://"+listener.Addr().String()+"/api/canvas/live", nil)
	if res != nil && res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gateway did not stop")
	}
	select {
	case <-upstreamClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown left upgraded connection alive")
	}
}

func TestMutationWithLostReceiptIsNotReplayed(t *testing.T) {
	var commands atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "warm")
			return
		}
		commands.Add(1)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close() // The mutation committed, but no receipt reached the gateway.
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	res, err := get(t.Context(), proxy.Client(), proxy.URL+"/api/warm")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL+"/api/confirm", nil)
	req.Header.Set("Idempotency-Key", "17eb0538-0a3e-45b0-88b8-9751432c0c34")
	res, err = proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != 502 || !strings.Contains(string(body), "upstream_unavailable") {
		t.Fatalf("lost receipt: status=%d body=%s error=%v", res.StatusCode, body, err)
	}
	// The handler has finished before its connection closed and the gateway replied.
	if commands.Load() != 1 {
		t.Fatalf("gateway replayed an unknown mutation %d times", commands.Load())
	}
}
