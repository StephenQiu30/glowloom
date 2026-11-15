package identity_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestPublicSessionRevocationRecoversOnlyAuthenticatedOriginalRequest(t *testing.T) {
	source := newRevocationModel()
	coordinator := &sessionFenceModel{ticket: source.ticket, outcome: "unavailable"}
	clock := func() time.Time { return source.ticket.RequestedAt.Add(time.Second) }
	closures := identity.NewSessionRevocations(source, coordinator, clock)
	store := &publicRevocationModel{source: source}
	auth := &publicRevocationIdentity{actor: source.actor}
	service := identity.NewPublicSessionRevocations(auth, store, closures, clock)
	credentials := publicRevocationCredentials()
	key := uuid.New()
	if _, err := service.Revoke(t.Context(), credentials, key, source.ticket.SessionID); err == nil {
		t.Fatal("missing writer acknowledgement reported success")
	}
	if store.prepares != 1 || store.completes != 0 {
		t.Fatalf("prepare=%d complete=%d", store.prepares, store.completes)
	}
	// The parent session is now revoked. Its original evidence authorizes only recovery.
	auth.err = identity.ErrSessionInvalid
	coordinator.outcome = "complete"
	restarted := identity.NewPublicSessionRevocations(auth, store, identity.NewSessionRevocations(source, coordinator, clock), clock)
	if _, err := restarted.Revoke(t.Context(), credentials, key, source.ticket.SessionID); err != nil {
		t.Fatal(err)
	}
	if auth.calls != 1 || store.prepares != 1 || store.completes != 1 {
		t.Fatalf("recovery reauthenticated or prepared another request: auth=%d prepare=%d complete=%d", auth.calls, store.prepares, store.completes)
	}
	calls := coordinator.calls
	if _, err := restarted.Revoke(t.Context(), credentials, key, source.ticket.SessionID); err != nil || coordinator.calls != calls {
		t.Fatalf("completed retry repeated effects: %v", err)
	}
}

func TestPublicSessionRevocationRejectsChangedTargetAndMissingProof(t *testing.T) {
	source := newRevocationModel()
	clock := func() time.Time { return source.ticket.RequestedAt.Add(time.Second) }
	coordinator := &sessionFenceModel{ticket: source.ticket, outcome: "unavailable"}
	store := &publicRevocationModel{source: source}
	auth := &publicRevocationIdentity{actor: source.actor}
	service := identity.NewPublicSessionRevocations(auth, store, identity.NewSessionRevocations(source, coordinator, clock), clock)
	credentials := publicRevocationCredentials()
	key := uuid.New()
	_, _ = service.Revoke(t.Context(), credentials, key, source.ticket.SessionID)
	calls := coordinator.calls
	if _, err := service.Revoke(t.Context(), credentials, key, uuid.New()); !errors.Is(err, identity.ErrForbidden) || coordinator.calls != calls {
		t.Fatalf("retargeted request reached writer: %v", err)
	}
	auth.err = identity.ErrSessionInvalid
	credentials.CSRFToken = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	store.loadErr = identity.ErrSessionCSRFInvalid
	if _, err := service.Revoke(t.Context(), credentials, key, source.ticket.SessionID); !errors.Is(err, identity.ErrSessionCSRFInvalid) || coordinator.calls != calls {
		t.Fatalf("unmatched proof reached writer: %v", err)
	}
	store.loadErr = writefence.ErrNotFound
	if _, err := service.Revoke(t.Context(), credentials, uuid.New(), source.ticket.SessionID); !errors.Is(err, identity.ErrSessionInvalid) || store.prepares != 1 {
		t.Fatalf("invalid identity created a request: %v", err)
	}
}

