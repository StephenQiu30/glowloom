package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func (a *Auth) sessionTransaction(ctx context.Context, run func(AuthTx) error) error {
	return a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		return run(tx)
	})
}

func (a *Auth) csrfToken(token string, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(a.email.protector.Digest("session_csrf", id, token))
}

func (a *Auth) sessionProofValid(token string, s domain.AuthSession) bool {
	digest := sha256.Sum256([]byte(a.csrfToken(token, s.ID)))
	return hmac.Equal(s.CSRFHash, digest[:])
}

func (a *Auth) newSession(tx AuthTx, u domain.User, persistent bool, now time.Time) (AuthResult, error) {
	result, err := newAuthSession(tx, u, persistent, now)
	if err != nil || a.email == nil {
		return result, err
	}
	result.CSRFToken = a.csrfToken(result.Token, result.Session.ID)
	digest := sha256.Sum256([]byte(result.CSRFToken))
	result.Session.CSRFHash = digest[:]
	if err := tx.SetSessionSecurity(result.Session.ID, digest[:]); err != nil {
		return AuthResult{}, err
	}
	return result, nil
}

// Reauthenticate proves this credential on this session with the shared account guard.
func (a *Auth) Reauthenticate(ctx context.Context, token, password string) error {
	if !a.emailReady() {
		return ErrRegistrationUnavailable
	}
	var rejected error
	err := a.withSession(ctx, token, func(tx AuthTx, u domain.User, s domain.AuthSession, now time.Time) error {
		verified, err := a.verifyCurrentPassword(tx, u, password, now)
		if err != nil {
			return err
		}
		if !verified {
			rejected = ErrInvalidCredentials
			return nil
		}
		return tx.ReauthenticateSession(s.ID, u.SessionEpoch, now)
	})
	if err != nil {
		return err
	}
	return rejected
}

// verifyCurrentPassword shares one account guard across all password proof
// endpoints. A false result must commit this transaction before rejection is
// returned to the caller, so failures and their audit cannot be rolled back.
func (a *Auth) verifyCurrentPassword(tx AuthTx, u domain.User, password string, now time.Time) (bool, error) {
	key := hex.EncodeToString(a.email.protector.Digest("reauth_guard", u.ID, ""))
	guard, err := tx.Guard(key)
	if err != nil {
		return false, err
	}
	if guard.Locked(now) {
		return false, tx.AppendAuthEvent(u.ID, "login_failed", now)
	}
	if !domain.VerifyPassword(u.PasswordHash, password) {
		guard.Fail(now)
		if err := tx.SaveGuard(key, guard); err != nil {
			return false, err
		}
		return false, tx.AppendAuthEvent(u.ID, "login_failed", now)
	}
	if err := tx.SaveGuard(key, domain.LoginGuard{}); err != nil {
		return false, err
	}
	return true, nil
}
