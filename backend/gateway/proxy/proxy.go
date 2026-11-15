// Package proxy forwards the public HTTP contract without owning business data.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

const probeHeader = "X-Glowloom-Gateway-Probe"

// NewHandler is the explicit pre-migration single-backend mode. It must not be
// used by the four-owner mode, including when any owner becomes unavailable.
func NewHandler(origin string, logger *zap.Logger) (http.Handler, func(), error) {
	target, err := parseOrigin(origin)
	if err != nil {
		return nil, nil, err
	}
	return newHandler(map[string]*url.URL{"legacy": target}, true, logger)
}

// NewDomainHandler routes only registered method/path pairs to their unique
// owner. All four distinct origins are required; there is no fallback backend.
func NewDomainHandler(origins Origins, logger *zap.Logger) (http.Handler, func(), error) {
	targets, err := origins.targets()
	if err != nil {
		return nil, nil, err
	}
	return newHandler(targets, false, logger)
}

func newHandler(targets map[string]*url.URL, legacy bool, logger *zap.Logger) (http.Handler, func(), error) {
	if logger == nil {
		return nil, nil, errors.New("gateway requires a logger")
	}
	reads := http.DefaultTransport.(*http.Transport).Clone()
	reads.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	reads.ResponseHeaderTimeout = 60 * time.Second
	writes := reads.Clone()
	// Transport can transparently replay requests carrying Idempotency-Key on
	// a reused connection. A mutation always uses a fresh connection; recovery
	// after unknown receipt belongs to the owner and the original command.
	writes.DisableKeepAlives = true
	transport := &proxyTransport{reads: reads, writes: writes}
	cleanup := func() { reads.CloseIdleConnections(); writes.CloseIdleConnections() }
	proxies := make(map[string]*httputil.ReverseProxy, len(targets))
	for owner, target := range targets {
		proxies[owner] = newForwarder(target, transport, logger)
	}
	probe := &http.Client{
		Transport:     reads,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	routes := publicRoutes()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if parsed, err := uuid.Parse(id); err != nil || parsed == uuid.Nil {
			id = uuid.NewString()
		}
		r.Header.Set("X-Request-Id", id)
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("Cache-Control", "no-store")
		switch {
		case r.URL.Path == "/healthz" && r.Method == http.MethodGet:
			writeHealth(w, http.StatusOK)
		case r.URL.Path == "/readyz" && r.Method == http.MethodGet:
			// A readiness dependency must terminate at a business service, never
			// at another gateway. The marker stops direct and indirect cycles.
			if r.Header.Get(probeHeader) != "" {
				writeHealth(w, http.StatusServiceUnavailable)
				return
			}
			writeHealth(w, readiness(r.Context(), probe, targets))
		case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
			if !validPath(r.URL.Path) || (!legacy && ambiguousEscaping(r.URL.EscapedPath())) {
				writeProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
				writeProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			if r.URL.Path == "/api/internal" || strings.HasPrefix(r.URL.Path, "/api/internal/") {
				writeProblem(w, r, http.StatusNotFound, "not_found")
				return
			}
			owner := "legacy"
			if !legacy {
				owner = routeOwner(routes, r.Method, r.URL.Path)
			}
			forwarder := proxies[owner]
			if forwarder == nil {
				writeProblem(w, r, http.StatusNotFound, "not_found")
				return
			}
			forwarder.ServeHTTP(w, r)
		default:
			writeProblem(w, r, http.StatusNotFound, "not_found")
		}
	})
	return handler, cleanup, nil
}

func newForwarder(target *url.URL, transport http.RoundTripper, logger *zap.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
			// Keep byte ordering and escaping after validation; do not silently
			// drop query fields, rebuild bodies or inspect authentication payloads.
			r.Out.URL.RawQuery = r.In.URL.RawQuery
			for name := range r.Out.Header {
				lower := strings.ToLower(name)
				if strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "x-authenticated-") {
					r.Out.Header.Del(name)
				}
			}
			for _, name := range []string{"X-Real-IP", "X-User-ID", "X-Team-ID", "X-Project-ID", "X-Actor-ID", "X-Scope-ID", "X-Session-ID", probeHeader} {
				r.Out.Header.Del(name)
			}
		},
		Transport:     transport,
		FlushInterval: -1,
		ErrorLog:      zap.NewStdLog(logger.Named("proxy")),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status, code := http.StatusBadGateway, "upstream_unavailable"
			var timeout net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
				status, code = http.StatusGatewayTimeout, "upstream_timeout"
			}
			if r.Context().Err() == nil {
				logger.Warn("gateway upstream unavailable", zap.String("request_id", r.Header.Get("X-Request-Id")), zap.Int("status", status))
			}
			writeProblem(w, r, status, code)
		},
	}
}

func readiness(parent context.Context, client *http.Client, targets map[string]*url.URL) int {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	var group sync.WaitGroup
	results := make(chan bool, len(targets))
	for _, target := range targets {
		group.Go(func() {
			readyURL := *target
			readyURL.Path = "/readyz"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL.String(), nil)
			if err != nil {
				results <- false
				return
			}
			req.Header.Set(probeHeader, "1")
			res, err := client.Do(req)
			if err != nil {
				results <- false
				return
			}
			_ = res.Body.Close()
			results <- res.StatusCode == http.StatusOK
		})
	}
	group.Wait()
	close(results)
	status := http.StatusOK
	for ready := range results {
		if !ready {
			status = http.StatusServiceUnavailable
		}
	}
	return status
}

// proxyTransport shares connections only for safe methods. It does not retry
// application commands, replace idempotency identities or rebuild request bodies.
type proxyTransport struct {
	reads  *http.Transport
	writes *http.Transport
}

func (t *proxyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return t.reads.RoundTrip(r)
	default:
		return t.writes.RoundTrip(r)
	}
}

func ambiguousEscaping(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%25")
}

func validPath(path string) bool {
	for _, ch := range path {
		if ch < 0x20 || ch == 0x7f || ch == '\\' {
			return false
		}
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "." || part == ".." || (part == "" && i != 0 && i != len(parts)-1) {
			return false
		}
	}
	return true
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httpapi.Problem{
		Type:  "https://glowloom.local/errors/" + code,
		Title: http.StatusText(status), Status: status, Code: code,
		Detail: http.StatusText(status), RequestID: r.Header.Get("X-Request-Id"),
	})
}

func writeHealth(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	state := "unavailable"
	if status == http.StatusOK {
		state = "ok"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}
