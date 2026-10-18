package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

type sessionSecurity struct {
	codec       AccessTokenCodec
	receipts    RefreshReceiptProtector
	coordinator RefreshCoordinator
	issuance    bool
}

// WithSessionSecurity retains verification and refresh independently of issuance.
// Configure once before serving requests; default issuance is enabled.
func (a *Auth) WithSessionSecurity(c AccessTokenCodec, r RefreshReceiptProtector, q RefreshCoordinator) *Auth {
	a.security = &sessionSecurity{c, r, q, true}
	return a
}

// WithSessionIssuance disables only creation of new v2 sessions during rollback.
// Existing v2 verification, refresh and revocation keep their dependencies.
func (a *Auth) WithSessionIssuance(enabled bool) *Auth {
	if a.security != nil {
		a.security.issuance = enabled
	}
	return a
}
func (a *Auth) sessionSecurityReady() bool {
	return a.sessionVerificationReady() && a.security.receipts != nil && a.security.codec.CanSign()
}
func (a *Auth) sessionVerificationReady() bool {
	return a.emailReady() && a.security != nil && a.security.codec != nil && a.security.coordinator != nil
}
func (a *Auth) sessionIssuanceEnabled() bool { return a.security != nil && a.security.issuance }
func randomSessionToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	d := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(b), d[:], nil
}
func refreshDigest(token string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 32 {
		return nil, ErrSessionInvalid
	}
	d := sha256.Sum256(b)
	return d[:], nil
}
func csrfProof(csrf string, s domain.AuthSession) bool {
	b, err := base64.RawURLEncoding.DecodeString(csrf)
	if err != nil || len(b) != 32 {
		return false
	}
	d := sha256.Sum256([]byte(csrf))
	return hmac.Equal(d[:], s.CSRFHash)
}
func (a *Auth) newV2Session(tx AuthTx, u domain.User, persistent bool, now time.Time) (AuthResult, error) {
	if !a.sessionSecurityReady() {
		return AuthResult{}, ErrRefreshDependency
	}
	token, d, err := randomSessionToken()
	if err != nil {
		return AuthResult{}, err
	}
	csrf, _, err := randomSessionToken()
	if err != nil {
		return AuthResult{}, err
	}
	h := sha256.Sum256([]byte(csrf))
	policy := domain.SessionPolicyFor(persistent)
	s := domain.AuthSession{ID: uuid.New(), AccountID: u.ID, TokenHash: d, CredentialRevision: u.SessionEpoch, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(policy.AbsoluteTTL), Persistent: persistent, ProtocolVersion: 2, IdleTimeoutSeconds: int32(policy.IdleTTL / time.Second), CSRFHash: h[:]}
	if err = tx.InsertSession(s); err != nil {
		return AuthResult{}, err
	}
	if err = tx.SetSessionSecurity(s.ID, h[:]); err != nil {
		return AuthResult{}, err
	}
	if err = tx.InsertRefresh(domain.RefreshToken{ID: uuid.New(), SessionID: s.ID, AccountID: u.ID, Digest: d, IssuedAt: now}); err != nil {
		return AuthResult{}, err
	}
	return a.issueAccess(u, s, csrf, token, now)
}
func (a *Auth) issueAccess(u domain.User, s domain.AuthSession, csrf, refresh string, now time.Time) (AuthResult, error) {
	end := now.Add(15 * time.Minute)
	if s.AbsoluteExpiresAt.Before(end) {
		end = s.AbsoluteExpiresAt
	}
	end = end.Truncate(time.Second)
	claims := AccessClaims{UserID: u.ID, SessionID: s.ID, TokenID: uuid.New(), CredentialEpoch: u.SessionEpoch, IssuedAt: now, NotBefore: now, ExpiresAt: end}
	token, err := a.security.codec.Sign(claims)
	if err != nil {
		return AuthResult{}, err
	}
	after := end.Add(-time.Minute)
	if after.Before(now) {
		after = now
	}
	return AuthResult{User: u, Session: s, AccessToken: token, RefreshToken: refresh, CSRFToken: csrf, AccessExpiresAt: end, RefreshAfterAt: after, ServerTime: now}, nil
}
func (a *Auth) lockedPrincipal(tx AuthTx, actor Principal) (domain.User, domain.AuthSession, time.Time, error) {
	u, err := tx.AccountByID(actor.ID)
	if errors.Is(err, domain.ErrUserNotFound) {
		err = ErrSessionInvalid
	}
	if err != nil {
		return u, domain.AuthSession{}, time.Time{}, err
	}
	s, err := tx.SessionByID(actor.SessionID, true)
	now := a.now().UTC()
	if err != nil {
		return u, s, now, err
	}
	if s.AccountID != u.ID || s.ProtocolVersion != 2 || actor.CredentialEpoch != u.SessionEpoch || u.EmailVerifiedAt.IsZero() || !s.Valid(u, now) {
		return u, s, now, ErrSessionInvalid
	}
	return u, s, now, nil
}

// CurrentAccess checks current PG authority even when the optional Redis hint fails.
func (a *Auth) CurrentAccess(ctx context.Context, token, csrf string, activity SessionActivity) (AuthResult, error) {
	if !a.sessionVerificationReady() {
		return AuthResult{}, ErrRefreshDependency
	}
	claims, err := a.security.codec.Verify(token, a.now().UTC())
	if err != nil {
		return AuthResult{}, err
	}
	revoked, _ := a.security.coordinator.IsRevoked(ctx, claims.SessionID)
	if revoked {
		return AuthResult{}, ErrSessionInvalid
	}
	var result AuthResult
	err = a.sessionTransaction(ctx, func(tx AuthTx) error {
		u, s, now, err := a.lockedPrincipal(tx, Principal{ID: claims.UserID, SessionID: claims.SessionID, CredentialEpoch: claims.CredentialEpoch})
		if err != nil {
			return err
		}
		if !now.Before(claims.ExpiresAt) {
			return ErrAccessExpired
		}
		if claims.IssuedAt.Before(s.CreatedAt.Truncate(time.Second)) || claims.IssuedAt.After(now.Add(30*time.Second)) || !csrfProof(csrf, s) {
			return ErrSessionInvalid
		}
		if activity == UserActivity {
			if err = tx.TouchSession(s.ID, now); err != nil {
				return err
			}
			s.LastActiveAt = now
		}
		result = AuthResult{User: u, Session: s, CSRFToken: csrf, AccessExpiresAt: claims.ExpiresAt, RefreshAfterAt: claims.ExpiresAt.Add(-time.Minute), ServerTime: now}
		return nil
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, nil
}
