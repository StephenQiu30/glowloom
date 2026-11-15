package identityregistration_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	creationrpc "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcregistration"

	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/grpcregistration"

	execution "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"

	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcregistration"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

type tlsFixture struct {
	roots     *x509.CertPool
	authority *x509.Certificate
	key       *ecdsa.PrivateKey
}

func newTLS(t *testing.T) *tlsFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "registration test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	raw, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return &tlsFixture{roots: pool, authority: parsed, key: key}
}
func (f *tlsFixture) certificate(t *testing.T, name string, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: serial, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	raw, err := x509.CreateCertificate(rand.Reader, cert, f.authority, &key.PublicKey, f.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}
}
func (f *tlsFixture) server(t *testing.T, name string) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{f.certificate(t, name, x509.ExtKeyUsageServerAuth)}, ClientCAs: f.roots, ClientAuth: tls.RequireAndVerifyClientCert})
}
func (f *tlsFixture) client(t *testing.T, server, caller string) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: f.roots, ServerName: server, Certificates: []tls.Certificate{f.certificate(t, caller, x509.ExtKeyUsageClientAuth)}})
}
func serve(t *testing.T, s *grpc.Server) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(listener) }()
	t.Cleanup(func() {
		s.Stop()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	return listener.Addr().String()
}

type scopeParticipant struct{}

func (scopeParticipant) PreparePersonalRegistration(_ context.Context, p workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error) {
	return p, nil
}
func (scopeParticipant) ResolvePersonalRegistration(_ context.Context, p workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error) {
	return p, nil
}

type walletParticipant struct{}

func (walletParticipant) PrepareRegistrationWallet(_ context.Context, p execution.RegistrationWallet) (execution.RegistrationWallet, error) {
	return p, nil
}
func (walletParticipant) ResolveRegistrationWallet(_ context.Context, p execution.RegistrationWallet) (execution.RegistrationWallet, error) {
	return p, nil
}

func TestRegistrationOwnersRequireDedicatedMTLSAndExactReceipt(t *testing.T) {
	f := newTLS(t)
	token := strings.Repeat("registration-only-", 3)
	scope, err := workspacesource.NewPersonalRegistrations(scopeParticipant{})
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := execution.NewRegistrationWallets(walletParticipant{})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := creationrpc.NewServer(scope, token, "identity.registration", f.server(t, "creation.registration"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := executionrpc.NewServer(wallet, token+"wallet", "identity.registration", f.server(t, "execution.registration"))
	if err != nil {
		t.Fatal(err)
	}
	ca, wa := serve(t, cs), serve(t, ws)
	p := identity.RegistrationPreparation{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: strings.Repeat("a", 64), CreationReady: true}
	for _, tc := range []struct {
		owner, address, server, token string
		newClient                     func(identityrpc.Target) (*identityrpc.Client, error)
	}{{"creation", ca, "creation.registration", token, identityrpc.NewPersonalScopeClient}, {"execution", wa, "execution.registration", token + "wallet", identityrpc.NewWalletClient}} {
		t.Run(tc.owner, func(t *testing.T) {
			good, err := tc.newClient(identityrpc.Target{Address: tc.address, Token: tc.token, Transport: f.client(t, tc.server, "identity.registration")})
			if err != nil {
				t.Fatal(err)
			}
			r, err := good.PrepareRegistration(t.Context(), p)
			if err != nil || r != p.ExpectedReady(tc.owner) {
				t.Fatalf("prepare=%#v %v", r, err)
			}
			r, err = good.ResolveRegistration(t.Context(), p)
			if err != nil || r != p.ExpectedReady(tc.owner) {
				t.Fatalf("resolve=%#v %v", r, err)
			}
			for _, bad := range []struct{ token, caller string }{{tc.token, "creation.untrusted"}, {strings.Repeat("wrong-", 8), "identity.registration"}} {
				client, err := tc.newClient(identityrpc.Target{Address: tc.address, Token: bad.token, Transport: f.client(t, tc.server, bad.caller)})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := client.PrepareRegistration(t.Context(), p); err == nil {
					t.Fatal("untrusted service obtained registration receipt")
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := good.PrepareRegistration(ctx, p); err == nil {
				t.Fatal("canceled preparation succeeded")
			}
		})
	}
}
