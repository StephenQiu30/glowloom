package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"time"

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
	SessionByID(uuid.UUID, bool) (domain.AuthSession, error)
	FindRefresh([]byte, bool) (domain.RefreshToken, error)
	RefreshByGeneration(uuid.UUID, int64, bool) (domain.RefreshToken, error)
	InsertRefresh(domain.RefreshToken) error
	RotateRefresh(domain.RefreshToken, domain.RefreshToken, uuid.UUID, time.Time, []byte) error
	RevokeRefreshChain(uuid.UUID, time.Time) error
	AppendSessionSecurityEvent(uuid.UUID, uuid.UUID, uuid.UUID, string, string, time.Time) error
	CleanupExpiredRefresh(time.Time, int) (int, error)
	InsertSession(domain.AuthSession) error
	SetSessionSecurity(uuid.UUID, []byte) error
	UpgradeLegacySession(uuid.UUID, uuid.UUID, []byte) error
	ReauthenticateSession(uuid.UUID, int64, time.Time) error
	TouchSession(uuid.UUID, time.Time) error
	RevokeSession(uuid.UUID, uuid.UUID, time.Time) error
	RevokeAccountSessions(uuid.UUID, time.Time) error
	AppendAuthEvent(uuid.UUID, string, time.Time) error
	EnqueueMail(QueuedMail) error
}

// Auth coordinates registration and server-authoritative authentication.
type Auth struct {
	now                func() time.Time
	email              *emailRuntime
	security           *sessionSecurity
	legacyUpgradeUntil time.Time
}

// AuthResult keeps transport credentials separate from public DTOs.
type AuthResult struct {
	User                                        domain.User
	Session                                     domain.AuthSession
	Token                                       string `json:"-"`
	CSRFToken                                   string `json:"-"`
	AccessToken                                 string `json:"-"`
	RefreshToken                                string `json:"-"`
	AccessExpiresAt, RefreshAfterAt, ServerTime time.Time
}

// Login verifies the existing email credential and creates an ordinary session.
func (a *Auth) Login(ctx context.Context, email, password string, persistent bool) (AuthResult, error) {
	return a.LoginFrom(ctx, email, password, persistent, "unknown")
}

// LoginFrom verifies a credential with a trusted transport peer source budget.
// Callers without a transport peer share the bounded unknown source bucket.
func (a *Auth) LoginFrom(ctx context.Context, email, password string, persistent bool, source string) (AuthResult, error) {
	if source == "" {
		source = "unknown"
	}
	return a.loginEmail(ctx, email, password, persistent, source)
}

// Current preserves the existing user-activity behavior for legacy callers.
func (a *Auth) Current(ctx context.Context, token string) (AuthResult, error) {
	return a.CurrentWithActivity(ctx, token, UserActivity)
}

// CurrentWithActivity validates durable state using a transport-selected activity.
// The transport must explicitly classify background traffic as NoActivity.
func (a *Auth) CurrentWithActivity(ctx context.Context, token string, activity SessionActivity) (AuthResult, error) {
	var result AuthResult
	err := a.withSession(ctx, token, func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if activity == UserActivity {
			if err := tx.TouchSession(session.ID, now); err != nil {
				return err
			}
			session.LastActiveAt = now
		}
		result = AuthResult{User: user, Session: session, ServerTime: now}
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
	var rejected error
	err := a.withSession(ctx, token, func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if revision != user.SessionEpoch {
			return domain.ErrRevisionConflict
		}
		verified, err := a.verifyCurrentPassword(tx, user, current, now)
		if err != nil {
			return err
		}
		if !verified {
			rejected = ErrCurrentPassword
			return nil
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
		if err := tx.AppendAuthEvent(user.ID, "password_changed", now); err != nil {
			return err
		}
		return a.queueEmail(tx, "password_changed", user.Email, map[string]string{}, now, now.Add(24*time.Hour))
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, rejected
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
		if !a.legacyWindowOpen(now) || session.ProtocolVersion == 2 || !session.Valid(user, now) {
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
	session := domain.AuthSession{ID: uuid.New(), AccountID: user.ID, TokenHash: digest[:], CredentialRevision: user.SessionEpoch, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour), Persistent: persistent, ProtocolVersion: 1, IdleTimeoutSeconds: 43200}
	if err := tx.InsertSession(session); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user, Session: session, Token: base64.RawURLEncoding.EncodeToString(raw), ServerTime: now}, nil
}
