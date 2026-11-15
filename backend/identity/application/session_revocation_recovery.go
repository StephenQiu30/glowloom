package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SessionRevocationRecoveryTask identifies Identity's original-request wakeups.
// The business ID is the accepted request UUID; the task carries no credential.
const SessionRevocationRecoveryTask = "identity.session_revocation"

// SessionRevocationRecoveryStore exposes accepted responsibilities only to the
// trusted owner worker. It is deliberately absent from the public HTTP ports.
type SessionRevocationRecoveryStore interface {
	LoadSessionRevocationRequestByID(context.Context, uuid.UUID) (SessionRevocationRequest, error)
	CompleteSessionRevocationRequest(context.Context, SessionRevocationRequest, time.Time) error
}

type sessionRevocationRequestCompleter interface {
	CompleteSessionRevocationRequest(context.Context, SessionRevocationRequest, time.Time) error
}

// SessionRevocationRecovery finishes already accepted requests without minting a
// Principal, reauthenticating a browser, or selecting a different target set.
type SessionRevocationRecovery struct {
	store    SessionRevocationRecoveryStore
	closures *SessionRevocations
	now      func() time.Time
}

// NewSessionRevocationRecovery injects the owner-only original-request reader.
func NewSessionRevocationRecovery(store SessionRevocationRecoveryStore, closures *SessionRevocations, now func() time.Time) *SessionRevocationRecovery {
	return &SessionRevocationRecovery{store: store, closures: closures, now: now}
}

// Resume queries the durable request and preserves its exact source decisions.
// Callers must be trusted owner workers, never an unauthenticated UUID endpoint.
func (s *SessionRevocationRecovery) Resume(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.store == nil || s.closures == nil || !s.closures.ready() || s.now == nil || id == uuid.Nil {
		return ErrSessionRevocationUnavailable
	}
	request, err := s.store.LoadSessionRevocationRequestByID(ctx, id)
	if err != nil {
		return err
	}
	if request.ID != id {
		return ErrSessionRevocationUnavailable
	}
	return finishSessionRevocationRequest(ctx, request, s.store, s.closures, s.now)
}

func finishSessionRevocationRequest(ctx context.Context, request SessionRevocationRequest, store sessionRevocationRequestCompleter, closures *SessionRevocations, now func() time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CompletedAt != nil {
		return nil
	}
	for _, revocation := range request.Revocations {
		if err := closures.finish(ctx, revocation); err != nil {
			return err
		}
	}
	if err := store.CompleteSessionRevocationRequest(ctx, request, now().UTC()); err != nil {
		return fmt.Errorf("complete original public session request: %w", err)
	}
	return nil
}