func TestPublicSessionRevocationDoesNotAuthenticateOnStorageFailure(t *testing.T) {
	source := newRevocationModel()
	clock := time.Now
	auth := &publicRevocationIdentity{actor: source.actor}
	store := &publicRevocationModel{source: source, loadErr: errors.New("synthetic proof store unavailable")}
	service := identity.NewPublicSessionRevocations(auth, store, identity.NewSessionRevocations(source, &sessionFenceModel{ticket: source.ticket}, clock), clock)
	if _, err := service.Revoke(t.Context(), publicRevocationCredentials(), uuid.New(), source.ticket.SessionID); err == nil || auth.calls != 0 || store.prepares != 0 {
		t.Fatalf("proof lookup failure fell through: %v", err)
	}
}

func TestPublicSessionRevocationRejectsPrematureAggregateReceipt(t *testing.T) {
	source := newRevocationModel()
	now := source.ticket.RequestedAt
	request := identity.SessionRevocationRequest{ID: uuid.New(), Key: uuid.New(), AccountID: source.actor.ID, RequestSessionID: source.actor.SessionID, Action: identity.RevokeSessionRequest, TargetID: source.ticket.SessionID, RequestedAt: now, CompletedAt: &now, Revocations: []identity.SessionRevocation{source.ticket}}
	if err := request.Validate(); !errors.Is(err, identity.ErrSessionRevocationUnavailable) {
		t.Fatalf("aggregate claimed success before its target completed: %v", err)
	}
}

func publicRevocationCredentials() identity.SessionRevocationCredentials {
	raw := make([]byte, 32)
	raw[0] = 1
	return identity.SessionRevocationCredentials{LegacyToken: base64.RawURLEncoding.EncodeToString(raw), CSRFToken: base64.RawURLEncoding.EncodeToString(raw)}
}

type publicRevocationIdentity struct {
	actor identity.Principal
	err   error
	calls int
}

func (a *publicRevocationIdentity) AuthenticateSessionRevocation(_ context.Context, _ identity.SessionRevocationCredentials, _ identity.SessionRevocationAction, _ uuid.UUID) (identity.Principal, identity.SessionCredentialKind, error) {
	a.calls++
	return a.actor, identity.SessionCredentialLegacy, a.err
}

type publicRevocationModel struct {
	source              *revocationModel
	request             *identity.SessionRevocationRequest
	prepares, completes int
	loadErr             error
}

func (s *publicRevocationModel) PrepareSessionRevocationRequest(_ context.Context, actor identity.Principal, input identity.SessionRevocationInput, _ func() time.Time) (identity.SessionRevocationRequest, error) {
	s.prepares++
	if actor != s.source.actor {
		return identity.SessionRevocationRequest{}, identity.ErrForbidden
	}
	s.request = &identity.SessionRevocationRequest{ID: uuid.New(), Key: input.Key, Action: input.Action, TargetID: input.TargetID, AccountID: actor.ID, RequestSessionID: actor.SessionID, RequestedAt: s.source.ticket.RequestedAt, Revocations: []identity.SessionRevocation{s.source.ticket}}
	return *s.request, nil
}
func (s *publicRevocationModel) LoadSessionRevocationRequest(_ context.Context, input identity.SessionRevocationInput) (identity.SessionRevocationRequest, error) {
	if s.loadErr != nil {
		return identity.SessionRevocationRequest{}, s.loadErr
	}
	if s.request == nil || s.request.Key != input.Key {
		return identity.SessionRevocationRequest{}, writefence.ErrNotFound
	}
	if s.request.Action != input.Action || s.request.TargetID != input.TargetID {
		return identity.SessionRevocationRequest{}, identity.ErrForbidden
	}
	return *s.request, nil
}
func (s *publicRevocationModel) CompleteSessionRevocationRequest(_ context.Context, request identity.SessionRevocationRequest, now time.Time) error {
	if s.request == nil || s.request.ID != request.ID || s.source.ticket.CompletedAt == nil {
		return errors.New("request completed before target evidence")
	}
	s.completes++
	s.request.CompletedAt = &now
	s.request.Revocations[0] = s.source.ticket
	return nil
}
