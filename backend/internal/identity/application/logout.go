package application

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
)

var (
	// ErrInvalidLogoutInput means the token or request metadata is missing or malformed.
	ErrInvalidLogoutInput = errors.New("invalid logout input")
	// ErrLogoutUnavailable means revocation or its required audit write failed.
	ErrLogoutUnavailable = ErrAuthenticationUnavailable
)

// LogoutInput holds the current bearer token and trusted request metadata.
type LogoutInput struct {
	Token     string
	RequestID string
	ClientIP  string
}

// LogoutResult tells the transport whether the token was revoked even if the
// later audit write failed. A revoked token's cookies must be cleared.
type LogoutResult struct {
	Revoked bool
}

// LogoutSessionStore revokes an existing session token.
type LogoutSessionStore interface {
	Destroy(context.Context, string) error
}

// LogoutAuditStore records a completed revocation in the audit Outbox.
type LogoutAuditStore interface {
	RecordLogoutAudit(context.Context, OutboxEvent) error
}

// LogoutCommand validates the session, revokes it, and audits the action.
type LogoutCommand struct {
	auth     *Authenticator
	sessions LogoutSessionStore
	audits   LogoutAuditStore
	now      func() time.Time
}

// NewLogoutCommand injects the current-account check, session store, and audit store.
func NewLogoutCommand(auth *Authenticator, sessions LogoutSessionStore, audits LogoutAuditStore, now func() time.Time) *LogoutCommand {
	return &LogoutCommand{auth: auth, sessions: sessions, audits: audits, now: now}
}

// Execute never reports a completed logout before Redis confirms revocation.
// If the audit write then fails, Revoked remains true for cookie cleanup.
func (c *LogoutCommand) Execute(ctx context.Context, input LogoutInput) (LogoutResult, error) {
	if c == nil || c.auth == nil || c.sessions == nil || c.audits == nil || c.now == nil {
		return LogoutResult{}, ErrLogoutUnavailable
	}
	ip, err := netip.ParseAddr(input.ClientIP)
	if input.Token == "" || input.RequestID == "" || len(input.RequestID) > 128 || err != nil {
		return LogoutResult{}, ErrInvalidLogoutInput
	}
	principal, err := c.auth.Authenticate(ctx, input.Token)
	if err != nil {
		return LogoutResult{}, err
	}
	if err := c.sessions.Destroy(ctx, input.Token); err != nil {
		return LogoutResult{}, fmt.Errorf("%w: revoke session: %w", ErrLogoutUnavailable, err)
	}
	result := LogoutResult{Revoked: true}
	event, err := authAuditEvent(authAuditMetadata{
		orgID: principal.OrgID, loginName: principal.LoginName,
		clientIP: ip.Unmap().String(), requestID: input.RequestID, userID: principal.ID,
	}, "auth.logout", "", 0, c.now().UTC())
	if err != nil {
		return result, fmt.Errorf("%w: build logout audit: %w", ErrLogoutUnavailable, err)
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := c.audits.RecordLogoutAudit(auditCtx, event); err != nil {
		return result, fmt.Errorf("%w: record logout audit: %w", ErrLogoutUnavailable, err)
	}
	return result, nil
}
