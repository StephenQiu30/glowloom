// Package config loads and validates process configuration from LV_* environment variables.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ErrInvalid reports a configuration value that fails validation.
var ErrInvalid = errors.New("invalid configuration")

// Config is the validated configuration shared by every backend role.
type Config struct {
	Env      string
	HTTPAddr string
	LogLevel string
}

var (
	validEnvs      = map[string]bool{"local": true, "staging": true, "prod": true}
	validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// Load reads configuration from the environment, applies defaults and validates the result.
func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("LV")
	v.AutomaticEnv()
	v.SetDefault("env", "local")
	v.SetDefault("http_addr", ":8080")
	v.SetDefault("log_level", "info")

	cfg := Config{
		Env:      strings.TrimSpace(v.GetString("env")),
		HTTPAddr: strings.TrimSpace(v.GetString("http_addr")),
		LogLevel: strings.TrimSpace(v.GetString("log_level")),
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
	return nil
}
