package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// EmailTransaction commits proofs, accounts and mail together.
func (s *Store) EmailTransaction(ctx context.Context, run func(application.EmailTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return run(&emailAuthTx{authTx: &authTx{db: tx, invitationTeams: s.invitationTeams}})
	})
}

// emailAuthTx writes the current security event contract. Legacy routes retain
// their old event adapter until the coordinated public-router cutover.
type emailAuthTx struct{ *authTx }

func (t *emailAuthTx) AppendAuthEvent(account uuid.UUID, action string, now time.Time) error {
	var target, actor *uuid.UUID
	if account != uuid.Nil {
		target = &account
		actor = &account
	}
	result := "success"
	switch action {
	case "registered":
		action = "account.registered"
	case "logged_in":
		action = "session.created"
	case "logged_out":
		action = "session.revoked"
	case "password_changed":
		action = "password.changed"
	case "login_failed":
		action = "authentication.failed"
		result = "failure"
		actor = nil
	default:
		return fmt.Errorf("unsupported email authentication event")
	}
	if err := t.db.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,created_at) VALUES(?,?,?,'self',?,?,?)`, uuid.New(), actor, target, action, result, now).Error; err != nil {
		return fmt.Errorf("append email security event: %w", err)
	}
	return nil
}

// RegistrationEnabled takes the governance lock before accounts and proofs.
func (t *authTx) RegistrationEnabled() (bool, error) {
	var enabled bool
	r := t.db.Raw(`SELECT registration_enabled FROM identity.platform_settings WHERE singleton=true FOR UPDATE`).Scan(&enabled)
	if r.Error != nil {
		return false, fmt.Errorf("lock registration settings: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return false, application.ErrRegistrationUnavailable
	}
	return enabled, nil
}

func (t *authTx) AccountByEmail(email string) (domain.User, error) {
	return t.account(`email_key=?`, email)
}

// EmailReserved includes deleted identities; deletion never releases the mailbox.
func (t *authTx) EmailReserved(email string) (bool, error) {
	var reserved bool
	err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" WHERE email_key=?)`, email).Scan(&reserved).Error
	return reserved, err
}

func (t *authTx) Challenge(operation uuid.UUID, purpose string) (domain.EmailChallenge, error) {
	var c domain.EmailChallenge
	r := t.db.Raw(`SELECT * FROM identity.auth_challenge WHERE operation_id=? AND purpose=? ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, operation, purpose).Scan(&c)
	if r.Error != nil {
		return c, fmt.Errorf("lock email proof: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return c, application.ErrProofInvalid
	}
	return c, nil
}

func (t *authTx) InsertChallenge(c domain.EmailChallenge) error {
	if err := t.db.Exec(`INSERT INTO identity.auth_challenge(id,operation_id,purpose,email_key,user_id,email_version,credential_epoch,proof_digest,proof_key_id,flow_secret_digest,attempts,expires_at,created_at,invitation_id,invitation_revision)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.OperationID, c.Purpose, c.EmailKey, c.UserID, c.EmailVersion, c.CredentialEpoch, c.ProofDigest, c.ProofKeyID, c.FlowSecretDigest, c.Attempts, c.ExpiresAt, c.CreatedAt, c.InvitationID, c.InvitationRevision).Error; err != nil {
		return fmt.Errorf("insert email proof: %w", err)
	}
	return nil
}

