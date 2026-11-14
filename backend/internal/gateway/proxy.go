// Package gateway forwards the public HTTP contract without owning business data.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// NewHandler builds the transitional public gateway. Only /api is forwarded;
// internal service routes remain private. The caller closes idle connections
// with cleanup after stopping the HTTP server. Authentication and resource
// authorization remain with the business API, not with client-supplied headers.
func NewHandler(origin string, logger *zap.Logger) (handler http.Handler, cleanup func(), err error) {
	target, err := url.Parse(origin)
	if err != nil || target == nil || (target.Scheme != "http" && target.Scheme != "https") ||
		target.Hostname() == "" || target.User != nil || target.Opaque != "" ||
		(target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" {
		return nil, nil, errors.New("gateway requires an HTTP(S) backend origin without credentials, path, query or fragment")
	}
	target.Path = ""
	if logger == nil {
		return nil, nil, errors.New("gateway requires a logger")
	}

	reads := http.DefaultTransport.(*http.Transport).Clone()
	reads.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	reads.ResponseHeaderTimeout = 60 * time.Second
	writes := reads.Clone()
	// Transport can transparently replay requests carrying Idempotency-Key on
	// a reused connection. Do not replay a mutation after an unknown receipt:
	// use a fresh connection for unsafe methods and leave recovery to the owner.
	writes.DisableKeepAlives = true
	transport := &proxyTransport{reads: reads, writes: writes}
	cleanup = func() {
		reads.CloseIdleConnections()
		writes.CloseIdleConnections()
	}
	probe := &http.Client{
		Transport: reads,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	readyURL := *target
	readyURL.Path = "/readyz"
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
			// The handler validates the query before forwarding it. Preserve its
			// original ordering and escaping rather than silently dropping fields.
			r.Out.URL.RawQuery = r.In.URL.RawQuery
			// Rewrite already strips standard Forwarded headers. Drop extensions
			// too rather than introducing an implicit forwarding/identity protocol.
			for name := range r.Out.Header {
				if strings.HasPrefix(strings.ToLower(name), "x-forwarded-") {
					r.Out.Header.Del(name)
				}
			}
			for _, name := range []string{"X-Real-IP", "X-User-ID", "X-Team-ID", "X-Project-ID", "X-Authenticated-User"} {
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
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			status := http.StatusServiceUnavailable
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL.String(), nil)
			if err == nil {
				res, err := probe.Do(req)
				if err == nil {
					_ = res.Body.Close()
					if res.StatusCode == http.StatusOK {
						status = http.StatusOK
					}
				}
			}
			writeHealth(w, status)
		case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
			if !validPath(r.URL.Path) {
				writeProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
				writeProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			proxy.ServeHTTP(w, r)
		default:
			writeProblem(w, r, http.StatusNotFound, "not_found")
		}
	})
	return handler, cleanup, nil
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

func validPath(path string) bool {
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
