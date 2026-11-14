package mediaruntime_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/mediaruntime"
)

func localConfig() mediaruntime.Config {
	return mediaruntime.Config{
		Env:        "local",
		GRPCAddr:   "127.0.0.1:0",
		HTTPAddr:   "localhost:0",
		Token:      strings.Repeat("m", 40),
		Concurrent: 2,
	}
}

func clearMediaEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GLOWLOOM_MEDIA_ENV", "GLOWLOOM_MEDIA_GRPC_ADDR", "GLOWLOOM_MEDIA_HTTP_ADDR", "GLOWLOOM_MEDIA_SERVICE_TOKEN",
		"GLOWLOOM_MEDIA_TLS_CERT_FILE", "GLOWLOOM_MEDIA_TLS_KEY_FILE", "GLOWLOOM_MEDIA_TLS_CA_FILE", "GLOWLOOM_MEDIA_CONCURRENCY",
	} {
		t.Setenv(key, "")
	}
}

func TestConfigLoadsOnlyMediaEnvironment(t *testing.T) {
	clearMediaEnvironment(t)
	t.Setenv("GLOWLOOM_ENV", "prod")
	t.Setenv("GLOWLOOM_MEDIA_SERVICE_TOKEN", strings.Repeat("m", 32))
	cfg, err := mediaruntime.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "local" || cfg.GRPCAddr != ":9091" || cfg.HTTPAddr != ":8081" || cfg.Concurrent != 2 || cfg.Token != strings.Repeat("m", 32) || cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.TLSCAFile != "" {
		t.Fatalf("independent media defaults=%+v", cfg)
	}

	for key, value := range map[string]string{
		"GLOWLOOM_MEDIA_ENV":           "staging",
		"GLOWLOOM_MEDIA_GRPC_ADDR":     "127.0.0.1:19091",
		"GLOWLOOM_MEDIA_HTTP_ADDR":     "127.0.0.1:18081",
		"GLOWLOOM_MEDIA_SERVICE_TOKEN": strings.Repeat("s", 64),
		"GLOWLOOM_MEDIA_TLS_CERT_FILE": "server.crt",
		"GLOWLOOM_MEDIA_TLS_KEY_FILE":  "server.key",
		"GLOWLOOM_MEDIA_TLS_CA_FILE":   "ca.crt",
		"GLOWLOOM_MEDIA_CONCURRENCY":   "8",
	} {
		t.Setenv(key, value)
	}
	cfg, err = mediaruntime.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "staging" || cfg.GRPCAddr != "127.0.0.1:19091" || cfg.HTTPAddr != "127.0.0.1:18081" || cfg.Concurrent != 8 || cfg.Token != strings.Repeat("s", 64) || cfg.TLSCertFile != "server.crt" || cfg.TLSKeyFile != "server.key" || cfg.TLSCAFile != "ca.crt" {
		t.Fatalf("explicit media configuration=%+v", cfg)
	}
}

