package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func storageOperatorEnvironment(t *testing.T, extra string) {
	t.Helper()
	cfg := storagefixture.Configuration(t, "http://127.0.0.1:1", "fixture-storage")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "runtime.env")
	body := "GLOWLOOM_ENV=local\nGLOWLOOM_OBJECT_STORAGE_CONFIG=" + string(raw) + "\nTEST_STORAGE_AK=fixture-private-ak\nTEST_STORAGE_SK=fixture-private-sk\n" + extra
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", file)
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", string(raw))
	t.Setenv("TEST_STORAGE_AK", "fixture-private-ak")
	t.Setenv("TEST_STORAGE_SK", "fixture-private-sk")
}

func TestStorageOperatorUsesUnifiedEnvironmentWithoutYAMLOrSecretOutput(t *testing.T) {
	storageOperatorEnvironment(t, "")
	var out bytes.Buffer
	if err := storageoperator.Execute(t.Context(), []string{"validate"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "test-main") || strings.Contains(out.String(), "fixture-private") {
		t.Fatal("unexpected public configuration receipt", out.String())
	}
	if err := storageoperator.Execute(t.Context(), []string{"validate", "--config", "unread.yaml"}, &out); err == nil {
		t.Fatal("retired configuration path was accepted")
	}
	for _, command := range []string{"prove-empty", "check-schema"} {
		t.Setenv("GLOWLOOM_STORAGE_MAINTENANCE_DSN", "")
		t.Setenv("LV_STORAGE_MAINTENANCE_DSN", "")
		if err := storageoperator.Execute(t.Context(), []string{command}, &out); err == nil || !strings.Contains(err.Error(), "MAINTENANCE_DSN") {
			t.Fatal("maintenance gate missing", command, err)
		}
	}
}

func TestStorageOperatorMaintenanceDSNComesFromUnifiedEnvFile(t *testing.T) {
	storageOperatorEnvironment(t, "GLOWLOOM_STORAGE_MAINTENANCE_DSN=invalid-maintenance-dsn\n")
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
	if strings.Contains(output.String(), "fixture-private") || strings.Contains(output.String(), "invalid-maintenance-dsn") {
		t.Fatal("private configuration escaped maintenance output")
	}
}
