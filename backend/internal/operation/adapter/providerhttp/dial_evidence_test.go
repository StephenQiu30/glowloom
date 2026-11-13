package providerhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// Before TCP connect the server cannot have accepted the POST. A peer that
// reads the request and closes without a response must remain uncertain.
func TestDialEvidenceRequiresNoConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	cfg := domain.ProviderConfig{BaseURL: server.URL, TimeoutSeconds: 5}
	c := NewClient(server.Client())
	_, _, err := c.request(t.Context(), cfg, "fixture", http.MethodPost, "/", map[string]int{"input": 1}, 128)
	server.Close()
	if err == nil || errors.Is(err, errDialNotSent) {
		t.Fatalf("lost response classified as %v", err)
	}
	c = NewClient(&http.Client{Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: errors.New("offline")}
	}}})
	_, _, err = c.request(t.Context(), cfg, "fixture", http.MethodPost, "/", map[string]int{}, 128)
	if !errors.Is(err, errDialNotSent) {
		t.Fatalf("dial error classified as %v", err)
	}
}
