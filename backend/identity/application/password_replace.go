package application

import (
	"encoding/hex"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
)

// replacePassword is the existing local credential mutation, shared by the
// legacy transaction and the source-prepared owner transaction. Its caller has
// already validated policy and holds the account/session locks.
func (a *Auth) replacePassword(tx AuthTx, user domain.User, persistent bool, hash string, now time.Time) (AuthResult, error) {
	if user.SessionEpoch >= math.MaxInt32 || user.Revision >= math.MaxInt32 {
		return AuthResult{}, domain.ErrInvalidAccountState
	}
	user.PasswordHash = hash
	user.SessionEpoch++
	user.Revision++
	user.MustChangePassword = false
	user.PasswordChangedAt = now
	if err := tx.UpdateAccount(user); err != nil {
		return AuthResult{}, err
	}
	if err := tx.RevokeAccountSessions(user.ID, now); err != nil {
		return AuthResult{}, err
	}
	result, err := a.newSession(tx, user, persistent, now)
	if err != nil {
		return AuthResult{}, err
	}
	if err := tx.AppendAuthEvent(user.ID, "password_changed", now); err != nil {
		return AuthResult{}, err
	}
	if err := a.queueEmail(tx, "password_changed", user.Email, map[string]string{}, now, now.Add(24*time.Hour)); err != nil {
		return AuthResult{}, err
	}
	return result, nil
}

// applyResetPassword preserves the original reset mutation and notification in
// one Identity transaction; it never activates an account or creates a session.
func (a *Auth) applyResetPassword(tx EmailTx, user domain.User, hash string, now time.Time) error {
	if user.SessionEpoch >= math.MaxInt32 || user.Revision >= math.MaxInt32 {
		return domain.ErrInvalidAccountState
	}
	user.PasswordHash = hash
	user.SessionEpoch++
	user.Revision++
	user.PasswordChangedAt = now
	user.MustChangePassword = false
	user.FailedLoginCount = 0
	user.LockedUntil = time.Time{}
	if err := tx.UpdateAccount(user); err != nil {
		return err
	}
	if err := tx.RevokeAccountSessions(user.ID, now); err != nil {
		return err
	}
	key := hex.EncodeToString(a.email.protector.Digest("login_guard", uuid.Nil, user.Email))
	if _, err := tx.Guard(key); err != nil {
		return err
	}
	if err := tx.SaveGuard(key, domain.LoginGuard{}); err != nil {
		return err
	}
	if err := tx.AppendAuthEvent(user.ID, "password_changed", now); err != nil {
		return err
	}
	return a.queueEmail(tx, "password_changed", user.Email, map[string]string{}, now, now.Add(24*time.Hour))
}
