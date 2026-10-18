package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

const unavailableArgonHash = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func (a *Auth) loginEmail(ctx context.Context, email, password string, persistent bool, source string) (AuthResult, error) {
	if !a.emailReady() {
		return AuthResult{}, ErrRegistrationUnavailable
	}
	if a.sessionIssuanceEnabled() {
		if !a.sessionSecurityReady() {
			return AuthResult{}, ErrRefreshDependency
		}
		release, err := a.security.coordinator.Admit(ctx, uuid.New(), source)
		if err != nil {
			return AuthResult{}, err
		}
		defer func() { _ = release(ctx) }()
	}
	var result AuthResult
	var rejected error
	err := a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		// The settings row serializes the lock order with credential/governance changes.
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		now := a.now().UTC()
		if err := a.reserveLoginSource(tx, source, now); err != nil {
			if errors.Is(err, ErrRateLimited) {
				rejected = err
				return nil
			}
			return err
		}
		key, err := domain.NormalizeEmail(email)
		if err != nil {
			domain.VerifyPassword(unavailableArgonHash, password)
			rejected = ErrInvalidCredentials
			return tx.AppendAuthEvent(uuid.Nil, "login_failed", now)
		}
		guardKey := hex.EncodeToString(a.email.protector.Digest("login_guard", uuid.Nil, key))
		guard, err := tx.Guard(guardKey)
		if err != nil {
			return err
		}
		u, lookupErr := tx.AccountByEmail(key)
		if lookupErr != nil && !errors.Is(lookupErr, domain.ErrUserNotFound) {
			return lookupErr
		}
		hash := u.PasswordHash
		if lookupErr != nil {
			hash = unavailableArgonHash
		}
		correct := domain.VerifyPassword(hash, password)
		if !correct || lookupErr != nil || u.EmailVerifiedAt.IsZero() || u.Status != domain.StatusActive || guard.Locked(now) || now.Before(u.LockedUntil) {
			guard.Fail(now)
			if err := tx.SaveGuard(guardKey, guard); err != nil {
				return err
			}
			rejected = ErrInvalidCredentials
			return tx.AppendAuthEvent(u.ID, "login_failed", now)
		}
		// New-password rules apply only when the user chooses a new credential.
		// Preserve a valid historical password even when it cannot be rehashed
		// under the current policy; it still creates an ordinary session.
		if domain.PasswordNeedsUpgrade(hash) && domain.ValidateNewPassword(password) == nil {
			upgraded, err := domain.HashPassword(password, "")
			if err != nil {
				return err
			}
			u.PasswordHash = upgraded
		}
		if err := tx.SaveGuard(guardKey, domain.LoginGuard{}); err != nil {
			return err
		}
		u.RegisterLoginSuccess(now)
		if err := tx.UpdateAccount(u); err != nil {
			return err
		}
		result, err = a.newSession(tx, u, persistent, now)
		if err != nil {
			return err
		}
		return tx.AppendAuthEvent(u.ID, "logged_in", now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, rejected
}

// reserveLoginSource uses a distinct persistent bucket, independent of email
// failure guards and SMTP quotas. FailedAt stores all admitted login attempts
// here, including successful, unknown-account, and malformed-email attempts.
func (a *Auth) reserveLoginSource(tx EmailTx, source string, now time.Time) error {
	const limit = 100
	const window = 15 * time.Minute
	key := "login_source/" + hex.EncodeToString(a.email.protector.Digest("login_source", uuid.Nil, source))
	budget, err := tx.Budget(key)
	if err != nil {
		return err
	}
	recent := budget.FailedAt[:0]
	for _, attempt := range budget.FailedAt {
		if attempt.After(now.Add(-window)) {
			recent = append(recent, attempt)
		}
	}
	budget.FailedAt = recent
	if len(recent) >= limit {
		return &RateLimit{RetryAfter: recent[0].Add(window).Sub(now)}
	}
	budget.FailedAt = append(budget.FailedAt, now)
	return tx.SaveBudget(key, budget)
}

// StartPasswordReset always accepts the same shape; only verified accounts get
// a recoverable proof. Unknown addresses receive no account existence detail.
func (a *Auth) StartPasswordReset(ctx context.Context, email, source string) (RegistrationStart, error) {
	if !a.mailReady() {
		return RegistrationStart{}, ErrRegistrationUnavailable
	}
	key, err := domain.NormalizeEmail(email)
	if err != nil {
		return RegistrationStart{}, err
	}
	id := uuid.New()
	err = a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		now := a.now().UTC()
		if err := a.reserveEmail(tx, key, source, now); err != nil {
			return err
		}
		u, err := tx.AccountByEmail(key)
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if u.EmailVerifiedAt.IsZero() || (u.Status != domain.StatusActive && u.Status != domain.StatusDisabled) {
			return nil
		}
		return a.issuePasswordReset(tx, u, id, now)
	})
	if err != nil {
		return RegistrationStart{}, err
	}
	return RegistrationStart{OperationID: id, RetryAfterSeconds: 60}, nil
}

