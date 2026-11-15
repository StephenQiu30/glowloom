package microservices_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/app"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	budgetsource "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/creationpreparation"
	budgetrpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/grpcprojectbudget"
	budgetpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationgovernance"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type creationRuntimePKI struct {
	roots *x509.CertPool
	caPEM string
	certs map[string]tls.Certificate
}

func newCreationRuntimePKI(t *testing.T) *creationRuntimePKI {
	t.Helper()
	now := time.Now()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Creation runtime synthetic CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	p := &creationRuntimePKI{roots: x509.NewCertPool(), caPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), certs: map[string]tls.Certificate{}}
	p.roots.AddCert(ca)
	for i, name := range []string{"creation.runtime", "identity.runtime", "execution.runtime", "media.runtime"} {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		encoded, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		p.certs[name] = tls.Certificate{Certificate: [][]byte{encoded, der}, PrivateKey: leafKey}
	}
	return p
}
func (p *creationRuntimePKI) server(name string) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.certs[name]}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.roots})
}
func (p *creationRuntimePKI) client(caller, target string) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.certs[caller]}, RootCAs: p.roots, ServerName: target})
}
func (p *creationRuntimePKI) config(t *testing.T, name string) workspaceapp.ServiceTLS {
	t.Helper()
	cert := p.certs[name]
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	return workspaceapp.ServiceTLS{CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})), PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})), CAPEM: p.caPEM, OwnName: name}
}
func reserveCreationRuntimeAddress(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

// All upstreams are actual role-bound applications and mTLS endpoints. Account /
// scope bootstrap remains explicit fixture; this tests this real process scope.
func TestCreationWorkspaceRuntimeActualAPIRecoveryKafkaAndJoinedShutdown(t *testing.T) {
	brokers := os.Getenv("GLOWLOOM_TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("requires disposable GLOWLOOM_TEST_KAFKA_BROKERS")
	}
	for _, address := range strings.Split(brokers, ",") {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
			t.Fatal("runtime accepts only explicit loopback test Kafka")
		}
	}
	pki := newCreationRuntimePKI(t)
	f, _, credential, _, identityAddress, identityToken := nativeProjectIdentityTLS(t, pki.server("identity.runtime"), pki.client("creation.runtime", "identity.runtime"))
	parsed, err := url.Parse(os.Getenv("GLOWLOOM_TEST_WRITE_FENCE_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.User("owner_b_creation_test")
	cfg := workspaceapp.Config{HTTPAddress: "127.0.0.1:0", WriterAddress: "127.0.0.1:0", RegistrationAddress: "127.0.0.1:0", PreparationAddress: reserveCreationRuntimeAddress(t), GovernanceAddress: "127.0.0.1:0", GovernanceToken: strings.Repeat("identity-governance-", 3), PublicOrigin: "http://127.0.0.1", DatabaseDSN: parsed.String(), KafkaBrokers: brokers, TLS: pki.config(t, "creation.runtime"), IdentityAuthority: workspaceapp.Peer{Address: identityAddress, Token: identityToken, ServerName: "identity.runtime"}, IdentityWriterToken: strings.Repeat("identity-to-creation-", 3), RegistrationToken: strings.Repeat("registration-to-creation-", 3), PreparationToken: strings.Repeat("execution-proof-", 3), Writers: map[writefence.Writer]workspaceapp.Peer{}}
	for _, writer := range writefence.Writers() {
		name := string(writer) + ".runtime"
		token := strings.Repeat("creation-control-", 3) + string(writer)
		cfg.Writers[writer] = workspaceapp.Peer{Token: token, ServerName: name}
		if writer == writefence.WriterCreation {
			continue
		}
		handler, err := fencerpc.NewServer(writer, pgfence.New(authorityDatabase(t, string(writer))), strings.Repeat("identity-control-", 3)+string(writer), token)
		if err != nil {
			t.Fatal(err)
		}
		server := grpc.NewServer(grpc.Creds(pki.server(name)))
		coordination.RegisterWriterFenceServer(server, handler)
		p := cfg.Writers[writer]
		p.Address = serveProjectPreparation(t, server)
		cfg.Writers[writer] = p
	}
	proof, err := budgetsource.New(budgetsource.Target{Address: cfg.PreparationAddress, Token: cfg.PreparationToken, Transport: pki.client("execution.runtime", "creation.runtime")})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := billing.NewProjectBudgets(budgetpg.NewProjectBudgetStore(f.execution), proof)
	if err != nil {
		t.Fatal(err)
	}
	budget := &lostProjectBudgetACK{actual: actual, lose: true}
	cfg.ExecutionBudget = workspaceapp.Peer{Token: strings.Repeat("creation-budget-", 3), ServerName: "execution.runtime"}
	budgetServer, err := budgetrpc.NewServer(budget, cfg.ExecutionBudget.Token, pki.server("execution.runtime"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ExecutionBudget.Address = serveProjectPreparation(t, budgetServer)
	runtime, err := workspaceapp.New(t.Context(), cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- runtime.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-stopped:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(15 * time.Second):
				runtime.Close()
				t.Error("Creation runtime did not join owned loops")
			}
		})
	}
	t.Cleanup(stop)
	address, writerAddress, registrationAddress, preparationAddress := runtime.Addresses()
	govConnection, err := grpc.NewClient(runtime.GovernanceAddress(), grpc.WithTransportCredentials(pki.client("identity.runtime", "creation.runtime")), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = govConnection.Close() }()
	govID := uuid.New()
	govReader := creationgovernance.NewAccountGovernanceClient(govConnection)
	govDeadline, govCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer govCancel()
	govContext := metadata.NewOutgoingContext(govDeadline, metadata.Pairs("authorization", "Bearer "+cfg.GovernanceToken))
	if _, err := govReader.Resolve(govContext, &creationgovernance.ResolveAccountGovernanceRequest{RequestId: govID[:]}); status.Code(err) != codes.NotFound {
		t.Fatalf("independent actual governance listener: %v", err)
	}
	registrationContext := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+cfg.RegistrationToken))
	if _, err := govReader.Resolve(registrationContext, &creationgovernance.ResolveAccountGovernanceRequest{RequestId: govID[:]}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("registration audience entered governance: %v", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	requestID, key := uuid.NewString(), uuid.New()
	_, legacy, csrf := credential.IdentityProof()
	post := func(origin, proof string) (int, []byte) {
		request, err := http.NewRequestWithContext(t.Context(), "POST", "http://"+address+"/api/projects", bytes.NewBufferString(`{"name":"real native API","description":"original","aspect_ratio":"16:9"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		request.Header.Set("Idempotency-Key", key.String())
		request.Header.Set("X-Request-Id", requestID)
		request.Header.Set("X-Creation-Scope", f.request.ScopeID.String())
		request.Header.Set("X-CSRF-Token", proof)
		request.AddCookie(&http.Cookie{Name: "glowloom_session", Value: legacy})
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}
	if code, body := post("http://other.invalid", csrf); code != 403 || !bytes.Contains(body, []byte("origin_forbidden")) {
		t.Fatalf("origin policy: %d %s", code, body)
	}
	if code, body := post(cfg.PublicOrigin, "wrong"); code != 403 || !bytes.Contains(body, []byte("csrf_invalid")) {
		t.Fatalf("CSRF policy: %d %s", code, body)
	}
	if code, body := post(cfg.PublicOrigin, csrf); code != 503 && code != 201 {
		t.Fatalf("concurrent original recovery returned neither unknown nor committed: %d %s", code, body)
	}
	id := uuid.NewSHA1(f.request.ActorID, []byte("creation.project.preparation/"+key.String()))
	waitNativeProjectTask(t, f, id)
	code, body := post(cfg.PublicOrigin, csrf)
	if code != 201 {
		t.Fatalf("original API receipt: %d %s", code, body)
	}
	var created source.CreatedProject
	if json.Unmarshal(body, &created) != nil || created.Status != "active" || created.ID != uuid.NewSHA1(id, []byte("creation.project")) {
		t.Fatalf("actual original project result: %s", body)
	}
	consumer, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...), kgo.ConsumeTopics(source.ProjectChangedTopic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	bounded, release := context.WithTimeout(t.Context(), 15*time.Second)
	defer release()
	received := false
	for !received && bounded.Err() == nil {
		batch := consumer.PollFetches(bounded)
		batch.EachRecord(func(record *kgo.Record) {
			var event struct {
				ProjectID uuid.UUID `json:"project_id"`
				Data      struct {
					Change string `json:"change"`
				} `json:"data"`
			}
			if json.Unmarshal(record.Value, &event) == nil && string(record.Key) == created.ID.String() && event.ProjectID == created.ID && event.Data.Change == "created" {
				received = true
			}
		})
	}
	if !received {
		t.Fatal("real Kafka did not receive the original committed project event")
	}
	var published bool
	ackPoll := time.NewTicker(5 * time.Millisecond)
	defer ackPoll.Stop()
	for !published {
		if err := f.creation.Raw(`SELECT published_at IS NOT NULL FROM infra.outbox WHERE id=?`, uuid.NewSHA1(id, []byte("project.changed"))).Scan(&published).Error; err != nil {
			t.Fatal(err)
		}
		if !published {
			select {
			case <-ackPoll.C:
			case <-bounded.Done():
				t.Fatal("broker ACK not persisted by actual relay")
			}
		}
	}
	stop()
	for _, endpoint := range []string{address, writerAddress, registrationAddress, preparationAddress, runtime.GovernanceAddress()} {
		connection, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext(t.Context(), "tcp", endpoint)
		if err == nil {
			_ = connection.Close()
			t.Errorf("listener remained after joined shutdown: %s", endpoint)
		}
	}
	budget.mu.Lock()
	prepares, lost := budget.prepares, !budget.lose
	budget.mu.Unlock()
	if prepares < 1 || !lost {
		t.Fatal("actual budget ACK loss was not exercised")
	}
	var budgets int
	if err := f.execution.Raw(`SELECT count(*) FROM billing.budget WHERE project_id=?`, created.ID).Scan(&budgets).Error; err != nil || budgets != 1 {
		t.Fatalf("concurrent original recovery created %d budgets: %v", budgets, err)
	}
}
