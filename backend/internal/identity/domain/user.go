// Package domain owns account and password invariants for identity.
package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	passwordCost = 12
	lockFailures = 5
	lockDuration = 15 * time.Minute
)

var (
	// ErrWeakPassword means a password violates the account policy.
	ErrWeakPassword = errors.New("password must have at least 10 characters, a letter, and a digit, and fit bcrypt's 72-byte limit")
	// ErrPasswordReused means the new password equals the current password.
	ErrPasswordReused = errors.New("new password must differ from current password")
	// ErrAccountLocked means a temporary login lock has not expired.
	ErrAccountLocked = errors.New("account is temporarily locked")
	// ErrAccountDisabled means the account is unavailable for login or another disable.
	ErrAccountDisabled = errors.New("account is disabled")
	// ErrAccountActive means an account cannot be enabled again.
	ErrAccountActive = errors.New("account is already active")
	// ErrInvalidAccountState means a mutation would overflow a persisted counter.
	ErrInvalidAccountState = errors.New("invalid account state")
	// ErrLastActiveAdmin means disabling this account would leave no active administrator.
	ErrLastActiveAdmin = errors.New("cannot disable the last active administrator")
	// ErrUserNotFound means an account is unavailable in the requested organization.
	ErrUserNotFound = errors.New("user not found")
	// ErrRevisionConflict means an account changed after the caller read it.
	ErrRevisionConflict = errors.New("user revision conflict")
)

// Role is the account's organization-level permission tier.
type Role string

// Account roles are the two permission tiers in the MVP.
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
// PasswordHash is a bcrypt hash and must never be logged or returned in an API DTO.
type User struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               Role
	Status             Status
	PasswordHash       string
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
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 10 || len(password) > 72 {
		return ErrWeakPassword
	}
	var letter, digit bool
	for _, r := range password {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	if !letter || !digit {
		return ErrWeakPassword
	}
	if currentHash != "" && VerifyPassword(currentHash, password) {
		return ErrPasswordReused
	}
	return nil
}

// HashPassword validates and hashes a new password at the accepted bcrypt cost.
func HashPassword(password, currentHash string) (string, error) {
	if err := ValidatePassword(password, currentHash); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword compares a candidate without exposing the password or hash.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ValidPasswordHash reports whether a stored hash uses the required bcrypt cost.
func ValidPasswordHash(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost == passwordCost
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

// Disable invalidates all sessions while preserving the last active admin.
// The caller must count active administrators under a database lock.
func (u *User) Disable(activeAdmins int) error {
	if u.Status != StatusActive {
		return ErrAccountDisabled
	}
	if u.Role == RoleAdmin && activeAdmins <= 1 {
		return ErrLastActiveAdmin
	}
	u.Status = StatusDisabled
	u.SessionEpoch++
	u.Revision++
	return nil
}

// Enable restores an account and invalidates every session from before disable.
func (u *User) Enable() error {
	if u.Status == StatusActive {
		return ErrAccountActive
	}
	if u.Status != StatusDisabled || u.SessionEpoch < 1 || u.SessionEpoch >= math.MaxInt32 ||
		u.Revision < 1 || u.Revision >= math.MaxInt32 {
		return ErrInvalidAccountState
	}
	u.Status = StatusActive
	u.FailedLoginCount = 0
	u.LockedUntil = time.Time{}
	u.SessionEpoch++
	u.Revision++
	return nil
}

// ResetPassword replaces a credential, requires a first-login change, and
// invalidates every existing session. Disabled accounts remain disabled.
func (u *User) ResetPassword(password string) error {
	if (u.Status != StatusActive && u.Status != StatusDisabled) ||
		!ValidPasswordHash(u.PasswordHash) || u.SessionEpoch < 1 ||
		u.SessionEpoch >= math.MaxInt32 || u.Revision < 1 || u.Revision >= math.MaxInt32 {
		return ErrInvalidAccountState
	}
	hash, err := HashPassword(password, u.PasswordHash)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.PasswordChangedAt = time.Time{}
	u.FailedLoginCount = 0
	u.LockedUntil = time.Time{}
	u.SessionEpoch++
	u.Revision++
	return nil
}

// SessionMatches checks the durable epoch and account availability.
func (u User) SessionMatches(epoch int64) bool {
	return u.Status == StatusActive && epoch > 0 && epoch == u.SessionEpoch
}
