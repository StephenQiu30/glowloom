package identity_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

type actualPGFixture struct {
	first, second *gorm.DB
	user          domain.User
	session       domain.AuthSession
	refresh, csrf string
	scope         uuid.UUID
	marker        string
}

func actualPG(t *testing.T) *actualPGFixture {
	t.Helper()
	if os.Getenv("LV_TEST_SESSION_V2_PG_ACTUAL") != "reviewed-exact-fixtures" {
		t.Skip("actual two-connection PostgreSQL NOT verified; safety review opt-in absent")
	}
	dsn := os.Getenv("LV_TEST_SESSION_V2_PG_DSN")
	if dsn == "" {
		t.Fatal("reviewed existing database DSN required")
	}
	f := &actualPGFixture{marker: "auth-task10-" + uuid.NewString()}
	for _, dest := range []**gorm.DB{&f.first, &f.second} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
		cancel()
		if err != nil {
			t.Fatal("connect existing PostgreSQL failed")
		}
		t.Cleanup(func() {
			if err := conn.Close(); err != nil {
				t.Error("close PostgreSQL fixture connection failed")
			}
		})
		pool, err := conn.DB.DB()
		if err != nil {
			t.Fatal("access existing pool failed")
		}
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		*dest = conn.DB
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	uid := uuid.New()
	f.user = domain.User{ID: uid, OrgID: uuid.New(), LoginName: f.marker, Email: f.marker + "@example.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: f.marker, PasswordHash: "not-a-login-credential", PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		t.Fatal("generate fixture token failed")
	}
	f.refresh = base64.RawURLEncoding.EncodeToString(token)
	digest := sha256.Sum256(token)
	if _, err := rand.Read(token); err != nil {
		t.Fatal("generate fixture csrf failed")
	}
	f.csrf = base64.RawURLEncoding.EncodeToString(token)
	proof := sha256.Sum256([]byte(f.csrf))
	f.session = domain.AuthSession{ID: uuid.New(), AccountID: uid, TokenHash: digest[:], CredentialRevision: 1, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour), ProtocolVersion: 2, IdleTimeoutSeconds: 43200, CSRFHash: proof[:]}
	// Registered before the only committed fixture transaction. Cleanup never uses
	// operation context, broad deletes, cascades, or an application cleanup worker.
	t.Cleanup(func() { f.cleanup(t) })
	t.Logf("synthetic setup IDs: user=%s org=%s sid=%s marker=%s", uid, f.user.OrgID, f.session.ID, f.marker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := f.first.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout='1s'").Error; err != nil {
			return err
		}
		return pgidentity.NewStore(tx).EmailTransaction(ctx, func(et identity.EmailTx) error {
			if err := et.CreatePersonalAccount(f.user); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE workspace.organization SET name=? WHERE id=?`, f.marker, f.user.OrgID).Error; err != nil {
				return err
			}
			var scope struct{ ID uuid.UUID }
			if err := tx.Raw(`SELECT id FROM workspace.creation_scope WHERE owner_user_id=? AND physical_scope_id=? AND kind='personal'`, uid, f.user.OrgID).Scan(&scope).Error; err != nil {
				return err
			}
			f.scope = scope.ID
			if f.scope == uuid.Nil {
				return errors.New("fixture scope absent")
			}
			if err := et.InsertSession(f.session); err != nil {
				return err
			}
			if err := et.SetSessionSecurity(f.session.ID, proof[:]); err != nil {
				return err
			}
			return et.InsertRefresh(domain.RefreshToken{ID: uuid.New(), SessionID: f.session.ID, AccountID: uid, Digest: digest[:], IssuedAt: now})
		})
	})
	if err != nil {
		t.Fatal("create exact synthetic PG fixture failed")
	}
	// Identifiers and marker are safe cleanup evidence; never emit token/CSRF/DSN.
	t.Logf("synthetic cleanup IDs: user=%s org=%s scope=%s sid=%s marker=%s", uid, f.user.OrgID, f.scope, f.session.ID, f.marker)
	return f
}

func (f *actualPGFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := f.first.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, sql := range []string{"SET LOCAL lock_timeout='2s'", "SET LOCAL statement_timeout='5s'"} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		if err := pgidentity.NewStore(tx).EmailTransaction(ctx, func(et identity.EmailTx) error { _, err := et.RegistrationEnabled(); return err }); err != nil {
			return err
		}
		// Lock the ownership marker before deleting dependencies. A missing row is an
		// idempotent no-op (including a rolled-back setup); mismatch is a hard failure.
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM identity."user" WHERE id=?`, f.user.ID).Scan(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return f.checkRemoved(tx)
		}
		var owned struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT id FROM identity."user" WHERE id=? AND org_id=? AND login_name=? AND email_key=? AND display_name=? AND platform_role='creator' FOR UPDATE`, f.user.ID, f.user.OrgID, f.marker, f.user.Email, f.marker).Scan(&owned).Error; err != nil {
			return err
		}
		owner := owned.ID
		if owner != f.user.ID {
			return errors.New("fixture ownership marker mismatch")
		}
		var parent struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT id FROM identity.user_session WHERE id=? AND account_id=? FOR UPDATE`, f.session.ID, owner).Scan(&parent).Error; err != nil {
			return err
		}
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM identity.security_event WHERE actor_id=? AND target_user_id=? AND scope_kind='self' AND safe_diff->>'session_id'=? AND action IN ('session.refreshed','session.revoked')`, []any{owner, owner, f.session.ID.String()}},
			{`DELETE FROM identity.session_refresh_token WHERE session_id=? AND EXISTS (SELECT 1 FROM identity.user_session WHERE id=? AND account_id=?)`, []any{f.session.ID, f.session.ID, owner}},
			{`DELETE FROM identity.user_session WHERE id=? AND account_id=?`, []any{f.session.ID, owner}},
			{`DELETE FROM workspace.creation_scope WHERE id=? AND owner_user_id=? AND physical_scope_id=? AND kind='personal'`, []any{f.scope, owner, f.user.OrgID}},
			{`DELETE FROM identity."user" WHERE id=? AND org_id=? AND login_name=? AND email_key=? AND display_name=? AND platform_role='creator'`, []any{owner, f.user.OrgID, f.marker, f.user.Email, f.marker}},
			{`DELETE FROM workspace.organization WHERE id=? AND name=?`, []any{f.user.OrgID, f.marker}},
		} {
			if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
				return err
			}
		}
		return f.checkRemoved(tx)
	})
	if err != nil {
		t.Errorf("exact PG cleanup failed; preserve logged UUIDs for controller reconciliation")
	}
}

// Read only the fixture UUID set, including dependencies that must not survive.
func (f *actualPGFixture) checkRemoved(tx *gorm.DB) error {
	var remaining int64
	err := tx.Raw(`SELECT
 (SELECT count(*) FROM identity."user" WHERE id=?) +
 (SELECT count(*) FROM workspace.organization WHERE id=?) +
 (SELECT count(*) FROM workspace.creation_scope WHERE id=?) +
 (SELECT count(*) FROM identity.user_session WHERE id=?) +
 (SELECT count(*) FROM identity.session_refresh_token WHERE session_id=?) +
 (SELECT count(*) FROM identity.security_event WHERE actor_id=? OR target_user_id=?)`,
		f.user.ID, f.user.OrgID, f.scope, f.session.ID, f.session.ID, f.user.ID, f.user.ID).Scan(&remaining).Error
	if err != nil {
		return err
	}
	if remaining != 0 {
		return errors.New("fixture rows remain; cleanup rolled back")
	}
	return nil
}

func actualPGRevoke(ctx context.Context, tx *gorm.DB, f *actualPGFixture) error {
	return pgidentity.NewStore(tx).EmailTransaction(ctx, func(et identity.EmailTx) error {
		if _, err := et.RegistrationEnabled(); err != nil {
			return err
		}
		if _, err := et.AccountByID(f.user.ID); err != nil {
			return err
		}
		if _, err := et.SessionByID(f.session.ID, true); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := et.RevokeSession(f.user.ID, f.session.ID, now); err != nil {
			return err
		}
		if err := et.RevokeRefreshChain(f.session.ID, now); err != nil {
			return err
		}
		return et.AppendSessionSecurityEvent(f.user.ID, f.session.ID, uuid.New(), "session.revoked", "task10 synthetic race", now)
	})
}

// Real PG transactions and locks; codec and coordination ports are local test
// adapters. The cache adapter deliberately rejects writes: durable receipt
// recovery here is NOT evidence of a shared Redis outage or deployed HTTP.
func TestSessionV2ActualPostgresCompetition(t *testing.T) {
	if os.Getenv("LV_TEST_SESSION_V2_PG_ACTUAL") != "reviewed-exact-fixtures" {
		t.Skip("actual two-connection PostgreSQL NOT verified; safety review opt-in absent")
	}
	for _, scenario := range []string{"refresh_refresh", "revoke_refresh", "refresh_revoke"} {
		t.Run(scenario, func(t *testing.T) {
			f := actualPG(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			first := f.first.WithContext(ctx).Begin()
			if first.Error != nil {
				t.Fatal("begin first transaction failed")
			}
			defer func() { _ = first.Rollback().Error }()
			second := f.second.WithContext(ctx).Begin()
			if second.Error != nil {
				t.Fatal("begin second transaction failed")
			}
			defer func() { _ = second.Rollback().Error }()
			var firstPID, secondPID int
			for _, item := range []struct {
				tx  *gorm.DB
				pid *int
			}{{first, &firstPID}, {second, &secondPID}} {
				if err := item.tx.Exec("SET LOCAL statement_timeout='10s'").Error; err != nil {
					t.Fatal("set local statement deadline failed")
				}
				if err := item.tx.Raw("SELECT pg_backend_pid()").Scan(item.pid).Error; err != nil {
					t.Fatal("read connection identity failed")
				}
			}
			if firstPID == secondPID {
				t.Fatal("competition requires distinct PostgreSQL connections")
			}
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				t.Fatal("receipt fixture key failed")
			}
			protector, err := sessiontoken.NewReceiptProtector(key)
			if err != nil {
				t.Fatal(err)
			}
			auth := func(tx *gorm.DB) *identity.Auth {
				return identity.NewEmailAuth(pgidentity.NewStore(tx), v2Proof{}, identity.EmailPolicy{}, time.Now).WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, protector, &v2Coordinator{})
			}
			var child identity.AuthResult
			if scenario == "revoke_refresh" {
				err = actualPGRevoke(ctx, first, f)
			} else {
				child, err = auth(first).Refresh(ctx, f.refresh, f.csrf, uuid.NewString(), uuid.New())
			}
			if err != nil {
				t.Fatal("first operation failed")
			}
			type outcome struct {
				result identity.AuthResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				var out outcome
				if scenario == "refresh_revoke" {
					out.err = actualPGRevoke(ctx, second, f)
				} else {
					out.result, out.err = auth(second).Refresh(ctx, f.refresh, f.csrf, uuid.NewString(), uuid.New())
				}
				if out.err == nil {
					out.err = second.Commit().Error
				} else {
					_ = second.Rollback().Error
				}
				done <- out
			}()
			// On every failure, cancel and join before rollback, pool close and cleanup.
			joined := false
			defer func() {
				if !joined {
					cancel()
					<-done
				}
			}()
			// Observe the actual blocking edge, never infer contention from a sleep.
			probe, stop := context.WithTimeout(ctx, 3*time.Second)
			blocked := false
			ticker := time.NewTicker(10 * time.Millisecond)
			for probe.Err() == nil {
				if err := first.WithContext(probe).Raw(`SELECT ? = ANY(pg_blocking_pids(?))`, firstPID, secondPID).Scan(&blocked).Error; err != nil {
					break
				}
				if blocked {
					break
				}
				select {
				case <-probe.Done():
				case <-ticker.C:
				}
			}
			ticker.Stop()
			stop()
			if !blocked {
				t.Fatal("second connection never demonstrated the expected PG lock wait")
			}
			if err := first.Commit().Error; err != nil {
				t.Fatal("first commit failed")
			}
			out := <-done
			joined = true
			if scenario == "revoke_refresh" {
				if !errors.Is(out.err, identity.ErrSessionInvalid) {
					t.Fatal("refresh resurrected committed revocation")
				}
			} else if out.err != nil {
				t.Fatal("second operation failed")
			}
			if scenario == "refresh_refresh" && (out.result.RefreshToken != child.RefreshToken || out.result.AccessToken != child.AccessToken || out.result.Session.RefreshGeneration != 1) {
				t.Fatal("competing refresh did not recover the same durable child")
			}
			var state struct {
				RefreshGeneration               int64
				RevokedAt                       *time.Time
				AbsoluteExpiresAt, LastActiveAt time.Time
			}
			if err := f.first.WithContext(ctx).Raw(`SELECT refresh_generation,revoked_at,absolute_expires_at,last_active_at FROM identity.user_session WHERE id=? AND account_id=?`, f.session.ID, f.user.ID).Scan(&state).Error; err != nil {
				t.Fatal("read exact fixture parent failed")
			}
			expectedGeneration := int64(1)
			expectedRows := int64(2)
			if scenario == "revoke_refresh" {
				expectedGeneration = 0
				expectedRows = 1
			}
			if state.RefreshGeneration != expectedGeneration || !state.AbsoluteExpiresAt.Equal(f.session.AbsoluteExpiresAt) || !state.LastActiveAt.Equal(f.session.LastActiveAt) {
				t.Fatal("race changed generation or immutable/idle deadlines")
			}
			var rows, live int64
			if err := f.first.WithContext(ctx).Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=?`, f.session.ID).Scan(&rows).Error; err != nil || rows != expectedRows {
				t.Fatal("duplicate or missing refresh rows")
			}
			if scenario != "refresh_refresh" {
				if state.RevokedAt == nil {
					t.Fatal("parent revocation lost")
				}
				if err := f.first.WithContext(ctx).Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=? AND (revoked_at IS NULL OR receipt_ciphertext IS NOT NULL OR replay_until IS NOT NULL)`, f.session.ID).Scan(&live).Error; err != nil || live != 0 {
					t.Fatal("revocation left live chain or recovery material")
				}
			}
		})
	}
}
