package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidChangePassword means the command is missing trusted request data.
	ErrInvalidChangePassword = errors.New("invalid change-password command")
	// ErrCurrentPasswordInvalid means the current password did not match.
	ErrCurrentPasswordInvalid = errors.New("current password is incorrect")
)

// ChangePasswordInput holds the current token and password replacement request.
type ChangePasswordInput struct {
	Token           string
	CurrentPassword string
	NewPassword     string
	RequestID       string
	ClientIP        string
}

// ChangePasswordResult contains a fresh token for the current client only.
// The transport must replace its existing session cookie after this succeeds.
type ChangePasswordResult struct {
	Token              string
	MustChangePassword bool
	Revision           int64
}

// ChangePasswordSessionStore prepares a replacement session and revokes it on
// transaction failure. All other sessions become stale when the epoch changes.
type ChangePasswordSessionStore interface {
	Create(context.Context, uuid.UUID, uuid.UUID, int64) (string, domain.Session, error)
	Destroy(context.Context, string) error
}

// ChangePasswordStore commits a password change and its two Outbox events.
type ChangePasswordStore interface {
	ChangePasswordWithEvents(context.Context, domain.User, string, []OutboxEvent) (domain.User, error)
}

// ChangePasswordCommand replaces the current client's session after an audited
// password change and invalidates every previous session through the epoch.
type ChangePasswordCommand struct {
	auth     *Authenticator
	accounts AccountReader
	sessions ChangePasswordSessionStore
	store    ChangePasswordStore
	now      func() time.Time
}

// NewChangePasswordCommand injects the required account, session, and event stores.
func NewChangePasswordCommand(auth *Authenticator, accounts AccountReader, sessions ChangePasswordSessionStore, store ChangePasswordStore, now func() time.Time) *ChangePasswordCommand {
	return &ChangePasswordCommand{auth: auth, accounts: accounts, sessions: sessions, store: store, now: now}
}

// Execute returns the new token only after the database transaction commits.
func (c *ChangePasswordCommand) Execute(ctx context.Context, input ChangePasswordInput) (ChangePasswordResult, error) {
	if c == nil || c.auth == nil || c.accounts == nil || c.sessions == nil || c.store == nil || c.now == nil {
		return ChangePasswordResult{}, ErrInvalidChangePassword
	}
	ip, err := netip.ParseAddr(input.ClientIP)
	if input.Token == "" || input.CurrentPassword == "" || input.RequestID == "" ||
		len(input.RequestID) > 128 || err != nil {
		return ChangePasswordResult{}, ErrInvalidChangePassword
	}
	principal, err := c.auth.Authenticate(ctx, input.Token)
	if err != nil {
		return ChangePasswordResult{}, err
	}
	user, err := c.accounts.FindByID(ctx, principal.OrgID, principal.ID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return ChangePasswordResult{}, ErrUnauthenticated
	}
	if err != nil {
		return ChangePasswordResult{}, fmt.Errorf("%w: read account for password change: %w", ErrAuthenticationUnavailable, err)
	}
	if user.ID != principal.ID || user.OrgID != principal.OrgID || user.Status != domain.StatusActive {
		return ChangePasswordResult{}, ErrUnauthenticated
	}
	if !domain.VerifyPassword(user.PasswordHash, input.CurrentPassword) {
		return ChangePasswordResult{}, ErrCurrentPasswordInvalid
	}
	newHash, err := domain.HashPassword(input.NewPassword, user.PasswordHash)
	if err != nil {
		return ChangePasswordResult{}, err
	}
	if user.SessionEpoch < 1 || user.SessionEpoch >= math.MaxInt32 ||
		user.Revision < 1 || user.Revision >= math.MaxInt32 {
		return ChangePasswordResult{}, ErrInvalidChangePassword
	}
	events, err := changedPasswordEvents(user, input.RequestID, ip.Unmap().String(), c.now().UTC())
	if err != nil {
		return ChangePasswordResult{}, fmt.Errorf("build password change events: %w", err)
	}
	token, _, err := c.sessions.Create(ctx, user.OrgID, user.ID, user.SessionEpoch+1)
	if err != nil {
		return ChangePasswordResult{}, fmt.Errorf("%w: prepare replacement session: %w", ErrAuthenticationUnavailable, err)
	}
	if token == "" {
		return ChangePasswordResult{}, fmt.Errorf("%w: replacement session token is empty", ErrAuthenticationUnavailable)
	}
	saved, err := c.store.ChangePasswordWithEvents(ctx, user, newHash, events)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if cleanupErr := c.sessions.Destroy(cleanupCtx, token); cleanupErr != nil {
			return ChangePasswordResult{}, errors.Join(fmt.Errorf("change password transaction: %w", err),
				fmt.Errorf("revoke uncommitted replacement session: %w", cleanupErr))
		}
		return ChangePasswordResult{}, fmt.Errorf("change password transaction: %w", err)
	}
	if saved.ID != user.ID || saved.OrgID != user.OrgID || saved.SessionEpoch != user.SessionEpoch+1 ||
		saved.Revision != user.Revision+1 || saved.MustChangePassword {
		return ChangePasswordResult{}, errors.New("password change result does not match committed command")
	}
	return ChangePasswordResult{Token: token, Revision: saved.Revision}, nil
}

func changedPasswordEvents(user domain.User, requestID, ip string, now time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": now, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "user", "id": user.ID},
		"aggregate": map[string]any{"type": "user", "id": user.ID, "revision": user.Revision + 1},
		"data":      map[string]any{"change": "password_changed"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode password change: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": now, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "user", "id": user.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.password_changed", "object": map[string]any{"type": "user", "id": user.ID},
			"before":     map[string]any{"must_change_password": user.MustChangePassword},
			"after":      map[string]any{"must_change_password": false},
			"request_id": requestID, "ip": ip,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode password change audit: %w", err)
	}
	key := user.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
