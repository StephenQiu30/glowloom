package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	appgateway "github.com/StephenQiu30/glowloom-server/backend/gateway/app"
	gateway "github.com/StephenQiu30/glowloom-server/backend/gateway/proxy"
)

func domainProxy(t *testing.T, serve func(string, http.ResponseWriter, *http.Request)) (*httptest.Server, gateway.Origins) {
	t.Helper()
	origins := gateway.Origins{}
	for owner, origin := range map[string]*string{"identity": &origins.Identity, "creation": &origins.Creation, "media": &origins.Media, "execution": &origins.Execution} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(owner, w, r) }))
		t.Cleanup(upstream.Close)
		*origin = upstream.URL
	}
	handler, cleanup, err := gateway.NewDomainHandler(origins, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, origins
}

func TestDomainRoutesHaveOneOwner(t *testing.T) {
	var calls atomic.Int32
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Owner", owner)
		w.WriteHeader(http.StatusNoContent)
	})
	for _, step := range []struct{ method, path, owner string }{
		{"GET", "/api/session", "identity"},
		{"DELETE", "/api/sessions/device", "identity"},
		{"GET", "/api/me/spaces", "creation"},
		{"GET", "/api/projects/p", "creation"},
		{"GET", "/api/projects/p/budget", "creation"},
		{"POST", "/api/projects/p/prompt-drafts", "creation"},
		{"GET", "/api/projects/p/operations", "execution"},
		{"POST", "/api/projects/p/quotes", "execution"},
		{"POST", "/api/operations/o/confirm", "execution"},
		{"GET", "/api/billing/wallet", "execution"},
		{"POST", "/api/platform/billing/scopes/s/credit-commands", "execution"},
		{"POST", "/api/admin/providers", "execution"},
		{"POST", "/api/admin/provider-logos", "media"},
		{"GET", "/api/projects/p/media", "media"},
		{"POST", "/api/projects/p/media-exports", "media"},
		{"GET", "/api/projects/p/voice-library", "media"},
		{"GET", "/api/me/avatar", "media"},
		{"GET", "/api/subjects/s/versions/v/media/b", "creation"},
		{"GET", "/api/works/w", "execution"},
		{"GET", "/api/projects/p/agent/sessions/s/runs/r/events", "creation"},
	} {
		t.Run(step.method+" "+step.path, func(t *testing.T) {
			before := calls.Load()
			req, _ := http.NewRequestWithContext(t.Context(), step.method, server.URL+step.path, nil)
			res, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != 204 || res.Header.Get("Owner") != step.owner || calls.Load() != before+1 {
				t.Fatalf("status=%d owner=%q calls=%d", res.StatusCode, res.Header.Get("Owner"), calls.Load()-before)
			}
		})
	}
}

func TestDomainUnknownAndPrivateRoutesNeverForward(t *testing.T) {
	var calls atomic.Int32
	server, _ := domainProxy(t, func(_ string, w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) })
	for _, step := range []struct{ method, path string }{
		{"GET", "/api"}, {"GET", "/api/unknown"}, {"TRACE", "/api/projects"},
		{"POST", "/api/projects/p/operations"}, {"GET", "/api/projects/p/unknown"},
		{"GET", "/api/internal/billing/scopes/s/wallet"}, {"POST", "/internal/agent-business/tools/x/execute"},
		{"POST", "/api/internal/agent-business/tools/x/execute"}, {"GET", "/api/projects/p%2Foperations"},
		{"GET", "/api/projects/p/"}, {"GET", "/api//projects"}, {"GET", "/api/projects?q=%zz"},
	} {
		req, _ := http.NewRequestWithContext(t.Context(), step.method, server.URL+step.path, nil)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode < 400 || res.StatusCode >= 500 {
			t.Errorf("%s %s = %d", step.method, step.path, res.StatusCode)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected routes forwarded %d times", calls.Load())
	}
}

