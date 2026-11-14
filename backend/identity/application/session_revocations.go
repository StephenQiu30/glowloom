package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ErrSessionRevocationUnavailable means durable session closure is not confirmed.
var ErrSessionRevocationUnavailable = errors.New("session revocation unavailable")

// SessionRevocation binds one irreversible session closure to its original fence.
// CompletedAt is source-side evidence, not merely a locally revoked session row.
type SessionRevocation struct {
	AccountID        uuid.UUID
	SessionID        uuid.UUID
	RequestSessionID uuid.UUID
	Control          writefence.Control
	RequestedAt      time.Time
	CompletedAt      *time.Time
}

// Validate rejects incomplete or retargeted source-side recovery records.
func (r SessionRevocation) Validate() error {
	if r.AccountID == uuid.Nil || r.SessionID == uuid.Nil || r.RequestSessionID == uuid.Nil || r.RequestedAt.IsZero() {
		return ErrSessionRevocationUnavailable
	}
	if r.Control.Validate() != nil || r.Control.Issuer != writefence.Identity || r.Control.Kind != writefence.Session || r.Control.ID != r.SessionID || !r.Control.Blocked {
		return ErrSessionRevocationUnavailable
	}
	if r.CompletedAt != nil && r.CompletedAt.Before(r.RequestedAt) {
		return ErrSessionRevocationUnavailable
	}
	return nil
}

// SessionRevocationStore keeps policy, local revocation and source closure atomic.
// Prepare must commit before returning; callers must not supply an outer SQL tx.
type SessionRevocationStore interface {
	PrepareSessionRevocation(context.Context, Principal, uuid.UUID, func() time.Time) (SessionRevocation, error)
	LoadSessionRevocation(context.Context, uuid.UUID) (SessionRevocation, error)
	CompleteSessionRevocation(context.Context, SessionRevocation, time.Time) error
}

// SessionFenceCoordinator acknowledges only durably installed exact controls.
type SessionFenceCoordinator interface {
	Set(context.Context, writefence.Authority, uuid.UUID, bool) (writefence.Coordination, error)
}

// SessionRevocations coordinates an irreversible session closure after its local
// policy transaction commits. It is not an unauthenticated session-directory API.
type SessionRevocations struct {
	store       SessionRevocationStore
	coordinator SessionFenceCoordinator
	now         func() time.Time
}

// NewSessionRevocations requires issuer-local policy persistence and fixed writers.
func NewSessionRevocations(store SessionRevocationStore, coordinator SessionFenceCoordinator, now func() time.Time) *SessionRevocations {
	return &SessionRevocations{store: store, coordinator: coordinator, now: now}
}

// Revoke checks the requesting identity before preparing the original operation.
// A failed or canceled propagation leaves a durable, locally closed operation.
func (s *SessionRevocations) Revoke(ctx context.Context, actor Principal, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrSessionInvalid
	}
	if !s.ready() {
		return ErrSessionRevocationUnavailable
	}
	revocation, err := s.store.PrepareSessionRevocation(ctx, actor, id, s.now)
	if err != nil {
		return err
	}
	if revocation.SessionID != id || revocation.AccountID != actor.ID {
		return ErrSessionRevocationUnavailable
	}
	return s.finish(ctx, revocation)
}

// Resume completes only the already-persisted session closure. This method is for
// Identity's trusted responsibility recovery, never an anonymous public endpoint.
// It cannot authorize a fresh command, change its target, or reopen a session.
func (s *SessionRevocations) Resume(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrSessionInvalid
	}
	if !s.ready() {
		return ErrSessionRevocationUnavailable
	}
	revocation, err := s.store.LoadSessionRevocation(ctx, id)
	if err != nil {
		return err
	}
	if revocation.SessionID != id {
		return ErrSessionRevocationUnavailable
	}
	return s.finish(ctx, revocation)
}

func (s *SessionRevocations) ready() bool {
	return s != nil && s.store != nil && s.coordinator != nil && s.now != nil
}

func (s *SessionRevocations) finish(ctx context.Context, revocation SessionRevocation) error {
	if err := revocation.Validate(); err != nil {
		return err
	}
	if revocation.CompletedAt != nil {
		return nil
	}
	expected := revocation.Control.Authority
	expected.Epoch--
	state, err := s.coordinator.Set(ctx, expected, revocation.Control.CoordinationID, true)
	if err != nil {
		return fmt.Errorf("propagate original session revocation: %w", err)
	}
	if !state.Complete || state.Control != revocation.Control || len(state.Acknowledged) != len(writefence.Writers()) {
		return ErrSessionRevocationUnavailable
	}
	seen := make(map[writefence.Writer]bool, len(state.Acknowledged))
	for _, writer := range state.Acknowledged {
		if writer.Validate() != nil || seen[writer] {
			return ErrSessionRevocationUnavailable
		}
		seen[writer] = true
	}
	if err := s.store.CompleteSessionRevocation(ctx, revocation, s.now().UTC()); err != nil {
		return fmt.Errorf("complete original session revocation: %w", err)
	}
	return nil
}