func TestConfigRejectsInvalidEnvironmentListenersTokenAndCapacity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*mediaruntime.Config)
	}{
		{"missing environment", func(c *mediaruntime.Config) { c.Env = "" }},
		{"unknown environment", func(c *mediaruntime.Config) { c.Env = "production" }},
		{"missing RPC address", func(c *mediaruntime.Config) { c.GRPCAddr = "" }},
		{"missing health address", func(c *mediaruntime.Config) { c.HTTPAddr = "" }},
		{"RPC missing port", func(c *mediaruntime.Config) { c.GRPCAddr = "127.0.0.1" }},
		{"health missing port", func(c *mediaruntime.Config) { c.HTTPAddr = "localhost" }},
		{"RPC invalid port", func(c *mediaruntime.Config) { c.GRPCAddr = "127.0.0.1:grpc" }},
		{"health negative port", func(c *mediaruntime.Config) { c.HTTPAddr = "127.0.0.1:-1" }},
		{"RPC port overflow", func(c *mediaruntime.Config) { c.GRPCAddr = "127.0.0.1:65536" }},
		{"unbracketed IPv6", func(c *mediaruntime.Config) { c.GRPCAddr = "::1:9091" }},
		{"shared listeners", func(c *mediaruntime.Config) { c.HTTPAddr = c.GRPCAddr }},
		{"missing token", func(c *mediaruntime.Config) { c.Token = "" }},
		{"short token", func(c *mediaruntime.Config) { c.Token = strings.Repeat("x", 31) }},
		{"long token", func(c *mediaruntime.Config) { c.Token = strings.Repeat("x", 4097) }},
		{"leading token whitespace", func(c *mediaruntime.Config) { c.Token = " " + c.Token }},
		{"trailing token whitespace", func(c *mediaruntime.Config) { c.Token += " " }},
		{"embedded token LF", func(c *mediaruntime.Config) { c.Token += "\nx" }},
		{"embedded token CR", func(c *mediaruntime.Config) { c.Token += "\rx" }},
		{"embedded token TAB", func(c *mediaruntime.Config) { c.Token += "\tx" }},
		{"embedded token NUL", func(c *mediaruntime.Config) { c.Token += "\x00x" }},
		{"embedded token space", func(c *mediaruntime.Config) { c.Token += " x" }},
		{"embedded token Unicode", func(c *mediaruntime.Config) { c.Token += "令牌" }},
		{"embedded token DEL", func(c *mediaruntime.Config) { c.Token += "\x7fx" }},
		{"zero concurrency", func(c *mediaruntime.Config) { c.Concurrent = 0 }},
		{"negative concurrency", func(c *mediaruntime.Config) { c.Concurrent = -1 }},
		{"excessive concurrency", func(c *mediaruntime.Config) { c.Concurrent = 9 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			tc.change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid media configuration accepted")
			}
		})
	}
}

func TestConfigAcceptsLocalBoundaryValues(t *testing.T) {
	for _, tc := range []struct {
		name        string
		concurrency int
		tokenBytes  int
		rpc         string
		health      string
	}{
		{"minimum", 1, 32, ":0", "[::1]:65535"},
		{"maximum", 8, 4096, "127.0.0.1:65535", ":0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			cfg.Concurrent, cfg.Token, cfg.GRPCAddr, cfg.HTTPAddr = tc.concurrency, strings.Repeat("x", tc.tokenBytes), tc.rpc, tc.health
			if err := cfg.Validate(); err != nil {
				t.Fatalf("valid media boundary rejected: %v", err)
			}
		})
	}
}

func TestConfigRequiresCompleteMTLSOutsideLocal(t *testing.T) {
	for _, env := range []string{"local", "staging", "prod"} {
		for mask := range 8 {
			name := env + "/"
			if mask&1 != 0 {
				name += "cert/"
			}
			if mask&2 != 0 {
				name += "key/"
			}
			if mask&4 != 0 {
				name += "ca/"
			}
			t.Run(name, func(t *testing.T) {
				cfg := localConfig()
				cfg.Env = env
				if mask&1 != 0 {
					cfg.TLSCertFile = "server.crt"
				}
				if mask&2 != 0 {
					cfg.TLSKeyFile = "server.key"
				}
				if mask&4 != 0 {
					cfg.TLSCAFile = "ca.crt"
				}
				err := cfg.Validate()
				wantValid := mask == 7 || env == "local" && mask == 0
				if (err == nil) != wantValid {
					t.Fatalf("environment=%s TLS mask=%d: error=%v, want valid=%v", env, mask, err, wantValid)
				}
			})
		}
	}
}

func TestLoadConfigRejectsInvalidConcurrency(t *testing.T) {
	clearMediaEnvironment(t)
	t.Setenv("GLOWLOOM_MEDIA_SERVICE_TOKEN", strings.Repeat("m", 40))
	for _, raw := range []string{"not-a-number", "1.5", " 2 ", "-1", "0", "9"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("GLOWLOOM_MEDIA_CONCURRENCY", raw)
			if _, err := mediaruntime.LoadConfig(); err == nil {
				t.Fatalf("invalid concurrency %q accepted", raw)
			}
		})
	}
}
