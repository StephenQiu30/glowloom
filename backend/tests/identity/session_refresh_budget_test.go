package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func refreshBudgetFixture(t *testing.T) (*v2Model, *time.Time, *identity.Auth) {
	t.Helper()
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	hash, err := bcrypt.GenerateFromPassword([]byte("short"), 12)
	if err != nil {
		t.Fatal(err)
	}
	model := &v2Model{user: domain.User{ID: uuid.New(), Email: "budget@example.invalid", EmailVerifiedAt: now, Status: domain.StatusActive, PasswordHash: string(hash), SessionEpoch: 1, Revision: 1}, sessions: map[uuid.UUID]domain.AuthSession{}, refresh: map[string]domain.RefreshToken{}, guards: map[string]domain.LoginGuard{}}
	auth := identity.NewEmailAuth(model, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return now }).WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, &v2Coordinator{})
	return model, &now, auth
}

type budgetCoordinator struct {
	v2Coordinator
	inspect func(context.Context, string) error
}

type budgetEmailStore struct {
	*v2Model
	inspect func(context.Context)
}

func (s *budgetEmailStore) EmailTransaction(ctx context.Context, fn func(identity.EmailTx) error) error {
	s.inspect(ctx)
	return s.v2Model.EmailTransaction(ctx, fn)
}

func (c *budgetCoordinator) Admit(ctx context.Context, _ uuid.UUID, _ string) (func(context.Context) error, error) {
	if err := c.inspect(ctx, "admit"); err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { return c.inspect(ctx, "release") }, nil
}
func (c *budgetCoordinator) ReadReceipt(ctx context.Context, _ uuid.UUID, _ int64) ([]byte, error) {
	return nil, c.inspect(ctx, "read")
}
func (c *budgetCoordinator) WriteReceipt(ctx context.Context, _ uuid.UUID, _ int64, _ []byte, _ time.Time) error {
	return c.inspect(ctx, "write")
}
func (c *budgetCoordinator) MarkRevoked(ctx context.Context, _ uuid.UUID, _ time.Time) error {
	return c.inspect(ctx, "revoke")
}

func TestSessionRefreshBoundsOptionalCoordination(t *testing.T) {
	for _, phase := range []string{"read", "write", "release", "revoke"} {
		t.Run(phase, func(t *testing.T) {
			model, now, auth := refreshBudgetFixture(t)
			login, err := auth.Login(t.Context(), model.user.Email, "short", false)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "revoke" {
				if _, err := auth.Refresh(t.Context(), login.RefreshToken, login.CSRFToken, "peer", uuid.New()); err != nil {
					t.Fatal(err)
				}
				*now = now.Add(11 * time.Second)
			}
			called := false
			coordinator := &budgetCoordinator{inspect: func(ctx context.Context, current string) error {
				if current != phase {
					return nil
				}
				called = true
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Errorf("optional %s can block the committed response beyond one second", phase)
					return errors.New("unbounded optional coordination")
				}
				// A failed cache must be bounded and cannot change durable authority.
				<-ctx.Done()
				return ctx.Err()
			}}
			auth.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, coordinator)
			result, err := auth.Refresh(t.Context(), login.RefreshToken, login.CSRFToken, "peer", uuid.New())
			if !called {
				t.Fatal("coordination stage was not exercised")
			}
			if phase == "revoke" {
				if !errors.Is(err, identity.ErrSessionInvalid) || model.sessions[login.Session.ID].RevokedAt == nil {
					t.Fatal("revocation lost after hint timeout", err)
				}
			} else if err != nil || result.RefreshToken == "" || model.sessions[login.Session.ID].RefreshGeneration != 1 {
				t.Fatal("optional failure hid or repeated the committed rotation", err)
			}
		})
	}
}

func TestSessionRefreshBudgetAndCallerCancellation(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		t.Run(map[bool]string{false: "before proxy deadline", true: "caller cancellation"}[cancelCaller], func(t *testing.T) {
			model, now, auth := refreshBudgetFixture(t)
			login, err := auth.Login(t.Context(), model.user.Email, "short", false)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			transactions := 0
			store := &budgetEmailStore{v2Model: model, inspect: func(ctx context.Context) {
				transactions++
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) >= 10*time.Second {
					t.Error("candidate lookup has no budget below the proxy deadline")
				}
			}}
			auth = identity.NewEmailAuth(store, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return *now })
			coordinator := &budgetCoordinator{inspect: func(ctx context.Context, stage string) error {
				if stage != "admit" {
					t.Fatal("failed admission reached another stage")
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) >= 10*time.Second {
					t.Error("refresh has no budget below the proxy deadline")
				}
				if cancelCaller {
					cancel()
					<-ctx.Done()
					return ctx.Err()
				}
				return identity.ErrRefreshDependency
			}}
			auth.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, coordinator)
			_, err = auth.Refresh(ctx, login.RefreshToken, login.CSRFToken, "peer", uuid.New())
			if transactions != 1 {
				t.Fatalf("admission failure reached %d transactions, want candidate lookup only", transactions)
			}
			want := identity.ErrRefreshDependency
			if cancelCaller {
				want = context.Canceled
			}
			if !errors.Is(err, want) || len(model.refresh) != 1 || model.sessions[login.Session.ID].RefreshGeneration != 0 {
				t.Fatal("failed or canceled refresh changed durable authority", err)
			}
		})
	}
}
