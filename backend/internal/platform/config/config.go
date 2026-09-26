// Package config loads and validates process configuration from LV_* environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
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

// Load reads the optional environment file and process variables, then validates the result.
func Load() (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("LV_ENV", "local")
	v.SetDefault("LV_HTTP_ADDR", ":8080")
	v.SetDefault("LV_LOG_LEVEL", "info")
	if path := os.Getenv("LV_ENV_FILE"); path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("%w: read LV_ENV_FILE: %w", ErrInvalid, err)
		}
	}

	cfg := Config{
		Env:      strings.TrimSpace(v.GetString("LV_ENV")),
		HTTPAddr: strings.TrimSpace(v.GetString("LV_HTTP_ADDR")),
		LogLevel: strings.TrimSpace(v.GetString("LV_LOG_LEVEL")),
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