// CompletePasswordReset consumes a bound proof and revokes all old sessions atomically.
func (a *Auth) CompletePasswordReset(ctx context.Context, token, password, confirmation string) error {
	if !a.emailReady() {
		return ErrRegistrationUnavailable
	}
	idText, secret, ok := strings.Cut(token, ".")
	id, err := uuid.Parse(idText)
	if !ok || err != nil || len(secret) != 43 {
		return ErrProofInvalid
	}
	if password != confirmation {
		return domain.ErrInvalidProfile
	}
	if err := a.validateNewPassword(password); err != nil {
		return err
	}
	return a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		c, err := tx.Challenge(id, "password_reset")
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if !c.Available(now) || c.UserID == nil || c.CredentialEpoch == nil || c.EmailVersion == nil || c.ProofKeyID != a.email.protector.KeyID() || !hmac.Equal(c.ProofDigest, a.email.protector.Digest(c.Purpose, c.ID, secret)) {
			return ErrProofInvalid
		}
		u, err := tx.AccountByID(*c.UserID)
		if err != nil {
			return err
		}
		if u.SessionEpoch != *c.CredentialEpoch || u.EmailVersion != *c.EmailVersion || u.Email != c.EmailKey || (u.Status != domain.StatusActive && u.Status != domain.StatusDisabled) {
			return ErrProofInvalid
		}
		if u.SessionEpoch >= math.MaxInt32 || u.Revision >= math.MaxInt32 {
			return domain.ErrInvalidAccountState
		}
		hash, err := domain.HashPassword(password, u.PasswordHash)
		if err != nil {
			return err
		}
		u.PasswordHash = hash
		u.SessionEpoch++
		u.Revision++
		u.PasswordChangedAt = now
		u.MustChangePassword = false
		u.FailedLoginCount = 0
		u.LockedUntil = time.Time{}
		if err := tx.UpdateAccount(u); err != nil {
			return err
		}
		if err := tx.RevokeAccountSessions(u.ID, now); err != nil {
			return err
		}
		guardKey := hex.EncodeToString(a.email.protector.Digest("login_guard", uuid.Nil, u.Email))
		if _, err := tx.Guard(guardKey); err != nil {
			return err
		}
		if err := tx.SaveGuard(guardKey, domain.LoginGuard{}); err != nil {
			return err
		}
		c.ConsumedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		if err := tx.AppendAuthEvent(u.ID, "password_changed", now); err != nil {
			return err
		}
		return a.queueEmail(tx, "password_changed", u.Email, map[string]string{}, now, now.Add(24*time.Hour))
	})
}

func (a *Auth) issuePasswordReset(tx EmailTx, u domain.User, id uuid.UUID, now time.Time) error {
	secret, err := newProofToken()
	if err != nil {
		return err
	}
	c := domain.EmailChallenge{ID: uuid.New(), OperationID: id, Purpose: "password_reset", EmailKey: u.Email, UserID: &u.ID, EmailVersion: &u.EmailVersion, CredentialEpoch: &u.SessionEpoch, ProofKeyID: a.email.protector.KeyID(), CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	c.ProofDigest = a.email.protector.Digest(c.Purpose, c.ID, secret)
	if err := tx.InsertChallenge(c); err != nil {
		return err
	}
	return a.queueEmail(tx, "password_reset", u.Email, map[string]string{"token": id.String() + "." + secret}, now, c.ExpiresAt, c.ID)
}
