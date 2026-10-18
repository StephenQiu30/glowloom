package application

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ErrRefreshRaced means the current chain has advanced past a recoverable child.
var ErrRefreshRaced = errors.New("refresh_raced")

type refreshReceipt struct {
	Access, Refresh        string
	Expires, After, Server time.Time
}

func (a *Auth) refreshCandidate(ctx context.Context, token string) (domain.RefreshToken, error) {
	d, err := refreshDigest(token)
	if err != nil {
		return domain.RefreshToken{}, err
	}
	var r domain.RefreshToken
	err = a.sessionTransaction(ctx, func(tx AuthTx) error { var err error; r, err = tx.FindRefresh(d, false); return err })
	return r, err
}
func (a *Auth) recoverRefresh(u domain.User, s domain.AuthSession, parent, child domain.RefreshToken, csrf string, now time.Time, cached []byte) (AuthResult, error) {
	if parent.AccountID != u.ID || parent.SessionID != s.ID || parent.RevokedAt != nil || parent.ConsumedAt == nil || parent.ReplayUntil == nil || !now.Before(*parent.ReplayUntil) || now.Before(*parent.ConsumedAt) || parent.Generation+1 != s.RefreshGeneration || parent.RotationRequestID == uuid.Nil {
		return AuthResult{}, ErrRefreshRaced
	}
	ciphertext := parent.ReceiptCiphertext
	if len(cached) > 0 && hmac.Equal(cached, ciphertext) {
		ciphertext = cached
	}
	plain, err := a.security.receipts.Open(s.ID, parent.Generation, parent.RotationRequestID, ciphertext)
	if err != nil {
		return AuthResult{}, err
	}
	var saved refreshReceipt
	if err = json.Unmarshal(plain, &saved); err != nil {
		return AuthResult{}, err
	}
	d, err := refreshDigest(saved.Refresh)
	if err != nil || child.AccountID != u.ID || child.SessionID != s.ID || child.ConsumedAt != nil || child.RevokedAt != nil || now.Before(child.IssuedAt) || child.Generation != s.RefreshGeneration || !hmac.Equal(d, child.Digest) {
		return AuthResult{}, ErrRefreshRaced
	}
	if saved.Access == "" || !now.Before(saved.Expires) || saved.Server.Truncate(time.Microsecond).Before(parent.ConsumedAt.Truncate(time.Microsecond)) || saved.Server.After(now) {
		return AuthResult{}, ErrSessionInvalid
	}
	return AuthResult{User: u, Session: s, AccessToken: saved.Access, RefreshToken: saved.Refresh, CSRFToken: csrf, AccessExpiresAt: saved.Expires, RefreshAfterAt: saved.After, ServerTime: saved.Server}, nil
}

