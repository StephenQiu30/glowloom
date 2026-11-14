package creationruntime_test

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
	"errors"
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

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/creationruntime"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
)

type runtimeAddresses struct {
	rpc, health string
}

// Bound addresses are reported by the runtime instead of reserving ephemeral
// ports and racing the process under test to bind them.
type runtimeLogHandler struct {
	listening chan runtimeAddresses
	completed chan slog.Record
}

func (*runtimeLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *runtimeLogHandler) Handle(_ context.Context, record slog.Record) error {
	switch record.Message {
	case "creation RPC completed":
		select {
		case h.completed <- record.Clone():
		default:
		}
	case "creation extractor listening":
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

func startCreationRuntime(t *testing.T, cfg creationruntime.Config) *runningRuntime {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	handler := &runtimeLogHandler{listening: make(chan runtimeAddresses, 1), completed: make(chan slog.Record, 16)}
	running := &runningRuntime{cancel: cancel, done: make(chan error, 1), completed: handler.completed}
	go func() { running.done <- creationruntime.Run(ctx, cfg, slog.New(handler)) }()
	select {
	case running.addresses = <-handler.listening:
		if running.addresses.rpc == "" || running.addresses.health == "" {
			cancel()
			t.Fatal("runtime omitted bound listener addresses")
		}
	case err := <-running.done:
		cancel()
		t.Fatalf("start Creation runtime: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Creation runtime did not bind its listeners")
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
				t.Errorf("cancelled runtime failed to stop cleanly: %v", err)
			}
		case <-time.After(12 * time.Second):
			t.Error("cancelled Creation runtime did not stop within shutdown bound")
		}
	})
}

func assertHealth(t *testing.T, address string) {
	t.Helper()
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/health/live", http.StatusNoContent},
		{http.MethodGet, "/health/ready", http.StatusNoContent},
		{http.MethodPost, "/health/live", http.StatusMethodNotAllowed},
		{http.MethodPost, "/health/ready", http.StatusMethodNotAllowed},
		{http.MethodGet, "/", http.StatusNotFound},
	} {
		request, err := http.NewRequestWithContext(t.Context(), tc.method, "http://"+address+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != tc.status {
			t.Fatalf("%s %s status=%d, want %d", tc.method, tc.path, response.StatusCode, tc.status)
		}
	}
}

func emptyRuntimeRPC(t *testing.T, address, token string, transport credentials.TransportCredentials) error {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///"+address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry(), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	stream, err := pb.NewExtractionClient(conn).Extract(ctx)
	if err != nil {
		return err
	}
	_ = stream.CloseSend()
	_, err = stream.Recv()
	return err
}

