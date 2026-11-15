package application

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
)

// ReauthenticateCurrent proves a password against a PG-validated v2 principal.
func (a *Auth) ReauthenticateCurrent(ctx context.Context, actor Principal, password string) error {
	if !a.emailReady() {
		return ErrRegistrationUnavailable
	}
	var rejected error
	err := a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, s, now, err := a.lockedPrincipal(tx, actor)
		if err != nil {
			return err
		}
		ok, err := a.verifyCurrentPassword(tx, u, password, now)
		if err != nil {
			return err
		}
		if !ok {
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

// ChangePasswordCurrent rotates credentials and this device atomically.
func (a *Auth) ChangePasswordCurrent(ctx context.Context, actor Principal, epoch int64, current, next, confirmation string) (AuthResult, error) {
	if a.passwordChanges != nil {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	if !a.emailReady() {
		return AuthResult{}, ErrRegistrationUnavailable
	}
	if next != confirmation {
		return AuthResult{}, domain.ErrInvalidProfile
	}
	if err := a.validateNewPassword(next); err != nil {
		return AuthResult{}, err
	}
	if a.sessionIssuanceEnabled() {
		if !a.sessionSecurityReady() {
			return AuthResult{}, ErrRefreshDependency
		}
		release, err := a.security.coordinator.Admit(ctx, actor.SessionID, "password_change")
		if err != nil {
			return AuthResult{}, err
		}
		defer func() { _ = release(ctx) }()
	}
	var result AuthResult
	var rejected error
	err := a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, s, now, err := a.lockedPrincipal(tx, actor)
		if err != nil {
			return err
		}
		if u.SessionEpoch != epoch {
			return domain.ErrRevisionConflict
		}
		ok, err := a.verifyCurrentPassword(tx, u, current, now)
		if err != nil {
			return err
		}
		if !ok {
			rejected = ErrCurrentPassword
			return nil
		}
		h, err := domain.HashPassword(next, u.PasswordHash)
		if err != nil {
			return err
		}
		result, err = a.replacePassword(tx, u, s.Persistent, h, now)
		return err
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, rejected
}
