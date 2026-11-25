package identity_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	token "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

type capabilityStore struct {
	identity.EmailStore
	calls int
}

func (s *capabilityStore) EmailTransaction(ctx context.Context, fn func(identity.EmailTx) error) error {
	s.calls++
	return s.EmailStore.EmailTransaction(ctx, fn)
}

type capabilityCoordinator struct {
	identity.RefreshCoordinator
	calls int
}

func (c *capabilityCoordinator) Admit(ctx context.Context, sid uuid.UUID, source string) (func(context.Context) error, error) {
	c.calls++
	return c.RefreshCoordinator.Admit(ctx, sid, source)
}
func (c *capabilityCoordinator) ReadReceipt(ctx context.Context, sid uuid.UUID, generation int64) ([]byte, error) {
	c.calls++
	return c.RefreshCoordinator.ReadReceipt(ctx, sid, generation)
}
func (c *capabilityCoordinator) WriteReceipt(ctx context.Context, sid uuid.UUID, generation int64, b []byte, until time.Time) error {
	c.calls++
	return c.RefreshCoordinator.WriteReceipt(ctx, sid, generation, b, until)
}
func (c *capabilityCoordinator) MarkRevoked(ctx context.Context, sid uuid.UUID, until time.Time) error {
	c.calls++
	return c.RefreshCoordinator.MarkRevoked(ctx, sid, until)
}
func (c *capabilityCoordinator) IsRevoked(ctx context.Context, sid uuid.UUID) (bool, error) {
	c.calls++
	return c.RefreshCoordinator.IsRevoked(ctx, sid)
}

func TestSessionV2VerifierCannotRefreshExistingChain(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	signer, err := token.New("fixture", private, map[string][]byte{"fixture": public})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(map[string][]byte{"fixture": public})
	if err != nil {
		t.Fatal(err)
	}
	if !signer.CanSign() || verifier.CanSign() {
		t.Fatal("codec signing capability mismatch")
	}
	receipts, err := token.NewReceiptProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("short"), 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, delay := range []time.Duration{time.Second, 11 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
			model := &v2Model{user: domain.User{ID: uuid.New(), Email: "fixture@example.invalid", EmailVerifiedAt: now, Status: domain.StatusActive, PasswordHash: string(hash), SessionEpoch: 1, Revision: 1}, sessions: map[uuid.UUID]domain.AuthSession{}, refresh: map[string]domain.RefreshToken{}, guards: map[string]domain.LoginGuard{}}
			store := &capabilityStore{EmailStore: model}
			coordinator := &capabilityCoordinator{RefreshCoordinator: &v2Coordinator{}}
			auth := identity.NewEmailAuth(store, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return now }).WithSessionSecurity(signer, receipts, coordinator)
			first, err := auth.Login(t.Context(), model.user.Email, "short", false)
			if err != nil {
				t.Fatal(err)
			}
			child, err := auth.Refresh(t.Context(), first.RefreshToken, first.CSRFToken, "peer", uuid.New())
			if err != nil || child.AccessToken == "" || child.RefreshToken == first.RefreshToken {
				t.Fatalf("normal signer rotation: %v", err)
			}
			auth.WithSessionSecurity(verifier, receipts, coordinator).WithSessionIssuance(false)
			now = now.Add(delay)
			if _, err := auth.CurrentAccess(t.Context(), child.AccessToken, first.CSRFToken, identity.NoActivity); err != nil {
				t.Fatalf("real verifier lost current access: %v", err)
			}
			store.calls = 0
			coordinator.calls = 0
			result, err := auth.Refresh(t.Context(), first.RefreshToken, first.CSRFToken, "peer", uuid.New())
			if !errors.Is(err, identity.ErrRefreshDependency) || result.AccessToken != "" || result.RefreshToken != "" {
				t.Fatalf("verifier refresh must reject dependency before recovery/reuse: result=%t err=%v", result.AccessToken != "", err)
			}
			if store.calls != 0 || coordinator.calls != 0 {
				t.Fatalf("verifier refresh touched authority/cache: %d/%d", store.calls, coordinator.calls)
			}
			if model.sessions[first.Session.ID].RevokedAt != nil {
				t.Fatal("verifier refresh revoked chain")
			}
		})
	}
}
