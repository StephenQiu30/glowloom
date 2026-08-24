package config

import (
	"testing"
	"time"
)

func TestLoadBuildsRunnableAPIConfiguration(t *testing.T) {
	t.Setenv("API_HOST", "127.0.0.1")
	t.Setenv("API_PORT", "8765")
	t.Setenv("LEGACY_API_URL", "http://agent-api:8686")
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "2.5")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if configuration.ListenAddress != "127.0.0.1:8765" {
		t.Fatalf("listen address = %q", configuration.ListenAddress)
	}
	if configuration.LegacyAPIURL.String() != "http://agent-api:8686" {
		t.Fatalf("legacy URL = %q", configuration.LegacyAPIURL)
	}
	if configuration.UpstreamTimeout != 2500*time.Millisecond {
		t.Fatalf("upstream timeout = %s", configuration.UpstreamTimeout)
	}
}

func TestLoadRejectsNonHTTPUpstream(t *testing.T) {
	t.Setenv("LEGACY_API_URL", "postgres://database/lanverse")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a non-HTTP legacy API URL")
	}
}
