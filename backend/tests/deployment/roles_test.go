package deployment_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// These source-contract tests do not read .env, expand credentials, connect to
// middleware, or substitute for Docker Compose's own rendered-config check.
func TestComposeSeparatesAPITasksAndRelay(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		name := "ordinary"
		if overlay {
			name = "development"
		}
		t.Run(name, func(t *testing.T) {
			v := readDeployment(t, overlay)
			for _, role := range []struct {
				service string
				command []string
				port    string
			}{
				{"backend", []string{"--role=api"}, "18080"},
				{"worker", []string{"--role=worker", "--queues=auto"}, "8081"},
				{"relay", []string{"--role=relay"}, "8082"},
			} {
				prefix := "services." + role.service + "."
				if got := v.GetStringSlice(prefix + "command"); !slices.Equal(got, role.command) {
					t.Errorf("%s command = %v; want %v", role.service, got, role.command)
				}
				if got := v.GetStringSlice(prefix + "ports"); len(got) != 0 {
					t.Errorf("%s publishes host ports: %v", role.service, got)
				}
				if got := v.GetStringSlice(prefix + "expose"); !slices.Equal(got, []string{role.port}) {
					t.Errorf("%s exposes %v; want only %s", role.service, got, role.port)
				}
				probe := strings.Join(v.GetStringSlice(prefix+"healthcheck.test"), " ")
				if !strings.Contains(probe, "http://127.0.0.1:"+role.port+"/healthz") || strings.Contains(probe, "&&") {
					t.Errorf("%s health couples roles or omits its listener: %q", role.service, probe)
				}
				if v.GetString(prefix+"image") != v.GetString("services.backend.image") {
					t.Errorf("%s uses a different application build", role.service)
				}
				for _, other := range []string{"backend", "worker", "relay"} {
					if v.IsSet(prefix + "depends_on." + other) {
						t.Errorf("%s startup depends on application role %s", role.service, other)
					}
				}
				if role.service != "backend" && v.GetBool(prefix+"environment.GLOWLOOM_COMPOSE_API_PROXY") {
					t.Errorf("%s enables the public API bridge", role.service)
				}
			}
			if !v.GetBool("services.backend.environment.GLOWLOOM_COMPOSE_API_PROXY") {
				t.Error("API role lost its required loopback bridge")
			}
			if got := v.GetString("services.gateway.environment.GLOWLOOM_GATEWAY_BACKEND_ORIGIN"); got != "http://backend:18080" {
				t.Errorf("gateway origin = %q; must remain the API role", got)
			}
			if overlay {
				for _, service := range []string{"worker", "relay"} {
					if got := v.Get("services." + service + ".develop.watch"); got == nil {
						t.Errorf("%s does not watch current backend source", service)
					}
				}
			}
		})
	}
}

func readDeployment(t *testing.T, overlay bool) *viper.Viper {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate deployment sources")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../.."))
	v := viper.New()
	v.SetConfigType("yaml")
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return data
	}
	if err := v.ReadConfig(bytes.NewReader(read("docker-compose.yml"))); err != nil {
		t.Fatalf("parse ordinary deployment: %v", err)
	}
	if overlay {
		if err := v.MergeConfig(bytes.NewReader(read("docker-compose.dev.yml"))); err != nil {
			t.Fatalf("parse development overlay: %v", err)
		}
	}
	return v
}
