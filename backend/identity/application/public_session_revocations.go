package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// PublicSessionRevocations authorizes immutable public retries separately from
// current identity. A successful proof lookup can finish only its original request.
// It never turns a revoked credential into a Principal or new business authority.
type PublicSessionRevocations struct {
	authenticator SessionRevocationAuthenticator
	store         SessionRevocationRequestStore
	closures      *SessionRevocations
	now           func() time.Time
}

// NewPublicSessionRevocations keeps public proofs and trusted internal recovery distinct.
func NewPublicSessionRevocations(authenticator SessionRevocationAuthenticator, store SessionRevocationRequestStore, closures *SessionRevocations, now func() time.Time) *PublicSessionRevocations {
	return &PublicSessionRevocations{authenticator: authenticator, store: store, closures: closures, now: now}
}

// Revoke returns whether confirmed completion requires clearing this requester's cookies.
// The target and UUID key are part of the original durable intent, not recovery powers.
func (s *PublicSessionRevocations) Revoke(ctx context.Context, credentials SessionRevocationCredentials, key, target uuid.UUID) (bool, error) {
	request, err := s.execute(ctx, credentials, key, RevokeSessionRequest, target)
	if err != nil {
		return false, err
	}
	return request.TargetID == request.RequestSessionID, nil
}

// RevokeOthers completes the first accepted set, never a fresh live-session query
// on retry. It never clears the requesting device's cookies.
func (s *PublicSessionRevocations) RevokeOthers(ctx context.Context, credentials SessionRevocationCredentials, key uuid.UUID) error {
	_, err := s.execute(ctx, credentials, key, RevokeOtherSessionsRequest, uuid.Nil)
	return err
}

func (s *PublicSessionRevocations) execute(ctx context.Context, credentials SessionRevocationCredentials, key uuid.UUID, action SessionRevocationAction, target uuid.UUID) (SessionRevocationRequest, error) {
	if s == nil || s.authenticator == nil || s.store == nil || s.closures == nil || !s.closures.ready() || s.now == nil {
		return SessionRevocationRequest{}, ErrSessionRevocationUnavailable
	}
	input, err := sessionRevocationInput(credentials, key, action, target)
	if err != nil {
		return SessionRevocationRequest{}, err
	}
	// Look up immutable evidence before normal session authentication: Prepare may
	// already have revoked the parent. A storage error is never an authentication miss.
	request, err := s.store.LoadSessionRevocationRequest(ctx, input)
	if errors.Is(err, writefence.ErrNotFound) {
		actor, kind, authErr := s.authenticator.AuthenticateSessionRevocation(ctx, credentials, action, target)
		if authErr != nil {
			return SessionRevocationRequest{}, authErr
		}
		input.Primary = kind
		if err := input.Validate(); err != nil {
			return SessionRevocationRequest{}, err
		}
		request, err = s.store.PrepareSessionRevocationRequest(ctx, actor, input, s.now)
		if err == nil && (request.AccountID != actor.ID || request.RequestSessionID != actor.SessionID) {
			return SessionRevocationRequest{}, ErrSessionRevocationUnavailable
		}
	}
	if err != nil {
		return SessionRevocationRequest{}, err
	}
	if request.Key != key || request.Action != action || request.TargetID != target {
		return SessionRevocationRequest{}, ErrSessionRevocationUnavailable
	}
	if err := finishSessionRevocationRequest(ctx, request, s.store, s.closures, s.now); err != nil {
		return SessionRevocationRequest{}, err
	}
	return request, nil
}
