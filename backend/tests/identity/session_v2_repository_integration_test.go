package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// No schema replay, settings mutation, real account lookup, or committed fixture writes.
func sessionV2RollbackTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		t.Skip("existing database absent; rollback integration NOT verified")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("connect existing database failed")
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error("close existing connection failed")
		}
	})
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	// LIFO cleanup: rollback runs before connection close and context cancellation.
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback synthetic session fixtures failed")
		}
	})
	for _, sql := range []string{"SET LOCAL lock_timeout='1s'", "SET LOCAL statement_timeout='10s'"} {
		if err := tx.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

func refreshFixture(s domain.AuthSession, generation int64, now time.Time) domain.RefreshToken {
	digest := sha256.Sum256([]byte(uuid.NewString()))
	return domain.RefreshToken{ID: uuid.New(), SessionID: s.ID, AccountID: s.AccountID, Generation: generation, Digest: digest[:], IssuedAt: now}
}

func TestSessionV2RepositoryRollback(t *testing.T) {
	tx := sessionV2RollbackTransaction(t)
	store := pgidentity.NewStore(tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	uid := uuid.New()
	user := domain.User{ID: uid, OrgID: uuid.New(), LoginName: uuid.NewString(), Email: uid.String() + "@example.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "synthetic session fixture", PasswordHash: "not-a-login-credential", PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}
	var persistent, ordinary domain.AuthSession
	err := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		if err := et.CreatePersonalAccount(user); err != nil {
			return err
		}
		u, err := et.AccountByID(uid)
		if err != nil {
			return err
		}
		for _, keep := range []bool{true, false} {
			digest := sha256.Sum256([]byte(uuid.NewString()))
			s := domain.AuthSession{ID: uuid.New(), AccountID: uid, TokenHash: digest[:], CredentialRevision: u.SessionEpoch, CreatedAt: now.Add(-48 * time.Hour), LastActiveAt: now.Add(-24 * time.Hour), AbsoluteExpiresAt: now.Add(5 * 24 * time.Hour), ProtocolVersion: 2, IdleTimeoutSeconds: 43200, Persistent: keep}
			if keep {
				s.IdleTimeoutSeconds = 604800
				persistent = s
			} else {
				ordinary = s
			}
			if err := et.InsertSession(s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	old := refreshFixture(persistent, 0, now)
	if err := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error { return et.InsertRefresh(old) }); err != nil {
		t.Fatal(err)
	}
	next := refreshFixture(persistent, 1, now)
	op := uuid.New()
	until := now.Add(10 * time.Second)
	receipt := []byte("opaque-test-only-ciphertext")
	rotate := func(et identity.EmailTx) error {
		if _, err := et.AccountByID(uid); err != nil {
			return err
		}
		if _, err := et.SessionByID(persistent.ID, true); err != nil {
			return err
		}
		locked, err := et.FindRefresh(old.Digest, true)
		if err != nil {
			return err
		}
		if locked.AccountID != uid || locked.SessionID != persistent.ID {
			return errors.New("binding lost")
		}
		return et.RotateRefresh(locked, next, op, until, receipt)
	}
	if err := store.EmailTransaction(t.Context(), rotate); err != nil {
		t.Fatal(err)
	}
	if err := store.EmailTransaction(t.Context(), rotate); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("duplicate consumption = %v", err)
	}
	if err := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		got, err := et.FindRefresh(old.Digest, false)
		if err != nil {
			return err
		}
		if got.ConsumedAt == nil || got.RotationRequestID != op || got.ReplayUntil == nil || !got.ReplayUntil.Equal(until) || !bytes.Equal(got.ReceiptCiphertext, receipt) {
			t.Fatal("rotation receipt binding lost")
		}
		s, err := et.SessionByID(persistent.ID, false)
		if err != nil {
			return err
		}
		if s.RefreshGeneration != 1 || !s.LastActiveAt.Equal(persistent.LastActiveAt) || !s.AbsoluteExpiresAt.Equal(persistent.AbsoluteExpiresAt) {
			t.Fatal("rotation mutated session deadlines or lost generation")
		}
		_, err = et.FindRefresh([]byte("unknown"), false)
		if !errors.Is(err, identity.ErrSessionInvalid) {
			t.Fatal("unknown digest accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Force only a synthetic audit FK failure inside a savepoint; all rotation effects roll back.
	third := refreshFixture(persistent, 2, now.Add(time.Second))
	err = store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		if _, err := et.AccountByID(uid); err != nil {
			return err
		}
		if _, err := et.SessionByID(persistent.ID, true); err != nil {
			return err
		}
		locked, err := et.FindRefresh(next.Digest, true)
		if err != nil {
			return err
		}
		if err := et.RotateRefresh(locked, third, uuid.New(), now.Add(11*time.Second), receipt); err != nil {
			return err
		}
		return et.AppendSessionSecurityEvent(uuid.New(), persistent.ID, uuid.New(), "session.refreshed", "synthetic audit failure", now)
	})
	if err == nil {
		t.Fatal("audit failure accepted")
	}
	if err := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		got, err := et.FindRefresh(next.Digest, false)
		if err != nil {
			return err
		}
		if got.ConsumedAt != nil {
			t.Fatal("audit failure committed consumption")
		}
		_, err = et.FindRefresh(third.Digest, false)
		if !errors.Is(err, identity.ErrSessionInvalid) {
			t.Fatal("audit failure committed child")
		}
		s, err := et.SessionByID(persistent.ID, false)
		if err != nil {
			return err
		}
		if s.RefreshGeneration != 1 {
			t.Fatal("audit failure committed generation")
		}
		return et.AppendSessionSecurityEvent(uid, persistent.ID, op, "session.refreshed", "rotation", now)
	}); err != nil {
		t.Fatal(err)
	}
	actor := identity.Principal{ID: uid, SessionID: persistent.ID, CredentialEpoch: persistent.CredentialRevision}
	directory := identity.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(tx), func() time.Time { return now })
	sessions, err := directory.List(t.Context(), actor)
	if err != nil || len(sessions) != 1 || sessions[0].ID != persistent.ID {
		t.Fatalf("persistent/ordinary idle projection: %v %v", sessions, err)
	}
	actor.SessionID = ordinary.ID
	if _, err := directory.List(t.Context(), actor); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("ordinary idle session accepted: %v", err)
	}
	// Global cleanup is deliberately not run against the shared business DB.
	// Revoke only the synthetic chain, then inspect its retained digest history.
	if err := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		if err := et.RevokeRefreshChain(persistent.ID, until); err != nil {
			return err
		}
		for _, digest := range [][]byte{old.Digest, next.Digest} {
			got, err := et.FindRefresh(digest, false)
			if err != nil {
				return err
			}
			if got.RevokedAt == nil || got.ReceiptCiphertext != nil || got.ReplayUntil != nil {
				t.Fatal("revocation retained recovery material")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
