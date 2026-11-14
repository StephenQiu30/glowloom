package mediaruntime_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/mediaruntime"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
)

type runtimeAddresses struct {
	rpc    string
	health string
}

// Runtime reports the actual ephemeral listener addresses through structured
// logs, so tests never reserve a port and race with another process to bind it.
type runtimeLogHandler struct {
	listening chan runtimeAddresses
	completed chan slog.Record
}

func (*runtimeLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *runtimeLogHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "media RPC completed" {
		select {
		case h.completed <- record.Clone():
		default:
		}
	}
	if record.Message != "media processor listening" {
		return nil
	}
	var addresses runtimeAddresses
	record.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "grpc_addr":
			addresses.rpc = attr.Value.String()
		case "health_addr":
			addresses.health = attr.Value.String()
		}
		return true
	})
	select {
	case h.listening <- addresses:
	default:
	}
	return nil
}

func (h *runtimeLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *runtimeLogHandler) WithGroup(string) slog.Handler      { return h }

type runningRuntime struct {
	addresses runtimeAddresses
	cancel    context.CancelFunc
	done      chan error
	completed <-chan slog.Record
	stopOnce  sync.Once
}

func startMediaRuntime(t *testing.T, cfg mediaruntime.Config) *runningRuntime {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	handler := &runtimeLogHandler{listening: make(chan runtimeAddresses, 1), completed: make(chan slog.Record, 16)}
	running := &runningRuntime{cancel: cancel, done: make(chan error, 1), completed: handler.completed}
	go func() { running.done <- mediaruntime.Run(ctx, cfg, slog.New(handler)) }()
	select {
	case running.addresses = <-handler.listening:
		if running.addresses.rpc == "" || running.addresses.health == "" {
			cancel()
			t.Fatalf("runtime omitted bound addresses: %+v", running.addresses)
		}
	case err := <-running.done:
		cancel()
		t.Fatalf("start media runtime: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("media runtime did not bind its listeners")
	}
	t.Cleanup(func() { running.stop(t) })
	return running
}

func (r *runningRuntime) stop(t *testing.T) {
	t.Helper()
	r.stopOnce.Do(func() {
		r.cancel()
		select {
		case err := <-r.done:
			if err != nil {
				t.Errorf("cancelled media runtime failed to stop cleanly: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("cancelled media runtime did not stop")
		}
	})
}

func assertHealth(t *testing.T, address string) {
	t.Helper()
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	for _, path := range []string{"/health/live", "/health/ready"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("GET %s status=%d, want 204", path, response.StatusCode)
		}
	}
}

// An authenticated empty stream must reach the application and be rejected for
// its missing source header. It never needs a database or invokes the encoder.
func emptyRuntimeRPC(t *testing.T, address, token string, transport credentials.TransportCredentials) error {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///"+address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry(), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close runtime RPC connection: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	stream, err := pb.NewNormalizationClient(conn).Normalize(ctx)
	if err != nil {
		return err
	}
	// Recv reports the authoritative server status even if CloseSend races
	// with transport authentication rejecting the stream.
	_ = stream.CloseSend()
	_, err = stream.Recv()
	return err
}

func TestRunBindsRealTCPHealthRPCAndStopsOnCancellation(t *testing.T) {
	cfg := localConfig()
	running := startMediaRuntime(t, cfg)
	assertHealth(t, running.addresses.health)
	for _, tc := range []struct {
		name  string
		token string
		code  codes.Code
	}{
		{"valid private token", cfg.Token, codes.InvalidArgument},
		{"missing private token", "", codes.Unauthenticated},
		{"wrong private token", strings.Repeat("x", 40), codes.Unauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := emptyRuntimeRPC(t, running.addresses.rpc, tc.token, insecure.NewCredentials())
			if status.Code(err) != tc.code {
				t.Fatalf("empty real TCP RPC error=%v code=%v, want %v", err, status.Code(err), tc.code)
			}
		})
	}
	running.stop(t)
	for _, address := range []string{running.addresses.rpc, running.addresses.health} {
		dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
		conn, err := dialer.DialContext(t.Context(), "tcp", address)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("cancelled runtime retained listening socket %s", address)
		}
	}
}

func TestRunRejectsStartupFailuresWithoutServing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *mediaruntime.Config)
		logger  bool
		want    string
	}{
		{"invalid configuration", func(_ *testing.T, c *mediaruntime.Config) { c.Concurrent = 0 }, true, "media concurrency"},
		{"missing logger", func(*testing.T, *mediaruntime.Config) {}, false, "media logger required"},
		{"missing encoder", func(t *testing.T, _ *mediaruntime.Config) { t.Setenv("PATH", t.TempDir()) }, true, "media requires ffmpeg"},
		{"missing prober", func(t *testing.T, _ *mediaruntime.Config) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
		}, true, "media requires ffprobe"},
		{"temporary storage unavailable", func(t *testing.T, _ *mediaruntime.Config) {
			t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))
		}, true, "media temporary storage unavailable"},
		{"invalid TLS identity", func(_ *testing.T, c *mediaruntime.Config) {
			c.Env, c.TLSCertFile, c.TLSKeyFile, c.TLSCAFile = "prod", "does-not-exist.crt", "does-not-exist.key", "does-not-exist-ca.crt"
		}, true, "load media TLS identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			tc.prepare(t, &cfg)
			var logger *slog.Logger
			if tc.logger {
				logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			}
			if err := mediaruntime.Run(t.Context(), cfg, logger); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("startup error=%v, want %q", err, tc.want)
			}
		})
	}
}

