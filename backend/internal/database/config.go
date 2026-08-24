package database

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

func MigrationURL() (string, error) {
	rawURL := os.Getenv("MIGRATION_DATABASE_URL")
	if rawURL == "" {
		rawURL = os.Getenv("DATABASE_URL")
	}
	if rawURL == "" {
		return "", errors.New("MIGRATION_DATABASE_URL or DATABASE_URL is required")
	}
	return NormalizePostgresURL(rawURL)
}

func NormalizePostgresURL(rawURL string) (string, error) {
	normalized := strings.Replace(rawURL, "postgresql+asyncpg://", "postgresql://", 1)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", errors.New("database URL must use the postgres or postgresql scheme")
	}
	if parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
		return "", errors.New("database URL must include a host and database name")
	}
	return parsed.String(), nil
}
