package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
)

func TestStorageOperatorUsesUnifiedEnvironmentWithoutYAMLOrSecretOutput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runtime.env")
	raw := "GLOWLOOM_ENV=local\nGLOWLOOM_OBJECT_STORAGE_PROVIDER=tos\nGLOWLOOM_OBJECT_STORAGE_ENDPOINT=https://tos-cn-beijing.volces.com\nGLOWLOOM_OBJECT_STORAGE_BUCKET=fixture-storage\nGLOWLOOM_OBJECT_STORAGE_REGION=cn-beijing\nGLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=fixture-private-ak\nGLOWLOOM_OBJECT_STORAGE_SECRET_KEY=fixture-private-sk\n"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		key, value, _ := strings.Cut(line, "=")
		t.Setenv(key, value)
	}
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "")
	var out bytes.Buffer
	if err := storageoperator.Execute(t.Context(), []string{"validate"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tos-main") || strings.Contains(out.String(), "fixture-private") {
		t.Fatal("unexpected public configuration receipt", out.String())
	}
	if err := storageoperator.Execute(t.Context(), []string{"validate", "--config", "unread.yaml"}, &out); err == nil {
		t.Fatal("retired configuration path was accepted")
	}
	for _, command := range []string{"prove-empty", "finalize"} {
		t.Setenv("GLOWLOOM_STORAGE_MAINTENANCE_DSN", "")
		t.Setenv("LV_STORAGE_MAINTENANCE_DSN", "")
		if err := storageoperator.Execute(t.Context(), []string{command}, &out); err == nil || !strings.Contains(err.Error(), "MAINTENANCE_DSN") {
			t.Fatal("maintenance gate missing", command, err)
		}
	}
}

func TestStorageOperatorMaintenanceDSNComesFromUnifiedEnvFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runtime.env")
	raw := "GLOWLOOM_ENV=local\nGLOWLOOM_OBJECT_STORAGE_PROVIDER=tos\nGLOWLOOM_OBJECT_STORAGE_ENDPOINT=https://tos-cn-beijing.volces.com\nGLOWLOOM_OBJECT_STORAGE_BUCKET=fixture-storage\nGLOWLOOM_OBJECT_STORAGE_REGION=cn-beijing\nGLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=fixture-ak\nGLOWLOOM_OBJECT_STORAGE_SECRET_KEY=fixture-sk\nGLOWLOOM_STORAGE_MAINTENANCE_DSN=invalid-maintenance-dsn\n"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	// No process override for this key: it must be read by the same viper getter.
	old, exists := os.LookupEnv("GLOWLOOM_STORAGE_MAINTENANCE_DSN")
	if err := os.Unsetenv("GLOWLOOM_STORAGE_MAINTENANCE_DSN"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv("GLOWLOOM_STORAGE_MAINTENANCE_DSN", old)
		} else {
			_ = os.Unsetenv("GLOWLOOM_STORAGE_MAINTENANCE_DSN")
		}
	})
	var output bytes.Buffer
	err := storageoperator.Execute(t.Context(), []string{"register"}, &output)
	if err == nil || err.Error() != "storage maintenance database unavailable" {
		t.Fatal("maintenance DSN was not taken from unified env file", err)
	}
	if strings.Contains(output.String(), "fixture-") || strings.Contains(output.String(), "invalid-maintenance-dsn") {
		t.Fatal("private configuration escaped maintenance output")
	}
}