func TestRunBindsRealTCPHealthRPCAndStopsOnCancellation(t *testing.T) {
	// Creation extraction has no external encoder or probe process dependency.
	t.Setenv("PATH", t.TempDir())
	cfg := localConfig()
	running := startCreationRuntime(t, cfg)
	assertHealth(t, running.addresses.health)
	for _, tc := range []struct {
		name, token string
		code        codes.Code
	}{
		{"valid private token", cfg.Token, codes.InvalidArgument},
		{"missing private token", "", codes.Unauthenticated},
		{"wrong private token", strings.Repeat("x", 40), codes.Unauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := emptyRuntimeRPC(t, running.addresses.rpc, tc.token, insecure.NewCredentials())
			if status.Code(err) != tc.code {
				t.Fatalf("real TCP RPC error=%v, want %v", err, tc.code)
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

func TestRunForcesBlockedStreamToStopAfterGracePeriod(t *testing.T) {
	work := t.TempDir()
	t.Setenv("TMPDIR", work)
	cfg := localConfig()
	running := startCreationRuntime(t, cfg)
	conn, err := grpc.NewClient("passthrough:///"+running.addresses.rpc, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+cfg.Token)
	stream, err := pb.NewExtractionClient(conn).Extract(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&pb.ExtractRequest{Payload: &pb.ExtractRequest_Source{Source: &pb.Source{
		ByteSize: 1, Sha256: strings.Repeat("0", 64), MimeType: "text/plain",
	}}}); err != nil {
		t.Fatal(err)
	}
	// The server's source file proves it admitted this stream and is waiting for
	// its remaining bytes. No scheduling sleep or test-only runtime hook is needed.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	admitted, admissionCancel := context.WithTimeout(ctx, 2*time.Second)
	defer admissionCancel()
waitForAdmission:
	for {
		files, err := filepath.Glob(filepath.Join(work, "glowloom-creation-source-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) > 0 {
			break waitForAdmission
		}
		select {
		case <-ticker.C:
		case <-admitted.Done():
			t.Fatal("stream was not admitted before shutdown test")
		}
	}
	start := time.Now()
	running.stop(t)
	if elapsed := time.Since(start); elapsed < 9*time.Second || elapsed > 12*time.Second {
		t.Fatalf("blocked stream shutdown took %v, want bounded 10s grace period", elapsed)
	}
	if _, err := stream.Recv(); err == nil {
		t.Fatal("force-stopped stream unexpectedly succeeded")
	}
	files, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatal("runtime returned before the cancelled stream removed its temporary original")
	}
}

func TestRunRejectsStartupFailuresWithoutServing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *creationruntime.Config)
		logger  bool
		want    string
	}{
		{"invalid configuration", func(_ *testing.T, c *creationruntime.Config) { c.Concurrent = 0 }, true, "creation concurrency"},
		{"missing logger", func(*testing.T, *creationruntime.Config) {}, false, "creation logger required"},
		{"temporary storage unavailable", func(t *testing.T, _ *creationruntime.Config) {
			t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
		}, true, "creation temporary storage unavailable"},
		{"invalid TLS identity", func(_ *testing.T, c *creationruntime.Config) {
			c.Env, c.TLSCertFile, c.TLSKeyFile, c.TLSCAFile = "prod", "missing.crt", "missing.key", "missing-ca.crt"
		}, true, "load creation TLS identity"},
		{"RPC listener already bound", func(t *testing.T, c *creationruntime.Config) {
			var lc net.ListenConfig
			listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			c.GRPCAddr = listener.Addr().String()
		}, true, "listen creation RPC"},
		{"health listener already bound", func(t *testing.T, c *creationruntime.Config) {
			var lc net.ListenConfig
			listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			c.HTTPAddr = listener.Addr().String()
		}, true, "listen creation health"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := localConfig()
			tc.prepare(t, &cfg)
			var logger *slog.Logger
			if tc.logger {
				logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			}
			if err := creationruntime.Run(t.Context(), cfg, logger); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("startup error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunWithCancelledContextDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := creationruntime.Run(ctx, localConfig(), logger); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled startup error=%v, want context.Canceled", err)
	}
}

type runtimeTLSIdentity struct{ certPEM, keyPEM []byte }
type runtimeCertificates struct {
	ca             []byte
	server, client runtimeTLSIdentity
}

func createRuntimeCertificates(t *testing.T) runtimeCertificates {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Creation runtime test CA"},
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
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: fmt.Sprintf("Creation test identity %d", serial)},
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
		server: issue(2, x509.ExtKeyUsageServerAuth), client: issue(3, x509.ExtKeyUsageClientAuth),
	}
}

func runtimeTLSConfig(t *testing.T, certificates runtimeCertificates) (creationruntime.Config, *tls.Config) {
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
	pair, err := tls.X509KeyPair(certificates.client.certPEM, certificates.client.keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificates.ca) {
		t.Fatal("invalid test CA")
	}
	return cfg, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "localhost", Certificates: []tls.Certificate{pair}}
}

