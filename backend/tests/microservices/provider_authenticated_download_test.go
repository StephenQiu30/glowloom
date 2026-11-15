package microservices_test

import (
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
)

func TestProviderAuthenticatedDownloadNeverFollowsRedirectWithCredential(t *testing.T) {
	var first, redirect atomic.Int32
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/original" {
			first.Add(1)
			if r.Header.Get("Authorization") != "Bearer synthetic-test-only" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/redirect", http.StatusTemporaryRedirect)
			return
		}
		redirect.Add(1)
		w.Header().Set("Content-Type", "audio/wave")
		_, _ = w.Write(providerMusicWAV())
	}))
	defer peer.Close()
	roots := x509.NewCertPool()
	roots.AddCert(peer.Certificate())
	downloader, err := fetch.NewSecureDownloader(fetch.DownloadPolicy{AllowedOrigins: []string{peer.URL}, AcceptedMIME: []string{"audio/wave"}, TLSRootCAs: roots, MaxBytes: 128 << 20, AllowTestLoopbackTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := downloader.DownloadAuthenticated(t.Context(), peer.URL+"/original", "Bearer synthetic-test-only"); !errors.Is(err, fetch.ErrResultUnavailable) {
		if result != nil {
			_ = result.Close()
		}
		t.Fatalf("redirect with credential accepted: %v", err)
	}
	if first.Load() != 1 || redirect.Load() != 0 {
		t.Fatalf("authenticated request redirected: first=%d redirect=%d", first.Load(), redirect.Load())
	}
	if _, err := downloader.DownloadAuthenticated(t.Context(), peer.URL+"/original", "Bearer value\r\nInjected: true"); !errors.Is(err, fetch.ErrUnsafeResultURL) {
		t.Fatalf("header injection accepted: %v", err)
	}
	if first.Load() != 1 {
		t.Fatal("invalid credential header caused IO")
	}
}
