package catalog_test

import (
	"net/netip"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestGatewayBaseURLBoundary(t *testing.T) {
	for _, raw := range []string{"https://gateway.example.com/v1", "https://gateway.example.com/api/openai/v1/", "https://8.8.8.8:8443/v1", "https://[2606:4700:4700::1111]/v1"} {
		if err := domain.ValidateGatewayBaseURL(raw); err != nil {
			t.Errorf("public gateway %q rejected: %v", raw, err)
		}
	}
	for _, raw := range []string{"http://gateway.example.com", "https://user:secret@gateway.example.com/v1", "https://gateway.example.com/v1?key=secret", "https://gateway.example.com/v1?", "https://gateway.example.com/v1#fragment", "https://localhost/v1", "https://service.local/v1", "https://127.0.0.1/v1", "https://[::ffff:127.0.0.1]/v1", "https://169.254.169.254/v1", "https://100.64.0.1/v1", "https://192.0.2.1/v1", "https://[2002:7f00:1::]/v1", "https://2130706433/v1", "https://0177.0.0.1/v1", "https://gateway.example.com:0/v1", "https://gateway.example.com:65536/v1", "https://gateway.example.com/a/../v1", "https://gateway.example.com/%2e%2e/v1", "https://gateway.example.com//v1"} {
		if err := domain.ValidateGatewayBaseURL(raw); err == nil {
			t.Errorf("unsafe gateway %q accepted", raw)
		}
	}
}

func TestGatewayOnlyExtendsInstalledOpenAIProtocol(t *testing.T) {
	for _, adapter := range []string{"openai", "deepseek", "openrouter", "vercel", "volcengine_ark"} {
		spec, _ := domain.GetAdapterSpec(adapter)
		config := domain.ProviderConfig{AdapterKey: adapter, BaseURL: "https://gateway.example.com/v1", Region: spec.AllowedRegion, Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 60}
		if got := config.Validate() == nil; got != (adapter == "openai") {
			t.Errorf("%s gateway admission = %v", adapter, got)
		}
		if got := config.IsCustomGateway(); got != (adapter == "openai") {
			t.Errorf("%s custom classification = %v", adapter, got)
		}
		config.BaseURL = spec.AllowedBaseURL
		if config.Validate() != nil || config.IsCustomGateway() {
			t.Errorf("%s official configuration changed", adapter)
		}
	}
}

func TestPublicProviderIPRejectsSpecialRanges(t *testing.T) {
	for _, raw := range []string{"0.0.0.0", "10.0.0.1", "100.100.100.200", "127.0.0.1", "168.63.129.16", "169.254.169.254", "172.31.1.1", "192.168.1.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "::", "::1", "::ffff:10.0.0.1", "fc00::1", "fe80::1", "2001:db8::1", "2002:0a00:1::", "3fff::1"} {
		if domain.IsPublicProviderIP(netip.MustParseAddr(raw)) {
			t.Errorf("special address %s accepted", raw)
		}
	}
}