func TestRunRequiresMutualTLS13AndPrivateTokenOverRealTCP(t *testing.T) {
	cfg, clientTLS := runtimeTLSConfig(t, createRuntimeCertificates(t))
	running := startCreationRuntime(t, cfg)
	assertHealth(t, running.addresses.health)
	noCertificate := clientTLS.Clone()
	noCertificate.Certificates = nil
	oldTLS := clientTLS.Clone()
	oldTLS.MinVersion, oldTLS.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	for _, tc := range []struct {
		name, token string
		transport   credentials.TransportCredentials
		code        codes.Code
	}{
		{"mTLS with private token", cfg.Token, credentials.NewTLS(clientTLS.Clone()), codes.InvalidArgument},
		{"mTLS without private token", "", credentials.NewTLS(clientTLS.Clone()), codes.Unauthenticated},
		{"no client certificate", cfg.Token, credentials.NewTLS(noCertificate), codes.Unavailable},
		{"plaintext", cfg.Token, insecure.NewCredentials(), codes.Unavailable},
		{"TLS 1.2", cfg.Token, credentials.NewTLS(oldTLS), codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := emptyRuntimeRPC(t, running.addresses.rpc, tc.token, tc.transport)
			if status.Code(err) != tc.code {
				t.Fatalf("transport RPC error=%v, want %v", err, tc.code)
			}
		})
	}
}

func TestRunCorrelatesOnlyValidSingleTraceparentWithoutLoggingSensitiveMetadata(t *testing.T) {
	cfg := localConfig()
	running := startCreationRuntime(t, cfg)
	conn, err := grpc.NewClient("passthrough:///"+running.addresses.rpc, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const traceparent = "00-" + traceID + "-00f067aa0ba902b7-01"
	for _, tc := range []struct {
		name        string
		parents     []string
		wantTraceID string
	}{
		{"valid context", []string{traceparent}, traceID},
		{"invalid zero context", []string{"00-00000000000000000000000000000000-00f067aa0ba902b7-01"}, ""},
		{"untrusted raw context", []string{"private-document-trace-value"}, ""},
		{"duplicate context", []string{traceparent, traceparent}, ""},
		{"no context", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			md := metadata.Pairs("authorization", "Bearer "+cfg.Token, "tracestate", "sensitive=private-document", "x-original-text", "private-document-content")
			md["traceparent"] = tc.parents
			ctx = metadata.NewOutgoingContext(ctx, md)
			stream, err := pb.NewExtractionClient(conn).Extract(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_ = stream.CloseSend()
			if _, err := stream.Recv(); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("traced empty RPC error=%v", err)
			}
			var record slog.Record
			select {
			case record = <-running.completed:
			case <-ctx.Done():
				t.Fatal("missing Creation completion log")
			}
			fields := make(map[string]string)
			record.Attrs(func(attr slog.Attr) bool { fields[attr.Key] = attr.Value.String(); return true })
			if fields["service"] != "creation" || fields["status"] != codes.InvalidArgument.String() || fields["trace_id"] != tc.wantTraceID || fields["method"] != "/glowloom.creation.Extraction/Extract" {
				t.Fatalf("unexpected completion fields=%v", fields)
			}
			if _, ok := fields["duration_ms"]; !ok {
				t.Fatal("missing RPC duration")
			}
			if _, ok := fields["trace_id"]; tc.wantTraceID == "" && ok {
				t.Fatal("invalid or duplicate traceparent emitted trace ID")
			}
			var serialized bytes.Buffer
			if err := slog.NewJSONHandler(&serialized, nil).Handle(ctx, record); err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{cfg.Token, "Bearer ", "private-document", traceparent} {
				if strings.Contains(serialized.String(), secret) {
					t.Fatal("completion log leaked credential or raw metadata")
				}
			}
		})
	}
}

func TestRunRejectsInvalidTLSAuthoritiesBeforeBinding(t *testing.T) {
	for _, tc := range []struct{ name, ca string }{
		{"missing authority", ""}, {"invalid authority", "not a certificate"},
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
			if err := creationruntime.Run(t.Context(), cfg, logger); err == nil || !strings.Contains(err.Error(), "TLS authorities") {
				t.Fatalf("invalid TLS authorities accepted or misclassified: %v", err)
			}
		})
	}
}