type runtimeTLSIdentity struct {
	certPEM []byte
	keyPEM  []byte
}

type runtimeCertificates struct {
	ca     []byte
	server runtimeTLSIdentity
	client runtimeTLSIdentity
}

func createRuntimeCertificates(t *testing.T) runtimeCertificates {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Media runtime test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(serial int64, usage x509.ExtKeyUsage) runtimeTLSIdentity {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		certificate := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: fmt.Sprintf("Media test identity %d", serial)},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
			DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, err := x509.CreateCertificate(rand.Reader, certificate, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return runtimeTLSIdentity{
			certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}),
		}
	}
	return runtimeCertificates{
		ca:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		server: issue(2, x509.ExtKeyUsageServerAuth),
		client: issue(3, x509.ExtKeyUsageClientAuth),
	}
}

func runtimeTLSConfig(t *testing.T, certificates runtimeCertificates) (mediaruntime.Config, *tls.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := localConfig()
	cfg.Env = "prod"
	cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSCAFile = filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"), filepath.Join(dir, "ca.crt")
	for name, data := range map[string][]byte{cfg.TLSCertFile: certificates.server.certPEM, cfg.TLSKeyFile: certificates.server.keyPEM, cfg.TLSCAFile: certificates.ca} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	clientPair, err := tls.X509KeyPair(certificates.client.certPEM, certificates.client.keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificates.ca) {
		t.Fatal("invalid test certificate authority")
	}
	return cfg, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "localhost", Certificates: []tls.Certificate{clientPair}}
}

func TestRunUsesMutualTLSWithPrivateTokenOverRealTCP(t *testing.T) {
	cfg, clientTLS := runtimeTLSConfig(t, createRuntimeCertificates(t))
	running := startMediaRuntime(t, cfg)
	assertHealth(t, running.addresses.health)

	err := emptyRuntimeRPC(t, running.addresses.rpc, cfg.Token, credentials.NewTLS(clientTLS.Clone()))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("authenticated mTLS stream did not reach application: %v", err)
	}
	err = emptyRuntimeRPC(t, running.addresses.rpc, "", credentials.NewTLS(clientTLS.Clone()))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("mTLS bypassed private service token: %v", err)
	}
	noCertificate := clientTLS.Clone()
	noCertificate.Certificates = nil
	err = emptyRuntimeRPC(t, running.addresses.rpc, cfg.Token, credentials.NewTLS(noCertificate))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("missing mTLS client certificate was not refused at transport: %v", err)
	}
	err = emptyRuntimeRPC(t, running.addresses.rpc, cfg.Token, insecure.NewCredentials())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("plaintext RPC was not refused by TLS listener: %v", err)
	}
	running.stop(t)
}

func TestRunCorrelatesCompletionLogsWithoutLeakingPrivateToken(t *testing.T) {
	cfg := localConfig()
	running := startMediaRuntime(t, cfg)
	conn, err := grpc.NewClient("passthrough:///"+running.addresses.rpc, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close traced runtime RPC connection: %v", err)
		}
	}()
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	for _, tc := range []struct {
		name        string
		traceparent string
		wantTraceID string
	}{
		{"valid trace context", "00-" + traceID + "-00f067aa0ba902b7-01", traceID},
		{"invalid trace context", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+cfg.Token, "traceparent", tc.traceparent, "tracestate", "test=media")
			stream, err := pb.NewNormalizationClient(conn).Normalize(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_ = stream.CloseSend()
			if _, err := stream.Recv(); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("traced empty RPC error=%v, want InvalidArgument", err)
			}
			var record slog.Record
			select {
			case record = <-running.completed:
			case <-ctx.Done():
				t.Fatal("missing media RPC completion log")
			}
			fields := make(map[string]string)
			record.Attrs(func(attr slog.Attr) bool {
				fields[attr.Key] = attr.Value.String()
				return true
			})
			if record.Message != "media RPC completed" || fields["service"] != "media" || fields["code"] != codes.InvalidArgument.String() || fields["trace_id"] != tc.wantTraceID {
				t.Fatalf("completion log message=%q fields=%v, want trace_id=%q", record.Message, fields, tc.wantTraceID)
			}
			if _, exists := fields["trace_id"]; tc.wantTraceID == "" && exists {
				t.Fatal("invalid traceparent produced a trace_id field")
			}
			var serialized bytes.Buffer
			if err := slog.NewJSONHandler(&serialized, nil).Handle(ctx, record); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(serialized.String(), cfg.Token) || strings.Contains(serialized.String(), "Bearer ") {
				t.Fatal("completion log exposed a private service credential")
			}
		})
	}
}

func TestRunRejectsInvalidTLSAuthoritiesBeforeBinding(t *testing.T) {
	for _, tc := range []struct{ name, ca string }{
		{"missing authority", ""},
		{"invalid authority", "not a certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := runtimeTLSConfig(t, createRuntimeCertificates(t))
			if tc.ca == "" {
				if err := os.Remove(cfg.TLSCAFile); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(cfg.TLSCAFile, []byte(tc.ca), 0600); err != nil {
				t.Fatal(err)
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			if err := mediaruntime.Run(t.Context(), cfg, logger); err == nil || !strings.Contains(err.Error(), "TLS authorities") {
				t.Fatalf("invalid TLS authorities accepted or misclassified: %v", err)
			}
		})
	}
}
