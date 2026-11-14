// Package mediaruntime composes the independently deployed Media processor.
package mediaruntime

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Config contains only this process's listeners, identity and capacity.
// It deliberately has no business database, broker or object-storage settings.
type Config struct {
	Env         string
	GRPCAddr    string
	HTTPAddr    string
	Token       string
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
	Concurrent  int
}

// LoadConfig does not load the business environment file or global config.
func LoadConfig() (Config, error) {
	cfg := Config{Env: envOr("GLOWLOOM_MEDIA_ENV", "local"), GRPCAddr: envOr("GLOWLOOM_MEDIA_GRPC_ADDR", ":9091"), HTTPAddr: envOr("GLOWLOOM_MEDIA_HTTP_ADDR", ":8081"), Token: os.Getenv("GLOWLOOM_MEDIA_SERVICE_TOKEN"), TLSCertFile: os.Getenv("GLOWLOOM_MEDIA_TLS_CERT_FILE"), TLSKeyFile: os.Getenv("GLOWLOOM_MEDIA_TLS_KEY_FILE"), TLSCAFile: os.Getenv("GLOWLOOM_MEDIA_TLS_CA_FILE"), Concurrent: 2}
	if raw := os.Getenv("GLOWLOOM_MEDIA_CONCURRENCY"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, errors.New("invalid media concurrency")
		}
		cfg.Concurrent = value
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses partial identity settings and plaintext outside local mode.
func (c Config) Validate() error {
	if c.Env != "local" && c.Env != "staging" && c.Env != "prod" {
		return errors.New("invalid media environment")
	}
	if !listenAddress(c.GRPCAddr) || !listenAddress(c.HTTPAddr) || c.GRPCAddr == c.HTTPAddr {
		return errors.New("invalid media listener configuration")
	}
	if !serviceauth.ValidToken(c.Token) {
		return errors.New("media service token must contain 32 to 4096 visible ASCII bytes without whitespace")
	}
	if c.Concurrent < 1 || c.Concurrent > 8 {
		return errors.New("media concurrency must be between 1 and 8")
	}
	anyTLS := c.TLSCertFile != "" || c.TLSKeyFile != "" || c.TLSCAFile != ""
	allTLS := c.TLSCertFile != "" && c.TLSKeyFile != "" && c.TLSCAFile != ""
	if (anyTLS && !allTLS) || (c.Env != "local" && !allTLS) {
		return errors.New("media requires a complete mTLS identity outside local mode")
	}
	return nil
}

func (c Config) serverTLS() (*tls.Config, error) {
	if c.TLSCertFile == "" {
		return nil, nil
	}
	pair, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load media TLS identity: %w", err)
	}
	data, err := os.ReadFile(c.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("load media TLS authorities: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("invalid media TLS authorities")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}

func listenAddress(value string) bool {
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 0 && number <= 65535
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
