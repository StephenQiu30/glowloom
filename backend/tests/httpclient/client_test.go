package httpclient_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpclient"
)

func TestIsPublicAddr(t *testing.T) {
	cases := []struct {
		addr   string
		public bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"100.64.0.1", false},
		{"169.254.169.254", false},
		{"168.63.129.16", false},
		{"0.0.0.0", false},
		{"198.18.0.1", false},
		{"2001:db8::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:8.8.8.8", true},
	}
	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.addr)
		if got := httpclient.IsPublicAddr(addr); got != tc.public {
			t.Errorf("IsPublicAddr(%s) = %v, want %v", tc.addr, got, tc.public)
		}
	}
	if httpclient.IsPublicAddr(netip.Addr{}) {
		t.Error("zero netip.Addr must not be public")
	}
}

func TestNewRestyRejectsRedirectsAndEnforcesBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/target", http.StatusFound)
		case "/large":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(strings.Repeat("a", 256)))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	client := httpclient.NewResty(
		httpclient.WithBaseClient(srv.Client()),
		httpclient.WithTimeout(5*time.Second),
		httpclient.WithResponseBodyLimit(64),
	)

	resp, err := client.R().SetContext(t.Context()).Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusFound {
		t.Fatalf("redirect should not be followed: got status %d, want 302", resp.StatusCode())
	}

	_, err = client.R().SetContext(t.Context()).Get(srv.URL + "/large")
	if !errors.Is(err, httpclient.ErrResponseBodyTooLarge) {
		t.Fatalf("expected ErrResponseBodyTooLarge, got %v", err)
	}
}

func TestNewRestyDefaultsToNoRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := httpclient.NewResty(httpclient.WithBaseClient(srv.Client()))
	resp, err := client.R().SetContext(t.Context()).Post(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want 503", resp.StatusCode())
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("default Resty client must not retry: got %d attempts, want 1", got)
	}
}
