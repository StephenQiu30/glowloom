package toolkit

import (
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	if token, ok := BearerToken("  bearer  token-value "); !ok || token != "token-value" {
		t.Fatalf("BearerToken() = %q, %v", token, ok)
	}
	for _, header := range []string{"", "Bearer", "Basic token-value", "Bearer token one"} {
		if token, ok := BearerToken(header); ok || token != "" {
			t.Fatalf("BearerToken(%q) = %q, %v; want invalid", header, token, ok)
		}
	}
}

func TestClientIPUsesRemoteAddrOnly(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	if got := ClientIP(request); got != "192.0.2.10" {
		t.Fatalf("ClientIP() = %q, want 192.0.2.10", got)
	}
}
