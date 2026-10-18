package identity_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func legacyUpgradeFixture(t *testing.T) (*identity.Auth, *v2Model, *time.Time, identity.AuthResult) {
	t.Helper()
	_, m, now, a := sessionHTTPAuthFixture(t, false)
	a.WithSessionIssuance(false)
	r, err := a.Login(t.Context(), m.user.Email, "short", true)
	if err != nil {
		t.Fatal(err)
	}
	a.WithSessionIssuance(true)
	return a, m, now, r
}

func TestSessionV2NearIdle(t *testing.T) {
	for _, touch := range []bool{false, true} {
		t.Run(map[bool]string{false: "no foreground", true: "foreground extends idle"}[touch], func(t *testing.T) {
			_, m, now, a := sessionHTTPAuthFixture(t, false)
			r, err := a.Login(t.Context(), m.user.Email, "short", true)
			if err != nil {
				t.Fatal(err)
			}
			s := m.sessions[r.Session.ID]
			s.LastActiveAt = now.Add(-7*24*time.Hour + 5*time.Second)
			s.CreatedAt = s.LastActiveAt
			m.sessions[s.ID] = s
			child, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			if !child.AccessExpiresAt.Equal(now.Add(15 * time.Minute)) {
				t.Fatal("JWT clipped to old idle deadline")
			}
			*now = now.Add(4 * time.Second)
			if touch {
				if _, err = a.CurrentAccess(t.Context(), child.AccessToken, r.CSRFToken, identity.UserActivity); err != nil {
					t.Fatal(err)
				}
			}
			*now = now.Add(2 * time.Second)
			recovered, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New())
			if touch {
				if err != nil || recovered.AccessToken != child.AccessToken || recovered.RefreshToken != child.RefreshToken {
					t.Fatal("same child lost after previous idle", err)
				}
			} else if !errors.Is(err, identity.ErrSessionInvalid) {
				t.Fatal("idle expiry bypassed", err)
			}
		})
	}
}

// Model evidence only; the real PG rollback entry is added separately for controller review.
func TestSessionV2UpgradeModel(t *testing.T) {
	a, m, now, old := legacyUpgradeFixture(t)
	upgrade, ok := any(a).(interface {
		WithLegacyUpgradeUntil(time.Time) *identity.Auth
		UpgradeLegacy(context.Context, string, string, string, uuid.UUID) (identity.AuthResult, error)
	})
	if !ok {
		t.Fatal("legacy upgrade capability absent")
	}
	upgrade.WithLegacyUpgradeUntil(now.Add(24 * time.Hour))
	before := m.sessions[old.Session.ID]
	hash := m.user.PasswordHash
	upgraded, err := upgrade.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "192.0.2.10", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	after := upgraded.Session
	if after.ID != before.ID || after.ProtocolVersion != 2 || after.RefreshGeneration != 1 || after.CreatedAt != before.CreatedAt || after.LastActiveAt != before.LastActiveAt || after.AbsoluteExpiresAt != before.AbsoluteExpiresAt || after.IdleTimeoutSeconds != before.IdleTimeoutSeconds || !bytes.Equal(after.CSRFHash, before.CSRFHash) || m.user.PasswordHash != hash || upgraded.CSRFToken != old.CSRFToken {
		t.Fatal("upgrade changed original authority")
	}
	if _, err = a.Current(t.Context(), old.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("v2 fell back to v1", err)
	}
	again, err := upgrade.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "192.0.2.10", uuid.New())
	if err != nil || again.RefreshToken != upgraded.RefreshToken || again.AccessToken != upgraded.AccessToken {
		t.Fatal("lost response not recovered", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(old.Token)
	digest := sha256.Sum256(raw)
	if len(m.refresh) != 2 || m.refresh[string(digest[:])].ConsumedAt == nil {
		t.Fatal("legacy receipt absent")
	}
	*now = now.Add(10 * time.Second)
	if _, err = upgrade.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "192.0.2.10", uuid.New()); !errors.Is(err, identity.ErrSessionInvalid) || m.sessions[old.Session.ID].RevokedAt == nil {
		t.Fatal("old reuse did not commit revocation", err)
	}
}

