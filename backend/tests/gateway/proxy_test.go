package gateway_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/gateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func get(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func newProxy(t *testing.T, origin string) *httptest.Server {
	t.Helper()
	handler, cleanup, err := gateway.NewHandler(origin, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestProxyPreservesPublicContract(t *testing.T) {
	key, requestID := uuid.NewString(), uuid.NewString()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api/projects/a%2Fb" || r.URL.RawQuery != "q=a%2Bb&q=c" || string(body) != `{"name":"draft"}` {
			t.Errorf("request contract changed: method=%s path=%s query=%s body=%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery, body)
		}
		for header, want := range map[string]string{
			"Host": "localhost:1122", "Origin": "http://localhost:1122", "Authorization": "Bearer fixture", "Cookie": "session=fixture", "Idempotency-Key": key, "X-Request-Id": requestID,
		} {
			got := r.Header.Get(header)
			if header == "Host" {
				got = r.Host
			}
			if got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "rotated", Path: "/api", HttpOnly: true})
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":"original_conflict"}`)
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL+"/api/projects/a%2Fb?q=a%2Bb&q=c", strings.NewReader(`{"name":"draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:1122"
	for header, value := range map[string]string{"Origin": "http://localhost:1122", "Authorization": "Bearer fixture", "Cookie": "session=fixture", "Idempotency-Key": key, "X-Request-Id": requestID} {
		req.Header.Set(header, value)
	}
	res, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusConflict || string(body) != `{"code":"original_conflict"}` || res.Header.Get("Content-Type") != "application/problem+json" || len(res.Cookies()) != 1 || res.Cookies()[0].Value != "rotated" || calls.Load() != 1 {
		t.Fatalf("response contract changed: status=%d headers=%v body=%s calls=%d", res.StatusCode, res.Header, body, calls.Load())
	}
}

func TestProxyDoesNotTrustForwardedIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Prefix", "X-Forwarded-Port", "X-Forwarded-Arbitrary", "X-Forwarded-User", "X-Real-IP", "X-User-ID", "X-Team-ID", "X-Project-ID", "X-Authenticated-User"} {
			if got := r.Header.Get(header); got != "" {
				t.Errorf("untrusted %s survived: %q", header, got)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, proxy.URL+"/api/projects", nil)
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Prefix", "X-Forwarded-Port", "X-Forwarded-Arbitrary", "X-Forwarded-User", "X-Real-IP", "X-User-ID", "X-Team-ID", "X-Project-ID", "X-Authenticated-User"} {
		req.Header.Set(header, "forged")
	}
	res, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", res.StatusCode)
	}
}

func TestProxyRejectsNonPublicAndAmbiguousPaths(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	for _, path := range []string{"/internal/agent", "/metrics", "/api-private", "/api/../internal/agent", "/api/%2e%2e/internal/agent", "/api//projects", "/api/projects?q=%zz", "/api/projects?q=a;b"} {
		t.Run(path, func(t *testing.T) {
			res, err := get(t.Context(), proxy.Client(), proxy.URL+path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode < 400 || res.StatusCode >= 500 {
				t.Fatalf("status=%d, want local rejection", res.StatusCode)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected routes reached upstream %d times", calls.Load())
	}
}

func TestProxyHealthAndReadinessAreSeparate(t *testing.T) {
	var ready atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("unexpected probe: %s", r.URL.Path)
		}
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "private dependency failure")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	proxy := newProxy(t, upstream.URL)
	for _, step := range []struct {
		path  string
		ready bool
		want  int
	}{{"/healthz", false, 200}, {"/readyz", false, 503}, {"/readyz", true, 200}} {
		ready.Store(step.ready)
		res, err := get(t.Context(), proxy.Client(), proxy.URL+step.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil || res.StatusCode != step.want || strings.Contains(string(body), "private") {
			t.Fatalf("%s: status=%d body=%s error=%v", step.path, res.StatusCode, body, readErr)
		}
	}
	upstream.Close()
	res, err := get(t.Context(), proxy.Client(), proxy.URL+"/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("liveness depends on downstream: %d", res.StatusCode)
	}
}

func TestProxyFailureUsesPublicProblem(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	origin := upstream.URL
	upstream.Close()
	proxy := newProxy(t, origin)
	res, err := get(t.Context(), proxy.Client(), proxy.URL+"/api/projects?secret=do-not-echo")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var problem httpapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 502 || problem.Code != "upstream_unavailable" || problem.RequestID == "" || strings.Contains(problem.Detail, origin) || strings.Contains(problem.Detail, "secret") || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unsafe error response: status=%d problem=%+v", res.StatusCode, problem)
	}
}

func TestProxyStreamsBeforeUpstreamCompletesAndCancels(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/api/events", nil)
	res, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	_ = res.Body.Close()
	if err != nil || line != "data: first\n" {
		t.Fatalf("stream buffered: line=%q error=%v", line, err)
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("client cancellation did not reach upstream")
	}
}

func TestProxyWebSocketRoundTrip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		typ, data, err := conn.Read(r.Context())
		if err == nil {
			_ = conn.Write(r.Context(), typ, data)
		}
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(proxy.URL, "http")+"/api/canvas/live", nil)
	if res != nil && res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	if err := conn.Write(ctx, websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	typ, data, err := conn.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || string(data) != string([]byte{1, 2, 3}) {
		t.Fatalf("websocket changed: type=%v data=%v error=%v", typ, data, err)
	}
}

func TestProxyRejectsUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{"", "file:///tmp/backend", "http://user:secret@localhost", "http://localhost/api", "http://localhost?token=secret", "http://localhost#fragment"} {
		t.Run(origin, func(t *testing.T) {
			_, cleanup, err := gateway.NewHandler(origin, zap.NewNop())
			if cleanup != nil {
				cleanup()
			}
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("origin validation error=%v", err)
			}
		})
	}
}
