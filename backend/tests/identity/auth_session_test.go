package identity_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

func TestLoginGuardRollingWindowAndLockBoundary(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	guard := domain.LoginGuard{Failures: []time.Time{now.Add(-15 * time.Minute)}}
	for range 4 {
		guard.Fail(now)
	}
	if guard.Locked(now) {
		t.Fatal("expired failure counted toward lock")
	}
	guard.Fail(now)
	if !guard.Locked(now.Add(15*time.Minute-time.Nanosecond)) || guard.Locked(now.Add(15*time.Minute)) {
		t.Fatal("lock boundary must be exact")
	}
	guard.Fail(now.Add(15 * time.Minute))
	if guard.Locked(now.Add(15*time.Minute)) || len(guard.Failures) != 1 {
		t.Fatal("expired lock did not restart failure window")
	}
}

func TestAuthSessionExpiresAtBothBoundaries(t *testing.T) {
	now := time.Now().UTC()
	session := domain.AuthSession{LastActiveAt: now, AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour), CredentialRevision: 2}
	user := domain.User{Status: domain.StatusActive, SessionEpoch: 2}
	if !session.Valid(user, now.Add(12*time.Hour-time.Nanosecond)) || session.Valid(user, now.Add(12*time.Hour)) {
		t.Fatal("idle boundary is incorrect")
	}
	session.LastActiveAt = session.AbsoluteExpiresAt.Add(-time.Minute)
	if session.Valid(user, session.AbsoluteExpiresAt) {
		t.Fatal("absolute boundary is incorrect")
	}
	user.SessionEpoch++
	if session.Valid(user, session.LastActiveAt) {
		t.Fatal("changed credential version accepted")
	}
	user.SessionEpoch--
	user.Status = domain.StatusDisabled
	if session.Valid(user, session.LastActiveAt) {
		t.Fatal("disabled account accepted")
	}
}

func TestNormalizeLoginRejectsUnsupportedCharacters(t *testing.T) {
	key, err := domain.NormalizeLogin(" AbC._-12 ")
	if err != nil || key != "abc._-12" {
		t.Fatal("ASCII case and boundary whitespace normalization failed")
	}
	for _, input := range []string{"", "中文", "a b", "a\nb", "Ａｂｃ", string(make([]byte, 65))} {
		if _, err := domain.NormalizeLogin(input); err == nil {
			t.Fatalf("unsupported login accepted: %q", input)
		}
	}
}