func (m *v2Model) UpgradeLegacySession(sid, account uuid.UUID, digest []byte) error {
	s, ok := m.sessions[sid]
	if !ok || s.AccountID != account || s.ProtocolVersion != 1 || s.RefreshGeneration != 0 || s.RevokedAt != nil || !bytes.Equal(s.TokenHash, digest) {
		return identity.ErrSessionInvalid
	}
	s.ProtocolVersion = 2
	m.sessions[sid] = s
	return nil
}

func TestSessionV2LegacyWindow(t *testing.T) {
	a, m, now, old := legacyUpgradeFixture(t)
	a.WithLegacyUpgradeUntil(*now)
	if _, err := a.Current(t.Context(), old.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("closed legacy validation", err)
	}
	a.WithSessionIssuance(false)
	if _, err := a.Login(t.Context(), m.user.Email, "short", true); !errors.Is(err, identity.ErrRefreshDependency) {
		t.Fatal("rollback minted unusable legacy", err)
	}
	a.WithSessionIssuance(true)
	if _, err := a.Login(t.Context(), m.user.Email, "short", true); err != nil {
		t.Fatal("original password unavailable on v2", err)
	}
}

// Controller must review this exact outer-rollback entry before any existing DB run.
func TestSessionV2UpgradeRollback(t *testing.T) {
	tx := sessionV2RollbackTransaction(t)
	store := pgidentity.NewStore(tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	hash, err := domain.HashPassword("OriginalPassword123!", "")
	if err != nil {
		t.Fatal(err)
	}
	uid := uuid.New()
	user := domain.User{ID: uid, OrgID: uuid.New(), LoginName: uuid.NewString(), Email: uid.String() + "@example.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "synthetic upgrade fixture", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}
	if err = store.EmailTransaction(t.Context(), func(et identity.EmailTx) error { return et.CreatePersonalAccount(user) }); err != nil {
		t.Fatal(err)
	}
	a := identity.NewEmailAuth(store, v2Proof{}, identity.EmailPolicy{TermsVersion: "test"}, func() time.Time { return now })
	source := "upgrade-rollback-" + uid.String()
	legacy, err := a.LoginFrom(t.Context(), user.Email, "OriginalPassword123!", true, source)
	if err != nil {
		t.Fatal(err)
	}
	// Give the synthetic device an existing proof so preservation is not a nil-only check.
	if err = store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		return et.ReauthenticateSession(legacy.Session.ID, legacy.User.SessionEpoch, now)
	}); err != nil {
		t.Fatal(err)
	}
	readFixture := func(stage string) (domain.AuthSession, string) {
		var session domain.AuthSession
		row := tx.Raw(`SELECT * FROM identity.user_session WHERE id=? AND account_id=?`, legacy.Session.ID, uid).Scan(&session)
		if row.Error != nil || row.RowsAffected != 1 {
			t.Fatalf("%s synthetic session readback failed", stage)
		}
		var passwordHash string
		row = tx.Raw(`SELECT password_hash FROM identity."user" WHERE id=?`, uid).Scan(&passwordHash)
		if row.Error != nil || row.RowsAffected != 1 {
			t.Fatalf("%s synthetic password readback failed", stage)
		}
		return session, passwordHash
	}
	assertPreserved := func(stage string, before, after domain.AuthSession) {
		for _, field := range upgradePreservationChanges(before, after) {
			t.Errorf("%s changed %s", stage, field)
		}
		if t.Failed() {
			t.FailNow()
		}
	}
	before, beforeHash := readFixture("before upgrade")
	// Compare the login result's instants to the actual pre-upgrade row, separately
	// from the proof intentionally seeded above. No rounding or tolerance is used.
	for _, field := range []struct {
		name             string
		returned, stored time.Time
	}{
		{"created_at", legacy.Session.CreatedAt, before.CreatedAt},
		{"last_active_at", legacy.Session.LastActiveAt, before.LastActiveAt},
		{"absolute_expires_at", legacy.Session.AbsoluteExpiresAt, before.AbsoluteExpiresAt},
	} {
		if !field.returned.Equal(field.stored) {
			t.Fatalf("pre-upgrade persistence changed %s", field.name)
		}
	}
	if beforeHash != hash {
		t.Fatal("pre-upgrade persistence changed password_hash")
	}
	a.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, &v2Coordinator{}).WithLegacyUpgradeUntil(now.Add(24 * time.Hour))
	upgraded, err := a.UpgradeLegacy(t.Context(), legacy.Token, legacy.CSRFToken, source, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	assertPreserved("upgrade result", before, upgraded.Session)
	after, afterHash := readFixture("after upgrade")
	assertPreserved("PG upgrade", before, after)
	if afterHash != beforeHash {
		t.Fatal("PG upgrade changed password_hash")
	}
	if upgraded.CSRFToken != legacy.CSRFToken {
		t.Fatal("upgrade result changed csrf_token")
	}
	if upgraded.Session.ProtocolVersion != 2 || after.ProtocolVersion != 2 {
		t.Fatal("upgrade protocol_version is not 2")
	}
	if upgraded.Session.RefreshGeneration != 1 || after.RefreshGeneration != 1 {
		t.Fatal("upgrade refresh_generation is not 1")
	}

	again, err := a.UpgradeLegacy(t.Context(), legacy.Token, legacy.CSRFToken, source, uuid.New())
	if err != nil || again.RefreshToken != upgraded.RefreshToken {
		t.Fatal("durable upgrade receipt missing", err)
	}
	recovered, recoveredHash := readFixture("after recovery")
	assertPreserved("PG recovery", before, recovered)
	if recoveredHash != beforeHash {
		t.Fatal("PG recovery changed password_hash")
	}
	if err = store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		u, e := et.AccountByID(uid)
		if e != nil {
			return e
		}
		if u.PasswordHash != hash || !domain.VerifyPassword(u.PasswordHash, "OriginalPassword123!") {
			t.Fatal("original credential altered")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Current(t.Context(), legacy.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("legacy fallback", err)
	}
	now = now.Add(10 * time.Second)
	if _, err = a.UpgradeLegacy(t.Context(), legacy.Token, legacy.CSRFToken, source, uuid.New()); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("reuse accepted", err)
	}
	if err = store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		s, e := et.SessionByID(legacy.Session.ID, false)
		if e == nil && s.RevokedAt == nil {
			t.Fatal("reuse revoke not persisted")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionV2UpgradeRejects(t *testing.T) {
	for _, kind := range []string{"idle", "absolute", "revoked", "deleted", "csrf", "cutoff", "audit"} {
		t.Run(kind, func(t *testing.T) {
			a, m, now, old := legacyUpgradeFixture(t)
			a.WithLegacyUpgradeUntil(now.Add(24 * time.Hour))
			s := m.sessions[old.Session.ID]
			csrf := old.CSRFToken
			switch kind {
			case "idle":
				s.LastActiveAt = now.Add(-12 * time.Hour)
				s.CreatedAt = s.LastActiveAt
			case "absolute":
				s.AbsoluteExpiresAt = *now
			case "revoked":
				s.RevokedAt = now
			case "deleted":
				m.user.DeletedAt = *now
				m.user.Status = domain.StatusDisabled
			case "csrf":
				csrf = "bad"
			case "cutoff":
				a.WithLegacyUpgradeUntil(*now)
			case "audit":
				m.failAudit = true
			}
			m.sessions[s.ID] = s
			if _, err := a.UpgradeLegacy(t.Context(), old.Token, csrf, "192.0.2.10", uuid.New()); err == nil {
				t.Fatal("invalid upgrade accepted")
			}
			if len(m.refresh) != 0 || m.sessions[s.ID].ProtocolVersion != 1 {
				t.Fatal("failed upgrade persisted")
			}
		})
	}
}

// The spy observes the application/coordination boundary, not a Redis simulation.
type upgradeSourceCoordinator struct {
	v2Coordinator
	sources     []string
	sessions    []uuid.UUID
	beforeAdmit func() error
}

func (q *upgradeSourceCoordinator) Admit(ctx context.Context, sid uuid.UUID, source string) (func(context.Context) error, error) {
	q.sources = append(q.sources, source)
	q.sessions = append(q.sessions, sid)
	if q.beforeAdmit != nil {
		run := q.beforeAdmit
		q.beforeAdmit = nil
		if err := run(); err != nil {
			return nil, err
		}
	}
	return q.v2Coordinator.Admit(ctx, sid, source)
}

func TestSessionV2UpgradeLockedRecoverySource(t *testing.T) {
	a, m, now, old := legacyUpgradeFixture(t)
	q := &upgradeSourceCoordinator{}
	codec := &v2Codec{claims: map[string]identity.AccessClaims{}}
	a.WithSessionSecurity(codec, &v2Receipt{}, q).WithLegacyUpgradeUntil(now.Add(time.Hour))
	winner := identity.NewEmailAuth(m, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return *now }).WithSessionSecurity(codec, &v2Receipt{}, &v2Coordinator{}).WithLegacyUpgradeUntil(now.Add(time.Hour))
	var first identity.AuthResult
	q.beforeAdmit = func() error {
		var err error
		first, err = winner.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "198.51.100.20", uuid.New())
		return err
	}
	recovered, err := a.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "192.0.2.10", uuid.New())
	if err != nil || recovered.RefreshToken != first.RefreshToken {
		t.Fatal("locked winner recovery failed", err)
	}
	if len(q.sources) != 2 || q.sources[0] != "192.0.2.10" || q.sources[1] != "192.0.2.10" {
		t.Fatalf("locked recovery changed source: %v", q.sources)
	}
	before := len(q.sources)
	if _, err = a.UpgradeLegacy(t.Context(), old.Token, old.CSRFToken, "", uuid.New()); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("missing trusted source accepted", err)
	}
	if len(q.sources) != before {
		t.Fatal("missing source reached coordination")
	}
}

