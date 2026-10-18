package app_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"

	"github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

type wiringTokens struct {
	identity.AccessTokenCodec
	verified        bool
	signingDisabled bool
}

func (f *wiringTokens) CanSign() bool { return !f.signingDisabled }

func (f *wiringTokens) Verify(string, time.Time) (identity.AccessClaims, error) {
	f.verified = true
	return identity.AccessClaims{SessionID: uuid.New()}, nil
}

type wiringCoordinator struct {
	identity.RefreshCoordinator
	checked bool
}

func (f *wiringCoordinator) IsRevoked(context.Context, uuid.UUID) (bool, error) {
	f.checked = true
	return true, nil
}

type wiringStore struct{ identity.EmailStore }

func TestSessionV2Wiring(t *testing.T) {
	auth := identity.NewEmailAuth(wiringStore{}, proofFixture{}, identity.EmailPolicy{}, time.Now)
	if err := identityruntime.ConfigureSessionSecurity(auth, config.Config{IdentitySessionV2Enabled: true, IdentityLegacyUpgradeUntil: time.Now().UTC().Add(time.Hour)}, identityruntime.SessionDependencies{}); err == nil {
		t.Fatal("enabled runtime accepted missing dependencies")
	}
	if err := identityruntime.ConfigureSessionSecurity(auth, config.Config{}, identityruntime.SessionDependencies{}); err != nil {
		t.Fatal(err)
	}
	codec := &wiringTokens{}
	coordinator := &wiringCoordinator{}
	deps := identityruntime.SessionDependencies{Tokens: codec, Receipts: receiptFixture{}, Coordinator: coordinator}
	if err := identityruntime.ConfigureSessionSecurity(auth, config.Config{IdentitySessionV2Enabled: true, IdentityLegacyUpgradeUntil: time.Now().UTC().Add(time.Hour)}, deps); err != nil {
		t.Fatal(err)
	}
	_, err := auth.CurrentAccess(t.Context(), "access", "csrf", identity.NoActivity)
	if !errors.Is(err, identity.ErrSessionInvalid) || !codec.verified || !coordinator.checked {
		t.Fatalf("security ports not consumed: %v %v %v", err, codec.verified, coordinator.checked)
	}
	if _, err := identityruntime.NewSessionDependencies(config.Config{IdentitySessionV2Enabled: true, IdentityLegacyUpgradeUntil: time.Now().UTC().Add(time.Hour)}, nil); err == nil {
		t.Fatal("enabled composition accepted missing keys/client")
	}
	if _, err := identityruntime.NewSessionDependencies(config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
}

type receiptFixture struct {
	identity.RefreshReceiptProtector
}

type cleanupStore struct {
	identity.EmailStore
	tx    cleanupTx
	calls int
}

func (f *cleanupStore) EmailTransaction(_ context.Context, fn func(identity.EmailTx) error) error {
	f.calls++
	return fn(&f.tx)
}

type cleanupTx struct {
	identity.EmailTx
	now     time.Time
	limit   int
	entered chan struct{}
}

func (f *cleanupTx) CleanupExpiredRefresh(now time.Time, limit int) (int, error) {
	f.now = now
	f.limit = limit
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	return 3, nil
}
func TestSessionReceiptCleanup(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	store := &cleanupStore{}
	cleanup := identity.NewSessionReceiptCleanup(store, func() time.Time { return now })
	n, err := cleanup.RunOnce(t.Context(), 7)
	if err != nil || n != 3 || store.calls != 1 || store.tx.limit != 7 || !store.tx.now.Equal(now) {
		t.Fatalf("cleanup boundary: n=%d err=%v store=%+v", n, err, store)
	}
	if _, err := cleanup.RunOnce(t.Context(), 0); err == nil {
		t.Fatal("unbounded cleanup accepted")
	}
	if _, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), cleanup, zap.NewNop(), 61*time.Second); err == nil {
		t.Fatal("overdue cleanup interval accepted")
	}
	store.tx.entered = make(chan struct{}, 1)
	stop, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), cleanup, zap.NewNop(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.tx.entered:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not run")
	}
	stop()
	calls := store.calls
	time.Sleep(5 * time.Millisecond)
	if calls != store.calls {
		t.Fatal("cleanup continued after stop")
	}
	stop()
}

type proofFixture struct{ identity.ProofProtector }

