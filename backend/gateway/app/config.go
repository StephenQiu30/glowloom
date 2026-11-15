// Package app assembles the independent, database-free public gateway.
package app

import (
	"errors"
	"net"
	"os"

	proxygateway "github.com/StephenQiu30/glowloom-server/backend/gateway/proxy"
)

// Config contains only the listener, upstream and logging settings. BackendOrigin
// selects the explicit pre-migration mode and cannot be mixed with Origins.
type Config struct {
	HTTPAddr      string
	BackendOrigin string
	Origins       proxygateway.Origins
	Env           string
	LogLevel      string
}

// LoadConfig reads explicit process settings without loading business secrets,
// environment files or database, Redis, Kafka and object storage configuration.
func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:      os.Getenv("GLOWLOOM_GATEWAY_HTTP_ADDR"),
		BackendOrigin: os.Getenv("GLOWLOOM_GATEWAY_BACKEND_ORIGIN"),
		Origins: proxygateway.Origins{
			Identity:  os.Getenv("GLOWLOOM_GATEWAY_IDENTITY_ORIGIN"),
			Creation:  os.Getenv("GLOWLOOM_GATEWAY_CREATION_ORIGIN"),
			Media:     os.Getenv("GLOWLOOM_GATEWAY_MEDIA_ORIGIN"),
			Execution: os.Getenv("GLOWLOOM_GATEWAY_EXECUTION_ORIGIN"),
		},
		Env:      os.Getenv("GLOWLOOM_ENV"),
		LogLevel: os.Getenv("GLOWLOOM_LOG_LEVEL"),
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
	if err := cfg.validate(cfg.HTTPAddr); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) validate(listener string) error {
	if _, _, err := net.SplitHostPort(listener); err != nil {
		return errors.New("GLOWLOOM_GATEWAY_HTTP_ADDR requires host:port")
	}
	if cfg.Origins.Configured() {
		if cfg.BackendOrigin != "" {
			return errors.New("gateway domain origins cannot be mixed with the pre-migration backend origin")
		}
		return proxygateway.ValidateOrigins(cfg.Origins, listener)
	}
	if cfg.BackendOrigin == "" {
		return errors.New("gateway requires four domain origins or an explicit pre-migration backend origin")
	}
	return proxygateway.ValidateLegacyOrigin(cfg.BackendOrigin, listener)
}
