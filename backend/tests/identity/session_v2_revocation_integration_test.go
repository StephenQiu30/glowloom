package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// Run only after controller review. All rows are synthetic in one outer rollback.
func TestSessionV2RevocationRollback(t *testing.T) {
	tx := sessionV2RollbackTransaction(t)
	store := pgidentity.NewStore(tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	hash, err := domain.HashPassword("ExistingPassword123!", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"current", "others", "admin", "disable", "password", "reset", "email", "delete", "audit_rollback"} {
		t.Run(action, func(t *testing.T) {
			uid := uuid.New()
			u := domain.User{ID: uid, OrgID: uuid.New(), LoginName: uuid.NewString(), Email: uid.String() + "@example.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "synthetic revocation", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}
			var s domain.AuthSession
			if e := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
				if e := et.CreatePersonalAccount(u); e != nil {
					return e
				}
				var e error
				u, e = et.AccountByID(uid)
				if e != nil {
					return e
				}
				s = domain.AuthSession{ID: uuid.New(), AccountID: uid, TokenHash: refreshFixture(domain.AuthSession{}, 0, now).Digest, CredentialRevision: u.SessionEpoch, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour), ProtocolVersion: 2, IdleTimeoutSeconds: 43200}
				if e = et.InsertSession(s); e != nil {
					return e
				}
				return et.InsertRefresh(refreshFixture(s, 0, now))
			}); e != nil {
				t.Fatal(e)
			}
			actor := identity.Principal{ID: uid, SessionID: s.ID, CredentialEpoch: u.SessionEpoch}
			revoke := func() error {
				switch action {
				case "current":
					return identity.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(tx), func() time.Time { return now }).Revoke(t.Context(), actor, s.ID)
				case "others":
					var other domain.AuthSession
					other = s
					other.ID = uuid.New()
					other.TokenHash = refreshFixture(s, 0, now).Digest
					if e := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
						if e := et.InsertSession(other); e != nil {
							return e
						}
						return et.InsertRefresh(refreshFixture(other, 0, now))
					}); e != nil {
						return e
					}
					actor.SessionID = other.ID
					return identity.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(tx), func() time.Time { return now }).RevokeOthers(t.Context(), actor)
				case "admin", "disable":
					return pgidentity.NewGovernanceStore(tx, nil).GovernanceTransaction(t.Context(), actor, false, func() time.Time { return now }, func(gt identity.GovernanceTx) error {
						v, e := gt.User(uid)
						if e != nil {
							return e
						}
						a := "revoke_sessions"
						if action == "disable" {
							a = "disable"
						}
						return gt.ChangeUser(v, identity.UserGovernanceChange{Action: a}, now)
					})
				case "email":
					return pgidentity.NewEmailChangeStore(tx).EmailChangeTransaction(t.Context(), actor, func() time.Time { return now }, func(et identity.EmailChangeTx) error {
						return et.ChangeEmail(uuid.NewString()+"@example.invalid", u.Revision, now)
					})
				case "delete":
					return pgidentity.NewAccountDeletionStore(tx).DeletionTransaction(t.Context(), actor, func() time.Time { return now }, func(dt identity.AccountDeletionTx) error { return dt.SoftDelete(now) })

				case "password":
					a := identity.NewEmailAuth(store, v2Proof{}, identity.EmailPolicy{TermsVersion: "test"}, func() time.Time { return now }).WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, &v2Coordinator{})
					_, e := a.ChangePasswordCurrent(t.Context(), actor, u.SessionEpoch, "ExistingPassword123!", "ChangedPassword456!", "ChangedPassword456!")
					return e
				case "reset":
					op := uuid.New()
					secret := strings.Repeat("x", 43)
					c := domain.EmailChallenge{ID: uuid.New(), OperationID: op, Purpose: "password_reset", EmailKey: u.Email, UserID: &uid, EmailVersion: &u.EmailVersion, CredentialEpoch: &u.SessionEpoch, ProofKeyID: "fixture", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
					c.ProofDigest = v2Proof{}.Digest(c.Purpose, c.ID, secret)
					if e := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error { return et.InsertChallenge(c) }); e != nil {
						return e
					}
					return identity.NewEmailAuth(store, v2Proof{}, identity.EmailPolicy{TermsVersion: "test"}, func() time.Time { return now }).CompletePasswordReset(t.Context(), op.String()+"."+secret, "ChangedPassword456!", "ChangedPassword456!")
				case "audit_rollback":
					e := store.EmailTransaction(t.Context(), func(et identity.EmailTx) error {
						if _, e := et.RegistrationEnabled(); e != nil {
							return e
						}
						if _, e := et.AccountByID(uid); e != nil {
							return e
						}
						if e := et.RevokeAccountSessions(uid, now); e != nil {
							return e
						}
						return et.AppendSessionSecurityEvent(uuid.New(), s.ID, uuid.New(), "session.revoked", "synthetic FK failure", now)
					})
					if e == nil {
						return errors.New("audit failure unexpectedly committed")
					}
					return nil
				default:
					return errors.New("unhandled synthetic action")
				}
			}
			if e := revoke(); e != nil {
				t.Fatal(e)
			}
			var parent struct{ RevokedAt *time.Time }
			if e := tx.Raw(`SELECT revoked_at FROM identity.user_session WHERE id=?`, s.ID).Scan(&parent).Error; e != nil {
				t.Fatal(e)
			}
			if (parent.RevokedAt == nil) != (action == "audit_rollback") {
				t.Fatal("parent revocation/audit rollback mismatch")
			}

			var active int64
			if e := tx.Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=? AND (revoked_at IS NULL OR receipt_ciphertext IS NOT NULL OR replay_until IS NOT NULL)`, s.ID).Scan(&active).Error; e != nil {
				t.Fatal(e)
			}
			want := int64(0)
			if action == "audit_rollback" {
				want = 1
			}
			if active != want {
				t.Fatal("parent revocation left refresh credentials")
			}
		})
	}
}