// Refresh consumes under PG locks, with all Redis work before or after transactions.
func (a *Auth) Refresh(ctx context.Context, token, csrf, source string, operation uuid.UUID) (AuthResult, error) {
	if !a.sessionSecurityReady() {
		return AuthResult{}, ErrRefreshDependency
	}
	if operation == uuid.Nil || source == "" {
		return AuthResult{}, ErrSessionInvalid
	}
	candidate, err := a.refreshCandidate(ctx, token)
	if err != nil {
		return AuthResult{}, err
	}
	release, err := a.security.coordinator.Admit(ctx, candidate.SessionID, source)
	if err != nil {
		return AuthResult{}, err
	}
	defer func() { _ = release(ctx) }()
	// Optional cache reads happen before PG locks; only matching durable ciphertext
	// can be used after the locked generation and recovery deadline checks.
	cached, _ := a.security.coordinator.ReadReceipt(ctx, candidate.SessionID, candidate.Generation)
	var result AuthResult
	var rejected error
	var revoked bool
	var receipt []byte
	var replayUntil time.Time
	var generation int64
	err = a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, err := tx.AccountByID(candidate.AccountID)
		if errors.Is(err, domain.ErrUserNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		s, err := tx.SessionByID(candidate.SessionID, true)
		if err != nil {
			return err
		}
		old, err := tx.FindRefresh(candidate.Digest, true)
		if err != nil {
			return err
		}
		var parent, child domain.RefreshToken
		if old.ConsumedAt != nil && s.RefreshGeneration == old.Generation+1 {
			child, err = tx.RefreshByGeneration(s.ID, s.RefreshGeneration, true)
			if err != nil {
				return err
			}
		} else if old.ConsumedAt == nil && old.Generation > 0 && old.Generation == s.RefreshGeneration {
			parent, err = tx.RefreshByGeneration(s.ID, old.Generation-1, true)
			if err != nil {
				return err
			}
		}
		// Cleanup can hold refresh rows without the parent session lock. Sample
		// only after every row needed for rotation/recovery is locked; recovery
		// below performs no further persistence reads or lock waits.
		now := a.now().UTC()
		if s.AccountID != u.ID || s.ProtocolVersion != 2 || u.EmailVerifiedAt.IsZero() || !s.Valid(u, now) {
			return ErrSessionInvalid
		}
		if !csrfProof(csrf, s) {
			return ErrSessionCSRFInvalid
		}
		if old.SessionID != s.ID || old.AccountID != u.ID || old.RevokedAt != nil || now.Before(old.IssuedAt) {
			return ErrSessionInvalid
		}
		if old.ConsumedAt != nil {
			if old.ReplayUntil == nil || !now.Before(*old.ReplayUntil) {
				if err = tx.RevokeSession(u.ID, s.ID, now); err != nil {
					return err
				}
				if err = tx.RevokeRefreshChain(s.ID, now); err != nil {
					return err
				}
				if err = tx.AppendSessionSecurityEvent(u.ID, s.ID, operation, "session.revoked", "refresh reuse", now); err != nil {
					return err
				}
				rejected = ErrSessionInvalid
				revoked = true
				result.Session = s
				return nil
			}
			if s.RefreshGeneration != old.Generation+1 {
				return ErrRefreshRaced
			}
			result, err = a.recoverRefresh(u, s, old, child, csrf, now, cached)
			return err
		}
		if old.Generation != s.RefreshGeneration {
			return ErrRefreshRaced
		}
		if old.Generation > 0 && parent.ReplayUntil != nil && now.Before(*parent.ReplayUntil) {
			result, err = a.recoverRefresh(u, s, parent, old, csrf, now, cached)
			return err
		}

		if old.Generation == math.MaxInt64 {
			return ErrSessionInvalid
		}
		refresh, d, err := randomSessionToken()
		if err != nil {
			return err
		}
		next := domain.RefreshToken{ID: uuid.New(), SessionID: s.ID, AccountID: u.ID, Generation: old.Generation + 1, Digest: d, IssuedAt: now}
		s.RefreshGeneration = next.Generation
		result, err = a.issueAccess(u, s, csrf, refresh, now)
		if err != nil {
			return err
		}
		plain, err := json.Marshal(refreshReceipt{result.AccessToken, result.RefreshToken, result.AccessExpiresAt, result.RefreshAfterAt, result.ServerTime})
		if err != nil {
			return err
		}
		receipt, err = a.security.receipts.Seal(s.ID, old.Generation, operation, plain)
		if err != nil {
			return err
		}
		replayUntil = now.Add(10 * time.Second)
		generation = old.Generation
		if err = tx.RotateRefresh(old, next, operation, replayUntil, receipt); err != nil {
			return err
		}
		return tx.AppendSessionSecurityEvent(u.ID, s.ID, operation, "session.refreshed", "rotation", now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	if revoked {
		_ = a.security.coordinator.MarkRevoked(ctx, result.Session.ID, result.Session.AbsoluteExpiresAt)
		return AuthResult{}, rejected
	}
	if len(receipt) > 0 {
		_ = a.security.coordinator.WriteReceipt(ctx, result.Session.ID, generation, receipt, replayUntil)
	}
	return result, nil
}

// LogoutCurrentWithRefresh revokes the current chain without rotating or touching it.
func (a *Auth) LogoutCurrentWithRefresh(ctx context.Context, token, csrf string, target uuid.UUID) error {
	if !a.sessionVerificationReady() {
		return ErrRefreshDependency
	}
	candidate, err := a.refreshCandidate(ctx, token)
	if errors.Is(err, ErrSessionInvalid) {
		return nil
	}
	if err != nil {
		return err
	}
	var s domain.AuthSession
	err = a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, err := tx.AccountByID(candidate.AccountID)
		if err != nil {
			return err
		}
		s, err = tx.SessionByID(candidate.SessionID, true)
		if err != nil {
			return err
		}
		r, err := tx.FindRefresh(candidate.Digest, true)
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if s.ID != target {
			return ErrForbidden
		}
		if s.AccountID != u.ID || s.ProtocolVersion != 2 || r.SessionID != s.ID {
			return ErrSessionInvalid
		}
		if !csrfProof(csrf, s) {
			return ErrSessionCSRFInvalid
		}
		if s.RevokedAt != nil {
			return nil
		}
		if !s.Valid(u, now) {
			return ErrSessionInvalid
		}
		if err = tx.RevokeSession(u.ID, s.ID, now); err != nil {
			return err
		}
		if err = tx.RevokeRefreshChain(s.ID, now); err != nil {
			return err
		}
		return tx.AppendSessionSecurityEvent(u.ID, s.ID, uuid.New(), "session.revoked", "logout", now)
	})
	if err == nil {
		_ = a.security.coordinator.MarkRevoked(ctx, s.ID, s.AbsoluteExpiresAt)
	}
	return err
}
