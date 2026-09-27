// Package config loads and validates process configuration from LV_* environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ErrInvalid reports a configuration value that fails validation.
var ErrInvalid = errors.New("invalid configuration")

// Config is the validated configuration shared by every backend role.
type Config struct {
	Env                       string
	HTTPAddr                  string
	WorkerHealthAddr          string
	RelayHealthAddr           string
	LogLevel                  string
	DBDSN                     string
	PartitionMaintenanceDBDSN string
	RedisURL                  string
	SessionIdleTTL            time.Duration
	SessionAbsoluteTTL        time.Duration
	KafkaBrokers              string
	TemporalAddr              string
	TemporalNamespace         string
	ObjectStorageEndpoint     string
	ObjectStorageBucket       string
	ObjectStorageAccessKey    string
	ObjectStorageSecretKey    string
	ObjectStorageRegion       string
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
	v.SetDefault("LV_HTTP_ADDR", ":8080")
	v.SetDefault("LV_WORKER_HEALTH_ADDR", ":8081")
	v.SetDefault("LV_RELAY_HEALTH_ADDR", ":8082")
	v.SetDefault("LV_LOG_LEVEL", "info")
	v.SetDefault("LV_SESSION_IDLE_TTL", "12h")
	v.SetDefault("LV_SESSION_ABSOLUTE_TTL", "168h")
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
		WorkerHealthAddr:          strings.TrimSpace(v.GetString("LV_WORKER_HEALTH_ADDR")),
		RelayHealthAddr:           strings.TrimSpace(v.GetString("LV_RELAY_HEALTH_ADDR")),
		LogLevel:                  strings.TrimSpace(v.GetString("LV_LOG_LEVEL")),
		DBDSN:                     strings.TrimSpace(v.GetString("LV_DB_DSN")),
		PartitionMaintenanceDBDSN: strings.TrimSpace(v.GetString("LV_PARTITION_MAINTENANCE_DB_DSN")),
		RedisURL:                  strings.TrimSpace(v.GetString("LV_REDIS_URL")),
		SessionIdleTTL:            v.GetDuration("LV_SESSION_IDLE_TTL"),
		SessionAbsoluteTTL:        v.GetDuration("LV_SESSION_ABSOLUTE_TTL"),
		KafkaBrokers:              strings.TrimSpace(v.GetString("LV_KAFKA_BROKERS")),
		TemporalAddr:              strings.TrimSpace(v.GetString("LV_TEMPORAL_ADDR")),
		TemporalNamespace:         strings.TrimSpace(v.GetString("LV_TEMPORAL_NAMESPACE")),
		ObjectStorageEndpoint:     strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageBucket:       strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_BUCKET")),
		ObjectStorageAccessKey:    v.GetString("LV_OBJECT_STORAGE_ACCESS_KEY"),
		ObjectStorageSecretKey:    v.GetString("LV_OBJECT_STORAGE_SECRET_KEY"),
		ObjectStorageRegion:       strings.TrimSpace(v.GetString("LV_OBJECT_STORAGE_REGION")),
		OTelEndpoint:              strings.TrimSpace(v.GetString("LV_OTEL_ENDPOINT")),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
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
	if c.SessionIdleTTL < time.Millisecond {
		return fmt.Errorf("%w: LV_SESSION_IDLE_TTL must be at least 1ms", ErrInvalid)
	}
	if c.SessionAbsoluteTTL < c.SessionIdleTTL {
		return fmt.Errorf("%w: LV_SESSION_ABSOLUTE_TTL must be at least LV_SESSION_IDLE_TTL", ErrInvalid)
	}
	return nil
}
