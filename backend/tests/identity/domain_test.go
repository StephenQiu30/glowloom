package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestPasswordPolicyAndHash(t *testing.T) {
	for _, candidate := range []string{"short1A", strings.Repeat("界", 7), strings.Repeat("a", 129)} {
		if err := domain.ValidatePassword(candidate, ""); !errors.Is(err, domain.ErrWeakPassword) {
			t.Errorf("invalid password accepted: %v", err)
		}
	}
	const password = "a7bc8def"
	hash, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatalf("hash valid password: %v", err)
	}
	if !domain.VerifyPassword(hash, password) || domain.VerifyPassword(hash, "wrongPassword123") {
		t.Fatal("password verification mismatch")
	}
	if !domain.ValidPasswordHash(hash) || domain.ValidPasswordHash(password) {
		t.Fatal("versioned hash format mismatch")
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

func TestSessionMatchesRequiresActiveAccountAndCurrentEpoch(t *testing.T) {
	user := domain.User{Status: domain.StatusActive, SessionEpoch: 4}
	if !user.SessionMatches(4) || user.SessionMatches(3) || user.SessionMatches(0) {
		t.Fatal("credential epoch boundary failed")
	}
	user.Status = domain.StatusDisabled
	if user.SessionMatches(4) {
		t.Fatal("disabled account session remains usable")
	}
}
