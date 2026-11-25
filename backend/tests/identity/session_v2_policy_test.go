package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// A wrong policy selection, inclusive deadline, or legacy-policy upgrade breaks this test.
func TestSessionV2Policy(t *testing.T) {
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	user := domain.User{Status: domain.StatusActive, SessionEpoch: 2}
	for _, tc := range []struct {
		name           string
		persistent     bool
		idle, absolute time.Duration
	}{
		{"persistent", true, 7 * 24 * time.Hour, 30 * 24 * time.Hour},
		{"ordinary", false, 12 * time.Hour, 7 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := domain.SessionPolicyFor(tc.persistent)
			if policy.IdleTTL != tc.idle || policy.AbsoluteTTL != tc.absolute {
				t.Fatalf("policy=%+v", policy)
			}
			s := domain.AuthSession{ProtocolVersion: 2, IdleTimeoutSeconds: int32(tc.idle / time.Second), CredentialRevision: 2, CreatedAt: start, LastActiveAt: start, AbsoluteExpiresAt: start.Add(tc.absolute), Persistent: tc.persistent}
			if !s.Valid(user, start.Add(tc.idle-time.Nanosecond)) || s.Valid(user, start.Add(tc.idle)) || s.Valid(user, start.Add(-time.Nanosecond)) {
				t.Fatal("idle boundary or rollback accepted")
			}
			s.LastActiveAt = s.AbsoluteExpiresAt.Add(-time.Minute)
			if !s.Valid(user, s.AbsoluteExpiresAt.Add(-time.Nanosecond)) || s.Valid(user, s.AbsoluteExpiresAt) {
				t.Fatal("absolute boundary incorrect")
			}
		})
	}
	for _, version := range []int16{0, 1} {
		s := domain.AuthSession{ProtocolVersion: version, IdleTimeoutSeconds: 604800, Persistent: true, CredentialRevision: 2, LastActiveAt: start, AbsoluteExpiresAt: start.Add(7 * 24 * time.Hour)}
		if !s.Valid(user, start.Add(12*time.Hour-time.Nanosecond)) || s.Valid(user, start.Add(12*time.Hour)) {
			t.Fatal("v1 persistent session was extended")
		}
	}
	s := domain.AuthSession{ProtocolVersion: 2, CredentialRevision: 2, LastActiveAt: start, AbsoluteExpiresAt: start.Add(time.Hour)}
	if s.Valid(user, start) {
		t.Fatal("missing v2 idle timeout accepted")
	}
}

// activityStore isolates persistence without replacing the authorization logic.
type activityStore struct {
	identity.EmailTx
	session domain.AuthSession
	user    domain.User
	touches int
}

func (s *activityStore) EmailTransaction(_ context.Context, run func(identity.EmailTx) error) error {
	return run(s)
}
func (s *activityStore) RegistrationEnabled() (bool, error) { return true, nil }
func (s *activityStore) SessionByHash(_ []byte, _ bool) (domain.AuthSession, error) {
	return s.session, nil
}
func (s *activityStore) AccountByID(_ uuid.UUID) (domain.User, error) { return s.user, nil }
func (s *activityStore) TouchSession(_ uuid.UUID, now time.Time) error {
	s.touches++
	s.session.LastActiveAt = now
	return nil
}

func TestSessionActivityPreservesReauthentication(t *testing.T) {
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	now := start.Add(4 * time.Minute)
	protector, err := credentialproof.New("fixture", bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	for _, tc := range []struct {
		name     string
		activity identity.SessionActivity
		touches  int
	}{
		{"background", identity.NoActivity, 0}, {"business", identity.UserActivity, 1}, {"unknown", identity.SessionActivity(99), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.New()
			csrf := base64.RawURLEncoding.EncodeToString(protector.Digest("session_csrf", id, token))
			hash := sha256.Sum256([]byte(csrf))
			epoch := int64(2)
			store := &activityStore{user: domain.User{Status: domain.StatusActive, SessionEpoch: 2, EmailVerifiedAt: start}, session: domain.AuthSession{ID: id, CredentialRevision: 2, CreatedAt: start, LastActiveAt: start, AbsoluteExpiresAt: start.Add(7 * 24 * time.Hour), ReauthAt: &start, ReauthEpoch: &epoch, CSRFHash: hash[:]}}
			auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{}, func() time.Time { return now })
			result, err := auth.CurrentWithActivity(t.Context(), token, tc.activity)
			if err != nil {
				t.Fatal(err)
			}
			want := start
			if tc.touches == 1 {
				want = now
			}
			if store.touches != tc.touches || !result.Session.LastActiveAt.Equal(want) {
				t.Fatalf("touches=%d last_active=%v", store.touches, result.Session.LastActiveAt)
			}
			if !result.Session.ReauthAt.Equal(start) || result.Session.RecentlyAuthenticated(store.user, start.Add(5*time.Minute)) {
				t.Fatal("activity extended reauthentication")
			}
		})
	}
}

func TestSessionV2PolicyMalformedAuthority(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	u := domain.User{Status: domain.StatusActive, SessionEpoch: 1}
	s := domain.AuthSession{ProtocolVersion: 2, IdleTimeoutSeconds: 43200, CredentialRevision: 1, CreatedAt: now.Add(-time.Hour), LastActiveAt: now, AbsoluteExpiresAt: now.Add(time.Hour)}
	for _, kind := range []string{"unknown protocol", "negative idle", "last active ahead"} {
		t.Run(kind, func(t *testing.T) {
			bad := s
			switch kind {
			case "unknown protocol":
				bad.ProtocolVersion = 3
			case "negative idle":
				bad.IdleTimeoutSeconds = -1
			case "last active ahead":
				bad.LastActiveAt = now.Add(time.Nanosecond)
			}
			if bad.Valid(u, now) {
				t.Fatal("malformed authority valid")
			}
		})
	}
}
