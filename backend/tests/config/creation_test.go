package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestCreationClientConfigurationUsesCurrentKeysAndHidesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.env.example")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOWLOOM_ENV_FILE", path)
	identity := strings.Repeat("c", 48) + " "
	for key, value := range map[string]string{
		"GLOWLOOM_CREATION_EXTRACTION_ADDRESS":       "creation:9092",
		"GLOWLOOM_CREATION_SERVICE_TOKEN":            identity,
		"GLOWLOOM_CREATION_EXTRACTION_TLS_CA_FILE":   "/fixture/ca.pem",
		"GLOWLOOM_CREATION_EXTRACTION_TLS_CERT_FILE": "/fixture/client.pem",
		"GLOWLOOM_CREATION_EXTRACTION_TLS_KEY_FILE":  "/fixture/client-key.pem",
		"GLOWLOOM_CREATION_EXTRACTION_SERVER_NAME":   "creation.internal",
	} {
		t.Setenv(key, value)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CreationExtractionAddress != "creation:9092" || cfg.CreationServiceToken != identity || cfg.CreationExtractionTLSCAFile != "/fixture/ca.pem" || cfg.CreationExtractionTLSCertFile != "/fixture/client.pem" || cfg.CreationExtractionTLSKeyFile != "/fixture/client-key.pem" || cfg.CreationExtractionServerName != "creation.internal" {
		t.Fatal("Creation configuration was not loaded exactly; identity must not be normalized")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), strings.TrimSpace(identity)) {
		t.Fatal("serialized configuration disclosed Creation service identity")
	}
}
