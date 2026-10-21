package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevEntrypointConfigurationPrefixPriority(t *testing.T) {
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	_, script, found := strings.Cut(string(dockerfile), "COPY --chmod=755 <<'ENTRYPOINT' /usr/local/bin/glowloom-dev-entrypoint\n")
	if !found {
		t.Fatal("development entrypoint unavailable")
	}
	script, _, found = strings.Cut(script, "\nENTRYPOINT\n")
	if !found {
		t.Fatal("development entrypoint delimiter unavailable")
	}
	for _, tc := range []struct {
		name   string
		env    []string
		want   []string
		reject []string
	}{
		{name: "legacy", env: []string{"LV_HOST_SERVICE_PORTS=5432", "LV_DEV_API_PROXY=true"}, want: []string{"TCP-LISTEN:5432", "TCP-LISTEN:18080"}},
		{name: "canonical", env: []string{"LV_HOST_SERVICE_PORTS=5432", "LV_DEV_API_PROXY=true", "GLOWLOOM_HOST_SERVICE_PORTS=6123", "GLOWLOOM_DEV_API_PROXY=false"}, want: []string{"TCP-LISTEN:6123"}, reject: []string{"TCP-LISTEN:5432", "TCP-LISTEN:18080"}},
		{name: "explicit_empty", env: []string{"LV_HOST_SERVICE_PORTS=5432", "LV_DEV_API_PROXY=true", "GLOWLOOM_HOST_SERVICE_PORTS=", "GLOWLOOM_DEV_API_PROXY="}, reject: []string{"TCP-LISTEN:"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			runtime := filepath.Join(root, "runtime")
			for _, dir := range []string{bin, runtime} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for path, body := range map[string]string{filepath.Join(bin, "go"): "#!/bin/sh\nexit 0\n", filepath.Join(bin, "socat"): "#!/bin/sh\nprintf '%s\\n' \"$*\"\n", filepath.Join(runtime, "glowloom"): "#!/bin/sh\necho entrypoint-started\n"} {
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", strings.ReplaceAll(script, "/tmp/glowloom-dev", runtime))
			cmd.Env = append([]string{"PATH=" + bin + ":/usr/bin:/bin", "GLOWLOOM_ENV_FILE="}, tc.env...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("entrypoint: %v\n%s", err, output)
			}
			text := string(output)
			if !strings.Contains(text, "entrypoint-started") {
				t.Fatal("application was not started")
			}
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s: %s", want, text)
				}
			}
			for _, reject := range tc.reject {
				if strings.Contains(text, reject) {
					t.Fatalf("unexpected legacy fallback %s: %s", reject, text)
				}
			}
		})
	}
}
