package creationruntime_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/creationruntime"
)

func localConfig() creationruntime.Config {
	return creationruntime.Config{
		Env: "local", GRPCAddr: "127.0.0.1:0", HTTPAddr: "localhost:0",
		Token: strings.Repeat("c", 40), Concurrent: 2,
	}
}

func clearCreationEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GLOWLOOM_CREATION_ENV", "GLOWLOOM_CREATION_GRPC_ADDR", "GLOWLOOM_CREATION_HTTP_ADDR", "GLOWLOOM_CREATION_SERVICE_TOKEN",
		"GLOWLOOM_CREATION_TLS_CERT_FILE", "GLOWLOOM_CREATION_TLS_KEY_FILE", "GLOWLOOM_CREATION_TLS_CA_FILE", "GLOWLOOM_CREATION_CONCURRENCY",
	} {
		t.Setenv(key, "")
	}
}

func TestConfigLoadsOnlyCreationEnvironment(t *testing.T) {
	clearCreationEnvironment(t)
	for key, value := range map[string]string{
		"GLOWLOOM_ENV": "prod", "GLOWLOOM_MEDIA_ENV": "prod", "GLOWLOOM_MEDIA_CONCURRENCY": "invalid",
		"GLOWLOOM_MEDIA_GRPC_ADDR": "bad", "GLOWLOOM_SERVICE_TOKEN": "unrelated", "GLOWLOOM_MEDIA_SERVICE_TOKEN": "unrelated",
	} {
		t.Setenv(key, value)
	}
	t.Setenv("GLOWLOOM_CREATION_SERVICE_TOKEN", strings.Repeat("c", 32))
	cfg, err := creationruntime.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "local" || cfg.GRPCAddr != "127.0.0.1:9092" || cfg.HTTPAddr != "127.0.0.1:8083" || cfg.Concurrent != 2 || cfg.Token != strings.Repeat("c", 32) || cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.TLSCAFile != "" {
		t.Fatal("Creation defaults do not match independent private configuration")
	}
	for key, value := range map[string]string{
		"GLOWLOOM_CREATION_ENV": "staging", "GLOWLOOM_CREATION_GRPC_ADDR": "0.0.0.0:19092", "GLOWLOOM_CREATION_HTTP_ADDR": "0.0.0.0:18083",
		"GLOWLOOM_CREATION_SERVICE_TOKEN": strings.Repeat("s", 64), "GLOWLOOM_CREATION_TLS_CERT_FILE": "server.crt",
		"GLOWLOOM_CREATION_TLS_KEY_FILE": "server.key", "GLOWLOOM_CREATION_TLS_CA_FILE": "ca.crt", "GLOWLOOM_CREATION_CONCURRENCY": "8",
	} {
		t.Setenv(key, value)
	}
	cfg, err = creationruntime.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "staging" || cfg.GRPCAddr != "0.0.0.0:19092" || cfg.HTTPAddr != "0.0.0.0:18083" || cfg.Concurrent != 8 || cfg.Token != strings.Repeat("s", 64) || cfg.TLSCertFile != "server.crt" || cfg.TLSKeyFile != "server.key" || cfg.TLSCAFile != "ca.crt" {
		t.Fatal("Creation environment overrides were not applied")
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*creationruntime.Config)
	}{
		{"missing environment", func(c *creationruntime.Config) { c.Env = "" }},
		{"unknown environment", func(c *creationruntime.Config) { c.Env = "production" }},
		{"missing RPC address", func(c *creationruntime.Config) { c.GRPCAddr = "" }},
		{"missing health address", func(c *creationruntime.Config) { c.HTTPAddr = "" }},
		{"implicit RPC host", func(c *creationruntime.Config) { c.GRPCAddr = ":9092" }},
		{"implicit health host", func(c *creationruntime.Config) { c.HTTPAddr = ":8083" }},
		{"RPC missing port", func(c *creationruntime.Config) { c.GRPCAddr = "127.0.0.1" }},
		{"health missing port", func(c *creationruntime.Config) { c.HTTPAddr = "localhost" }},
		{"RPC invalid port", func(c *creationruntime.Config) { c.GRPCAddr = "127.0.0.1:grpc" }},
		{"health negative port", func(c *creationruntime.Config) { c.HTTPAddr = "127.0.0.1:-1" }},
		{"RPC signed port", func(c *creationruntime.Config) { c.GRPCAddr = "127.0.0.1:+9092" }},
		{"RPC port overflow", func(c *creationruntime.Config) { c.GRPCAddr = "127.0.0.1:65536" }},
		{"unbracketed IPv6", func(c *creationruntime.Config) { c.GRPCAddr = "::1:9092" }},
		{"invalid IPv4", func(c *creationruntime.Config) { c.GRPCAddr = "999.0.0.1:9092" }},
		{"invalid IPv6", func(c *creationruntime.Config) { c.GRPCAddr = "[::bad::]:9092" }},
		{"space host", func(c *creationruntime.Config) { c.GRPCAddr = "bad host:9092" }},
		{"slash host", func(c *creationruntime.Config) { c.GRPCAddr = "bad/host:9092" }},
		{"empty DNS label", func(c *creationruntime.Config) { c.GRPCAddr = "bad..host:9092" }},
		{"hyphen DNS label", func(c *creationruntime.Config) { c.GRPCAddr = "-bad:9092" }},
		{"shared listeners", func(c *creationruntime.Config) { c.HTTPAddr = c.GRPCAddr }},
		{"missing token", func(c *creationruntime.Config) { c.Token = "" }},
		{"short token", func(c *creationruntime.Config) { c.Token = strings.Repeat("x", 31) }},
		{"long token", func(c *creationruntime.Config) { c.Token = strings.Repeat("x", 4097) }},
		{"token leading space", func(c *creationruntime.Config) { c.Token = " " + c.Token }},
		{"token space", func(c *creationruntime.Config) { c.Token += " x" }},
		{"token newline", func(c *creationruntime.Config) { c.Token += "\nx" }},
		{"token carriage return", func(c *creationruntime.Config) { c.Token += "\rx" }},
		{"token tab", func(c *creationruntime.Config) { c.Token += "\tx" }},
		{"token NUL", func(c *creationruntime.Config) { c.Token += "\x00x" }},
		{"token Unicode", func(c *creationruntime.Config) { c.Token += "令牌" }},
		{"token DEL", func(c *creationruntime.Config) { c.Token += "\x7fx" }},
		{"zero concurrency", func(c *creationruntime.Config) { c.Concurrent = 0 }},
		{"negative concurrency", func(c *creationruntime.Config) { c.Concurrent = -1 }},
		{"excessive concurrency", func(c *creationruntime.Config) { c.Concurrent = 9 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			tc.change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid Creation configuration accepted")
			}
		})
	}
}

