package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

func TestPasswordPolicyAndHash(t *testing.T) {
	for _, candidate := range []string{"short1A", "onlyletterslong", "12345678901", "a1" + strings.Repeat("a", 71)} {
		if err := domain.ValidatePassword(candidate, ""); !errors.Is(err, domain.ErrWeakPassword) {
			t.Errorf("invalid password accepted: %v", err)
		}
	}
	const password = "validPassword123"
	hash, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatalf("hash valid password: %v", err)
	}
	if !domain.VerifyPassword(hash, password) || domain.VerifyPassword(hash, "wrongPassword123") {
		t.Fatal("bcrypt password verification mismatch")
	}
	if !domain.ValidPasswordHash(hash) || domain.ValidPasswordHash(password) {
		t.Fatal("bcrypt hash format or cost mismatch")
	}
	if err := domain.ValidatePassword(password, hash); !errors.Is(err, domain.ErrPasswordReused) {
		t.Fatalf("same password validation = %v, want ErrPasswordReused", err)
	}
	if _, err := domain.HashPassword(password, hash); !errors.Is(err, domain.ErrPasswordReused) {
		t.Fatalf("same password replacement = %v, want ErrPasswordReused", err)
	}
}

func TestLoginLockAndRecovery(t *testing.T) {
	now := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)
	user := domain.User{Role: domain.RoleProducer, Status: domain.StatusActive, SessionEpoch: 1, Revision: 1}
	for attempt := 1; attempt <= 5; attempt++ {
		locked := user.RegisterLoginFailure(now)
		if locked != (attempt == 5) {
			t.Fatalf("attempt %d locked = %t", attempt, locked)
		}
	}
	if retry, err := user.CanLogin(now.Add(14 * time.Minute)); !errors.Is(err, domain.ErrAccountLocked) || retry != time.Minute {
		t.Fatalf("locked login = (%v, %v)", retry, err)
	}
	if retry, err := user.CanLogin(now.Add(15 * time.Minute)); err != nil || retry != 0 {
		t.Fatalf("expired lock = (%v, %v)", retry, err)
	}
	user.RegisterLoginSuccess(now.Add(15 * time.Minute))
	if user.FailedLoginCount != 0 || !user.LockedUntil.IsZero() {
		t.Fatalf("successful login did not clear lock: %+v", user)
	}
}

func TestDisableProtectsLastAdminAndInvalidatesSession(t *testing.T) {
	user := domain.User{Role: domain.RoleAdmin, Status: domain.StatusActive, SessionEpoch: 4, Revision: 2}
	if err := user.Disable(1); !errors.Is(err, domain.ErrLastActiveAdmin) {
		t.Fatalf("disable last admin = %v", err)
	}
	if !user.SessionMatches(4) {
		t.Fatal("rejected disable changed the account")
	}
	if err := user.Disable(2); err != nil {
		t.Fatalf("disable admin with peer: %v", err)
	}
	if user.Status != domain.StatusDisabled || user.SessionEpoch != 5 || user.Revision != 3 || user.SessionMatches(4) {
		t.Fatalf("disabled account state = %+v", user)
	}
}
