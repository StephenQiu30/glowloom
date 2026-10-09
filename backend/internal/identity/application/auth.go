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

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidCredentials intentionally hides existence, lock and availability.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrSessionInvalid means authentication state is absent, expired or revoked.
	ErrSessionInvalid = errors.New("invalid session")
	// ErrLoginTaken means the global registration namespace already owns the name.
	ErrLoginTaken = errors.New("login already exists")
	// ErrCurrentPassword means a password change failed current credential verification.
	ErrCurrentPassword = errors.New("current password invalid")
)

// AuthStore owns the transaction boundary for credentials, sessions and events.
type AuthStore interface {
	Transaction(context.Context, func(AuthTx) error) error
	LoginAvailable(context.Context, string) (bool, error)
}

// AuthTx exposes only the persistence operations used by this identity slice.
// Account reads lock the account; the first session lookup is deliberately unlocked.
type AuthTx interface {
	RegistrationOrganization() (uuid.UUID, error)
	CreateAccount(domain.User) error
	AccountByLogin(string) (domain.User, error)
	AccountByID(uuid.UUID) (domain.User, error)
	UpdateAccount(domain.User) error
	Guard(string) (domain.LoginGuard, error)
	SaveGuard(string, domain.LoginGuard) error
	SessionByHash([]byte, bool) (domain.AuthSession, error)
	InsertSession(domain.AuthSession) error
	TouchSession(uuid.UUID, time.Time) error
	RevokeSession(uuid.UUID, uuid.UUID, time.Time) error
	RevokeAccountSessions(uuid.UUID, time.Time) error
	AppendAuthEvent(uuid.UUID, string, time.Time) error
}

// Auth coordinates registration and server-authoritative authentication.
type Auth struct {
	store AuthStore
	now   func() time.Time
}

// NewAuth injects persistence and the server clock.
func NewAuth(store AuthStore, now func() time.Time) *Auth { return &Auth{store: store, now: now} }

// AuthResult keeps transport credentials separate from public DTOs.
type AuthResult struct {
	User    domain.User
	Session domain.AuthSession
	Token   string
}

// Register creates a producer and a session atomically, with no privileged input.
func (a *Auth) Register(ctx context.Context, login, display, password, confirmation string) (AuthResult, error) {
	key, err := domain.NormalizeLogin(login)
	display = strings.TrimSpace(display)
	if err != nil || !utf8.ValidString(display) || utf8.RuneCountInString(display) < 1 || utf8.RuneCountInString(display) > 32 || password != confirmation {
		return AuthResult{}, domain.ErrInvalidProfile
	}
	for _, r := range display {
		if unicode.IsControl(r) {
			return AuthResult{}, domain.ErrInvalidProfile
		}
	}
	hash, err := domain.HashPassword(password, "")
	if err != nil {
		return AuthResult{}, err
	}
	var result AuthResult
	err = a.store.Transaction(ctx, func(tx AuthTx) error {
		org, err := tx.RegistrationOrganization()
		if err != nil {
			return err
		}
		now := a.now().UTC()
		user := domain.User{ID: uuid.New(), OrgID: org, LoginName: key, DisplayName: display, Role: domain.RoleProducer, Status: domain.StatusActive, PasswordHash: hash, SessionEpoch: 1, Revision: 1, PasswordChangedAt: now}
		if err := tx.CreateAccount(user); err != nil {
			return err
		}
		result, err = newAuthSession(tx, user, false, now)
		if err != nil {
			return err
		}
		return tx.AppendAuthEvent(user.ID, "registered", now)
	})
	return result, err
}

const unavailableHash = "$2a$12$yf3ZHVlB9i79td5BSH3Gv.O.0R9lLRkVHKFWANWrD4JaTsx7VGoa2"

// Login commits failed attempts as well as successful sessions under the login lock.
func (a *Auth) Login(ctx context.Context, login, password string, persistent bool) (AuthResult, error) {
	key, err := domain.NormalizeLogin(login)
	if err != nil {
		domain.VerifyPassword(unavailableHash, password)
		return AuthResult{}, ErrInvalidCredentials
	}
	var result AuthResult
	var rejected error
	err = a.store.Transaction(ctx, func(tx AuthTx) error {
		guard, err := tx.Guard(key)
		if err != nil {
			return err
		}
		user, err := tx.AccountByLogin(key)
		if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
			return err
		}
		hash := unavailableHash
		if err == nil {
			hash = user.PasswordHash
		}
		correct := domain.VerifyPassword(hash, password)
		now := a.now().UTC()
		if !correct || err != nil || user.Status != domain.StatusActive || guard.Locked(now) || now.Before(user.LockedUntil) {
			guard.Fail(now)
			if err := tx.SaveGuard(key, guard); err != nil {
				return err
			}
			rejected = ErrInvalidCredentials
			return tx.AppendAuthEvent(user.ID, "login_failed", now)
		}
		if err := tx.SaveGuard(key, domain.LoginGuard{}); err != nil {
			return err
		}
		user.RegisterLoginSuccess(now)
		if err := tx.UpdateAccount(user); err != nil {
			return err
		}
		result, err = newAuthSession(tx, user, persistent, now)
		if err != nil {
			return err
		}
		return tx.AppendAuthEvent(user.ID, "logged_in", now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, rejected
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
		result, err = newAuthSession(tx, user, session.Persistent, now)
		if err != nil {
			return err
		}
		return tx.AppendAuthEvent(user.ID, "password_changed", now)
	})
	return result, err
}

// Available provides registration feedback; the unique constraint remains authoritative.
func (a *Auth) Available(ctx context.Context, login string) (bool, error) {
	key, err := domain.NormalizeLogin(login)
	if err != nil {
		return false, err
	}
	return a.store.LoginAvailable(ctx, key)
}

func (a *Auth) withSession(ctx context.Context, token string, execute func(AuthTx, domain.User, domain.AuthSession, time.Time) error) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ErrSessionInvalid
	}
	digest := sha256.Sum256(raw)
	return a.store.Transaction(ctx, func(tx AuthTx) error {
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
