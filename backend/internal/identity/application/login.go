package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const maxLoginRevisionAttempts = 3

// The synthetic bcrypt hash keeps absent-account checks comparable to real
// password checks. It is not a credential for any account.
const missingAccountHash = "$2a$12$EW3KgYs6RRSB4SSvDCOzae26SkT1uB/.H4U71FGLVYOzostQbywQe"

var (
	// ErrInvalidLoginInput means the trusted scope or request fields are invalid.
	ErrInvalidLoginInput = errors.New("invalid login input")
	// ErrInvalidCredentials hides whether an account exists or is disabled.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrLoginRateLimited means the client IP exceeded its attempt allowance.
	ErrLoginRateLimited = errors.New("login rate limited")
	// ErrLoginUnavailable means a required store or audit write failed.
	ErrLoginUnavailable = ErrAuthenticationUnavailable
)

// LoginAccountStore joins account state updates and audit Outbox writes.
type LoginAccountStore interface {
	FindByLogin(context.Context, uuid.UUID, string) (domain.User, error)
	SaveLoginAttempt(context.Context, domain.User, int64, []OutboxEvent) error
	RecordLoginAudit(context.Context, OutboxEvent) error
}

// LoginSessions creates opaque sessions and compensates failed transactions.
type LoginSessions interface {
	Create(context.Context, uuid.UUID, uuid.UUID, int64) (string, domain.Session, error)
	Destroy(context.Context, string) error
}

// LoginLimiter records attempts before any account lookup.
type LoginLimiter interface {
	AllowAttempt(context.Context, string) (bool, time.Duration, error)
}

// LoginInput contains trusted organization scope and request metadata.
type LoginInput struct {
	OrgID     uuid.UUID
	LoginName string
	Password  string
	ClientIP  string
	RequestID string
}

// LoginResult contains the token only after the account and audit commit.
// RetryAfter is set for account and IP locks.
type LoginResult struct {
	Token      string
	User       Principal
	RetryAfter time.Duration
}

// LoginCommand coordinates IP limits, account locks, sessions, and audit.
type LoginCommand struct {
	accounts LoginAccountStore
	sessions LoginSessions
	limiter  LoginLimiter
	now      func() time.Time
}

// NewLoginCommand injects the account, Redis, and clock dependencies.
func NewLoginCommand(accounts LoginAccountStore, sessions LoginSessions, limiter LoginLimiter, now func() time.Time) *LoginCommand {
	return &LoginCommand{accounts: accounts, sessions: sessions, limiter: limiter, now: now}
}

