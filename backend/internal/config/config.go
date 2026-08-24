package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	defaultAPIHost         = "0.0.0.0"
	defaultAPIPort         = 8686
	defaultLegacyAPIURL    = "http://127.0.0.1:8787"
	defaultUpstreamTimeout = 3 * time.Second
)

type Config struct {
	ListenAddress   string
	LegacyAPIURL    *url.URL
	UpstreamTimeout time.Duration
}

func Load() (Config, error) {
	host := environmentValue("API_HOST", defaultAPIHost)
	port, err := positiveInteger("API_PORT", defaultAPIPort)
	if err != nil {
		return Config{}, err
	}
	if port > 65535 {
		return Config{}, fmt.Errorf("API_PORT must not exceed 65535")
	}

	legacyAPIURL, err := url.Parse(environmentValue("LEGACY_API_URL", defaultLegacyAPIURL))
	if err != nil {
		return Config{}, fmt.Errorf("parse LEGACY_API_URL: %w", err)
	}
	if legacyAPIURL.Host == "" || (legacyAPIURL.Scheme != "http" && legacyAPIURL.Scheme != "https") {
		return Config{}, fmt.Errorf("LEGACY_API_URL must be an absolute HTTP(S) URL")
	}

	upstreamTimeout, err := positiveDurationSeconds(
		"UPSTREAM_TIMEOUT_SECONDS",
		defaultUpstreamTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ListenAddress:   net.JoinHostPort(host, strconv.Itoa(port)),
		LegacyAPIURL:    legacyAPIURL,
		UpstreamTimeout: upstreamTimeout,
	}, nil
}

func environmentValue(name string, fallback string) string {
	value, exists := os.LookupEnv(name)
	if !exists || value == "" {
		return fallback
	}
	return value
}

func positiveInteger(name string, fallback int) (int, error) {
	rawValue := environmentValue(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(rawValue)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func positiveDurationSeconds(name string, fallback time.Duration) (time.Duration, error) {
	rawValue := environmentValue(name, strconv.FormatFloat(fallback.Seconds(), 'f', -1, 64))
	seconds, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive number", name)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