func TestDomainMutationLostReceiptHasNoFallbackOrReplay(t *testing.T) {
	var commands atomic.Int32
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, r *http.Request) {
		if owner != "execution" {
			t.Errorf("command reached %s", owner)
		}
		if r.Method == "GET" {
			w.WriteHeader(204)
			return
		}
		commands.Add(1)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	res, err := get(t.Context(), server.Client(), server.URL+"/api/operations/o")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	req, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/operations/o/confirm", strings.NewReader(`{"frozen":true}`))
	req.Header.Set("Idempotency-Key", "17eb0538-0a3e-45b0-88b8-9751432c0c34")
	res, err = server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 502 || !strings.Contains(string(body), "upstream_unavailable") || commands.Load() != 1 {
		t.Fatalf("status=%d body=%s commands=%d", res.StatusCode, body, commands.Load())
	}
}

func TestDomainConfigRequiresCompleteExclusiveOrigins(t *testing.T) {
	for _, name := range []string{"BACKEND", "IDENTITY", "CREATION", "MEDIA", "EXECUTION"} {
		t.Setenv("GLOWLOOM_GATEWAY_"+name+"_ORIGIN", "")
	}
	t.Setenv("GLOWLOOM_ENV", "local")
	t.Setenv("GLOWLOOM_GATEWAY_HTTP_ADDR", "127.0.0.1:18080")
	t.Setenv("GLOWLOOM_GATEWAY_IDENTITY_ORIGIN", "http://127.0.0.1:18081")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("partial domain configuration accepted")
	}
	t.Setenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN", "http://127.0.0.1:8080")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("partial domains mixed with legacy accepted")
	}
	for i, name := range []string{"CREATION", "MEDIA", "EXECUTION"} {
		t.Setenv("GLOWLOOM_GATEWAY_"+name+"_ORIGIN", []string{"http://127.0.0.1:18082", "http://127.0.0.1:18083", "http://127.0.0.1:18084"}[i])
	}
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("complete domains mixed with legacy accepted")
	}
	t.Setenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN", "")
	cfg, err := appgateway.LoadConfig()
	if err != nil || cfg.Origins.Identity == "" || cfg.Origins.Execution == "" {
		t.Fatalf("complete domains rejected: %+v %v", cfg, err)
	}
	t.Setenv("GLOWLOOM_GATEWAY_MEDIA_ORIGIN", "http://127.0.0.1:18081")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("two owners sharing an origin accepted")
	}
	t.Setenv("GLOWLOOM_GATEWAY_MEDIA_ORIGIN", "http://localhost:18080")
	if _, err := appgateway.LoadConfig(); err == nil {
		t.Fatal("gateway self-loop accepted")
	}
}

func TestDomainReadinessChecksEveryOwnerWithoutCycles(t *testing.T) {
	var probes atomic.Int32
	var failing atomic.Bool
	failing.Store(true)
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" || r.Method != "GET" {
			t.Errorf("unexpected probe %s %s", r.Method, r.URL.Path)
		}
		probes.Add(1)
		if owner == "media" && failing.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	})
	for _, want := range []int{503, 200} {
		before := probes.Load()
		res, err := get(context.Background(), server.Client(), server.URL+"/readyz")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != want || probes.Load() != before+4 {
			t.Fatalf("status=%d probes=%d", res.StatusCode, probes.Load()-before)
		}
		failing.Store(false)
	}
}

func TestDomainOriginsRejectInvalidAndEquivalentTargets(t *testing.T) {
	for _, invalid := range []string{"", "http://localhost:", "http://localhost:0", "http://localhost:65536", "http://localhost/api", "http://localhost?private=value", "file:///tmp/backend"} {
		t.Run(invalid, func(t *testing.T) {
			origins := gateway.Origins{Identity: "http://127.0.0.1:18081", Creation: "http://127.0.0.1:18082", Media: invalid, Execution: "http://127.0.0.1:18084"}
			_, cleanup, err := gateway.NewDomainHandler(origins, zap.NewNop())
			if cleanup != nil {
				cleanup()
			}
			if err == nil || strings.Contains(err.Error(), "private=value") {
				t.Fatalf("invalid target accepted or disclosed: %v", err)
			}
		})
	}
	origins := gateway.Origins{Identity: "http://localhost:18081", Creation: "http://127.0.0.1:18081/", Media: "http://127.0.0.1:18083", Execution: "http://127.0.0.1:18084"}
	if err := gateway.ValidateOrigins(origins, "127.0.0.1:18080"); err == nil {
		t.Fatal("equivalent targets accepted")
	}
}
