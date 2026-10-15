package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

var (
	// ErrInvalidCredentials intentionally hides existence, lock and availability.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrSessionInvalid means authentication state is absent, expired or revoked.
	ErrSessionInvalid = errors.New("invalid session")
	// ErrCurrentPassword means a password change failed current credential verification.
	ErrCurrentPassword = errors.New("current password invalid")
)

// AuthTx exposes only the persistence operations used by this identity slice.
// Account reads lock the account; the first session lookup is deliberately unlocked.
type AuthTx interface {
	AccountByID(uuid.UUID) (domain.User, error)
	UpdateAccount(domain.User) error
	Guard(string) (domain.LoginGuard, error)
	SaveGuard(string, domain.LoginGuard) error
	SessionByHash([]byte, bool) (domain.AuthSession, error)
	InsertSession(domain.AuthSession) error
	SetSessionSecurity(uuid.UUID, []byte) error
	ReauthenticateSession(uuid.UUID, int64, time.Time) error
	TouchSession(uuid.UUID, time.Time) error
	RevokeSession(uuid.UUID, uuid.UUID, time.Time) error
	RevokeAccountSessions(uuid.UUID, time.Time) error
	AppendAuthEvent(uuid.UUID, string, time.Time) error
}

// Auth coordinates registration and server-authoritative authentication.
type Auth struct {
	now   func() time.Time
	email *emailRuntime
}

// AuthResult keeps transport credentials separate from public DTOs.
type AuthResult struct {
	User      domain.User
	Session   domain.AuthSession
	Token     string
	CSRFToken string
}

// Login verifies the existing email credential and creates an ordinary session.
func (a *Auth) Login(ctx context.Context, email, password string, persistent bool) (AuthResult, error) {
 return a.loginEmail(ctx, email, password, persistent)
}

// Current validates durable state and advances last activity under account/session locks.
func (a *Auth) Current(ctx context.Context, token string) (AuthResult, error) {
	var result AuthResult
	err := a.withSession(ctx, token, func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if err := tx.TouchSession(session.ID, now); err != nil {
			return err
		}
		session.LastActiveAt = now
		result = AuthResult{User: user, Session: session}
		if a.email != nil {
			result.CSRFToken = a.csrfToken(token, session.ID)
		}
		return nil
	})
	return result, err
}

// Logout revokes only the supplied current session; stale tokens are idempotent.
func (a *Auth) Logout(ctx context.Context, token string, id uuid.UUID) error {
	err := a.withSession(ctx, token, func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if session.ID != id {
			return ErrForbidden
		}
		if err := tx.RevokeSession(user.ID, id, now); err != nil {
			return err
		}
		return tx.AppendAuthEvent(user.ID, "logged_out", now)
	})
	if errors.Is(err, ErrSessionInvalid) {
		return nil
	}
	return err
}

// ChangePassword rotates this device and revokes all previous credential sessions.
func (a *Auth) ChangePassword(ctx context.Context, token string, revision int64, current, password, confirmation string) (AuthResult, error) {
	if password != confirmation {
		return AuthResult{}, domain.ErrInvalidProfile
	}
	if a.email != nil {
		if err := a.validateNewPassword(password); err != nil {
			return AuthResult{}, err
		}
	}
	var result AuthResult
	err := a.withSession(ctx, token, func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if revision != user.SessionEpoch {
			return domain.ErrRevisionConflict
		}
		if !domain.VerifyPassword(user.PasswordHash, current) {
			return ErrCurrentPassword
		}
		hash, err := domain.HashPassword(password, user.PasswordHash)
		if err != nil {
			return err
		}
		if user.SessionEpoch >= math.MaxInt32 || user.Revision >= math.MaxInt32 {
			return domain.ErrInvalidAccountState
		}
		user.PasswordHash = hash
		user.SessionEpoch++
		user.Revision++
		user.MustChangePassword = false
		user.PasswordChangedAt = now
		if err := tx.UpdateAccount(user); err != nil {
			return err
		}
		if err := tx.RevokeAccountSessions(user.ID, now); err != nil {
			return err
		}
		result, err = a.newSession(tx, user, session.Persistent, now)
		if err != nil {
			return err
		}
		return tx.AppendAuthEvent(user.ID, "password_changed", now)
	})
	return result, err
}

func (a *Auth) withSession(ctx context.Context, token string, execute func(AuthTx, domain.User, domain.AuthSession, time.Time) error) error {
	if a.email != nil && !a.emailReady() {
		return ErrSessionInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ErrSessionInvalid
	}
	digest := sha256.Sum256(raw)
	return a.sessionTransaction(ctx, func(tx AuthTx) error {
		session, err := tx.SessionByHash(digest[:], false)
		if err != nil {
			return err
		}
		user, err := tx.AccountByID(session.AccountID)
		if errors.Is(err, domain.ErrUserNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		session, err = tx.SessionByHash(digest[:], true)
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if !session.Valid(user, now) {
			return ErrSessionInvalid
		}
		if a.email != nil && (user.EmailVerifiedAt.IsZero() || !a.sessionProofValid(token, session)) {
			return ErrSessionInvalid
		}
		return execute(tx, user, session, now)
	})
}

func newAuthSession(tx AuthTx, user domain.User, persistent bool, now time.Time) (AuthResult, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return AuthResult{}, fmt.Errorf("generate session: %w", err)
	}
	digest := sha256.Sum256(raw)
	session := domain.AuthSession{ID: uuid.New(), AccountID: user.ID, TokenHash: digest[:], CredentialRevision: user.SessionEpoch, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour), Persistent: persistent}
	if err := tx.InsertSession(session); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user, Session: session, Token: base64.RawURLEncoding.EncodeToString(raw)}, nil
}