func (t *authTx) SaveChallenge(c domain.EmailChallenge) error {
	r := t.db.Exec(`UPDATE identity.auth_challenge SET attempts=?,consumed_at=?,revoked_at=? WHERE id=?`, c.Attempts, c.ConsumedAt, c.RevokedAt, c.ID)
	if r.Error != nil {
		return fmt.Errorf("update email proof: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrProofInvalid
	}
	return nil
}

func (t *authTx) Budget(key string) (domain.EmailBudget, error) {
	if err := t.db.Exec(`INSERT INTO identity.email_budget(bucket_key) VALUES(?) ON CONFLICT DO NOTHING`, key).Error; err != nil {
		return domain.EmailBudget{}, fmt.Errorf("ensure email budget: %w", err)
	}
	var row struct{ Sent, Failed string }
	if err := t.db.Raw(`SELECT array_to_json(sent_at)::text AS sent,array_to_json(failed_at)::text AS failed FROM identity.email_budget WHERE bucket_key=? FOR UPDATE`, key).Scan(&row).Error; err != nil {
		return domain.EmailBudget{}, fmt.Errorf("lock email budget: %w", err)
	}
	var b domain.EmailBudget
	if err := json.Unmarshal([]byte(row.Sent), &b.SentAt); err != nil {
		return b, err
	}
	if err := json.Unmarshal([]byte(row.Failed), &b.FailedAt); err != nil {
		return b, err
	}
	return b, nil
}

func (t *authTx) SaveBudget(key string, b domain.EmailBudget) error {
	sent, err := json.Marshal(b.SentAt)
	if err != nil {
		return err
	}
	failed, err := json.Marshal(b.FailedAt)
	if err != nil {
		return err
	}
	if err := t.db.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY(SELECT value::timestamptz FROM jsonb_array_elements_text(?::jsonb)),failed_at=ARRAY(SELECT value::timestamptz FROM jsonb_array_elements_text(?::jsonb)),revision=revision+1 WHERE bucket_key=?`, string(sent), string(failed), key).Error; err != nil {
		return fmt.Errorf("save email budget: %w", err)
	}
	return nil
}

func (t *authTx) CreatePersonalAccount(u domain.User) error {
	if err := t.db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'个人创作空间')`, u.OrgID).Error; err != nil {
		return fmt.Errorf("insert personal storage scope: %w", err)
	}
	if err := t.db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,email_key,email_verified_at,email_version,display_name,role,platform_role,platform_epoch,status,password_hash,must_change_password,password_changed_at,session_epoch,revision,terms_version,terms_accepted_at,theme)
	VALUES(?,?,?,?,?,?,?,'producer','creator',1,'active',?,false,?,1,1,?,?,'system')`, u.ID, u.OrgID, u.LoginName, u.Email, u.EmailVerifiedAt, u.EmailVersion, u.DisplayName, u.PasswordHash, u.PasswordChangedAt, u.TermsVersion, u.TermsAcceptedAt).Error; err != nil {
		return fmt.Errorf("insert verified identity: %w", err)
	}
	if err := t.db.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, uuid.New(), u.ID, u.OrgID).Error; err != nil {
		return fmt.Errorf("insert personal creation scope: %w", err)
	}
	return nil
}

func (t *authTx) EnqueueMail(m application.QueuedMail) error {
	if err := t.db.Exec(`INSERT INTO identity.mail_delivery(event_id,challenge_id,template,recipient,encrypted_payload,key_id,next_attempt_at,expires_at,created_at,updated_at,invitation_id,invitation_revision)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, m.EventID, m.ChallengeID, m.Template, m.Recipient, m.EncryptedPayload, m.KeyID, m.CreatedAt, m.ExpiresAt, m.CreatedAt, m.CreatedAt, m.InvitationID, m.InvitationRevision).Error; err != nil {
		return fmt.Errorf("enqueue identity mail: %w", err)
	}
	return nil
}

var _ application.EmailTx = (*authTx)(nil)

func (t *authTx) HasPlatformAdministrator() (bool, error) {
	var exists bool
	err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" WHERE platform_role='platform_admin' AND status='active' AND email_verified_at IS NOT NULL AND NOT is_delete)`).Scan(&exists).Error
	return exists, err
}

func (t *authTx) BindVerifiedAccount(u domain.User) error {
	r := t.db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=?,platform_role=?,platform_epoch=platform_epoch+1,session_epoch=?,revision=?,update_time=? WHERE id=? AND email_key IS NULL AND status='active' AND revision=?`, u.Email, u.EmailVerifiedAt, u.PlatformRole, u.SessionEpoch, u.Revision, u.EmailVerifiedAt, u.ID, u.Revision-1)
	if r.Error != nil {
		return fmt.Errorf("bind verified legacy account: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	if err := t.db.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, uuid.New(), u.ID, u.OrgID).Error; err != nil {
		return fmt.Errorf("create migrated personal scope: %w", err)
	}
	return t.db.Exec(`INSERT INTO identity.security_event(id,target_user_id,scope_kind,action,result,reason,safe_diff,created_at) VALUES(?,?,'platform','account.migrated','success','operator verified legacy email',jsonb_build_object('platform_role',?::text),?)`, uuid.New(), u.ID, u.PlatformRole, u.EmailVerifiedAt).Error
}
