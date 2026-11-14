// Package gateway assembles the independent, database-free public gateway.
package gateway

import (
	"errors"
	"net"
	"os"
)

// Config contains only the gateway's own listener, upstream and logging settings.
type Config struct {
	HTTPAddr      string
	BackendOrigin string
	Env           string
	LogLevel      string
}

// LoadConfig reads explicit process settings without loading business secrets,
// environment files or database, Redis, Kafka and object storage configuration.
func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:      os.Getenv("GLOWLOOM_GATEWAY_HTTP_ADDR"),
		BackendOrigin: os.Getenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN"),
		Env:           os.Getenv("GLOWLOOM_ENV"),
		LogLevel:      os.Getenv("GLOWLOOM_LOG_LEVEL"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:18080"
	}
	if cfg.Env == "" {
		cfg.Env = "local"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Env != "local" {
		return Config{}, errors.New("gateway currently requires the reviewed local topology")
	}
	if cfg.BackendOrigin == "" {
		return Config{}, errors.New("GLOWLOOM_GATEWAY_BACKEND_ORIGIN is required")
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return Config{}, errors.New("GLOWLOOM_GATEWAY_HTTP_ADDR requires host:port")
	}
	return cfg, nil
}
