package microservices_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"

	creationrpc "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func frozenTLS(t *testing.T) (credentials.TransportCredentials, credentials.TransportCredentials, credentials.TransportCredentials) {
	t.Helper()
	return ownerPeerTLS(t, "execution")
}

func ownerPeerTLS(t *testing.T, expectedPeer string) (credentials.TransportCredentials, credentials.TransportCredentials, credentials.TransportCredentials) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic handoff CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	leaf := func(name string, serial int64, server bool) tls.Certificate {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		if server {
			template.DNSNames = []string{"handoff.test"}
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}
	}
	server := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf("creation", 2, true)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, VerifyConnection: func(state tls.ConnectionState) error {
		if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || state.PeerCertificates[0].Subject.CommonName != expectedPeer {
			return errors.New("wrong synthetic handoff peer")
		}
		return nil
	}})
	client := func(name string, serial int64) credentials.TransportCredentials {
		return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "handoff.test", Certificates: []tls.Certificate{leaf(name, serial, false)}})
	}
	otherPeer := "media"
	if expectedPeer == otherPeer {
		otherPeer = "execution"
	}
	return server, client(expectedPeer, 3), client(otherPeer, 4)
}

func TestFrozenInputRPCInjectedMTLSBindsExecutionPeer(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	serverTransport, clientTransport, wrongPeer := frozenTLS(t)
	checked := &checkedFrozenSource{store: f.store}
	server, err := creationrpc.NewServer(checked, frozenExecutionToken, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	bad, err := executionrpc.NewSourceClient(executionrpc.Target{Address: address, Token: frozenExecutionToken, Transport: wrongPeer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.ReadFrozenInput(t.Context(), executionKey(envelope)); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("wrong mTLS peer err=%v", err)
	}
	if checked.calls.Load() != 0 {
		t.Fatal("wrong authenticated peer reached owner store")
	}
	good, err := executionrpc.NewSourceClient(executionrpc.Target{Address: address, Token: frozenExecutionToken, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	input, err := good.ReadFrozenInput(t.Context(), executionKey(envelope))
	if err != nil || input.Key != executionKey(envelope) {
		t.Fatalf("valid mTLS handoff err=%v", err)
	}
	if checked.calls.Load() != 1 {
		t.Fatal("valid input was retried")
	}
}
