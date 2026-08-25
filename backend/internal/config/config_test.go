package config

import (
	"testing"
	"time"
)

func TestLoadBuildsSingleDatabaseAPIConfiguration(t *testing.T) {
	t.Setenv("API_HOST", "127.0.0.1")
	t.Setenv("API_PORT", "8765")
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ListenAddress != "127.0.0.1:8765" {
		t.Fatalf("listen address = %q", configuration.ListenAddress)
	}
	if configuration.DatabaseURL != "postgresql://lanverse:secret@database:5432/lanverse" {
		t.Fatalf("database URL = %q", configuration.DatabaseURL)
	}
	if configuration.AccessTokenTTL != 30*time.Minute || configuration.SessionTTL != 30*24*time.Hour {
		t.Fatalf("unexpected token TTLs: %s, %s", configuration.AccessTokenTTL, configuration.SessionTTL)
	}
	if len(configuration.AllowedOrigins) != 2 {
		t.Fatalf("allowed origins = %#v", configuration.AllowedOrigins)
	}
	if configuration.ObjectStoreRegion != "us-east-1" {
		t.Fatalf("object store region = %q", configuration.ObjectStoreRegion)
	}
	if configuration.AgentURL != "http://127.0.0.1:8787" || configuration.AgentPollInterval != 500*time.Millisecond {
		t.Fatalf("unexpected agent configuration: %q, %s", configuration.AgentURL, configuration.AgentPollInterval)
	}
}

func TestLoadRejectsInvalidCORSOrigins(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	t.Setenv("CORS_ORIGINS", `["not-an-origin"]`)
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an invalid CORS origin")
	}
}

func TestLoadRejectsInvalidConfiguredRegistrationCode(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	t.Setenv("REGISTRATION_VERIFICATION_CODE", "12345")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an invalid registration verification code")
	}
}

func TestLoadRequiresStandardPostgreSQLDatabaseURL(t *testing.T) {
	for _, value := range []string{"", "postgresql+asyncpg://database/lanverse", "http://database/lanverse"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted DATABASE_URL %q", value)
			}
		})
	}
}
