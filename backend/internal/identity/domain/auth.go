package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// NormalizeLogin validates the accepted ASCII alphabet and returns the global key.
func NormalizeLogin(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 64 {
		return "", ErrInvalidProfile
	}
	for _, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return "", ErrInvalidProfile
	}
	return strings.ToLower(value), nil
}

// LoginGuard retains only failures in the rolling window for one normalized login.
type LoginGuard struct {
	Failures    []time.Time
	LockedUntil time.Time
}

// Locked checks the exclusive end of the temporary lock.
func (g LoginGuard) Locked(now time.Time) bool { return now.Before(g.LockedUntil) }

// Fail records an allowed attempt without extending an existing lock.
func (g *LoginGuard) Fail(now time.Time) {
	if g.Locked(now) {
		return
	}
	if !g.LockedUntil.IsZero() {
		g.Failures = nil
		g.LockedUntil = time.Time{}
	}
	recent := g.Failures[:0]
	for _, failure := range g.Failures {
		if failure.After(now.Add(-lockDuration)) {
			recent = append(recent, failure)
		}
	}
	g.Failures = recent
	g.Failures = append(g.Failures, now)
	if len(g.Failures) >= lockFailures {
		g.LockedUntil = now.Add(lockDuration)
	}
}

// AuthSession is durable authentication state; its token digest is never public.
type AuthSession struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	TokenHash          []byte
	CredentialRevision int64
	CreatedAt          time.Time
	LastActiveAt       time.Time
	AbsoluteExpiresAt  time.Time
	Persistent         bool
	ProtocolVersion    int16
	IdleTimeoutSeconds int32
	RefreshGeneration  int64
	RevokedAt          *time.Time
	CSRFHash           []byte
	DeviceSummary      string
	ReauthAt           *time.Time
	ReauthEpoch        *int64
}

// RecentlyAuthenticated binds the five-minute proof to this credential epoch.
func (s AuthSession) RecentlyAuthenticated(user User, now time.Time) bool {
	return s.Valid(user, now) && s.ReauthAt != nil && s.ReauthEpoch != nil && *s.ReauthEpoch == user.SessionEpoch && !now.Before(*s.ReauthAt) && now.Before(s.ReauthAt.Add(5*time.Minute))
}

// IdleExpiresAt returns the canonical stored-protocol idle deadline.
func (s AuthSession) IdleExpiresAt() time.Time {
	idleTTL := 12 * time.Hour
	if s.ProtocolVersion == 2 {
		idleTTL = time.Duration(s.IdleTimeoutSeconds) * time.Second
	}
	return s.LastActiveAt.Add(idleTTL)
}

// Valid enforces account revocation and both session deadlines at the exact boundary.
func (s AuthSession) Valid(user User, now time.Time) bool {
	if s.ProtocolVersion == 2 {
		if s.IdleTimeoutSeconds <= 0 {
			return false
		}
	} else if s.ProtocolVersion != 0 && s.ProtocolVersion != 1 {
		return false
	}
	return s.RevokedAt == nil && user.SessionMatches(s.CredentialRevision) &&
		!now.Before(s.CreatedAt) && !now.Before(s.LastActiveAt) && now.Before(s.IdleExpiresAt()) && now.Before(s.AbsoluteExpiresAt)
}
