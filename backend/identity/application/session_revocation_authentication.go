package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
)

var _ SessionRevocationAuthenticator = (*Auth)(nil)

// AuthenticateSessionRevocation checks fresh credentials without touching activity.
// Existing immutable proofs are handled by PublicSessionRevocations before this
// method; it never authenticates a revoked credential to obtain new authority.
func (a *Auth) AuthenticateSessionRevocation(ctx context.Context, credentials SessionRevocationCredentials, action SessionRevocationAction, target uuid.UUID) (Principal, SessionCredentialKind, error) {
	if a == nil || !a.emailReady() {
		return Principal{}, "", ErrRegistrationUnavailable
	}
	if credentials.LegacyToken != "" {
		if credentials.AccessToken != "" || credentials.RefreshToken != "" {
			return Principal{}, "", ErrSessionInvalid
		}
		result, err := a.CurrentWithActivity(ctx, credentials.LegacyToken, NoActivity)
		if err != nil {
			return Principal{}, "", err
		}
		if !csrfProof(credentials.CSRFToken, result.Session) {
			return Principal{}, "", ErrSessionCSRFInvalid
		}
		return revocationPrincipal(result), SessionCredentialLegacy, nil
	}
	if credentials.AccessToken != "" {
		result, err := a.CurrentAccess(ctx, credentials.AccessToken, credentials.CSRFToken, NoActivity)
		if err == nil {
			return revocationPrincipal(result), SessionCredentialAccess, nil
		}
		if !errors.Is(err, ErrAccessExpired) && !errors.Is(err, ErrSessionInvalid) {
			return Principal{}, "", err
		}
		if action != RevokeSessionRequest || credentials.RefreshToken == "" {
			return Principal{}, "", err
		}
	}
	if action != RevokeSessionRequest || target == uuid.Nil || credentials.RefreshToken == "" {
		return Principal{}, "", ErrSessionInvalid
	}
	if !a.sessionVerificationReady() {
		return Principal{}, "", ErrRefreshDependency
	}
	candidate, err := a.refreshCandidate(ctx, credentials.RefreshToken)
	if err != nil {
		return Principal{}, "", err
	}
	var result AuthResult
	err = a.sessionTransaction(ctx, func(tx AuthTx) error {
		user, err := tx.AccountByID(candidate.AccountID)
		if errors.Is(err, domain.ErrUserNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		session, err := tx.SessionByID(candidate.SessionID, true)
		if err != nil {
			return err
		}
		refresh, err := tx.FindRefresh(candidate.Digest, true)
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if session.ID != target {
			return ErrForbidden
		}
		if session.AccountID != user.ID || session.ProtocolVersion != 2 || refresh.AccountID != user.ID || refresh.SessionID != session.ID || user.EmailVerifiedAt.IsZero() || !session.Valid(user, now) || refresh.RevokedAt != nil || now.Before(refresh.IssuedAt) {
			return ErrSessionInvalid
		}
		if !csrfProof(credentials.CSRFToken, session) {
			return ErrSessionCSRFInvalid
		}
		result = AuthResult{User: user, Session: session, ServerTime: now}
		return nil
	})
	if err != nil {
		return Principal{}, "", err
	}
	return revocationPrincipal(result), SessionCredentialRefresh, nil
}

func revocationPrincipal(result AuthResult) Principal {
	return Principal{ID: result.User.ID, SessionID: result.Session.ID, CredentialEpoch: result.User.SessionEpoch}
}
