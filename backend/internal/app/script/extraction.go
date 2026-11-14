package script

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	document "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extractiongrpc"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

type remoteExtractor struct{ client *extractiongrpc.Client }

func (e *remoteExtractor) Extract(ctx context.Context, source *media.Downloaded) (app.ExtractedDocument, error) {
	var file *document.File
	if source != nil {
		file = &document.File{File: source.File, Size: source.Size, SHA256: source.SHA256, MIMEType: source.MIMEType}
	}
	result, err := e.client.Extract(ctx, file)
	if err != nil {
		if errors.Is(err, extractiongrpc.ErrUnavailable) {
			return app.ExtractedDocument{}, app.ErrUnavailable
		}
		if errors.Is(err, document.ErrExtractionFailed) {
			return app.ExtractedDocument{}, app.ErrExtractionFailed
		}
		return app.ExtractedDocument{}, err
	}
	return app.ExtractedDocument{Document: result.Document, Encoding: result.Encoding, Mapping: result.Mapping, Warnings: result.Warnings}, nil
}

// ProvideSourceExtractor wires one remote parser, never an embedded fallback.
// Disabled configuration preserves startup while extraction fails closed.
func ProvideSourceExtractor(cfg config.Config) (app.SourceExtractor, error) {
	anyTLS := cfg.CreationExtractionTLSCAFile != "" || cfg.CreationExtractionTLSCertFile != "" || cfg.CreationExtractionTLSKeyFile != "" || cfg.CreationExtractionServerName != ""
	if cfg.CreationExtractionAddress == "" && cfg.CreationServiceToken == "" {
		if anyTLS {
			return nil, errors.New("creation TLS identity requires a target")
		}
		client, err := extractiongrpc.New("", "", nil)
		return &remoteExtractor{client: client}, err
	}
	var transport credentials.TransportCredentials
	if !anyTLS {
		if cfg.Env != "local" {
			return nil, errors.New("creation extraction requires mTLS outside local mode")
		}
		transport = insecure.NewCredentials()
	} else {
		if cfg.CreationExtractionTLSCAFile == "" || cfg.CreationExtractionTLSCertFile == "" || cfg.CreationExtractionTLSKeyFile == "" {
			return nil, errors.New("creation extraction requires a complete mTLS identity")
		}
		pair, err := tls.LoadX509KeyPair(cfg.CreationExtractionTLSCertFile, cfg.CreationExtractionTLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load Creation client identity: %w", err)
		}
		data, err := os.ReadFile(cfg.CreationExtractionTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("load Creation authorities: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("invalid Creation authorities")
		}
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}, ServerName: cfg.CreationExtractionServerName})
	}
	client, err := extractiongrpc.New(cfg.CreationExtractionAddress, cfg.CreationServiceToken, transport)
	if err != nil {
		return nil, err
	}
	return &remoteExtractor{client: client}, nil
}
