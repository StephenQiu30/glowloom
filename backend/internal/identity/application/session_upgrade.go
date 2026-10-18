package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// WithLegacyUpgradeUntil retains the deployment's fixed deadline, including past
// deadlines. Zero is only the pre-rollout legacy baseline, never an upgrade window.
func (a *Auth) WithLegacyUpgradeUntil(until time.Time) *Auth {
	a.legacyUpgradeUntil = until
	return a
}
func (a *Auth) legacyWindowOpen(now time.Time) bool {
	return a.legacyUpgradeUntil.IsZero() || now.Before(a.legacyUpgradeUntil)
}

// LegacyUpgradeAvailable reports readiness without touching credentials or state.
func (a *Auth) LegacyUpgradeAvailable(now time.Time) bool {
	return !a.legacyUpgradeUntil.IsZero() && a.legacyWindowOpen(now) && a.sessionIssuanceEnabled() && a.sessionSecurityReady()
}

// UpgradeLegacy atomically changes protocol on the same device. The source
// must be the trusted transport peer used by Refresh, never a forwarded header. The old opaque
// digest is generation zero, consumed with the normal ten-second child receipt.
func (a *Auth) UpgradeLegacy(ctx context.Context, token, csrf, source string, operation uuid.UUID) (AuthResult, error) {
	if !a.sessionSecurityReady() {
		return AuthResult{}, ErrRefreshDependency
	}
	if operation == uuid.Nil || source == "" {
		return AuthResult{}, ErrSessionInvalid
	}
	digest, err := refreshDigest(token)
	if err != nil {
		return AuthResult{}, err
	}
	var candidate domain.AuthSession
	err = a.sessionTransaction(ctx, func(tx AuthTx) error { var e error; candidate, e = tx.SessionByHash(digest, false); return e })
	if err != nil {
		return AuthResult{}, err
	}
	// A committed upgrade remains recoverable using the ordinary durable receipt,
	// including when issuance is disabled or the migration deadline has just passed.
	if candidate.ProtocolVersion == 2 {
		return a.Refresh(ctx, token, csrf, source, operation)
	}
	if !a.LegacyUpgradeAvailable(a.now().UTC()) {
		return AuthResult{}, ErrRefreshDependency
	}
	release, err := a.security.coordinator.Admit(ctx, candidate.ID, source)
	if err != nil {
		return AuthResult{}, err
	}
	released := false
	defer func() {
		if !released {
			_ = release(ctx)
		}
	}()
	var result AuthResult
	var receipt []byte
	var until time.Time
	var alreadyUpgraded bool
	err = a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, e := tx.AccountByID(candidate.AccountID)
		if errors.Is(e, domain.ErrUserNotFound) {
			return ErrSessionInvalid
		}
		if e != nil {
			return e
		}
		s, e := tx.SessionByHash(digest, true)
		if e != nil {
			return e
		}
		// A concurrent upgrade won the session lock. Recover outside this transaction
		// after releasing the admission lease; Refresh locks all receipt rows itself.
		if s.ProtocolVersion == 2 {
			alreadyUpgraded = true
			return nil
		}
		now := a.now().UTC()
		if !a.LegacyUpgradeAvailable(now) {
			return ErrRefreshDependency
		}
		if s.AccountID != u.ID || s.ProtocolVersion != 1 || u.EmailVerifiedAt.IsZero() || !s.Valid(u, now) || !a.sessionProofValid(token, s) {
			return ErrSessionInvalid
		}
		if !csrfProof(csrf, s) {
			return ErrSessionCSRFInvalid
		}
		if e = tx.UpgradeLegacySession(s.ID, u.ID, digest); e != nil {
			return e
		}
		s.ProtocolVersion = 2
		old := domain.RefreshToken{ID: uuid.New(), SessionID: s.ID, AccountID: u.ID, Digest: digest, IssuedAt: s.CreatedAt}
		if e = tx.InsertRefresh(old); e != nil {
			return e
		}
		refresh, d, e := randomSessionToken()
		if e != nil {
			return e
		}
		next := domain.RefreshToken{ID: uuid.New(), SessionID: s.ID, AccountID: u.ID, Generation: 1, Digest: d, IssuedAt: now}
		s.RefreshGeneration = 1
		result, e = a.issueAccess(u, s, csrf, refresh, now)
		if e != nil {
			return e
		}
		plain, e := json.Marshal(refreshReceipt{result.AccessToken, result.RefreshToken, result.AccessExpiresAt, result.RefreshAfterAt, result.ServerTime})
		if e != nil {
			return e
		}
		receipt, e = a.security.receipts.Seal(s.ID, 0, operation, plain)
		if e != nil {
			return e
		}
		until = now.Add(10 * time.Second)
		if e = tx.RotateRefresh(old, next, operation, until, receipt); e != nil {
			return e
		}
		return tx.AppendSessionSecurityEvent(u.ID, s.ID, operation, "session.upgraded", "legacy upgrade", now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	if alreadyUpgraded {
		// Admission is owner-safe; avoid holding it while reacquiring Refresh admission.
		released = true
		if err = release(ctx); err != nil {
			return AuthResult{}, err
		}
		return a.Refresh(ctx, token, csrf, source, operation)
	}
	_ = a.security.coordinator.WriteReceipt(ctx, result.Session.ID, 0, receipt, until)
	return result, nil
}
