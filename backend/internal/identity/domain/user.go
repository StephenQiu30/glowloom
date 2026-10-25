// Package domain owns account and password invariants for identity.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	lockFailures = 5
	lockDuration = 15 * time.Minute
)

// DefaultTheme is used for newly created accounts without a saved preference.
const DefaultTheme = "dark"

var (
	// ErrWeakPassword means a password violates the account policy.
	ErrWeakPassword = errors.New("password must contain 8 to 128 Unicode characters")
	// ErrPasswordReused means the new password equals the current password.
	ErrPasswordReused = errors.New("new password must differ from current password")
	// ErrAccountLocked means a temporary login lock has not expired.
	ErrAccountLocked = errors.New("account is temporarily locked")
	// ErrAccountDisabled means the account is unavailable for login or another disable.
	ErrAccountDisabled = errors.New("account is disabled")
	// ErrInvalidAccountState means a mutation would overflow a persisted counter.
	ErrInvalidAccountState = errors.New("invalid account state")
	// ErrUserNotFound means an account is unavailable in the requested organization.
	ErrUserNotFound = errors.New("user not found")
	// ErrRevisionConflict means an account changed after the caller read it.
	ErrRevisionConflict = errors.New("user revision conflict")
	// ErrInvalidProfile means the requested display name, role, or stored state is invalid.
	ErrInvalidProfile = errors.New("invalid account profile")
)

// Role is retained organization-role metadata; current authority uses platform roles and scopes.
type Role string

// Account roles retain the legacy storage vocabulary for existing records.
const (
	RoleAdmin    Role = "admin"
	RoleProducer Role = "producer"
)

// Status is the durable account availability state.
type Status string

// Account statuses distinguish usable from disabled accounts.
const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// User contains the account state needed by authentication and account commands.
// PasswordHash is a versioned credential and must never be logged or returned in an API DTO.
type User struct {
	ID               uuid.UUID
	OrgID            uuid.UUID
	LoginName        string
	Email            string
	EmailVerifiedAt  time.Time
	EmailVersion     int64
	PlatformRole     string
	PlatformEpoch    int64
	AvatarAssetID    *uuid.UUID
	TermsVersion     string
	TermsAcceptedAt  time.Time
	Bio              string
	CanvasBackground string
	Theme            string
	PromptSuffix     string
	DeletedAt        time.Time
	DisplayName      string
	Role             Role
	Status           Status
	PasswordHash     string
	// MustChangePassword is historical storage data and never restricts access.
	MustChangePassword bool
	PasswordChangedAt  time.Time
	FailedLoginCount   int
	LockedUntil        time.Time
	SessionEpoch       int64
	Revision           int64
	LastLoginAt        time.Time
	CreateTime         time.Time
	UpdateTime         time.Time
}

// ValidatePassword checks both the password policy and replacement uniqueness.
func ValidatePassword(password, currentHash string) error {
	if err := ValidateNewPassword(password); err != nil {
		return err
	}
	if currentHash != "" && VerifyPassword(currentHash, password) {
		return ErrPasswordReused
	}
	return nil
}

// CanLogin rejects disabled or currently locked accounts. The returned duration
// is meaningful only with ErrAccountLocked.
func (u User) CanLogin(now time.Time) (time.Duration, error) {
	if u.Status != StatusActive {
		return 0, ErrAccountDisabled
	}
	if remaining := u.LockedUntil.Sub(now); remaining > 0 {
		return remaining, ErrAccountLocked
	}
	return 0, nil
}

// RegisterLoginFailure advances the per-account lock state after a bad password.
// It returns true only when this failure newly locks the account.
func (u *User) RegisterLoginFailure(now time.Time) bool {
	if now.Before(u.LockedUntil) || u.Status != StatusActive {
		return false
	}
	if !u.LockedUntil.IsZero() {
		u.FailedLoginCount = 0
		u.LockedUntil = time.Time{}
	}
	u.FailedLoginCount++
	u.Revision++
	if u.FailedLoginCount >= lockFailures {
		u.LockedUntil = now.Add(lockDuration)
		return true
	}
	return false
}

// RegisterLoginSuccess clears failed-login state after a verified password.
func (u *User) RegisterLoginSuccess(now time.Time) {
	u.FailedLoginCount = 0
	u.LockedUntil = time.Time{}
	u.LastLoginAt = now
	u.Revision++
}

// SessionMatches checks the durable epoch and account availability.
func (u User) SessionMatches(epoch int64) bool {
	return u.Status == StatusActive && epoch > 0 && epoch == u.SessionEpoch
}