func TestConfigAcceptsBoundaryValuesAndExplicitHosts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		concurrency int
		tokenBytes  int
		rpc, health string
	}{
		{"minimum", 1, 32, "127.0.0.1:0", "[::1]:65535"},
		{"maximum", 8, 4096, "0.0.0.0:65535", "[::]:0"},
		{"DNS", 2, 40, "creation:9092", "creation.internal.:8083"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			cfg.Concurrent, cfg.Token, cfg.GRPCAddr, cfg.HTTPAddr = tc.concurrency, strings.Repeat("x", tc.tokenBytes), tc.rpc, tc.health
			if err := cfg.Validate(); err != nil {
				t.Fatalf("valid boundary rejected: %v", err)
			}
		})
	}
}

func TestConfigRequiresCompleteMTLSOutsideLocal(t *testing.T) {
	for _, env := range []string{"local", "staging", "prod"} {
		for mask := range 8 {
			t.Run(fmt.Sprintf("%s/TLS-%d", env, mask), func(t *testing.T) {
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
					t.Fatalf("TLS mask=%d error=%v, want valid=%v", mask, err, wantValid)
				}
			})
		}
	}
}

func TestLoadConfigRejectsInvalidConcurrency(t *testing.T) {
	clearCreationEnvironment(t)
	t.Setenv("GLOWLOOM_CREATION_SERVICE_TOKEN", strings.Repeat("c", 40))
	for _, raw := range []string{"not-a-number", "1.5", " 2 ", "-1", "0", "9"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("GLOWLOOM_CREATION_CONCURRENCY", raw)
			if _, err := creationruntime.LoadConfig(); err == nil {
				t.Fatal("invalid concurrency accepted")
			}
		})
	}
}
