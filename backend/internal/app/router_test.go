package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestHealthz(t *testing.T) {
	router := NewRouter(zap.NewNop(), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"status":"ok"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	router := NewRouter(zap.NewNop(), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/unknown", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestReadyzReportsDependencyFailureWithoutDetails(t *testing.T) {
	check := func(context.Context) error { return errors.New("secret connection detail") }
	router := NewRouter(zap.NewNop(), check)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Body.String(); got != `{"status":"unavailable"}` {
		t.Errorf("body = %s, want generic unavailable status", got)
	}
}

func TestReadyzReportsHealthyDependencies(t *testing.T) {
	check := func(context.Context) error { return nil }
	router := NewRouter(zap.NewNop(), check)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("status = %d, body = %s, want 200 ok", rec.Code, rec.Body.String())
	}
}
