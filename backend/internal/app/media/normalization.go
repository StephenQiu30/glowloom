package media

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/normalizationgrpc"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ProvideUploadNormalizer configures the real remote processing dependency.
// Missing configuration disables only WebM conversion, never a local fallback.
func ProvideUploadNormalizer(cfg config.Config) (*normalizationgrpc.Client, error) {
	if cfg.MediaNormalizationAddress == "" && cfg.MediaServiceToken == "" {
		if cfg.MediaNormalizationTLSCAFile != "" || cfg.MediaNormalizationTLSCertFile != "" || cfg.MediaNormalizationTLSKeyFile != "" || cfg.MediaNormalizationServerName != "" {
			return nil, errors.New("media normalization TLS identity has no target")
		}
		return normalizationgrpc.New("", "", nil)
	}
	if _, _, err := net.SplitHostPort(cfg.MediaNormalizationAddress); err != nil {
		return nil, errors.New("invalid media normalization address")
	}
	var transport credentials.TransportCredentials
	anyTLS := cfg.MediaNormalizationTLSCAFile != "" || cfg.MediaNormalizationTLSCertFile != "" || cfg.MediaNormalizationTLSKeyFile != "" || cfg.MediaNormalizationServerName != ""
	if !anyTLS {
		if cfg.Env != "local" {
			return nil, errors.New("media normalization requires mTLS outside local mode")
		}
		transport = insecure.NewCredentials()
	} else {
		if cfg.MediaNormalizationTLSCAFile == "" || cfg.MediaNormalizationTLSCertFile == "" || cfg.MediaNormalizationTLSKeyFile == "" {
			return nil, errors.New("media normalization requires a complete mTLS identity")
		}
		pair, err := tls.LoadX509KeyPair(cfg.MediaNormalizationTLSCertFile, cfg.MediaNormalizationTLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load media normalization identity: %w", err)
		}
		data, err := os.ReadFile(cfg.MediaNormalizationTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("load media normalization authorities: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("invalid media normalization authorities")
		}
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}, ServerName: cfg.MediaNormalizationServerName})
	}
	return normalizationgrpc.New(cfg.MediaNormalizationAddress, cfg.MediaServiceToken, transport)
}
