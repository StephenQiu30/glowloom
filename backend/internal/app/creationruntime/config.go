// Package creationruntime composes the independently deployed Creation extractor.
package creationruntime

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Config contains only the extractor's listeners, private identity and capacity.
// Business databases, brokers and object storage belong to the caller process.
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

// LoadConfig reads only GLOWLOOM_CREATION_* variables, never a business .env file.
func LoadConfig() (Config, error) {
	cfg := Config{
		Env:         envOr("GLOWLOOM_CREATION_ENV", "local"),
		GRPCAddr:    envOr("GLOWLOOM_CREATION_GRPC_ADDR", "127.0.0.1:9092"),
		HTTPAddr:    envOr("GLOWLOOM_CREATION_HTTP_ADDR", "127.0.0.1:8083"),
		Token:       os.Getenv("GLOWLOOM_CREATION_SERVICE_TOKEN"),
		TLSCertFile: os.Getenv("GLOWLOOM_CREATION_TLS_CERT_FILE"),
		TLSKeyFile:  os.Getenv("GLOWLOOM_CREATION_TLS_KEY_FILE"),
		TLSCAFile:   os.Getenv("GLOWLOOM_CREATION_TLS_CA_FILE"),
		Concurrent:  2,
	}
	if raw := os.Getenv("GLOWLOOM_CREATION_CONCURRENCY"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, errors.New("invalid creation concurrency")
		}
		cfg.Concurrent = value
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses implicit listener hosts, partial identities and nonlocal plaintext.
func (c Config) Validate() error {
	if c.Env != "local" && c.Env != "staging" && c.Env != "prod" {
		return errors.New("invalid creation environment")
	}
	if !listenAddress(c.GRPCAddr) || !listenAddress(c.HTTPAddr) || c.GRPCAddr == c.HTTPAddr {
		return errors.New("invalid creation listener configuration")
	}
	if !serviceauth.ValidToken(c.Token) {
		return errors.New("creation service token must contain 32 to 4096 visible ASCII bytes without whitespace")
	}
	if c.Concurrent < 1 || c.Concurrent > 8 {
		return errors.New("creation concurrency must be between 1 and 8")
	}
	anyTLS := c.TLSCertFile != "" || c.TLSKeyFile != "" || c.TLSCAFile != ""
	allTLS := c.TLSCertFile != "" && c.TLSKeyFile != "" && c.TLSCAFile != ""
	if anyTLS && !allTLS || c.Env != "local" && !allTLS {
		return errors.New("creation requires a complete mTLS identity outside local mode")
	}
	return nil
}

func (c Config) serverTLS() (*tls.Config, error) {
	if c.TLSCertFile == "" {
		return nil, nil
	}
	pair, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load creation TLS identity: %w", err)
	}
	data, err := os.ReadFile(c.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("load creation TLS authorities: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("invalid creation TLS authorities")
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair},
		ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert,
	}, nil
}

func listenAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" || port == "" {
		return false
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(port)
	if err != nil || number > 65535 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	// Validate DNS syntax without resolving it at config load time. Reject an
	// invalid dotted numeric IP rather than treating it as a DNS hostname.
	if strings.Trim(host, "0123456789.") == "" {
		return false
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-':
			default:
				return false
			}
		}
	}
	return true
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
