package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func changeCode(t *testing.T, tx *gorm.DB, op uuid.UUID, side string) string {
	t.Helper()
	var m struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT m.event_id,m.key_id,m.encrypted_payload FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND c.purpose=?`, op, "email_"+side).Scan(&m).Error; err != nil {
		t.Fatal(err)
	}
	p, _ := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	b, err := p.Open(m.EventID, m.KeyID, m.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var d struct{ Code string }
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return d.Code
}
func TestEmailChangeAtomicity(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, a := verifiedTestActor(t, tx, "change-old@example.invalid", now)
	tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, a.CredentialEpoch, a.SessionID)
	var rev int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, a.ID).Scan(&rev)
	team, work := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Retained email team',?)`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, a.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,name,aspect_ratio,style_type) SELECT ?,org_id,id,'Retained email work','16:9','realistic' FROM identity."user" WHERE id=?`, work, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	s := app.NewEmailChange(auth, pg.NewEmailChangeStore(tx), func() time.Time { return now })
	op, err := s.Start(t.Context(), a, "change-new@example.invalid", rev)
	if err != nil {
		t.Fatal(err)
	}
	old, newCode := changeCode(t, tx, op, "old"), changeCode(t, tx, op, "new")
	if err := s.Verify(t.Context(), a, op, "old", old); !errors.Is(err, app.ErrProofInvalid) {
		t.Fatal("pending mail granted proof", err)
	}
	tx.Exec(`UPDATE identity.mail_delivery SET status='submitted' WHERE challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)`, op)
	if err := s.Verify(t.Context(), a, uuid.New(), "old", old); !errors.Is(err, app.ErrProofInvalid) {
		t.Fatal("wrong operation accepted", err)
	}
	if err := s.Verify(t.Context(), a, op, "new", old); !errors.Is(err, app.ErrProofInvalid) {
		t.Fatal("wrong side accepted", err)
	}
	if err := s.Verify(t.Context(), a, op, "old", old); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(t.Context(), a, op, "old", old); !errors.Is(err, app.ErrProofInvalid) {
		t.Fatal("proof replay", err)
	}
	if err := s.Complete(t.Context(), a, op, rev); !errors.Is(err, app.ErrProofInvalid) {
		t.Fatal("single proof complete", err)
	}
	if err := s.Verify(t.Context(), a, op, "new", newCode); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(t.Context(), a, op, rev-1); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale completion", err)
	}
	if err := s.Complete(t.Context(), a, op, rev); err != nil {
		t.Fatal(err)
	}
	var u struct {
		ID                                   uuid.UUID
		EmailKey                             string
		EmailVersion, SessionEpoch, Revision int64
	}
	tx.Raw(`SELECT id,email_key,email_version,session_epoch,revision FROM identity."user" WHERE id=?`, a.ID).Scan(&u)
	if u.ID != a.ID || u.EmailKey != "change-new@example.invalid" || u.EmailVersion != 2 || u.SessionEpoch != a.CredentialEpoch+1 || u.Revision != rev+1 {
		t.Fatalf("partial identity %+v", u)
	}
	var live, notices int64
	tx.Raw(`SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, a.ID).Scan(&live)
	tx.Raw(`SELECT count(*) FROM identity.mail_delivery WHERE template='email_changed' AND recipient IN ('change-old@example.invalid','change-new@example.invalid')`).Scan(&notices)
	var retained bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team_member m JOIN workspace.project p ON p.owner_id=m.user_id WHERE m.team_id=? AND m.user_id=? AND m.status='active' AND p.id=? AND NOT p.is_delete)`, team, a.ID, work).Scan(&retained).Error; err != nil || !retained {
		t.Fatal("email change mutated ownership", err)
	}
	if _, err := auth.Login(t.Context(), "change-old@example.invalid", "long password for tests", false); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatal("old email still logs in", err)
	}
	if live != 0 || notices != 2 {
		t.Fatalf("sessions/notifications %d/%d", live, notices)
	}
	if err := s.Complete(t.Context(), a, op, rev); !errors.Is(err, app.ErrSessionInvalid) {
		t.Fatal("completion replay", err)
	}
}
func TestEmailChangeBindings(t *testing.T) {
	for _, mode := range []string{"cancel", "expiry", "revision", "reserved", "late_conflict", "failed_mail", "session"} {
		t.Run(mode, func(t *testing.T) {
			tx := existingIdentityTransaction(t)
			now := time.Now().UTC().Truncate(time.Microsecond)
			auth, a := verifiedTestActor(t, tx, mode+"-old@example.invalid", now)
			target := mode + "-new@example.invalid"
			tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, a.CredentialEpoch, a.SessionID)
			var rev int64
			tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, a.ID).Scan(&rev)
			s := app.NewEmailChange(auth, pg.NewEmailChangeStore(tx), func() time.Time { return now })
			if mode == "reserved" {
				verifiedTestActor(t, tx, target, now)
				tx.Exec(`UPDATE identity."user" SET is_delete=true WHERE email_key=?`, target)
				if _, err := s.Start(t.Context(), a, target, rev); !errors.Is(err, app.ErrEmailUnavailable) {
					t.Fatal("deleted email released", err)
				}
				return
			}
			op, err := s.Start(t.Context(), a, target, rev)
			if err != nil {
				t.Fatal(err)
			}
			old, newCode := changeCode(t, tx, op, "old"), changeCode(t, tx, op, "new")
			tx.Exec(`UPDATE identity.mail_delivery SET status='submitted' WHERE challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)`, op)
			switch mode {
			case "cancel":
				if err := s.Cancel(t.Context(), a, op); err != nil {
					t.Fatal(err)
				}
			case "expiry":
				now = now.Add(10 * time.Minute)
				if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, a.CredentialEpoch, a.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			case "revision":
				tx.Exec(`UPDATE identity."user" SET revision=revision+1 WHERE id=?`, a.ID)
			case "failed_mail":
				tx.Exec(`UPDATE identity.mail_delivery SET status='failed' WHERE challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)`, op)
			case "session":
				login, err := auth.Login(t.Context(), mode+"-old@example.invalid", "long password for tests", false)
				if err != nil {
					t.Fatal(err)
				}
				a.SessionID = login.Session.ID
				tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, a.CredentialEpoch, a.SessionID)
			}
			if mode != "late_conflict" {
				if err := s.Verify(t.Context(), a, op, "old", old); !errors.Is(err, app.ErrProofInvalid) {
					t.Fatal("invalid binding accepted", err)
				}
				return
			}
			if err := s.Verify(t.Context(), a, op, "old", old); err != nil {
				t.Fatal(err)
			}
			if err := s.Verify(t.Context(), a, op, "new", newCode); err != nil {
				t.Fatal(err)
			}
			verifiedTestActor(t, tx, target, now)
			if err := s.Complete(t.Context(), a, op, rev); !errors.Is(err, app.ErrEmailUnavailable) {
				t.Fatal("conflict accepted", err)
			}
			var unchanged bool
			tx.Raw(`SELECT email_key=? AND session_epoch=? FROM identity."user" WHERE id=?`, mode+"-old@example.invalid", a.CredentialEpoch, a.ID).Scan(&unchanged)
			if !unchanged {
				t.Fatal("conflict partially committed")
			}
		})
	}
}