// upgradePreservationChanges names differences without printing credential hashes.
func upgradePreservationChanges(before, after domain.AuthSession) []string {
	var fields []string
	checks := []struct {
		name string
		same bool
	}{
		{"id", before.ID == after.ID}, {"account_id", before.AccountID == after.AccountID},
		{"created_at", before.CreatedAt.Equal(after.CreatedAt)}, {"last_active_at", before.LastActiveAt.Equal(after.LastActiveAt)}, {"absolute_expires_at", before.AbsoluteExpiresAt.Equal(after.AbsoluteExpiresAt)},
		{"idle_timeout_seconds", before.IdleTimeoutSeconds == after.IdleTimeoutSeconds}, {"persistent", before.Persistent == after.Persistent},
		{"credential_revision", before.CredentialRevision == after.CredentialRevision}, {"token_hash", bytes.Equal(before.TokenHash, after.TokenHash)}, {"csrf_hash", bytes.Equal(before.CSRFHash, after.CSRFHash)},
		{"reauth_at", sameUpgradeTime(before.ReauthAt, after.ReauthAt)}, {"reauth_epoch", sameUpgradeEpoch(before.ReauthEpoch, after.ReauthEpoch)}, {"revoked_at", sameUpgradeTime(before.RevokedAt, after.RevokedAt)},
	}
	for _, check := range checks {
		if !check.same {
			fields = append(fields, check.name)
		}
	}
	return fields
}
func sameUpgradeTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
func sameUpgradeEpoch(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestSessionV2UpgradeTimestampRepresentation(t *testing.T) {
	original := time.Date(2026, 10, 18, 3, 0, 0, 123456000, time.UTC)
	// Real installed pgx timestamptz codec round trip, without a connection.
	codec := pgtype.NewMap()
	raw, err := codec.Encode(pgtype.TimestamptzOID, pgtype.BinaryFormatCode, original, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded time.Time
	if err = codec.Scan(pgtype.TimestamptzOID, pgtype.BinaryFormatCode, raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !original.Equal(decoded) {
		t.Fatal("codec changed instant")
	}
	if original.Location() == decoded.Location() {
		t.Fatal("fixture must expose location representation difference")
	}
	before := domain.AuthSession{CreatedAt: original, LastActiveAt: original, AbsoluteExpiresAt: original, ReauthAt: &original}
	after := domain.AuthSession{CreatedAt: decoded, LastActiveAt: decoded, AbsoluteExpiresAt: decoded, ReauthAt: &decoded}
	if changes := upgradePreservationChanges(before, after); len(changes) != 0 {
		t.Fatalf("same instants reported changed: %v", changes)
	}
	after.LastActiveAt = decoded.Add(time.Nanosecond)
	changes := upgradePreservationChanges(before, after)
	if len(changes) != 1 || changes[0] != "last_active_at" {
		t.Fatalf("actual nanosecond change hidden or misnamed: %v", changes)
	}
}
