// Package application coordinates account and session checks for identity use cases.
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrUnauthenticated means the token does not identify an active, current account.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrAuthenticationUnavailable means a required session or account store failed.
	ErrAuthenticationUnavailable = errors.New("authentication unavailable")
)

// AccountReader reads the current durable account state within an organization.
type AccountReader interface {
	FindByID(context.Context, uuid.UUID, uuid.UUID) (domain.User, error)
}

// SessionReader loads and renews an existing token after account validation.
type SessionReader interface {
	Load(context.Context, string) (domain.Session, error)
	Touch(context.Context, string, domain.Session) error
}

// Principal is the authorized identity exposed to upper layers. It has no hash.
type Principal struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               domain.Role
	MustChangePassword bool
}

// Authenticator checks each request against the current account and session.
type Authenticator struct {
	accounts AccountReader
	sessions SessionReader
}

// NewAuthenticator injects the account and session readers.
func NewAuthenticator(accounts AccountReader, sessions SessionReader) *Authenticator {
	return &Authenticator{accounts: accounts, sessions: sessions}
}

// Authenticate rejects stale sessions before renewal and never exposes the hash.
func (a *Authenticator) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	session, err := a.sessions.Load(ctx, token)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("%w: read session: %w", ErrAuthenticationUnavailable, err)
	}
	user, err := a.accounts.FindByID(ctx, session.OrgID, session.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("%w: read account: %w", ErrAuthenticationUnavailable, err)
	}
	if user.ID != session.UserID || user.OrgID != session.OrgID || !user.SessionMatches(session.SessionEpoch) {
		return Principal{}, ErrUnauthenticated
	}
	if err := a.sessions.Touch(ctx, token, session); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, fmt.Errorf("%w: renew session: %w", ErrAuthenticationUnavailable, err)
	}
	return Principal{
		ID: user.ID, OrgID: user.OrgID, LoginName: user.LoginName,
		DisplayName: user.DisplayName, Role: user.Role,
		MustChangePassword: user.MustChangePassword,
	}, nil
}
