// Package config loads and validates process configuration from LV_* environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// ErrInvalid reports a configuration value that fails validation.
var ErrInvalid = errors.New("invalid configuration")

// Config is the validated configuration shared by every backend role.
type Config struct {
	Env                       string
	HTTPAddr                  string
	PublicOrigin              string
	WorkerHealthAddr          string
	RelayHealthAddr           string
	LogLevel                  string
	DBDSN                     string
	PartitionMaintenanceDBDSN string
	RedisURL                  string
	KafkaBrokers              string
	TemporalAddr              string
	TemporalNamespace         string
	ObjectStorageEndpoint     string
	ObjectStorageBucket       string
	ObjectStorageAccessKey    string
	ObjectStorageSecretKey    string
	ObjectStorageRegion       string
	CredentialPublicKeyFile   string
	CredentialKeyID           string
	MediaResultAllowedOrigins string
	MediaAllowTestLoopbackTLS bool
	OTelEndpoint              string
}

var (
	validEnvs      = map[string]bool{"local": true, "staging": true, "prod": true}
	validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// Load reads the optional environment file and process variables, then validates the result.
func Load() (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("LV_ENV", "local")
	v.SetDefault("LV_HTTP_ADDR", "127.0.0.1:8080")
	v.SetDefault("LV_PUBLIC_ORIGIN", "http://localhost:3000")
	v.SetDefault("LV_WORKER_HEALTH_ADDR", ":8081")
	v.SetDefault("LV_RELAY_HEALTH_ADDR", ":8082")
	v.SetDefault("LV_LOG_LEVEL", "info")
	if path := os.Getenv("LV_ENV_FILE"); path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("%w: read LV_ENV_FILE: %w", ErrInvalid, err)
		}
	}

	cfg := Config{
		Env:                       strings.TrimSpace(v.GetString("LV_ENV")),
		HTTPAddr:                  strings.TrimSpace(v.GetString("LV_HTTP_ADDR")),
		PublicOrigin:              strings.TrimSpace(v.GetString("LV_PUBLIC_ORIGIN")),
		WorkerHealthAddr:          strings.TrimSpace(v.GetString("LV_WORKER_HEALTH_ADDR")),
		RelayHealthAddr:           strings.TrimSpace(v.GetString("LV_RELAY_HEALTH_ADDR")),
		LogLevel:                  strings.TrimSpace(v.GetString("LV_LOG_LEVEL")),
		DBDSN:                     strings.TrimSpace(v.GetString("LV_DB_DSN")),
		PartitionMaintenanceDBDSN: strings.TrimSpace(v.GetString("LV_PARTITION_MAINTENANCE_DB_DSN")),
		RedisURL:                  strings.TrimSpace(v.GetString("LV_REDIS_URL")),
		KafkaBrokers:              strings.TrimSpace(v.GetString("LV_KAFKA_BROKERS")),
		TemporalAddr:              strings.TrimSpace(v.GetString("LV_TEMPORAL_ADDR")),
		TemporalNamespace:         strings.TrimSpace(v.GetString("LV_TEMPORAL_NAMESPACE")),
		ObjectStorageEndpoint:     strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageBucket:       strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_BUCKET")),
		ObjectStorageAccessKey:    v.GetString("LV_OBJECT_STORAGE_ACCESS_KEY"),
		ObjectStorageSecretKey:    v.GetString("LV_OBJECT_STORAGE_SECRET_KEY"),
		ObjectStorageRegion:       strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_REGION")),
		CredentialPublicKeyFile:   strings.TrimSpace(v.GetString("LV_CREDENTIAL_PUBLIC_KEY_FILE")),
		CredentialKeyID:           strings.TrimSpace(v.GetString("LV_CREDENTIAL_KEY_ID")),
		MediaResultAllowedOrigins: strings.TrimSpace(v.GetString("LV_MEDIA_RESULT_ALLOWED_ORIGINS")),
		MediaAllowTestLoopbackTLS: v.GetBool("LV_MEDIA_ALLOW_TEST_LOOPBACK_TLS"),
		OTelEndpoint:              strings.TrimSpace(v.GetString("LV_OTEL_ENDPOINT")),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if (c.CredentialPublicKeyFile == "") != (c.CredentialKeyID == "") || len(c.CredentialKeyID) > 128 {
		return fmt.Errorf("%w: credential public key file and key ID must be configured together", ErrInvalid)
	}
	if c.MediaAllowTestLoopbackTLS && c.Env != "local" {
		return fmt.Errorf("%w: LV_MEDIA_ALLOW_TEST_LOOPBACK_TLS is local-only", ErrInvalid)
	}
	if !validEnvs[c.Env] {
		return fmt.Errorf("%w: LV_ENV=%q, want local|staging|prod", ErrInvalid, c.Env)
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%w: LV_LOG_LEVEL=%q, want debug|info|warn|error", ErrInvalid, c.LogLevel)
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("%w: LV_HTTP_ADDR is empty", ErrInvalid)
	}
	if c.WorkerHealthAddr == "" {
		return fmt.Errorf("%w: LV_WORKER_HEALTH_ADDR is empty", ErrInvalid)
	}
	if c.RelayHealthAddr == "" {
		return fmt.Errorf("%w: LV_RELAY_HEALTH_ADDR is empty", ErrInvalid)
	}
	return nil
}

// ValidatePublicOrigin requires a configured browser origin, HTTPS outside local development.
func ValidatePublicOrigin(origin, env string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: LV_PUBLIC_ORIGIN must be an exact HTTP origin", ErrInvalid)
	}
	if u.Scheme == "http" && (env != "local" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return fmt.Errorf("%w: HTTP public origin is local-loopback only", ErrInvalid)
	}
	return nil
}
