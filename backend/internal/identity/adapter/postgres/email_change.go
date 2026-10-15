package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// EmailChangeStore serializes address reservation with all other identity writes.
type EmailChangeStore struct{ db *gorm.DB }

// NewEmailChangeStore binds account operations to the supplied transaction boundary.
func NewEmailChangeStore(db *gorm.DB) *EmailChangeStore { return &EmailChangeStore{db: db} }

type emailChangeTx struct {
	*emailAuthTx
	user    domain.User
	session domain.AuthSession
}

func (t *emailChangeTx) Account() domain.User        { return t.user }
func (t *emailChangeTx) Session() domain.AuthSession { return t.session }

// EmailChangeTransaction follows settings -> account -> session -> proof lock order.
func (s *EmailChangeStore) EmailChangeTransaction(ctx context.Context, a application.Principal, now time.Time, run func(application.EmailChangeTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		base := &emailAuthTx{authTx: &authTx{db: tx}}
		if _, err := base.RegistrationEnabled(); err != nil {
			return err
		}
		u, err := lockSelfActor(tx, a, now)
		if err != nil {
			return err
		}
		var session domain.AuthSession
		if err := tx.Raw(`SELECT * FROM identity.user_session WHERE id=?`, a.SessionID).Scan(&session).Error; err != nil {
			return err
		}
		return run(&emailChangeTx{emailAuthTx: base, user: u, session: session})
	})
}
func (t *emailChangeTx) ProofSubmitted(id uuid.UUID) (bool, error) {
	var ok bool
	err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.mail_delivery WHERE challenge_id=? AND status IN ('submitted','delivered'))`, id).Scan(&ok).Error
	return ok, err
}
func (t *emailChangeTx) RevokeEmailChanges(now time.Time) error {
	return t.db.Exec(`UPDATE identity.auth_challenge SET revoked_at=? WHERE user_id=? AND purpose IN ('email_old','email_new') AND revoked_at IS NULL`, now, t.user.ID).Error
}
func (t *emailChangeTx) ChangeEmail(email string, revision int64, now time.Time) error {
	r := t.db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=?,email_version=email_version+1,session_epoch=session_epoch+1,revision=revision+1,update_time=? WHERE id=? AND revision=? AND NOT is_delete`, email, now, now, t.user.ID, revision)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	if err := t.RevokeAccountSessions(t.user.ID, now); err != nil {
		return err
	}
	if err := t.db.Exec(`UPDATE identity.auth_challenge SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, now, t.user.ID).Error; err != nil {
		return err
	}
	return appendSelfEvent(t.db, t.user.ID, "email.changed", now)
}
