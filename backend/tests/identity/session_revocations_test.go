package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestSessionRevocationsWaitForExactDurableAcknowledgements(t *testing.T) {
	for _, outcome := range []string{"complete", "unavailable", "canceled", "pending", "wrong_control", "duplicate_ack", "wrong_target"} {
		t.Run(outcome, func(t *testing.T) {
			store := newRevocationModel()
			coordinator := &sessionFenceModel{ticket: store.ticket, outcome: outcome}
			service := identity.NewSessionRevocations(store, coordinator, func() time.Time { return store.ticket.RequestedAt.Add(time.Second) })
			err := service.Revoke(t.Context(), store.actor, store.ticket.SessionID)
			if outcome == "complete" {
				if err != nil || store.completes != 1 {
					t.Fatalf("revoke: completes=%d, err=%v", store.completes, err)
				}
				return
			}
			if err == nil || store.completes != 0 {
				t.Fatalf("incomplete revocation reported success: completes=%d, err=%v", store.completes, err)
			}
			if outcome == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}

func TestSessionRevocationsResumeOnlyOriginalPersistedOperation(t *testing.T) {
	store := newRevocationModel()
	coordinator := &sessionFenceModel{ticket: store.ticket, outcome: "unavailable"}
	service := identity.NewSessionRevocations(store, coordinator, func() time.Time { return store.ticket.RequestedAt.Add(time.Second) })
	if err := service.Revoke(t.Context(), store.actor, store.ticket.SessionID); err == nil {
		t.Fatal("missing writer acknowledgement reported success")
	}
	coordinator.outcome = "complete"
	// A new process has no authenticated session or in-memory operation identity.
	restarted := identity.NewSessionRevocations(store, coordinator, func() time.Time { return store.ticket.RequestedAt.Add(time.Second) })
	if err := restarted.Resume(t.Context(), store.ticket.SessionID); err != nil {
		t.Fatal(err)
	}
	if store.prepares != 1 || store.loads != 1 || store.completes != 1 || coordinator.calls != 2 {
		t.Fatalf("recovery restarted policy or lost original identity: %+v, calls=%d", store, coordinator.calls)
	}
	coordinator.outcome = "unavailable"
	if err := restarted.Resume(t.Context(), store.ticket.SessionID); err != nil || coordinator.calls != 2 || store.completes != 1 {
		t.Fatalf("completed recovery repeated external work: err=%v, calls=%d", err, coordinator.calls)
	}
}

func TestSessionRevocationsRejectPolicyAndCorruptRecovery(t *testing.T) {
	store := newRevocationModel()
	coordinator := &sessionFenceModel{ticket: store.ticket, outcome: "complete"}
	service := identity.NewSessionRevocations(store, coordinator, time.Now)
	store.prepareErr = identity.ErrForbidden
	if err := service.Revoke(t.Context(), store.actor, store.ticket.SessionID); !errors.Is(err, identity.ErrForbidden) || coordinator.calls != 0 {
		t.Fatalf("policy failure contacted writers: %v", err)
	}
	store.ticket.Control.ID = uuid.New()
	if err := service.Resume(t.Context(), store.ticket.SessionID); err == nil || coordinator.calls != 0 {
		t.Fatalf("corrupt target contacted writers: %v", err)
	}
	if err := service.Resume(t.Context(), uuid.Nil); !errors.Is(err, identity.ErrSessionInvalid) || store.loads != 1 {
		t.Fatalf("invalid recovery reached store: %v", err)
	}
}

func TestSessionRevocationsKeepCompletionFailurePending(t *testing.T) {
	store := newRevocationModel()
	store.completeErr = errors.New("synthetic local commit failure")
	coordinator := &sessionFenceModel{ticket: store.ticket, outcome: "complete"}
	service := identity.NewSessionRevocations(store, coordinator, func() time.Time { return store.ticket.RequestedAt.Add(time.Second) })
	if err := service.Revoke(t.Context(), store.actor, store.ticket.SessionID); !errors.Is(err, store.completeErr) {
		t.Fatalf("lost local completion failure: %v", err)
	}
	if store.ticket.CompletedAt != nil {
		t.Fatal("failed completion discarded recovery responsibility")
	}
}

type revocationModel struct {
	actor                      identity.Principal
	ticket                     identity.SessionRevocation
	prepareErr, completeErr    error
	prepares, loads, completes int
}

func newRevocationModel() *revocationModel {
	actor := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	sid := uuid.New()
	return &revocationModel{actor: actor, ticket: identity.SessionRevocation{
		AccountID: actor.ID, SessionID: sid, RequestSessionID: actor.SessionID,
		Control:     writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: sid, Epoch: 2}, CoordinationID: uuid.New(), Blocked: true},
		RequestedAt: time.Now().UTC(),
	}}
}

func (s *revocationModel) PrepareSessionRevocation(_ context.Context, actor identity.Principal, id uuid.UUID, _ func() time.Time) (identity.SessionRevocation, error) {
	s.prepares++
	if actor != s.actor || id != s.ticket.SessionID {
		return identity.SessionRevocation{}, identity.ErrForbidden
	}
	return s.ticket, s.prepareErr
}

func (s *revocationModel) LoadSessionRevocation(_ context.Context, id uuid.UUID) (identity.SessionRevocation, error) {
	s.loads++
	if id != s.ticket.SessionID {
		return identity.SessionRevocation{}, identity.ErrSessionInvalid
	}
	return s.ticket, nil
}

func (s *revocationModel) CompleteSessionRevocation(_ context.Context, ticket identity.SessionRevocation, at time.Time) error {
	s.completes++
	if s.completeErr != nil {
		return s.completeErr
	}
	if ticket.Control != s.ticket.Control || at.Before(s.ticket.RequestedAt) {
		return identity.ErrForbidden
	}
	s.ticket.CompletedAt = &at
	return nil
}

type sessionFenceModel struct {
	ticket  identity.SessionRevocation
	outcome string
	calls   int
}

func (s *sessionFenceModel) Set(_ context.Context, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	s.calls++
	if authority.Key() != s.ticket.Control.Key() || authority.Epoch+1 != s.ticket.Control.Epoch || id != s.ticket.Control.CoordinationID || !blocked {
		return writefence.Coordination{}, errors.New("changed original revocation identity")
	}
	state := writefence.Coordination{Control: s.ticket.Control, Acknowledged: writefence.Writers(), Complete: true}
	switch s.outcome {
	case "unavailable":
		return writefence.Coordination{}, writefence.ErrUnavailable
	case "canceled":
		return writefence.Coordination{}, context.Canceled
	case "pending":
		state.Complete = false
	case "wrong_control":
		state.Control.CoordinationID = uuid.New()
	case "wrong_target":
		state.Control.ID = uuid.New()
	case "duplicate_ack":
		state.Acknowledged[3] = state.Acknowledged[0]
	}
	return state, nil
}
