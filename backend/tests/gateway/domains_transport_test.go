package gateway_test

import (
	"bufio"
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
	"github.com/google/uuid"
	"go.uber.org/zap"

	appgateway "github.com/StephenQiu30/glowloom-server/backend/gateway/app"
	gateway "github.com/StephenQiu30/glowloom-server/backend/gateway/proxy"
)

func TestDomainProxyPreservesOpaquePublicSecurityContract(t *testing.T) {
	requestID, key := uuid.NewString(), uuid.NewString()
	var calls atomic.Int32
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "opaque\x00payload" || r.URL.RawQuery != "q=a%2Bb&q=c" || r.URL.EscapedPath() != "/api/projects/%70/quotes" || owner != "execution" {
			t.Errorf("request changed: owner=%s path=%s query=%s body=%q error=%v", owner, r.URL.EscapedPath(), r.URL.RawQuery, body, err)
		}
		if r.Host != "localhost:1122" {
			t.Errorf("Host=%q", r.Host)
		}
		for name, want := range map[string]string{"Origin": "http://localhost:1122", "Cookie": "session=fixture; csrf=fixture", "Authorization": "Bearer fixture", "X-CSRF-Token": "fixture", "Idempotency-Key": key, "X-Request-Id": requestID} {
			if r.Header.Get(name) != want {
				t.Errorf("%s=%q", name, r.Header.Get(name))
			}
		}
		for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Arbitrary", "X-Real-IP", "X-User-ID", "X-Actor-ID", "X-Scope-ID", "X-Team-ID", "X-Project-ID", "X-Session-ID", "X-Authenticated-User", "X-Authenticated-Role"} {
			if r.Header.Get(name) != "" {
				t.Errorf("forged %s survived", name)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "rotated", Path: "/api", HttpOnly: true})
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(409)
		_, _ = io.WriteString(w, `{"code":"original_conflict"}`)
	})
	req, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/projects/%70/quotes?q=a%2Bb&q=c", strings.NewReader("opaque\x00payload"))
	req.Host = "localhost:1122"
	for name, value := range map[string]string{"Origin": "http://localhost:1122", "Cookie": "session=fixture; csrf=fixture", "Authorization": "Bearer fixture", "X-CSRF-Token": "fixture", "Idempotency-Key": key, "X-Request-Id": requestID} {
		req.Header.Set(name, value)
	}
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Arbitrary", "X-Real-IP", "X-User-ID", "X-Actor-ID", "X-Scope-ID", "X-Team-ID", "X-Project-ID", "X-Session-ID", "X-Authenticated-User", "X-Authenticated-Role"} {
		req.Header.Set(name, "forged")
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 409 || string(body) != `{"code":"original_conflict"}` || len(res.Cookies()) != 1 || res.Cookies()[0].Value != "rotated" || calls.Load() != 1 || res.Header.Get("X-Request-Id") != requestID {
		t.Fatalf("response changed: status=%d body=%s cookies=%v calls=%d", res.StatusCode, body, res.Cookies(), calls.Load())
	}
}

func TestDomainServeStreamsAndStopsSSEAndWebSocket(t *testing.T) {
	sseClosed, socketClosed := make(chan struct{}), make(chan struct{})
	_, origins := domainProxy(t, func(owner string, w http.ResponseWriter, r *http.Request) {
		if owner != "creation" {
			t.Errorf("stream reached %s", owner)
		}
		if strings.HasSuffix(r.URL.Path, "/events") {
			defer close(sseClosed)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: first\n\n")
			_ = http.NewResponseController(w).Flush()
			<-r.Context().Done()
			return
		}
		defer close(socketClosed)
		if r.Header.Get("Origin") != "http://localhost:1122" || r.Header.Get("Cookie") != "session=fixture" {
			t.Errorf("websocket security headers changed")
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		typ, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		_ = conn.Write(r.Context(), typ, data)
		_, _, _ = conn.Read(r.Context())
	})
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- appgateway.ServeConfig(ctx, listener, appgateway.Config{Origins: origins}, zap.NewNop())
	}()
	requestCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	base := "http://" + listener.Addr().String()
	res, err := get(requestCtx, http.DefaultClient, base+"/api/projects/p/agent/sessions/s/runs/r/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatalf("buffered SSE: line=%q error=%v", line, err)
	}
	conn, response, err := websocket.Dial(requestCtx, "ws://"+listener.Addr().String()+"/api/canvases/c/live", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://localhost:1122"}, "Cookie": {"session=fixture"}}})
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	if err := conn.Write(requestCtx, websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	typ, data, err := conn.Read(requestCtx)
	if err != nil || typ != websocket.MessageBinary || string(data) != string([]byte{1, 2, 3}) {
		t.Fatalf("WS changed: type=%v data=%v error=%v", typ, data, err)
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gateway did not stop")
	}
	for _, closed := range []chan struct{}{sseClosed, socketClosed} {
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("upstream stream survived shutdown")
		}
	}
}

func TestReadinessCycleTerminatesAtGateway(t *testing.T) {
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	origin := "http://" + server.Listener.Addr().String()
	handler, cleanup, err := gateway.NewHandler(origin, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	res, err := get(ctx, server.Client(), server.URL+"/readyz")
	if err != nil {
		t.Fatalf("readiness cycle did not terminate: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatalf("readiness cycle status=%d", res.StatusCode)
	}
}

func TestServeConfigRejectsActualListenerAsUpstream(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	origin := "http://" + listener.Addr().String()
	if err := appgateway.ServeConfig(t.Context(), listener, appgateway.Config{BackendOrigin: origin}, zap.NewNop()); err == nil {
		t.Fatal("actual ephemeral listener accepted as upstream")
	}
}
