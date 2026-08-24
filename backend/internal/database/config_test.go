package database

import (
	"strings"
	"testing"
)

func TestNormalizePostgresURLAcceptsAgentAsyncURLWithoutExposingCredentials(t *testing.T) {
	normalized, err := NormalizePostgresURL(
		"postgresql+asyncpg://lanverse:secret@postgres:5432/lanverse",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(normalized, "postgresql://") {
		t.Fatalf("normalized scheme = %q", normalized)
	}
}

func TestNormalizePostgresURLRejectsIncompleteOrForeignURLs(t *testing.T) {
	for _, rawURL := range []string{
		"redis://redis:6379/0",
		"postgresql:///lanverse",
		"postgresql://postgres:5432",
	} {
		if _, err := NormalizePostgresURL(rawURL); err == nil {
			t.Fatalf("NormalizePostgresURL accepted %q", rawURL)
		}
	}
}