// Execute returns identical credential errors for absent, wrong, and disabled
// accounts while auditing their distinct internal reasons.
func (c *LoginCommand) Execute(ctx context.Context, input LoginInput) (LoginResult, error) {
	if c == nil || c.accounts == nil || c.sessions == nil || c.limiter == nil || c.now == nil {
		return LoginResult{}, ErrLoginUnavailable
	}
	loginName := strings.TrimSpace(input.LoginName)
	ip, ipErr := netip.ParseAddr(input.ClientIP)
	if input.OrgID == uuid.Nil || loginName == "" || len(loginName) > 128 ||
		input.Password == "" || input.RequestID == "" || len(input.RequestID) > 128 || ipErr != nil {
		return LoginResult{}, ErrInvalidLoginInput
	}
	ip = ip.Unmap()
	input.LoginName, input.ClientIP = loginName, ip.String()
	allowed, retryAfter, err := c.limiter.AllowAttempt(ctx, input.ClientIP)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: limit login attempts: %w", ErrLoginUnavailable, err)
	}
	if !allowed {
		if err := c.recordStandalone(ctx, input, uuid.Nil, "auth.login_failed", "ip_rate_limited", 0); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{RetryAfter: retryAfter}, ErrLoginRateLimited
	}
	for range maxLoginRevisionAttempts {
		user, err := c.accounts.FindByLogin(ctx, input.OrgID, loginName)
		if errors.Is(err, domain.ErrUserNotFound) {
			_ = domain.VerifyPassword(missingAccountHash, input.Password)
			if err := c.recordStandalone(ctx, input, uuid.Nil, "auth.login_failed", "login_not_found", 0); err != nil {
				return LoginResult{}, err
			}
			return LoginResult{}, ErrInvalidCredentials
		}
		if err != nil {
			return LoginResult{}, fmt.Errorf("%w: load account: %w", ErrLoginUnavailable, err)
		}
		if user.ID == uuid.Nil || user.OrgID != input.OrgID || user.SessionEpoch < 1 {
			return LoginResult{}, ErrLoginUnavailable
		}
		now := c.now().UTC()
		remaining, stateErr := user.CanLogin(now)
		if errors.Is(stateErr, domain.ErrAccountDisabled) {
			_ = domain.VerifyPassword(missingAccountHash, input.Password)
			if err := c.recordStandalone(ctx, input, user.ID, "auth.login_failed", "account_disabled", 0); err != nil {
				return LoginResult{}, err
			}
			return LoginResult{}, ErrInvalidCredentials
		}
		if errors.Is(stateErr, domain.ErrAccountLocked) {
			if err := c.recordStandalone(ctx, input, user.ID, "auth.locked", "", remaining); err != nil {
				return LoginResult{}, err
			}
			return LoginResult{RetryAfter: remaining}, domain.ErrAccountLocked
		}
		if stateErr != nil {
			return LoginResult{}, fmt.Errorf("%w: account state: %w", ErrLoginUnavailable, stateErr)
		}
		if !domain.VerifyPassword(user.PasswordHash, input.Password) {
			expectedRevision := user.Revision
			newlyLocked := user.RegisterLoginFailure(now)
			failure, err := authAuditEvent(loginAuditMetadata(input, user.ID), "auth.login_failed", "password_mismatch", 0, now)
			if err != nil {
				return LoginResult{}, fmt.Errorf("%w: build login failure audit: %w", ErrLoginUnavailable, err)
			}
			events := []OutboxEvent{failure}
			if newlyLocked {
				locked, err := authAuditEvent(loginAuditMetadata(input, user.ID), "auth.locked", "", user.LockedUntil.Sub(now), now)
				if err != nil {
					return LoginResult{}, fmt.Errorf("%w: build account lock audit: %w", ErrLoginUnavailable, err)
				}
				events = append(events, locked)
			}
			if err := c.accounts.SaveLoginAttempt(ctx, user, expectedRevision, events); err != nil {
				if errors.Is(err, domain.ErrRevisionConflict) {
					continue
				}
				return LoginResult{}, fmt.Errorf("%w: save failed login: %w", ErrLoginUnavailable, err)
			}
			return LoginResult{}, ErrInvalidCredentials
		}
		event, err := authAuditEvent(loginAuditMetadata(input, user.ID), "auth.login_succeeded", "", 0, now)
		if err != nil {
			return LoginResult{}, fmt.Errorf("%w: build login success audit: %w", ErrLoginUnavailable, err)
		}
		token, _, err := c.sessions.Create(ctx, user.OrgID, user.ID, user.SessionEpoch)
		if err != nil {
			return LoginResult{}, fmt.Errorf("%w: create session: %w", ErrLoginUnavailable, err)
		}
		if token == "" {
			return LoginResult{}, fmt.Errorf("%w: session store returned an empty token", ErrLoginUnavailable)
		}
		expectedRevision := user.Revision
		user.RegisterLoginSuccess(now)
		if err := c.accounts.SaveLoginAttempt(ctx, user, expectedRevision, []OutboxEvent{event}); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			destroyErr := c.sessions.Destroy(cleanupCtx, token)
			cancel()
			if destroyErr != nil {
				return LoginResult{}, fmt.Errorf("%w: save login and revoke provisional session: %w", ErrLoginUnavailable, errors.Join(err, destroyErr))
			}
			if errors.Is(err, domain.ErrRevisionConflict) {
				continue
			}
			return LoginResult{}, fmt.Errorf("%w: save login: %w", ErrLoginUnavailable, err)
		}
		return LoginResult{
			Token: token,
			User: Principal{
				ID: user.ID, OrgID: user.OrgID, LoginName: user.LoginName,
				DisplayName: user.DisplayName, Role: user.Role,
				MustChangePassword: user.MustChangePassword,
			},
		}, nil
	}
	return LoginResult{}, fmt.Errorf("%w: account changed during login", ErrLoginUnavailable)
}

func (c *LoginCommand) recordStandalone(ctx context.Context, input LoginInput, userID uuid.UUID, action, reason string, retryAfter time.Duration) error {
	event, err := authAuditEvent(loginAuditMetadata(input, userID), action, reason, retryAfter, c.now().UTC())
	if err != nil {
		return fmt.Errorf("%w: build login audit: %w", ErrLoginUnavailable, err)
	}
	if err := c.accounts.RecordLoginAudit(ctx, event); err != nil {
		return fmt.Errorf("%w: record login audit: %w", ErrLoginUnavailable, err)
	}
	return nil
}

// authAuditMetadata separates safe audit fields from the password and token.
type authAuditMetadata struct {
	orgID     uuid.UUID
	loginName string
	clientIP  string
	requestID string
	userID    uuid.UUID
}

func loginAuditMetadata(input LoginInput, userID uuid.UUID) authAuditMetadata {
	return authAuditMetadata{
		orgID: input.OrgID, loginName: input.LoginName,
		clientIP: input.ClientIP, requestID: input.RequestID, userID: userID,
	}
}

func authAuditEvent(meta authAuditMetadata, action, reason string, retryAfter time.Duration, now time.Time) (OutboxEvent, error) {
	eventID := uuid.New()
	actor := map[string]any{"kind": "system", "id": nil}
	object := map[string]any{"type": "login_name", "id": strings.ToLower(meta.loginName)}
	if meta.userID != uuid.Nil {
		object = map[string]any{"type": "user", "id": meta.userID}
	}
	if action == "auth.login_succeeded" || action == "auth.logout" {
		actor = map[string]any{"kind": "user", "id": meta.userID}
	}
	data := map[string]any{
		"action": action, "object": object,
		"request_id": meta.requestID, "ip": meta.clientIP,
	}
	if reason != "" {
		data["after"] = map[string]any{"reason": reason}
	}
	if action == "auth.locked" {
		seconds := max(int64((retryAfter+time.Second-1)/time.Second), 1)
		data["after"] = map[string]any{"retry_after_s": seconds}
	}
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": now, "org_id": meta.orgID,
		"actor": actor, "aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": data,
	})
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("encode login audit: %w", err)
	}
	return OutboxEvent{ID: eventID, Topic: auditTopic, PartitionKey: meta.orgID.String(), Payload: payload}, nil
}