func TestSessionV2PurposeFiles(t *testing.T) {
	dir := t.TempDir()
	proof := filepath.Join(dir, "proof")
	receipt := filepath.Join(dir, "receipt")
	if err := os.WriteFile(proof, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	alias := dir + "/unused/../proof"
	cfg := config.Config{IdentityKeyFile: proof, IdentityRefreshReceiptKeyFile: alias}
	if err := cfg.ValidateIdentitySessionV2(); err == nil {
		t.Fatal("cleaned purpose alias accepted")
	}
	if err := os.Link(proof, receipt); err != nil {
		t.Fatal(err)
	}
	cfg.IdentityRefreshReceiptKeyFile = receipt
	if _, err := identityruntime.NewSessionDependencies(cfg, nil); err == nil {
		t.Fatal("same-file purpose alias accepted")
	}
}

func TestSessionV2RealDependencies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	private := filepath.Join(dir, "private")
	public := filepath.Join(dir, "public")
	keyring := filepath.Join(dir, "keyring")
	receipt := filepath.Join(dir, "receipt")
	publicBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	mapping, _ := json.Marshal(map[string]string{"current": public})
	for path, b := range map[string][]byte{private: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), public: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes}), keyring: mapping, receipt: make([]byte, 32)} {
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	client := redis.NewClient(&redis.Options{Addr: "unused.invalid:6379"})
	hook := &wiringRedisHook{}
	client.AddHook(hook)
	defer func() { _ = client.Close() }()
	cfg := config.Config{IdentitySessionV2Enabled: true, IdentityLegacyUpgradeUntil: time.Now().UTC().Add(time.Hour), IdentityJWTKeyFile: private, IdentityJWTVerifyKeysFile: keyring, IdentityJWTActiveKID: "current", IdentityRefreshReceiptKeyFile: receipt}
	deps, err := identityruntime.NewSessionDependencies(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := deps.Coordinator.IsRevoked(t.Context(), uuid.New())
	if err != nil || !revoked || !hook.called {
		t.Fatalf("existing redis adapter not consumed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	claims := identity.AccessClaims{UserID: uuid.New(), SessionID: uuid.New(), TokenID: uuid.New(), CredentialEpoch: 1, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute)}
	token, err := deps.Tokens.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deps.Tokens.Verify(token, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := deps.Tokens.(*sessiontoken.Codec); !ok {
		t.Fatal("runtime did not compose JWT adapter")
	}
	cfg.IdentitySessionV2Enabled = false
	cfg.IdentityJWTKeyFile = ""
	cfg.IdentityJWTActiveKID = ""
	deps, err = identityruntime.NewSessionDependencies(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deps.Tokens.Verify(token, now); err != nil {
		t.Fatal(err)
	}
	if _, err := deps.Tokens.Sign(claims); !errors.Is(err, identity.ErrRefreshDependency) {
		t.Fatalf("verifier-only signing must be dependency failure: %v", err)
	}
}

func TestSessionV2RouterConsumesSecurity(t *testing.T) {
	cfg := config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000", IdentityKeyFile: publicTestKey(t), IdentitySessionV2Enabled: true, IdentityLegacyUpgradeUntil: time.Now().UTC().Add(time.Hour)}
	if _, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, &gorm.DB{}, nil, identityruntime.SessionDependencies{}); err == nil {
		t.Fatal("router allowed enabled security without ports")
	}
	codec := &wiringTokens{}
	coordinator := &wiringCoordinator{}
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, &gorm.DB{}, nil, identityruntime.SessionDependencies{Tokens: codec, Receipts: receiptFixture{}, Coordinator: coordinator})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/session", nil)
	request.AddCookie(&http.Cookie{Name: "glowloom_access", Value: "access"})
	request.AddCookie(&http.Cookie{Name: "glowloom_csrf", Value: "csrf"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !codec.verified || !coordinator.checked {
		t.Fatalf("router did not consume security: %d %v %v", response.Code, codec.verified, coordinator.checked)
	}
}

type blockedCleanupStore struct {
	identity.EmailStore
	entered, ended chan struct{}
}

func (f blockedCleanupStore) EmailTransaction(ctx context.Context, _ func(identity.EmailTx) error) error {
	close(f.entered)
	<-ctx.Done()
	close(f.ended)
	return ctx.Err()
}
func TestSessionReceiptCleanupJoinsActiveBatch(t *testing.T) {
	store := blockedCleanupStore{entered: make(chan struct{}), ended: make(chan struct{})}
	stop, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), identity.NewSessionReceiptCleanup(store, time.Now), zap.NewNop(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("active cleanup missing")
	}
	stop()
	select {
	case <-store.ended:
	default:
		t.Fatal("cleanup returned before active transaction ended")
	}
}

type wiringRedisHook struct{ called bool }

func (h *wiringRedisHook) DialHook(_ redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("test forbids network")
	}
}
func (h *wiringRedisHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		if cmd.Name() != "exists" {
			return errors.New("unexpected redis command")
		}
		h.called = true
		cmd.(*redis.IntCmd).SetVal(1)
		return nil
	}
}
func (h *wiringRedisHook) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return errors.New("test forbids pipeline") }
}
